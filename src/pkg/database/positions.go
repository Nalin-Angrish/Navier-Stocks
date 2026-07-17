package database

import (
	"database/sql"
	"fmt"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
)

// PositionStore wraps the positions table and provides CRUD access to
// the live portfolio state (open and closed positions).
type PositionStore struct {
	db *sql.DB
}

// NewPositionStore returns a PositionStore backed by the given database
// connection.
func NewPositionStore(db *sql.DB) *PositionStore {
	return &PositionStore{db: db}
}

// Insert creates a new position row and populates pos.ID with the
// auto-generated primary key.  The position is stored with the caller-
// provided status (typically models.PositionOpen).
func (s *PositionStore) Insert(pos *models.Position) error {
	query := `
		INSERT INTO positions (ticker, side, quantity, entry_price, stop_loss, take_profit, sector, status, execution_ref)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id`

	err := s.db.QueryRow(query,
		pos.Ticker,
		string(pos.Side),
		pos.Quantity,
		pos.EntryPrice,
		pos.StopLoss,
		pos.TakeProfit,
		pos.Sector,
		string(pos.Status),
		pos.ExecutionRef,
	).Scan(&pos.ID)
	if err != nil {
		return fmt.Errorf("insert position: %w", err)
	}
	return nil
}
