package riskmanager

import (
	"errors"
	"testing"
	"time"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
)

// stubPositions is a minimal PositionStore for unit tests.
type stubPositions struct {
	open     []models.Position
	closed   []int64
	listErr  error
	closeErr error
}

func (s *stubPositions) ListOpen() ([]models.Position, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.open, nil
}

func (s *stubPositions) MarkClosed(id int64, exitPrice float64, reason models.ExitReason) error {
	if s.closeErr != nil {
		return s.closeErr
	}
	s.closed = append(s.closed, id)
	return nil
}

func TestSquareOff_NilPositions(t *testing.T) {
	a := NewAgentForTest(nil)
	a.now = fixedClock(t, 15, 20) // after 15:15
	a.squareOff(a.now())
	// no panic, no side effects
}

func TestSquareOff_BeforeTrigger(t *testing.T) {
	store := &stubPositions{
		open: []models.Position{
			{ID: 1, Ticker: "RELIANCE", Side: models.SideLong, Quantity: 10, EntryPrice: 2500, Sector: "Energy", Status: models.PositionOpen, ExecutionRef: "ref-1"},
		},
	}
	a := NewAgentForTest(nil)
	a.positions = store
	a.now = fixedClock(t, 14, 0) // before 15:15
	a.squareOff(a.now())
	if len(store.closed) != 0 {
		t.Fatalf("expected no closures before trigger, got %d", len(store.closed))
	}
}

func TestSquareOff_AfterTrigger(t *testing.T) {
	store := &stubPositions{
		open: []models.Position{
			{ID: 1, Ticker: "RELIANCE", Side: models.SideLong, Quantity: 10, EntryPrice: 2500, Sector: "Energy", Status: models.PositionOpen, ExecutionRef: "ref-1"},
			{ID: 2, Ticker: "TCS", Side: models.SideShort, Quantity: 5, EntryPrice: 3800, Sector: "IT", Status: models.PositionOpen, ExecutionRef: "ref-2"},
		},
	}
	a := NewAgentForTest(nil) // js=nil → signalSquareOff is no-op
	a.positions = store
	a.now = fixedClock(t, 15, 20) // after 15:15
	a.squareOff(a.now())
	if len(store.closed) != 2 {
		t.Fatalf("expected 2 closures, got %d", len(store.closed))
	}
	if a.sqDone == "" {
		t.Fatal("expected sqDone to be set")
	}
}

func TestSquareOff_OnlyOncePerDay(t *testing.T) {
	store := &stubPositions{
		open: []models.Position{
			{ID: 3, Ticker: "INFY", Side: models.SideLong, Quantity: 20, EntryPrice: 1500, Sector: "IT", Status: models.PositionOpen, ExecutionRef: "ref-3"},
		},
	}
	a := NewAgentForTest(nil)
	a.positions = store
	a.now = fixedClock(t, 15, 30)
	a.squareOff(a.now())
	if len(store.closed) != 1 {
		t.Fatalf("expected 1 closure, got %d", len(store.closed))
	}
	// second call same day → no additional closure
	a.squareOff(a.now())
	if len(store.closed) != 1 {
		t.Fatalf("expected still 1 closure, got %d", len(store.closed))
	}
}

func TestSquareOff_EmptyOpenList(t *testing.T) {
	store := &stubPositions{open: nil}
	a := NewAgentForTest(nil)
	a.positions = store
	a.now = fixedClock(t, 15, 20)
	a.squareOff(a.now())
	if len(store.closed) != 0 {
		t.Fatalf("expected 0 closures, got %d", len(store.closed))
	}
}

func TestSquareOff_ListError(t *testing.T) {
	store := &stubPositions{listErr: errors.New("db down")}
	a := NewAgentForTest(nil)
	a.positions = store
	a.now = fixedClock(t, 15, 20)
	a.squareOff(a.now())
	// should not panic, just log
	if len(store.closed) != 0 {
		t.Fatalf("expected 0 closures on list error, got %d", len(store.closed))
	}
}

func TestSquareOff_MarkClosedError(t *testing.T) {
	store := &stubPositions{
		open: []models.Position{
			{ID: 4, Ticker: "HDFCBANK", Side: models.SideLong, Quantity: 8, EntryPrice: 1600, Sector: "Finance", Status: models.PositionOpen, ExecutionRef: "ref-4"},
		},
		closeErr: errors.New("lock timeout"),
	}
	a := NewAgentForTest(nil)
	a.positions = store
	a.now = fixedClock(t, 15, 20)
	a.squareOff(a.now())
	// MarkClosed failed → position stays open, store.closed empty
	if len(store.closed) != 0 {
		t.Fatalf("expected 0 closures on close error, got %d", len(store.closed))
	}
}

func TestSignalSquareOff_ExecDirection(t *testing.T) {
	// LONG position → square-off publishes SHORT, SHORT → LONG
	a := NewAgentForTest(nil)
	now := time.Date(2026, 8, 22, 15, 20, 0, 0, istLoc())

	longPos := &models.Position{
		ID: 10, Ticker: "RELIANCE", Side: models.SideLong,
		Quantity: 10, EntryPrice: 2500, Sector: "Energy",
		ExecutionRef: "orig-ref",
	}
	shortPos := &models.Position{
		ID: 11, Ticker: "TCS", Side: models.SideShort,
		Quantity: 5, EntryPrice: 3800, Sector: "IT",
		ExecutionRef: "orig-ref-2",
	}

	// js=nil → signalSquareOff returns nil (no publish)
	if err := a.signalSquareOff(longPos, now); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := a.signalSquareOff(shortPos, now); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func fixedClock(t *testing.T, hour, min int) func() time.Time {
	t.Helper()
	loc := istLoc()
	if loc == nil {
		t.Skip("Asia/Kolkata timezone not available")
	}
	return func() time.Time {
		return time.Date(2026, 8, 22, hour, min, 0, 0, loc)
	}
}

func istLoc() *time.Location {
	loc, _ := time.LoadLocation("Asia/Kolkata")
	return loc
}
