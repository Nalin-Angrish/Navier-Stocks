package database_test

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/database"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
)

var errStoreFailure = &mockDBError{"store unavailable"}

type mockDBError struct{ msg string }

func (e *mockDBError) Error() string { return e.msg }

func TestNewTradeLogStore(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	store := database.NewTradeLogStore(db)
	if store == nil {
		t.Fatal("NewTradeLogStore returned nil")
	}
}

func TestInsertTradeLog_ReceivedStatus(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	store := database.NewTradeLogStore(db)

	mock.ExpectExec(`INSERT INTO trade_log`).
		WithArgs("RELIANCE", "LONG", 10, 2500.50, "VWAP breakout", "RECEIVED").
		WillReturnResult(sqlmock.NewResult(1, 1))

	exec := &models.TradeExecution{
		Ticker:       "RELIANCE",
		Side:         models.SideLong,
		Quantity:     10,
		Price:        2500.50,
		SignalReason: "VWAP breakout",
		ExecutionRef: "exec-001",
	}

	if err := store.Insert(exec, models.StatusReceived); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestInsertTradeLog_SimulatedStatus(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	store := database.NewTradeLogStore(db)

	mock.ExpectExec(`INSERT INTO trade_log`).
		WithArgs("TCS", "SHORT", 5, 3500.00, "", "SIMULATED").
		WillReturnResult(sqlmock.NewResult(2, 1))

	exec := &models.TradeExecution{
		Ticker:       "TCS",
		Side:         models.SideShort,
		Quantity:     5,
		Price:        3500.00,
		ExecutionRef: "exec-002",
	}

	if err := store.Insert(exec, models.StatusSimulated); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestInsertTradeLog_DBError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	store := database.NewTradeLogStore(db)

	mock.ExpectExec(`INSERT INTO trade_log`).
		WithArgs("INFY", "LONG", 5, 1500.00, "Test", "RECEIVED").
		WillReturnError(errStoreFailure)

	exec := &models.TradeExecution{
		Ticker:       "INFY",
		Side:         models.SideLong,
		Quantity:     5,
		Price:        1500.00,
		SignalReason: "Test",
		ExecutionRef: "exec-003",
	}

	err = store.Insert(exec, models.StatusReceived)
	if err == nil {
		t.Fatal("expected error from Insert")
	}
	if err.Error() != "insert trade_log: store unavailable" {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestInsertTradeLog_AllStatuses(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	store := database.NewTradeLogStore(db)

	for _, status := range []models.Status{models.StatusReceived, models.StatusSimulated, models.StatusExecuted, models.StatusFailed} {
		mock.ExpectExec(`INSERT INTO trade_log`).
			WithArgs("HDFC", "LONG", 1, 100.00, "", string(status)).
			WillReturnResult(sqlmock.NewResult(1, 1))
	}

	exec := &models.TradeExecution{
		Ticker:       "HDFC",
		Side:         models.SideLong,
		Quantity:     1,
		Price:        100.00,
		ExecutionRef: "exec-statuses",
	}

	for _, status := range []models.Status{models.StatusReceived, models.StatusSimulated, models.StatusExecuted, models.StatusFailed} {
		if err := store.Insert(exec, status); err != nil {
			t.Fatalf("Insert with status %q: %v", status, err)
		}
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
