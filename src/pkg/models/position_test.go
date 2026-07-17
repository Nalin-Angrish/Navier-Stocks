package models_test

import (
	"encoding/json"
	"testing"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
)

func validPosition() models.Position {
	return models.Position{
		Ticker:       "RELIANCE",
		Side:         models.SideLong,
		Quantity:     10,
		EntryPrice:   2500.50,
		StopLoss:     2400.00,
		TakeProfit:   2750.00,
		Sector:       "Energy",
		Status:       models.PositionOpen,
		ExecutionRef: "exec-001",
	}
}

func TestPosition_Validate_Valid(t *testing.T) {
	p := validPosition()
	if err := p.Validate(); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestPosition_Validate_MissingTicker(t *testing.T) {
	p := validPosition()
	p.Ticker = ""
	err := p.Validate()
	if err == nil {
		t.Fatal("expected error for missing ticker")
	}
	if err.Error() != "ticker is required" {
		t.Fatalf("unexpected message: %v", err)
	}
}

func TestPosition_Validate_InvalidSide(t *testing.T) {
	p := validPosition()
	p.Side = "INVALID"
	err := p.Validate()
	if err == nil {
		t.Fatal("expected error for invalid side")
	}
	if err.Error() != "side must be LONG or SHORT" {
		t.Fatalf("unexpected message: %v", err)
	}
}

func TestPosition_Validate_ZeroQuantity(t *testing.T) {
	p := validPosition()
	p.Quantity = 0
	err := p.Validate()
	if err == nil {
		t.Fatal("expected error for zero quantity")
	}
}

func TestPosition_Validate_ZeroEntryPrice(t *testing.T) {
	p := validPosition()
	p.EntryPrice = 0
	err := p.Validate()
	if err == nil {
		t.Fatal("expected error for zero entry_price")
	}
	if err.Error() != "entry_price must be positive" {
		t.Fatalf("unexpected message: %v", err)
	}
}

func TestPosition_Validate_ZeroStopLoss(t *testing.T) {
	p := validPosition()
	p.StopLoss = 0
	err := p.Validate()
	if err == nil {
		t.Fatal("expected error for zero stop_loss")
	}
	if err.Error() != "stop_loss must be positive" {
		t.Fatalf("unexpected message: %v", err)
	}
}

func TestPosition_Validate_ZeroTakeProfit(t *testing.T) {
	p := validPosition()
	p.TakeProfit = 0
	err := p.Validate()
	if err == nil {
		t.Fatal("expected error for zero take_profit")
	}
	if err.Error() != "take_profit must be positive" {
		t.Fatalf("unexpected message: %v", err)
	}
}

func TestPosition_Validate_MissingSector(t *testing.T) {
	p := validPosition()
	p.Sector = ""
	err := p.Validate()
	if err == nil {
		t.Fatal("expected error for missing sector")
	}
	if err.Error() != "sector is required" {
		t.Fatalf("unexpected message: %v", err)
	}
}

func TestPosition_Validate_MissingExecutionRef(t *testing.T) {
	p := validPosition()
	p.ExecutionRef = ""
	err := p.Validate()
	if err == nil {
		t.Fatal("expected error for missing execution_ref")
	}
	if err.Error() != "execution_ref is required" {
		t.Fatalf("unexpected message: %v", err)
	}
}

func TestPosition_Validate_InvalidStatus(t *testing.T) {
	p := validPosition()
	p.Status = "INVALID"
	err := p.Validate()
	if err == nil {
		t.Fatal("expected error for invalid status")
	}
	if err.Error() != "status must be OPEN or CLOSED" {
		t.Fatalf("unexpected message: %v", err)
	}
}

func TestPosition_StatusConstants(t *testing.T) {
	if models.PositionOpen != "OPEN" {
		t.Fatalf("PositionOpen = %q, want %q", models.PositionOpen, "OPEN")
	}
	if models.PositionClosed != "CLOSED" {
		t.Fatalf("PositionClosed = %q, want %q", models.PositionClosed, "CLOSED")
	}
}

func TestPosition_JSONRoundTrip(t *testing.T) {
	p := validPosition()
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var decoded models.Position
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if decoded.Ticker != p.Ticker {
		t.Fatalf("Ticker = %q, want %q", decoded.Ticker, p.Ticker)
	}
	if decoded.Side != p.Side {
		t.Fatalf("Side = %q, want %q", decoded.Side, p.Side)
	}
	if decoded.Quantity != p.Quantity {
		t.Fatalf("Quantity = %d, want %d", decoded.Quantity, p.Quantity)
	}
	if decoded.EntryPrice != p.EntryPrice {
		t.Fatalf("EntryPrice = %f, want %f", decoded.EntryPrice, p.EntryPrice)
	}
	if decoded.StopLoss != p.StopLoss {
		t.Fatalf("StopLoss = %f, want %f", decoded.StopLoss, p.StopLoss)
	}
	if decoded.TakeProfit != p.TakeProfit {
		t.Fatalf("TakeProfit = %f, want %f", decoded.TakeProfit, p.TakeProfit)
	}
	if decoded.Sector != p.Sector {
		t.Fatalf("Sector = %q, want %q", decoded.Sector, p.Sector)
	}
	if decoded.Status != p.Status {
		t.Fatalf("Status = %q, want %q", decoded.Status, p.Status)
	}
	if decoded.ExecutionRef != p.ExecutionRef {
		t.Fatalf("ExecutionRef = %q, want %q", decoded.ExecutionRef, p.ExecutionRef)
	}
}
