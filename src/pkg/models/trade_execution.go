package models

import "errors"

type Side string

const (
	SideLong  Side = "LONG"
	SideShort Side = "SHORT"
)

type Status string

const (
	StatusReceived  Status = "RECEIVED"
	StatusExecuted  Status = "EXECUTED"
	StatusFailed    Status = "FAILED"
	StatusReclaimed Status = "RECLAIMED"
)

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
