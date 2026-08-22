// Package riskmanager implements the structural gatekeeper of Navier-Stocks.
//
// It subscribes to trade intents published by the Quantitative Scout on
// signal.intent.*, runs a pipeline of validation gates (time windows, sector
// concentration, directional bias, sentiment confidence, capital sizing), and
// promotes approved intents to execution signals on signal.execute.<TICKER>.
package riskmanager

import (
	"encoding/json"
	"log"
	"sync/atomic"
	"time"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/nats"
)

// Analyst is the Risk Manager agent.  It owns a single JetStream push consumer
// on the signal.intent.* wildcard subject and acknowledges each message only
// after validation (and, in later layers, the full gate pipeline) has decided
// its fate.  Terminal outcomes (malformed payloads, rejected intents) are
// Acked so the message is not redelivered; transient failures are Nacked so
// JetStream redelivers for replay.
type Analyst struct {
	js       *nats.JetStream    // NATS JetStream connection
	sub      *nats.Subscription // active signal.intent.* subscription
	accepted atomic.Int64       // count of intents accepted (test observability)
	now      func() time.Time   // injectable clock; nil means time.Now()

	exposure  *Exposure      // concurrent sector/total position tracker
	sectors   SectorResolver // ticker → sector lookup
	scores    ScoreStore     // latest sentiment score lookup
	minConf   float64        // sentiment confidence floor
	capital   float64        // deployable capital for the 2% allocator
	positions PositionStore  // open-position store for auto-square-off
	sqDone    string         // last square-off date (yyyy-mm-dd)

	stopChan chan struct{}
}

// NewAgent creates a fully-wired Risk Manager: it connects to NATS JetStream,
// resolves the trading universe for ticker→sector lookups, and prepares the
// concurrent exposure tracker.  The caller must call Stop() to release
// resources.
func NewAgent() (*Analyst, error) {
	js, err := nats.ConnectJetStream()
	if err != nil {
		return nil, err
	}

	sectors := resolveSectorMap()
	exposure := NewExposure(DefaultMaxSectorPositions, DefaultMaxTotalPositions)

	a := newAnalyst(js)
	a.sectors = sectors
	a.exposure = exposure
	a.minConf = minConfidenceFromEnv()
	a.capital = LoadTotalCapital()
	// Surface the effective risk parameters once at boot so operators are
	// never surprised by silent defaults (TOTAL_CAPITAL, RISK_MIN_CONFIDENCE).
	log.Printf("[Risk Manager] risk config: capital=%.0f minConfidence=%.2f",
		a.capital, a.minConf)
	return a, nil
}

// newAnalyst is the shared constructor used by NewAgent and NewAgentForTest.
func newAnalyst(js *nats.JetStream) *Analyst {
	return &Analyst{
		js:       js,
		stopChan: make(chan struct{}),
	}
}

// NewAgentForTest returns an Analyst wired to the supplied JetStream without
// opening any external connections.  Its clock is pinned to a fixed point well
// inside trading hours (11:00 AM IST) and its capital/confidence defaults are
// filled in so gate pipeline tests are deterministic regardless of when the
// test suite runs.
func NewAgentForTest(js *nats.JetStream) *Analyst {
	a := newAnalyst(js)
	a.capital = DefaultTotalCapital
	a.minConf = DefaultMinConfidence
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err == nil {
		a.now = func() time.Time {
			return time.Date(2026, 8, 10, 11, 0, 0, 0, loc)
		}
	}
	return a
}

// Run ensures the trading stream exists, subscribes to the signal.intent.*
// wildcard, and blocks until Stop() is called.  The subscription is torn down
// when the method returns.
func (g *Analyst) Run() {
	if g.js == nil {
		return
	}
	if err := g.js.EnsureStream(nats.StreamTrading); err != nil {
		log.Printf("[Risk Manager] Stream ensure failed: %v", err)
		return
	}

	sub, err := g.js.Subscribe("signal.intent.*", g.handleIntent)
	if err != nil {
		log.Printf("[Risk Manager] Subscribe failed: %v", err)
		return
	}
	g.sub = sub
	log.Println("[Risk Manager] Subscribed to signal.intent.*")

	ticker := time.NewTicker(SquareOffInterval)
	defer ticker.Stop()

	for {
		select {
		case <-g.stopChan:
			if g.sub != nil {
				if err := g.sub.Unsubscribe(); err != nil {
					log.Printf("[Risk Manager] Unsubscribe error: %v", err)
				}
			}
			return
		case now := <-ticker.C:
			g.squareOff(now)
		}
	}
}

// Stop signals the agent to shut down: it closes the stop channel (which
// unblocks Run()) and closes the NATS connection.
func (g *Analyst) Stop() {
	close(g.stopChan)
	if g.js != nil {
		g.js.Close()
	}
}

// Accepted reports how many intents have been accepted so far.  It is used by
// tests to observe processing without exposing internals.
func (g *Analyst) Accepted() int64 {
	return g.accepted.Load()
}

// handleIntent is the NATS message handler for signal.intent.*.  It
// deserialises the JSON payload into a TradeIntent, validates it, and
// acknowledges the message.  Malformed or invalid payloads are terminal: they
// are Acked so JetStream does not redeliver them.
//
// The gate pipeline (time windows, sector exposure, directional bias,
// sentiment, capital sizing) is layered onto this handler in later stories.
func (g *Analyst) handleIntent(m *nats.Msg) {
	var intent models.TradeIntent
	if err := json.Unmarshal(m.Data, &intent); err != nil {
		log.Printf("[Risk Manager] Invalid intent JSON: %v", err)
		g.ackOrLog(m)
		return
	}

	if err := intent.Validate(); err != nil {
		log.Printf("[Risk Manager] Invalid intent: %v", err)
		g.ackOrLog(m)
		return
	}

	log.Printf("[Risk Manager] Intent %s %s @ %.2f",
		intent.Side, intent.Ticker, intent.CurrentPrice)

	if err := g.evaluate(&intent); err != nil {
		log.Printf("[Risk Manager] Intent %s %s rejected: %v",
			intent.Side, intent.Ticker, err)
		g.ackOrLog(m) // rejection is terminal; ack to move past it
		return
	}

	if err := g.promote(&intent); err != nil {
		// Inability to promote (e.g. insufficient capital) is a terminal
		// decision, not a transient fault: ack to move past the intent.
		log.Printf("[Risk Manager] Intent %s %s not promoted: %v",
			intent.Side, intent.Ticker, err)
		g.ackOrLog(m)
		return
	}

	g.accepted.Add(1)
	if err := m.Ack(); err != nil {
		log.Printf("[Risk Manager] Ack error: %v", err)
	}
}

// evaluate runs the validation gates over an intent: time-window guard,
// sector-concentration gates (Gate 1 sector cap, Gate 2 concurrency ceiling),
// Gate 3 directional-bias control, and the sentiment-confidence gate.
func (g *Analyst) evaluate(intent *models.TradeIntent) error {
	if err := timeWindowGuard(g.currentTime()); err != nil {
		return err
	}
	if g.exposure != nil {
		if err := g.exposure.EntryGate(g.sectorOf(intent.Ticker)); err != nil {
			return err
		}
		if err := g.exposure.BiasGate(intent.Side); err != nil {
			return err
		}
	}
	if err := sentimentGate(g.scores, intent.Ticker, g.minConf); err != nil {
		return err
	}
	return nil
}

// currentTime returns the agent's clock, defaulting to time.Now when the
// injectable clock is nil (production).
func (g *Analyst) currentTime() time.Time {
	if g.now != nil {
		return g.now()
	}
	return time.Now()
}

// SetSectors installs the ticker→sector resolver used by the exposure gate;
// tests use this to inject a fixed mapping.
func (g *Analyst) SetSectors(s SectorResolver) { g.sectors = s }

// SetExposure installs the concurrent exposure tracker; tests use this to
// pre-seed state or observe gates.
func (g *Analyst) SetExposure(e *Exposure) { g.exposure = e }

// SetScoreStore installs the sentiment score store; tests inject a mock to
// exercise the confidence gate.
func (g *Analyst) SetScoreStore(s ScoreStore) { g.scores = s }

// SetMinConfidence sets the sentiment confidence floor; tests use this to
// verify the gate threshold.
func (g *Analyst) SetMinConfidence(v float64) { g.minConf = v }

// SetCapital sets the deployable capital used by the allocator; tests use
// this to pin sizing deterministically.
func (g *Analyst) SetCapital(v float64) { g.capital = v }

// SetPositions installs the position store used by the auto-square-off
// routine; tests inject a mock to exercise the liquidation path.
func (g *Analyst) SetPositions(p PositionStore) { g.positions = p }

// EvaluateRaw runs the current gate pipeline over an intent without touching
// NATS; tests use it to exercise wiring deterministically.
func (g *Analyst) EvaluateRaw(intent models.TradeIntent) error {
	return g.evaluate(&intent)
}

// RawStopLoss exposes the directional default stop-loss derivation for tests.
func (g *Analyst) RawStopLoss(intent *models.TradeIntent) float64 {
	return g.stopLossFor(intent)
}

// PromoteRaw promotes an intent through sizing/publishing/exposure-update
// without needing a NATS round trip for unit tests.
func (g *Analyst) PromoteRaw(intent *models.TradeIntent) error {
	return g.promote(intent)
}

// ExposeSector exposes whether the exposure tracker counts the given sector;
// tests use it to confirm post-publish registration.
func (g *Analyst) ExposeSector(sector string) int {
	if g.exposure == nil {
		return 0
	}
	return g.exposure.SectorCount(sector)
}

// ackOrLog attempts a best-effort ACK on a message that was determined to be
// unprocessable (invalid JSON or a rejected intent).  These outcomes are
// terminal — redelivery would only repeat the same rejection — so we ack to
// move past them and log if the ack itself fails.
func (g *Analyst) ackOrLog(m *nats.Msg) {
	if err := m.Ack(); err != nil {
		log.Printf("[Risk Manager] Ack error on terminal message: %v", err)
	}
}
