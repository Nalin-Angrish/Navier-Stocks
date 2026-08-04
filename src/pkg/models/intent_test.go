package models_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
)

func TestTradeIntent_Validate_Valid(t *testing.T) {
	intent := models.TradeIntent{
		Ticker:       "RELIANCE",
		Side:         models.SideLong,
		CurrentPrice: 2500.50,
		SignalReason: "VWAP_breakout_plus_volume",
		Timestamp:    time.Now(),
	}
	if err := intent.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTradeIntent_Validate_EmptyTicker(t *testing.T) {
	intent := models.TradeIntent{
		Ticker:       "",
		Side:         models.SideLong,
		CurrentPrice: 100,
		SignalReason: "test",
	}
	if err := intent.Validate(); err == nil {
		t.Fatal("expected error for empty ticker")
	}
}

func TestTradeIntent_Validate_InvalidSide(t *testing.T) {
	intent := models.TradeIntent{
		Ticker:       "TCS",
		Side:         "INVALID",
		CurrentPrice: 100,
		SignalReason: "test",
	}
	if err := intent.Validate(); err == nil {
		t.Fatal("expected error for invalid side")
	}
}

func TestTradeIntent_Validate_ZeroPrice(t *testing.T) {
	intent := models.TradeIntent{
		Ticker:       "TCS",
		Side:         models.SideShort,
		CurrentPrice: 0,
		SignalReason: "test",
	}
	if err := intent.Validate(); err == nil {
		t.Fatal("expected error for zero price")
	}
}

func TestTradeIntent_Validate_EmptyReason(t *testing.T) {
	intent := models.TradeIntent{
		Ticker:       "TCS",
		Side:         models.SideLong,
		CurrentPrice: 100,
		SignalReason: "",
	}
	if err := intent.Validate(); err == nil {
		t.Fatal("expected error for empty signal reason")
	}
}

func TestTradeIntent_Validate_NegativePrice(t *testing.T) {
	intent := models.TradeIntent{
		Ticker:       "TCS",
		Side:         models.SideShort,
		CurrentPrice: -10,
		SignalReason: "test",
	}
	if err := intent.Validate(); err == nil {
		t.Fatal("expected error for negative price")
	}
}

func TestTradeIntent_JSONRoundTrip(t *testing.T) {
	now := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	intent := models.TradeIntent{
		Ticker:       "SBIN",
		Side:         models.SideLong,
		CurrentPrice: 650.25,
		VWAP:         645.00,
		VolumeRatio:  2.5,
		SignalReason: "VWAP_breakout_plus_volume_plus_BB",
		Timestamp:    now,
	}

	data, err := json.Marshal(&intent)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded models.TradeIntent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Ticker != intent.Ticker {
		t.Fatalf("Ticker = %q, want %q", decoded.Ticker, intent.Ticker)
	}
	if decoded.Side != intent.Side {
		t.Fatalf("Side = %q, want %q", decoded.Side, intent.Side)
	}
	if decoded.CurrentPrice != intent.CurrentPrice {
		t.Fatalf("CurrentPrice = %f, want %f", decoded.CurrentPrice, intent.CurrentPrice)
	}
	if decoded.VWAP != intent.VWAP {
		t.Fatalf("VWAP = %f, want %f", decoded.VWAP, intent.VWAP)
	}
	if decoded.VolumeRatio != intent.VolumeRatio {
		t.Fatalf("VolumeRatio = %f, want %f", decoded.VolumeRatio, intent.VolumeRatio)
	}
	if decoded.SignalReason != intent.SignalReason {
		t.Fatalf("SignalReason = %q, want %q", decoded.SignalReason, intent.SignalReason)
	}
}
