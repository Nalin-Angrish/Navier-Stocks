package database_test

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/database"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
)

func TestNewPositionStore(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	store := database.NewPositionStore(db)
	if store == nil {
		t.Fatal("NewPositionStore returned nil")
	}
}

func TestInsertPosition_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	store := database.NewPositionStore(db)

	mock.ExpectQuery(`INSERT INTO positions`).
		WithArgs("RELIANCE", "LONG", 10, 2500.50, 2400.00, 2750.00, "Energy", "OPEN", "exec-001").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	pos := &models.Position{
		Ticker:       "RELIANCE",
		Side:         models.SideLong,
		Quantity:     10,
		EntryPrice:   2500.50,
		StopLoss:     2400.00,
		TakeProfit:   2750.00,
		Sector:       "Energy",
		Status:       models.PositionOpen,
		ExecutionRef: "exec-001",
	}

	if err := store.Insert(pos); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	if pos.ID != 1 {
		t.Fatalf("expected ID=1, got %d", pos.ID)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestInsertPosition_ShortSide(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	store := database.NewPositionStore(db)

	mock.ExpectQuery(`INSERT INTO positions`).
		WithArgs("TCS", "SHORT", 25, 3450.00, 3277.50, 3622.50, "IT", "OPEN", "exec-short-001").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(2))

	pos := &models.Position{
		Ticker:       "TCS",
		Side:         models.SideShort,
		Quantity:     25,
		EntryPrice:   3450.00,
		StopLoss:     3277.50,
		TakeProfit:   3622.50,
		Sector:       "IT",
		Status:       models.PositionOpen,
		ExecutionRef: "exec-short-001",
	}

	if err := store.Insert(pos); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	if pos.ID != 2 {
		t.Fatalf("expected ID=2, got %d", pos.ID)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestInsertPosition_DBError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	store := database.NewPositionStore(db)

	mock.ExpectQuery(`INSERT INTO positions`).
		WithArgs("INFY", "LONG", 5, 1500.00, 1425.00, 1575.00, "IT", "OPEN", "exec-fail").
		WillReturnError(errStoreFailure)

	pos := &models.Position{
		Ticker:       "INFY",
		Side:         models.SideLong,
		Quantity:     5,
		EntryPrice:   1500.00,
		StopLoss:     1425.00,
		TakeProfit:   1575.00,
		Sector:       "IT",
		Status:       models.PositionOpen,
		ExecutionRef: "exec-fail",
	}

	err = store.Insert(pos)
	if err == nil {
		t.Fatal("expected error from Insert")
	}
	if err.Error() != "insert position: store unavailable" {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
