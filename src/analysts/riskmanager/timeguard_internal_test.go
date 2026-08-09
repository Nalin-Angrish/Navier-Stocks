package riskmanager

import (
	"errors"
	"testing"
	"time"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
)

// validIntent builds a well-formed TradeIntent for gate tests.
func validIntent(ticker string, price float64) *models.TradeIntent {
	return &models.TradeIntent{
		Ticker:       ticker,
		Side:         models.SideLong,
		CurrentPrice: price,
		SignalReason: "test_signal",
		Timestamp:    time.Now(),
	}
}

// ist builds a fixed timestamp in the Asia/Kolkata timezone so gate tests do
// not depend on the host clock.
func ist(t *testing.T, h, m int) time.Time {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Fatalf("LoadLocation(Asia/Kolkata): %v", err)
	}
	return time.Date(2026, 8, 10, h, m, 0, 0, loc)
}

func TestTimeWindowGuard_OrdinaryHours(t *testing.T) {
	// 11:00 AM IST is well inside the 09:30–15:00 window.
	if err := timeWindowGuard(ist(t, 11, 0)); err != nil {
		t.Fatalf("expected no error during ordinary hours, got %v", err)
	}
}

func TestTimeWindowGuard_OpeningBuffer(t *testing.T) {
	if err := timeWindowGuard(ist(t, 9, 20)); !errors.Is(err, ErrOpeningBuffer) {
		t.Fatalf("expected ErrOpeningBuffer, got %v", err)
	}
}

func TestTimeWindowGuard_ClosingCutoff(t *testing.T) {
	// After 15:00 IST the market is closed to new entries.
	if err := timeWindowGuard(ist(t, 15, 1)); !errors.Is(err, ErrClosingCutoff) {
		t.Fatalf("expected ErrClosingCutoff after 15:00, got %v", err)
	}
	if err := timeWindowGuard(ist(t, 15, 30)); !errors.Is(err, ErrClosingCutoff) {
		t.Fatalf("expected ErrClosingCutoff after 15:00, got %v", err)
	}
}

func TestTimeWindowGuard_AfterMarketOpen(t *testing.T) {
	// 09:45 IST is after the buffer and before the cutoff, so it passes.
	if err := timeWindowGuard(ist(t, 9, 45)); err != nil {
		t.Fatalf("expected no error at 09:45, got %v", err)
	}
}

func TestAnalyse_TimeGateWired(t *testing.T) {
	// The analyst's evaluate must reject an intent outside trading hours.
	agent := NewAgentForTest(nil)
	agent.now = func() time.Time { return ist(t, 15, 1) }

	intent := validIntent("RELIANCE", 2500.50)
	if err := agent.evaluate(intent); err == nil {
		t.Fatal("expected time-window rejection after cutoff")
	}
}
