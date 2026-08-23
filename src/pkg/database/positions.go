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

// DB returns the underlying database connection for direct queries.
func (s *PositionStore) DB() *sql.DB {
	return s.db
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

// ListOpen returns every open position in the portfolio, newest first.  The
// Risk Manager's auto-square-off routine uses this to enumerate the MIS
// positions that must be liquidated before the exchange closes.
func (s *PositionStore) ListOpen() ([]models.Position, error) {
	query := `
		SELECT id, ticker, side, quantity, entry_price, stop_loss, take_profit, sector, status, execution_ref
		FROM positions
		WHERE status = 'OPEN'
		ORDER BY id DESC`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("list open positions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var positions []models.Position
	for rows.Next() {
		var pos models.Position
		if err := rows.Scan(
			&pos.ID,
			&pos.Ticker,
			&pos.Side,
			&pos.Quantity,
			&pos.EntryPrice,
			&pos.StopLoss,
			&pos.TakeProfit,
			&pos.Sector,
			&pos.Status,
			&pos.ExecutionRef,
		); err != nil {
			return nil, fmt.Errorf("scan open position: %w", err)
		}
		positions = append(positions, pos)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate open positions: %w", err)
	}
	return positions, nil
}

// MarkClosed transitions a single open position to CLOSED, stamping the
// exit price, exit reason, and realized P&L.  PnL is computed database-side
// as (exit_price − entry_price) × quantity, sign-flipped for SHORT positions,
// so the arithmetic lives in one authoritative place.  It returns
// sql.ErrNoRows if the position is not currently open (the update affects
// zero rows), so callers can treat already-closed positions as a no-op.
func (s *PositionStore) MarkClosed(id int64, exitPrice float64, reason models.ExitReason) error {
	query := `
		UPDATE positions
		SET status = 'CLOSED',
		    closed_at = NOW(),
		    exit_price = $2,
		    pnl = ($2 - entry_price) * quantity *
		          (CASE WHEN side = 'LONG' THEN 1 ELSE -1 END),
		    exit_reason = $3
		WHERE id = $1 AND status = 'OPEN'`

	res, err := s.db.Exec(query, id, exitPrice, string(reason))
	if err != nil {
		return fmt.Errorf("mark position closed: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}
