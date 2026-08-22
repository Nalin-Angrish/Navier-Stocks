package database

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
)

// Summary aggregates realized performance over a set of closed positions.
// It is produced by ComputeSummary, which is a pure function so the replay
// engine (8.4) can generate byte-identical reports without a database.
type Summary struct {
	ClosedTrades int                `json:"closed_trades"`
	TotalPnL     float64            `json:"total_pnl"`
	Wins         int                `json:"wins"`
	Losses       int                `json:"losses"`
	WinRate      float64            `json:"win_rate"` // 0..1; multiply by 100 for display
	AvgWin       float64            `json:"avg_win"`
	AvgLoss      float64            `json:"avg_loss"` // negative number
	MaxDrawdown  float64            `json:"max_drawdown"`
	ByTicker     map[string]float64 `json:"by_ticker"`
	ByDay        map[string]float64 `json:"by_day"` // YYYY-MM-DD → realized PnL
	ByExitReason map[string]float64 `json:"by_exit_reason"`
}

// ComputeSummary derives the aggregate performance view from closed
// positions in chronological close order (drawdown depends on sequence,
// so rows are sorted by ClosedAt internally — callers may pass any order).
// Positions that are not CLOSED are skipped defensively.
func ComputeSummary(positions []models.Position) Summary {
	s := Summary{
		ByTicker:     make(map[string]float64),
		ByDay:        make(map[string]float64),
		ByExitReason: make(map[string]float64),
	}

	ordered := append([]models.Position(nil), positions...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].ClosedAt.Before(ordered[j].ClosedAt)
	})

	var winSum, lossSum float64
	equity, peak := 0.0, 0.0

	for _, pos := range ordered {
		if pos.Status != models.PositionClosed {
			continue
		}

		s.ClosedTrades++
		s.TotalPnL += pos.PnL
		s.ByTicker[pos.Ticker] += pos.PnL
		s.ByDay[pos.ClosedAt.Format("2006-01-02")] += pos.PnL
		s.ByExitReason[string(pos.ExitReason)] += pos.PnL

		switch {
		case pos.PnL > 0:
			s.Wins++
			winSum += pos.PnL
		case pos.PnL < 0:
			s.Losses++
			lossSum += pos.PnL
		}

		equity += pos.PnL
		if equity > peak {
			peak = equity
		}
		if dd := peak - equity; dd > s.MaxDrawdown {
			s.MaxDrawdown = dd
		}
	}

	if s.Wins+s.Losses > 0 {
		s.WinRate = float64(s.Wins) / float64(s.Wins+s.Losses)
	}
	if s.Wins > 0 {
		s.AvgWin = winSum / float64(s.Wins)
	}
	if s.Losses > 0 {
		s.AvgLoss = lossSum / float64(s.Losses)
	}
	return s
}

// ReportStore reads closed-position history for performance reporting.
// It is read-only by design: reporting must never mutate live state.
type ReportStore struct {
	db *sql.DB
}

// NewReportStore returns a ReportStore backed by the given database
// connection.
func NewReportStore(db *sql.DB) *ReportStore {
	return &ReportStore{db: db}
}

// ClosedPositions returns every position closed within [from, to]
// (inclusive), ordered oldest → newest so callers can stream them through
// ComputeSummary or feed them to the replay engine.
func (r *ReportStore) ClosedPositions(ctx context.Context, from, to time.Time) ([]models.Position, error) {
	query := `
		SELECT id, ticker, side, quantity, entry_price, stop_loss, take_profit,
		       sector, status, execution_ref,
		       exit_price, pnl, exit_reason, closed_at
		FROM positions
		WHERE status = 'CLOSED' AND closed_at >= $1 AND closed_at < $2
		ORDER BY closed_at ASC`

	rows, err := r.db.QueryContext(ctx, query, from, to)
	if err != nil {
		return nil, fmt.Errorf("query closed positions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []models.Position{}
	for rows.Next() {
		var (
			pos      models.Position
			exitRef  sql.NullString
			exitPr   sql.NullFloat64
			pnl      sql.NullFloat64
			reason   sql.NullString
			closedAt sql.NullTime
		)
		if err := rows.Scan(
			&pos.ID, &pos.Ticker, &pos.Side, &pos.Quantity,
			&pos.EntryPrice, &pos.StopLoss, &pos.TakeProfit,
			&pos.Sector, &pos.Status, &exitRef,
			&exitPr, &pnl, &reason, &closedAt,
		); err != nil {
			return nil, fmt.Errorf("scan closed position: %w", err)
		}
		pos.ExecutionRef = exitRef.String
		pos.ExitPrice = exitPr.Float64
		pos.PnL = pnl.Float64
		if reason.Valid {
			pos.ExitReason = models.ExitReason(reason.String)
		}
		if closedAt.Valid {
			pos.ClosedAt = closedAt.Time
		}
		out = append(out, pos)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate closed positions: %w", err)
	}
	return out, nil
}
