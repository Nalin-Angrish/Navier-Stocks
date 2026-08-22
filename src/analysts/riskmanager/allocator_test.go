package riskmanager_test

import (
	"testing"

	"github.com/Nalin-Angrish/Navier-Stocks/src/analysts/riskmanager"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
)

func TestCalculateQuantity_StandardCase(t *testing.T) {
	// ₹1,000,000 capital, 2% budget = ₹20,000 risked.
	// Entry 100, stop 90 → risk ₹10/share → 2,000 shares.
	got := riskmanager.CalculateQuantity(1_000_000, 100, 90)
	if got != 2000 {
		t.Fatalf("CalculateQuantity = %d, want 2000", got)
	}
}

func TestCalculateQuantity_ShortSide(t *testing.T) {
	// Short entered at 100 with stop above at 110: risk distance is also 10.
	got := riskmanager.CalculateQuantity(1_000_000, 100, 110)
	if got != 2000 {
		t.Fatalf("CalculateQuantity(short) = %d, want 2000", got)
	}
}

func TestCalculateQuantity_FloorsToWholeLot(t *testing.T) {
	// Risk budget / per-share risk yields a fractional quantity; it is floored.
	got := riskmanager.CalculateQuantity(100_000, 333, 300)
	// Budget ₹2,000 / ₹33 → 60.6 → 60.
	if got != 60 {
		t.Fatalf("CalculateQuantity = %d, want 60", got)
	}
}

func TestCalculateQuantity_ZeroWhenBudgetExceeded(t *testing.T) {
	// Risk per share (₹1,000) exceeds the 2% budget (₹200) → cannot afford a
	// single share → 0, signalling no position should be opened.
	got := riskmanager.CalculateQuantity(10_000, 1000, 0)
	if got != 0 {
		t.Fatalf("CalculateQuantity = %d, want 0", got)
	}
}

func TestCalculateQuantity_PositiveGuards(t *testing.T) {
	if got := riskmanager.CalculateQuantity(0, 100, 90); got != 0 {
		t.Fatalf("zero capital: got %d, want 0", got)
	}
	if got := riskmanager.CalculateQuantity(1_000_000, 0, 90); got != 0 {
		t.Fatalf("zero entry: got %d, want 0", got)
	}
	// No risk distance (stop == entry) cannot be sized.
	if got := riskmanager.CalculateQuantity(1_000_000, 100, 100); got != 0 {
		t.Fatalf("zero risk distance: got %d, want 0", got)
	}
}

func TestStopLossFor_LongBelowEntry(t *testing.T) {
	a := riskmanager.NewAgentForTest(nil)
	got := a.RawStopLoss(&models.TradeIntent{Ticker: "TCS", Side: models.SideLong, CurrentPrice: 100})
	if got != 95 {
		t.Fatalf("long stop = %v, want 95", got)
	}
}

func TestStopLossFor_ShortAboveEntry(t *testing.T) {
	a := riskmanager.NewAgentForTest(nil)
	got := a.RawStopLoss(&models.TradeIntent{Ticker: "TCS", Side: models.SideShort, CurrentPrice: 100})
	if got != 105 {
		t.Fatalf("short stop = %v, want 105", got)
	}
}
