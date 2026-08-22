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
	stopChan chan struct{}
}

// NewAgent creates a fully-wired Risk Manager: it connects to NATS JetStream
// and prepares the wildcard subscription.  The caller must call Stop() to
// release resources.
func NewAgent() (*Analyst, error) {
	js, err := nats.ConnectJetStream()
	if err != nil {
		return nil, err
	}
	return newAnalyst(js), nil
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
// inside trading hours (11:00 AM IST) so gate pipeline tests are deterministic
// regardless of when the test suite runs.
func NewAgentForTest(js *nats.JetStream) *Analyst {
	a := newAnalyst(js)
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

	<-g.stopChan

	if g.sub != nil {
		if err := g.sub.Unsubscribe(); err != nil {
			log.Printf("[Risk Manager] Unsubscribe error: %v", err)
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

	g.accepted.Add(1)
	if err := m.Ack(); err != nil {
		log.Printf("[Risk Manager] Ack error: %v", err)
	}
}

// evaluate runs the validation gates over an intent.  This layer wires the
// time-window guard; subsequent stories add the exposure, bias, sentiment,
// and allocation checks here.
func (g *Analyst) evaluate(intent *models.TradeIntent) error {
	if err := timeWindowGuard(g.currentTime()); err != nil {
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

// ackOrLog attempts a best-effort ACK on a message that was determined to be
// unprocessable (invalid JSON or a rejected intent).  These outcomes are
// terminal — redelivery would only repeat the same rejection — so we ack to
// move past them and log if the ack itself fails.
func (g *Analyst) ackOrLog(m *nats.Msg) {
	if err := m.Ack(); err != nil {
		log.Printf("[Risk Manager] Ack error on terminal message: %v", err)
	}
}
