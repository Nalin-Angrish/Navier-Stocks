package models

import "errors"

// PositionStatus constrains the lifecycle state of a position row.
type PositionStatus string

const (
	PositionOpen   PositionStatus = "OPEN"
	PositionClosed PositionStatus = "CLOSED"
)

// ExitReason records why a position was closed.  It is persisted on the
// position row and drives performance attribution in the reporting layer.
type ExitReason string

const (
	ReasonStopLoss   ExitReason = "stop_loss"   // intraday exit monitor hit the risk boundary
	ReasonTakeProfit ExitReason = "take_profit" // intraday exit monitor hit the reward boundary
	ReasonSquareOff  ExitReason = "square_off"  // end-of-day forced liquidation
)

// Position represents an open or closed trade in the portfolio.  It is
// created by the Trader Gateway (via PaperTrader or GrowwTrader) when an
// execution signal is processed and stored in the positions table.
//
// The ExecutionRef links the position back to the originating
// TradeExecution for audit and reconciliation purposes.
type Position struct {
	ID           int64          `json:"id"`
	Ticker       string         `json:"ticker"`
	Side         Side           `json:"side"`
	Quantity     int            `json:"quantity"`
	EntryPrice   float64        `json:"entry_price"`
	StopLoss     float64        `json:"stop_loss"`
	TakeProfit   float64        `json:"take_profit"`
	Sector       string         `json:"sector"`
	Status       PositionStatus `json:"status"`
	ExecutionRef string         `json:"execution_ref,omitempty"`

	// Exit accounting — populated when the position transitions to CLOSED.
	// PnL is computed database-side as (exit_price - entry_price) * quantity
	// with the sign flipped for SHORT positions.
	ExitPrice  float64    `json:"exit_price,omitempty"`
	PnL        float64    `json:"pnl,omitempty"`
	ExitReason ExitReason `json:"exit_reason,omitempty"`
}

// Validate checks that all required fields are present and within expected
// ranges.  Unlike TradeExecution, a Position requires a Sector and both
// risk boundaries (stop-loss, take-profit).  Returns the first error or nil.
func (p *Position) Validate() error {
	if p.Ticker == "" {
		return errors.New("ticker is required")
	}
	if p.Side != SideLong && p.Side != SideShort {
		return errors.New("side must be LONG or SHORT")
	}
	if p.Quantity <= 0 {
		return errors.New("quantity must be positive")
	}
	if p.EntryPrice <= 0 {
		return errors.New("entry_price must be positive")
	}
	if p.StopLoss <= 0 {
		return errors.New("stop_loss must be positive")
	}
	if p.TakeProfit <= 0 {
		return errors.New("take_profit must be positive")
	}
	if p.Sector == "" {
		return errors.New("sector is required")
	}
	if p.ExecutionRef == "" {
		return errors.New("execution_ref is required")
	}
	if p.Status != PositionOpen && p.Status != PositionClosed {
		return errors.New("status must be OPEN or CLOSED")
	}
	return nil
}
