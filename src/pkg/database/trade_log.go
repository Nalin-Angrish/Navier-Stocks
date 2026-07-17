package database

import (
	"database/sql"
	"fmt"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
)

type TradeLogStore struct {
	db *sql.DB
}

func NewTradeLogStore(db *sql.DB) *TradeLogStore {
	return &TradeLogStore{db: db}
}

func (s *TradeLogStore) Insert(exec *models.TradeExecution, status models.Status) error {
	query := `
		INSERT INTO trade_log (ticker, side, quantity, price, signal_reason, status)
		VALUES ($1, $2, $3, $4, $5, $6)`

	_, err := s.db.Exec(query,
		exec.Ticker,
		string(exec.Side),
		exec.Quantity,
		exec.Price,
		exec.SignalReason,
		string(status),
	)
	if err != nil {
		return fmt.Errorf("insert trade_log: %w", err)
	}
	return nil
}
