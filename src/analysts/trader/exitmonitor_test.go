package trader

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/nats"
)

// stubCloser is a PositionCloser with canned open positions.
type stubCloser struct {
	mu       sync.Mutex
	open     []models.Position
	closed   []closedRec
	listErr  error
	closeErr error
}

type closedRec struct {
	id     int64
	price  float64
	reason models.ExitReason
}

func (s *stubCloser) ListOpen() ([]models.Position, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listErr != nil {
		return nil, s.listErr
	}
	return append([]models.Position(nil), s.open...), nil
}

func (s *stubCloser) MarkClosed(id int64, exitPrice float64, reason models.ExitReason) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closeErr != nil {
		return s.closeErr
	}
	s.closed = append(s.closed, closedRec{id, exitPrice, reason})
	var kept []models.Position
	for _, p := range s.open {
		if p.ID != id {
			kept = append(kept, p)
		}
	}
	s.open = kept
	return nil
}

// stubPub records published subjects and payloads.
type stubPub struct {
	mu   sync.Mutex
	sent map[string][]json.RawMessage
	err  error
}

func newStubPub() *stubPub { return &stubPub{sent: make(map[string][]json.RawMessage)} }

func (p *stubPub) Publish(subj string, data []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return p.err
	}
	p.sent[subj] = append(p.sent[subj], json.RawMessage(data))
	return nil
}

func (p *stubPub) count(subj string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.sent[subj])
}

func marketClock(t *testing.T) func() time.Time {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Skip("Asia/Kolkata unavailable")
	}
	return func() time.Time { return time.Date(2026, 8, 21, 11, 0, 0, 0, loc) }
}

func longPos() models.Position {
	return models.Position{
		ID: 1, Ticker: "RELIANCE", Side: models.SideLong,
		Quantity: 10, EntryPrice: 100, StopLoss: 95, TakeProfit: 110,
		Sector: "Energy", Status: models.PositionOpen,
	}
}

func shortPos() models.Position {
	return models.Position{
		ID: 2, Ticker: "TCS", Side: models.SideShort,
		Quantity: 5, EntryPrice: 4000, StopLoss: 4200, TakeProfit: 3800,
		Sector: "IT", Status: models.PositionOpen,
	}
}

func seededMonitor(t *testing.T, positions ...models.Position) (*ExitMonitor, *stubCloser, *stubPub) {
	t.Helper()
	store := &stubCloser{open: positions}
	pub := newStubPub()
	em := NewExitMonitor(store, pub)
	em.SetClock(marketClock(t))
	if err := em.Refresh(); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	return em, store, pub
}

func TestExitMonitor_LongStopLossFiresShortClose(t *testing.T) {
	em, store, pub := seededMonitor(t, longPos())

	em.evaluate("RELIANCE", 94.50) // below SL

	if got := pub.count(nats.ExecuteSubject("RELIANCE")); got != 1 {
		t.Fatalf("expected 1 closing execution, got %d", got)
	}
	store.mu.Lock()
	recs := store.closed
	store.mu.Unlock()
	if len(recs) != 1 || recs[0].reason != models.ReasonStopLoss || recs[0].price != 94.50 {
		t.Fatalf("unexpected close record: %+v", recs)
	}

	// The published order must be SHORT full quantity at the tick price.
	pub.mu.Lock()
	raw := pub.sent[nats.ExecuteSubject("RELIANCE")][0]
	pub.mu.Unlock()
	var exec models.TradeExecution
	if err := json.Unmarshal(raw, &exec); err != nil {
		t.Fatalf("unmarshal execution: %v", err)
	}
	if exec.Side != models.SideShort || exec.Quantity != 10 ||
		exec.SignalReason != string(models.ReasonStopLoss) ||
		!strings.HasPrefix(exec.ExecutionRef, "exit-1-") {
		t.Fatalf("unexpected execution: %+v", exec)
	}
}

func TestExitMonitor_LongTakeProfitAndMidRangeNoop(t *testing.T) {
	em, _, pub := seededMonitor(t, longPos())

	em.evaluate("RELIANCE", 105) // mid-range: nothing
	if got := pub.count(nats.ExecuteSubject("RELIANCE")); got != 0 {
		t.Fatalf("mid-range tick must not fire, got %d", got)
	}

	em.evaluate("RELIANCE", 110.25) // ≥ TP
	if got := pub.count(nats.ExecuteSubject("RELIANCE")); got != 1 {
		t.Fatalf("expected TP close, got %d executions", got)
	}
}

func TestExitMonitor_ShortBoundariesMirror(t *testing.T) {
	em, store, _ := seededMonitor(t, shortPos())

	em.evaluate("TCS", 4250) // ≥ SL for a short
	store.mu.Lock()
	recs := append([]closedRec(nil), store.closed...)
	store.mu.Unlock()
	if len(recs) != 1 || recs[0].reason != models.ReasonStopLoss {
		t.Fatalf("short SL not recorded: %+v", recs)
	}

	// Re-seed: position already closed by the stub; re-arm via Refresh.
	em2, _, pub2 := seededMonitor(t, shortPos())
	em2.evaluate("TCS", 3750) // ≤ TP for a short
	if got := pub2.count(nats.ExecuteSubject("TCS")); got != 1 {
		t.Fatalf("expected short TP close, got %d", got)
	}
}

func TestExitMonitor_OutsideHoursIgnored(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Skip("Asia/Kolkata unavailable")
	}
	store := &stubCloser{open: []models.Position{longPos()}}
	pub := newStubPub()
	em := NewExitMonitor(store, pub)
	loc2 := loc
	em.SetClock(func() time.Time {
		return time.Date(2026, 8, 21, 15, 5, 0, 0, loc2) // after cutoff
	})
	if err := em.Refresh(); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	em.evaluate("RELIANCE", 50)
	if got := pub.count(nats.ExecuteSubject("RELIANCE")); got != 0 {
		t.Fatalf("post-cutoff exit must be skipped, got %d", got)
	}
}

func TestExitMonitor_IdempotentWhileInFlight(t *testing.T) {
	em, store, pub := seededMonitor(t, longPos())

	// MarkClosed fails: position stays OPEN in the DB, in-flight mark keeps
	// subsequent ticks from double-firing within the same cache generation.
	store.mu.Lock()
	store.closeErr = errors.New("db locked")
	store.mu.Unlock()

	em.evaluate("RELIANCE", 90)
	if got := pub.count(nats.ExecuteSubject("RELIANCE")); got != 1 {
		t.Fatalf("expected exactly one attempt, got %d", got)
	}

	em.evaluate("RELIANCE", 89) // still marked in-flight → no second publish
	if got := pub.count(nats.ExecuteSubject("RELIANCE")); got != 1 {
		t.Fatalf("double-fire detected: %d executions", got)
	}
}

func TestExitMonitor_PublishFailureReleasesForRetry(t *testing.T) {
	store := &stubCloser{open: []models.Position{longPos()}}
	pub := newStubPub()
	pub.err = errors.New("nats down")
	em := NewExitMonitor(store, pub)
	em.SetClock(marketClock(t))
	if err := em.Refresh(); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	em.evaluate("RELIANCE", 90) // publish fails; nothing recorded

	// Publisher recovered: the released mark lets the next tick retry and
	// this time the closing order lands.
	pub.err = nil
	em.evaluate("RELIANCE", 89.5)
	if got := pub.count(nats.ExecuteSubject("RELIANCE")); got != 1 {
		t.Fatalf("expected retried close to land once, got %d", got)
	}
}

func TestExitMonitor_RefreshPicksUpNewPositions(t *testing.T) {
	em, store, pub := seededMonitor(t) // empty book

	em.evaluate("RELIANCE", 90) // unknown ticker: no-op

	store.mu.Lock()
	store.open = []models.Position{longPos()}
	store.mu.Unlock()
	if err := em.Refresh(); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	em.evaluate("RELIANCE", 94)
	if got := pub.count(nats.ExecuteSubject("RELIANCE")); got != 1 {
		t.Fatalf("refresh did not pick up new position, got %d executions", got)
	}
}

func TestTickerOfSubject(t *testing.T) {
	if got := tickerOf(nats.PriceSubject("INFY")); got != "INFY" {
		t.Fatalf("tickerOf = %q", got)
	}
	if got := tickerOf("signal.price."); got != "" {
		t.Fatalf("empty ticker should yield empty string, got %q", got)
	}
}

func TestCrossed_GuardsZeroBoundaries(t *testing.T) {
	pos := longPos()
	pos.StopLoss, pos.TakeProfit = 0, 0
	if _, hit := crossed(pos, 1); hit {
		t.Fatal("zero boundaries must never fire")
	}
}
