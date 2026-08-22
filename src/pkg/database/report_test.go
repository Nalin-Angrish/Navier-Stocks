package database_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/database"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
)

var reportCols = []string{
	"id", "ticker", "side", "quantity", "entry_price", "stop_loss",
	"take_profit", "sector", "status", "execution_ref",
	"exit_price", "pnl", "exit_reason", "closed_at",
}

func TestClosedPositions_ScansNullableExitFields(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()

	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.Local)
	to := from.AddDate(0, 0, 7)

	mock.ExpectQuery(`SELECT id, ticker, side`).
		WithArgs(from, to).
		WillReturnRows(sqlmock.NewRows(reportCols).
			AddRow(1, "RELIANCE", "LONG", 10, 100.0, 95.0, 110.0,
				"Energy", "CLOSED", "exec-a", 110.0, 100.0, "take_profit",
				time.Date(2026, 8, 3, 15, 30, 0, 0, time.Local)).
			AddRow(2, "TCS", "SHORT", 5, 4000.0, 4200.0, 3800.0,
				"IT", "CLOSED", nil, nil, nil, nil, nil)) // legacy row predating exit accounting

	got, err := database.NewReportStore(db).ClosedPositions(context.Background(), from, to)
	if err != nil {
		t.Fatalf("ClosedPositions: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(got))
	}
	if got[0].PnL != 100 || got[0].ExitReason != models.ReasonTakeProfit || got[0].ClosedAt.IsZero() {
		t.Fatalf("row 0 not scanned: %+v", got[0])
	}
	if got[1].ExecutionRef != "" || got[1].PnL != 0 || !got[1].ClosedAt.IsZero() {
		t.Fatalf("NULLs must scan to zero values: %+v", got[1])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestClosedPositions_QueryErrorPropagates(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()

	mock.ExpectQuery(`SELECT id, ticker, side`).
		WillReturnError(errors.New("db unavailable"))

	if _, err := database.NewReportStore(db).ClosedPositions(
		context.Background(), time.Now().AddDate(0, 0, -1), time.Now(),
	); err == nil {
		t.Fatal("expected query error")
	}
}

func TestComputeSummary_EmptyInput(t *testing.T) {
	s := database.ComputeSummary(nil)
	if s.ClosedTrades != 0 || s.TotalPnL != 0 || s.WinRate != 0 {
		t.Fatalf("zero-value summary expected: %+v", s)
	}
	if s.ByTicker == nil || s.ByDay == nil || s.ByExitReason == nil {
		t.Fatal("maps must be non-nil for JSON encoding")
	}
}

func TestComputeSummary_WinRateAndAverages(t *testing.T) {
	day := func(d int) time.Time {
		return time.Date(2026, 8, d, 15, 30, 0, 0, time.Local)
	}
	pos := func(ticker string, pnl float64, reason models.ExitReason, at time.Time) models.Position {
		return models.Position{
			Ticker: ticker, Status: models.PositionClosed,
			PnL: pnl, ExitReason: reason, ClosedAt: at,
		}
	}

	positions := []models.Position{
		pos("RELIANCE", 100, models.ReasonTakeProfit, day(4)),
		pos("TCS", -30, models.ReasonStopLoss, day(5)),
		pos("INFY", 50, models.ReasonSquareOff, day(6)),
	}

	s := database.ComputeSummary(positions)

	if s.ClosedTrades != 3 || s.TotalPnL != 120 {
		t.Fatalf("counts wrong: %+v", s)
	}
	if s.Wins != 2 || s.Losses != 1 {
		t.Fatalf("win/loss split wrong: %+v", s)
	}
	if s.WinRate < 0.6666 || s.WinRate > 0.6668 {
		t.Fatalf("win rate = %v", s.WinRate)
	}
	if s.AvgWin != 75 || s.AvgLoss != -30 {
		t.Fatalf("averages wrong: %+v", s)
	}
	if s.ByExitReason[string(models.ReasonStopLoss)] != -30 ||
		s.ByDay["2026-08-04"] != 100 ||
		s.ByTicker["INFY"] != 50 {
		t.Fatalf("breakdowns wrong: %+v", s)
	}
}

func TestComputeSummary_MaxDrawdownIsSequenceAware(t *testing.T) {
	at := func(d int) time.Time {
		return time.Date(2026, 8, d, 15, 30, 0, 0, time.Local)
	}
	mk := func(pnl float64, d int) models.Position {
		return models.Position{Status: models.PositionClosed, PnL: pnl, ClosedAt: at(d)}
	}

	// Chronological: +100 (peak), -150 (equity -50) → drawdown 150.
	// Deliberately shuffled to prove ComputeSummary re-sorts by ClosedAt.
	s := database.ComputeSummary([]models.Position{mk(-150, 5), mk(100, 4)})
	if s.MaxDrawdown != 150 {
		t.Fatalf("max drawdown = %v, want 150", s.MaxDrawdown)
	}
}

func TestComputeSummary_SkipsOpenPositions(t *testing.T) {
	open := models.Position{Status: models.PositionOpen, PnL: 999} // nonsense PnL on an open row
	closed := models.Position{
		Status: models.PositionClosed, PnL: 25,
		ClosedAt: time.Date(2026, 8, 10, 15, 0, 0, 0, time.Local),
	}

	s := database.ComputeSummary([]models.Position{open, closed})
	if s.ClosedTrades != 1 || s.TotalPnL != 25 || s.Wins != 1 {
		t.Fatalf("open row must be ignored: %+v", s)
	}
}
