package trader

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/nats"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/utils"
)

// ExitRefreshInterval is how often the exit monitor re-reads open positions
// from the database.  Price ticks are matched against this in-memory cache
// so a hot price stream never turns into a per-tick DB query.
const ExitRefreshInterval = 30 * time.Second

// PositionCloser is the persistence surface of the exit monitor.
type PositionCloser interface {
	ListOpen() ([]models.Position, error)
	MarkClosed(id int64, exitPrice float64, reason models.ExitReason) error
}

// ExitPublisher abstracts JetStream for publishing closing executions so
// the monitor can be tested without a live broker link.
type ExitPublisher interface {
	Publish(subj string, data []byte) error
}

// ExitMonitor watches the signal.price.* stream against every open
// position's stop-loss / take-profit boundaries and liquidates positions
// whose boundary has been crossed.  Closing orders reuse the market
// square-off shape: opposite side, full quantity, correlation ref tied to
// the original position.
type ExitMonitor struct {
	positions PositionCloser
	pub       ExitPublisher
	now       func() time.Time // injectable clock; nil means time.Now

	mu       sync.Mutex
	cache    map[string][]models.Position // ticker → open positions
	inFlight map[int64]bool               // ids already being closed (idempotency)
}

// NewExitMonitor returns an ExitMonitor over the given store and publisher.
func NewExitMonitor(positions PositionCloser, pub ExitPublisher) *ExitMonitor {
	return &ExitMonitor{
		positions: positions,
		pub:       pub,
		cache:     make(map[string][]models.Position),
		inFlight:  make(map[int64]bool),
	}
}

// SetClock pins the monitor's clock (tests).
func (em *ExitMonitor) SetClock(fn func() time.Time) { em.now = fn }

func (em *ExitMonitor) currentTime() time.Time {
	if em.now != nil {
		return em.now()
	}
	return time.Now()
}

// Refresh re-reads open positions from the store into the in-memory cache.
func (em *ExitMonitor) Refresh() error {
	open, err := em.positions.ListOpen()
	if err != nil {
		return fmt.Errorf("exit refresh: %w", err)
	}

	next := make(map[string][]models.Position, len(open))
	for _, pos := range open {
		next[pos.Ticker] = append(next[pos.Ticker], pos)
	}

	em.mu.Lock()
	em.cache = next
	em.mu.Unlock()
	return nil
}

// HandlePrice is the NATS callback for signal.price.<TICKER>.  Malformed
// messages are dropped silently — a bad tick must not kill the stream.
func (em *ExitMonitor) HandlePrice(m *nats.Msg) {
	var tick models.PriceTick
	if err := json.Unmarshal(m.Data, &tick); err != nil || tick.Ticker == "" || tick.Price <= 0 {
		return
	}
	em.evaluate(tickerOf(m.Subject), tick.Price)
}

// tickerOf extracts the trailing token from a signal.price.<TICKER> subject.
func tickerOf(subject string) string {
	const prefix = nats.SubjectPricePrefix
	if len(subject) <= len(prefix) {
		return ""
	}
	return subject[len(prefix):]
}

// evaluate checks one price against cached positions for that ticker and
// fires closing executions for crossed boundaries.
func (em *ExitMonitor) evaluate(ticker string, price float64) {
	if utils.IsOpeningBuffer(em.currentTime()) {
		return // 09:15–09:30 buffer: no exits yet
	}
	if utils.IsClosingCutoff(em.currentTime()) {
		return // ≥15:00: square-off owns liquidation from here
	}

	em.mu.Lock()
	candidates := em.cache[ticker]
	var targets []models.Position
	var reasons []models.ExitReason
	for _, pos := range candidates {
		if em.inFlight[pos.ID] {
			continue
		}
		if reason, hit := crossed(pos, price); hit {
			targets = append(targets, pos)
			reasons = append(reasons, reason)
			em.inFlight[pos.ID] = true // mark before unlock to avoid double-fire
		}
	}
	em.mu.Unlock()

	for i := range targets {
		em.close(&targets[i], reasons[i], price)
	}
}

// crossed reports whether price has breached the position's stop-loss or
// take-profit boundary, honouring side semantics:
//
//	LONG : SL when price ≤ stop, TP when price ≥ take_profit
//	SHORT: SL when price ≥ stop, TP when price ≤ take_profit
func crossed(pos models.Position, price float64) (models.ExitReason, bool) {
	switch pos.Side {
	case models.SideLong:
		switch {
		case pos.StopLoss > 0 && price <= pos.StopLoss:
			return models.ReasonStopLoss, true
		case pos.TakeProfit > 0 && price >= pos.TakeProfit:
			return models.ReasonTakeProfit, true
		}
	case models.SideShort:
		switch {
		case pos.StopLoss > 0 && price >= pos.StopLoss:
			return models.ReasonStopLoss, true
		case pos.TakeProfit > 0 && price <= pos.TakeProfit:
			return models.ReasonTakeProfit, true
		}
	}
	return "", false
}

// close publishes the closing execution and persists the exit.  A failed
// publish releases the in-flight mark so the next tick can retry; a failed
// MarkClosed leaves the position OPEN in the DB and is retried on refresh.
func (em *ExitMonitor) close(pos *models.Position, reason models.ExitReason, price float64) {
	exitSide := models.SideLong
	if pos.Side == models.SideLong {
		exitSide = models.SideShort
	}

	exec := &models.TradeExecution{
		Ticker:       pos.Ticker,
		Side:         exitSide,
		Quantity:     pos.Quantity,
		Price:        price,
		Sector:       pos.Sector,
		SignalReason: string(reason),
		ExecutionRef: fmt.Sprintf("exit-%d-%d", pos.ID, time.Now().UnixNano()),
	}
	data, err := json.Marshal(exec)
	if err != nil {
		log.Printf("[Trader] exit marshal %d: %v", pos.ID, err)
		em.release(pos.ID)
		return
	}
	if err := em.pub.Publish(nats.ExecuteSubject(pos.Ticker), data); err != nil {
		log.Printf("[Trader] exit publish %s %s: %v", pos.Ticker, reason, err)
		em.release(pos.ID) // allow retry on next tick
		return
	}

	if err := em.positions.MarkClosed(pos.ID, price, reason); err != nil {
		log.Printf("[Trader] exit close %d (%s): %v", pos.ID, pos.Ticker, err)
		// Keep the in-flight mark: the position will be picked up again by
		// Refresh once it disappears from ListOpen, which clears stale marks.
		em.dropFromCache(pos.ID, pos.Ticker)
		return
	}

	log.Printf("[Trader] exited %s %s x%d @ %.2f (%s)",
		pos.Side, pos.Ticker, pos.Quantity, price, reason)
	em.dropFromCache(pos.ID, pos.Ticker)
}

// release clears an in-flight mark after a recoverable failure.
func (em *ExitMonitor) release(id int64) {
	em.mu.Lock()
	delete(em.inFlight, id)
	em.mu.Unlock()
}

// dropFromCache removes a fully-closed position from the local view and its
// in-flight mark (the authoritative state now lives in the database).
func (em *ExitMonitor) dropFromCache(id int64, ticker string) {
	em.mu.Lock()
	kept := em.cache[ticker][:0]
	for _, p := range em.cache[ticker] {
		if p.ID != id {
			kept = append(kept, p)
		}
	}
	em.cache[ticker] = kept
	delete(em.inFlight, id)
	em.mu.Unlock()
}
