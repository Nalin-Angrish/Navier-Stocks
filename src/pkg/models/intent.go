package models

import (
	"errors"
	"time"
)

// TradeIntent is the signal published by the Quantitative Scout to the Risk
// Manager over NATS (signal.intent.new).  It carries the ticker, direction,
// current price, VWAP, volume-ratio confirmation, and a human-readable reason
// so the Risk Manager can evaluate sector exposure, timing guardrails, and
// capital allocation before promoting the intent to an execution.
type TradeIntent struct {
	Ticker       string    `json:"ticker"`
	Side         Side      `json:"side"`
	CurrentPrice float64   `json:"current_price"`
	VWAP         float64   `json:"vwap,omitempty"`
	VolumeRatio  float64   `json:"volume_ratio,omitempty"`
	SignalReason string    `json:"signal_reason"`
	Timestamp    time.Time `json:"timestamp"`
}

// Validate checks that required fields are present and the side is valid.
func (t *TradeIntent) Validate() error {
	if t.Ticker == "" {
		return errors.New("ticker is required")
	}
	if t.Side != SideLong && t.Side != SideShort {
		return errors.New("side must be LONG or SHORT")
	}
	if t.CurrentPrice <= 0 {
		return errors.New("current_price must be positive")
	}
	if t.SignalReason == "" {
		return errors.New("signal_reason is required")
	}
	return nil
}
