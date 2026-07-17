package models

import "errors"

type PositionStatus string

const (
	PositionOpen   PositionStatus = "OPEN"
	PositionClosed PositionStatus = "CLOSED"
)

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
}

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
