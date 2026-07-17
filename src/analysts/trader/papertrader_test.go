package trader_test

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/Nalin-Angrish/Navier-Stocks/src/analysts/trader"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/database"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
)

func TestNewPaperTrader(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	pt := trader.NewPaperTrader(
		database.NewPositionStore(db),
		database.NewTradeLogStore(db),
	)
	if pt == nil {
		t.Fatal("NewPaperTrader returned nil")
	}
}

func TestPaperTrader_Execute_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	pt := trader.NewPaperTrader(
		database.NewPositionStore(db),
		database.NewTradeLogStore(db),
	)

	mock.ExpectQuery(`INSERT INTO positions`).
		WithArgs("RELIANCE", "LONG", 10, 2500.50, 2400.00, 2750.00, "Energy", "OPEN", "exec-001").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	mock.ExpectExec(`INSERT INTO trade_log`).
		WithArgs("RELIANCE", "LONG", 10, 2500.50, "VWAP breakout", "SIMULATED").
		WillReturnResult(sqlmock.NewResult(1, 1))

	order := &models.TradeExecution{
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

	result, err := pt.Execute(order)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if result.BrokerOrderID == "" {
		t.Fatal("expected non-empty BrokerOrderID")
	}
	if result.ExecutedPrice != 2500.50 {
		t.Fatalf("ExecutedPrice = %f, want %f", result.ExecutedPrice, 2500.50)
	}
	if result.ExecutedQty != 10 {
		t.Fatalf("ExecutedQty = %d, want %d", result.ExecutedQty, 10)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestPaperTrader_Execute_ShortSide(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	pt := trader.NewPaperTrader(
		database.NewPositionStore(db),
		database.NewTradeLogStore(db),
	)

	mock.ExpectQuery(`INSERT INTO positions`).
		WithArgs("TCS", "SHORT", 25, 3450.00, 3277.50, 3622.50, "IT", "OPEN", "exec-short-001").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(2))

	mock.ExpectExec(`INSERT INTO trade_log`).
		WithArgs("TCS", "SHORT", 25, 3450.00, "Bearish", "SIMULATED").
		WillReturnResult(sqlmock.NewResult(1, 1))

	order := &models.TradeExecution{
		Ticker:       "TCS",
		Side:         models.SideShort,
		Quantity:     25,
		Price:        3450.00,
		StopLoss:     3277.50,
		TakeProfit:   3622.50,
		Sector:       "IT",
		SignalReason: "Bearish",
		ExecutionRef: "exec-short-001",
	}

	result, err := pt.Execute(order)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if result.BrokerOrderID == "" {
		t.Fatal("expected non-empty BrokerOrderID")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestPaperTrader_Execute_PositionInsertFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	pt := trader.NewPaperTrader(
		database.NewPositionStore(db),
		database.NewTradeLogStore(db),
	)

	mock.ExpectQuery(`INSERT INTO positions`).
		WillReturnError(assertAnError)

	order := &models.TradeExecution{
		Ticker:       "INFY",
		Side:         models.SideLong,
		Quantity:     5,
		Price:        1500.00,
		StopLoss:     1425.00,
		TakeProfit:   1575.00,
		Sector:       "IT",
		ExecutionRef: "exec-fail",
	}

	_, err = pt.Execute(order)
	if err == nil {
		t.Fatal("expected error from Execute when position insert fails")
	}
}

func TestPaperTrader_Execute_TradeLogInsertFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	pt := trader.NewPaperTrader(
		database.NewPositionStore(db),
		database.NewTradeLogStore(db),
	)

	mock.ExpectQuery(`INSERT INTO positions`).
		WithArgs("HDFC", "LONG", 15, 1650.00, 1567.50, 1732.50, "BANKING", "OPEN", "exec-tradefail").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3))

	mock.ExpectExec(`INSERT INTO trade_log`).
		WillReturnError(assertAnError)

	order := &models.TradeExecution{
		Ticker:       "HDFC",
		Side:         models.SideLong,
		Quantity:     15,
		Price:        1650.00,
		StopLoss:     1567.50,
		TakeProfit:   1732.50,
		Sector:       "BANKING",
		ExecutionRef: "exec-tradefail",
	}

	_, err = pt.Execute(order)
	if err == nil {
		t.Fatal("expected error from Execute when trade_log insert fails")
	}
}

func TestPaperTrader_Execute_DefaultsMissingFields(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	pt := trader.NewPaperTrader(
		database.NewPositionStore(db),
		database.NewTradeLogStore(db),
	)

	mock.ExpectQuery(`INSERT INTO positions`).
		WithArgs("WIPRO", "LONG", 20, 500.00, 475.00, 525.00, "GENERAL", "OPEN", "exec-defaults").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(4))

	mock.ExpectExec(`INSERT INTO trade_log`).
		WithArgs("WIPRO", "LONG", 20, 500.00, "", "SIMULATED").
		WillReturnResult(sqlmock.NewResult(1, 1))

	order := &models.TradeExecution{
		Ticker:       "WIPRO",
		Side:         models.SideLong,
		Quantity:     20,
		Price:        500.00,
		ExecutionRef: "exec-defaults",
	}

	result, err := pt.Execute(order)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if result.ExecutedPrice != 500.00 {
		t.Fatalf("ExecutedPrice = %f, want %f", result.ExecutedPrice, 500.00)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
