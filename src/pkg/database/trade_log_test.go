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

func TestInsertTradeLogEntry_Success(t *testing.T) {
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
		StopLoss:     2400.00,
		TakeProfit:   2750.00,
		Sector:       "Energy",
		SignalReason: "VWAP breakout",
		ExecutionRef: "exec-001",
	}

	if err := store.InsertTradeLogEntry(exec); err != nil {
		t.Fatalf("InsertTradeLogEntry: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestInsertTradeLogEntry_ShortSideNoOptionalFields(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	store := database.NewTradeLogStore(db)

	mock.ExpectExec(`INSERT INTO trade_log`).
		WithArgs("TCS", "SHORT", 5, 3500.00, "", "RECEIVED").
		WillReturnResult(sqlmock.NewResult(2, 1))

	exec := &models.TradeExecution{
		Ticker:       "TCS",
		Side:         models.SideShort,
		Quantity:     5,
		Price:        3500.00,
		ExecutionRef: "exec-002",
	}

	if err := store.InsertTradeLogEntry(exec); err != nil {
		t.Fatalf("InsertTradeLogEntry: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestInsertTradeLogEntry_DBError(t *testing.T) {
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

	err = store.InsertTradeLogEntry(exec)
	if err == nil {
		t.Fatal("expected error from InsertTradeLogEntry")
	}

	if err.Error() != "insert trade_log: store unavailable" {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestInsertTradeLogEntry_ParameterOrder(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	store := database.NewTradeLogStore(db)

	mock.ExpectExec(`INSERT INTO trade_log`).
		WithArgs("HDFC", "LONG", 100, 1650.75, "SMA crossover", "RECEIVED").
		WillReturnResult(sqlmock.NewResult(3, 1))

	exec := &models.TradeExecution{
		Ticker:       "HDFC",
		Side:         models.SideLong,
		Quantity:     100,
		Price:        1650.75,
		SignalReason: "SMA crossover",
		ExecutionRef: "exec-004",
	}

	if err := store.InsertTradeLogEntry(exec); err != nil {
		t.Fatalf("InsertTradeLogEntry: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
