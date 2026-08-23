package database

import (
	"database/sql"
	"fmt"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
)

// TradeLogStore wraps the trade_log table, which serves as the immutable
// audit trail for every execution attempt — both simulated and live.
type TradeLogStore struct {
	db *sql.DB
}

// NewTradeLogStore returns a TradeLogStore backed by the given database
// connection.
func NewTradeLogStore(db *sql.DB) *TradeLogStore {
	return &TradeLogStore{db: db}
}

// Insert appends a row to the trade_log table with the given status
// (e.g. RECEIVED, SIMULATED, EXECUTED, FAILED).  The caller is responsible
// for supplying the appropriate status for the execution phase.
func (s *TradeLogStore) Insert(exec *models.TradeExecution, status models.Status) error {
	query := `
		INSERT INTO trade_log (ticker, side, quantity, price, signal_reason, status, position_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`

	_, err := s.db.Exec(query,
		exec.Ticker,
		string(exec.Side),
		exec.Quantity,
		exec.Price,
		exec.SignalReason,
		string(status),
		exec.PositionID,
	)
	if err != nil {
		return fmt.Errorf("insert trade_log: %w", err)
	}
	return nil
}
