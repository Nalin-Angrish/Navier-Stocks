// Package models defines the domain types shared across all four agents.
// Each struct maps to a NATS message payload or a database row and includes
// JSON tags for serialisation and a Validate() method for guard checks.
package models

import "errors"

// Side represents the direction of a trade.
type Side string

const (
	SideLong  Side = "LONG"
	SideShort Side = "SHORT"
)

// Status represents the lifecycle state of a trade-log entry or position.
type Status string

const (
	StatusReceived  Status = "RECEIVED"  // logged before execution
	StatusSimulated Status = "SIMULATED" // paper-trader fill
	StatusExecuted  Status = "EXECUTED"  // confirmed live fill
	StatusFailed    Status = "FAILED"    // execution rejected by broker
	StatusOpen      Status = "OPEN"      // position is active
	StatusClosed    Status = "CLOSED"    // position has been squared off
	StatusReclaimed Status = "RECLAIMED" // order was cancelled / expired
)

// TradeExecution is the command sent by the Risk Manager over NATS
// (signal.execute.*).  It carries everything the Trader Gateway needs
// to place a trade: ticker, side, quantity, price, and optional risk
// parameters (stop-loss, take-profit, sector).  The ExecutionRef links
// back to the original intent for reconciliation.
type TradeExecution struct {
	Ticker       string  `json:"ticker"`
	Side         Side    `json:"side"`
	Quantity     int     `json:"quantity"`
	Price        float64 `json:"price"`
	StopLoss     float64 `json:"stop_loss,omitempty"`
	TakeProfit   float64 `json:"take_profit,omitempty"`
	Sector       string  `json:"sector,omitempty"`
	SignalReason string  `json:"signal_reason,omitempty"`
	ExecutionRef string  `json:"execution_ref,omitempty"`
}

// Validate checks that all required fields are present and within expected
// ranges.  It returns the first error encountered or nil if the execution
// is well-formed.
func (t *TradeExecution) Validate() error {
	if t.Ticker == "" {
		return errors.New("ticker is required")
	}
	if t.Side != SideLong && t.Side != SideShort {
		return errors.New("side must be LONG or SHORT")
	}
	if t.Quantity <= 0 {
		return errors.New("quantity must be positive")
	}
	if t.Price <= 0 {
		return errors.New("price must be positive")
	}
	if t.ExecutionRef == "" {
		return errors.New("execution_ref is required")
	}
	return nil
}
