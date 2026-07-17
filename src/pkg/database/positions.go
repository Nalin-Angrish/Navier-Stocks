package database

import (
	"database/sql"
	"fmt"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
)

type PositionStore struct {
	db *sql.DB
}

func NewPositionStore(db *sql.DB) *PositionStore {
	return &PositionStore{db: db}
}

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
