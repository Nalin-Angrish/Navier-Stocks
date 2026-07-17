package models_test

import (
	"encoding/json"
	"testing"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
)

func validExecution() models.TradeExecution {
	return models.TradeExecution{
		Ticker:       "RELIANCE",
		Side:         models.SideLong,
		Quantity:     10,
		Price:        2500.50,
		StopLoss:     2400.00,
		TakeProfit:   2750.00,
		Sector:       "Energy",
		SignalReason: "VWAP breakout",
		ExecutionRef: "exec-001",
	}
}

func TestTradeExecution_Validate_Valid(t *testing.T) {
	exec := validExecution()
	if err := exec.Validate(); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestTradeExecution_Validate_MissingTicker(t *testing.T) {
	exec := validExecution()
	exec.Ticker = ""
	err := exec.Validate()
	if err == nil {
		t.Fatal("expected error for missing ticker")
	}
	if err.Error() != "ticker is required" {
		t.Fatalf("unexpected message: %v", err)
	}
}

func TestTradeExecution_Validate_InvalidSide(t *testing.T) {
	exec := validExecution()
	exec.Side = "INVALID"
	err := exec.Validate()
	if err == nil {
		t.Fatal("expected error for invalid side")
	}
	if err.Error() != "side must be LONG or SHORT" {
		t.Fatalf("unexpected message: %v", err)
	}
}

func TestTradeExecution_Validate_ZeroQuantity(t *testing.T) {
	exec := validExecution()
	exec.Quantity = 0
	err := exec.Validate()
	if err == nil {
		t.Fatal("expected error for zero quantity")
	}
	if err.Error() != "quantity must be positive" {
		t.Fatalf("unexpected message: %v", err)
	}
}

func TestTradeExecution_Validate_NegativeQuantity(t *testing.T) {
	exec := validExecution()
	exec.Quantity = -1
	err := exec.Validate()
	if err == nil {
		t.Fatal("expected error for negative quantity")
	}
}

func TestTradeExecution_Validate_ZeroPrice(t *testing.T) {
	exec := validExecution()
	exec.Price = 0
	err := exec.Validate()
	if err == nil {
		t.Fatal("expected error for zero price")
	}
	if err.Error() != "price must be positive" {
		t.Fatalf("unexpected message: %v", err)
	}
}

func TestTradeExecution_Validate_NegativePrice(t *testing.T) {
	exec := validExecution()
	exec.Price = -100
	err := exec.Validate()
	if err == nil {
		t.Fatal("expected error for negative price")
	}
}

func TestTradeExecution_Validate_MissingExecutionRef(t *testing.T) {
	exec := validExecution()
	exec.ExecutionRef = ""
	err := exec.Validate()
	if err == nil {
		t.Fatal("expected error for missing execution_ref")
	}
	if err.Error() != "execution_ref is required" {
		t.Fatalf("unexpected message: %v", err)
	}
}

func TestTradeExecution_Validate_AllErrorsSimultaneously(t *testing.T) {
	exec := models.TradeExecution{}
	err := exec.Validate()
	if err == nil {
		t.Fatal("expected error for empty execution")
	}
}

func TestSideConstants(t *testing.T) {
	if models.SideLong != "LONG" {
		t.Fatalf("SideLong = %q, want %q", models.SideLong, "LONG")
	}
	if models.SideShort != "SHORT" {
		t.Fatalf("SideShort = %q, want %q", models.SideShort, "SHORT")
	}
}

func TestStatusConstants(t *testing.T) {
	if models.StatusReceived != "RECEIVED" {
		t.Fatalf("StatusReceived = %q, want %q", models.StatusReceived, "RECEIVED")
	}
	if models.StatusExecuted != "EXECUTED" {
		t.Fatalf("StatusExecuted = %q, want %q", models.StatusExecuted, "EXECUTED")
	}
	if models.StatusFailed != "FAILED" {
		t.Fatalf("StatusFailed = %q, want %q", models.StatusFailed, "FAILED")
	}
	if models.StatusReclaimed != "RECLAIMED" {
		t.Fatalf("StatusReclaimed = %q, want %q", models.StatusReclaimed, "RECLAIMED")
	}
}

func TestTradeExecution_JSONRoundTrip(t *testing.T) {
	exec := validExecution()
	data, err := json.Marshal(exec)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var decoded models.TradeExecution
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if decoded.Ticker != exec.Ticker {
		t.Fatalf("Ticker = %q, want %q", decoded.Ticker, exec.Ticker)
	}
	if decoded.Side != exec.Side {
		t.Fatalf("Side = %q, want %q", decoded.Side, exec.Side)
	}
	if decoded.Quantity != exec.Quantity {
		t.Fatalf("Quantity = %d, want %d", decoded.Quantity, exec.Quantity)
	}
	if decoded.Price != exec.Price {
		t.Fatalf("Price = %f, want %f", decoded.Price, exec.Price)
	}
	if decoded.StopLoss != exec.StopLoss {
		t.Fatalf("StopLoss = %f, want %f", decoded.StopLoss, exec.StopLoss)
	}
	if decoded.TakeProfit != exec.TakeProfit {
		t.Fatalf("TakeProfit = %f, want %f", decoded.TakeProfit, exec.TakeProfit)
	}
	if decoded.Sector != exec.Sector {
		t.Fatalf("Sector = %q, want %q", decoded.Sector, exec.Sector)
	}
	if decoded.SignalReason != exec.SignalReason {
		t.Fatalf("SignalReason = %q, want %q", decoded.SignalReason, exec.SignalReason)
	}
	if decoded.ExecutionRef != exec.ExecutionRef {
		t.Fatalf("ExecutionRef = %q, want %q", decoded.ExecutionRef, exec.ExecutionRef)
	}
}

func TestTradeExecution_JSONFieldsOmitWhenEmpty(t *testing.T) {
	exec := models.TradeExecution{
		Ticker:       "TCS",
		Side:         models.SideShort,
		Quantity:     5,
		Price:        3500,
		ExecutionRef: "exec-002",
	}
	data, err := json.Marshal(exec)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if _, ok := raw["sector"]; ok {
		t.Fatal("expected sector to be omitted from JSON")
	}
	if _, ok := raw["signal_reason"]; ok {
		t.Fatal("expected signal_reason to be omitted from JSON")
	}
	if _, ok := raw["stop_loss"]; ok {
		t.Fatal("expected stop_loss to be omitted from JSON")
	}
	if _, ok := raw["take_profit"]; ok {
		t.Fatal("expected take_profit to be omitted from JSON")
	}
}
