package riskmanager_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Nalin-Angrish/Navier-Stocks/src/analysts/riskmanager"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/nats"
)

func TestBuildExecution_SizesQuantity(t *testing.T) {
	agent := riskmanager.NewAgentForTest(nil)
	agent.SetSectors(staticSectors{"RELIANCE": "ENERGY"})
	agent.SetExposure(riskmanager.NewExposure(1, 4))
	agent.SetCapital(1_000_000)

	intent := &models.TradeIntent{
		Ticker:       "RELIANCE",
		Side:         models.SideLong,
		CurrentPrice: 100,
		SignalReason: "VWAP breakout",
	}
	if err := agent.PromoteRaw(intent); err != nil {
		t.Fatalf("PromoteRaw: %v", err)
	}

	// ₹1M capital, 2% = ₹20,000 risk; entry 100, stop 95 → ₹5/share → 4,000.
	if got := agent.ExposeSector("ENERGY"); got != 1 {
		t.Fatalf("sector exposure after promote = %d, want 1", got)
	}
}

func TestPromote_InsufficientCapital(t *testing.T) {
	agent := riskmanager.NewAgentForTest(nil)
	agent.SetExposure(riskmanager.NewExposure(1, 4))
	agent.SetCapital(1_000)

	// ₹1,000 capital, entry 1000: per-share risk 50; 2% = ₹20 budget < ₹50.
	intent := &models.TradeIntent{
		Ticker:       "RELIANCE",
		Side:         models.SideLong,
		CurrentPrice: 1000,
		SignalReason: "test",
	}
	err := agent.PromoteRaw(intent)
	if !errors.Is(err, riskmanager.ErrInsufficientCapital) {
		t.Fatalf("expected ErrInsufficientCapital, got %v", err)
	}
}

func TestPromote_PublishesToPerTickerSubject(t *testing.T) {
	s := startJetStreamServer(t)
	js := connectJS(t, s)

	if err := js.EnsureStream(nats.StreamTrading); err != nil {
		t.Fatalf("EnsureStream: %v", err)
	}

	agent := riskmanager.NewAgentForTest(js)
	agent.SetSectors(staticSectors{"RELIANCE": "ENERGY"})
	agent.SetExposure(riskmanager.NewExposure(1, 4))
	agent.SetCapital(1_000_000)

	// Subscribe ahead of the publish so we observe what the agent sends.
	received := make(chan *models.TradeExecution, 1)
	sub, err := js.Subscribe("signal.execute.*", func(m *nats.Msg) {
		var exec models.TradeExecution
		if err := json.Unmarshal(m.Data, &exec); err == nil {
			received <- &exec
		}
		_ = m.Ack()
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })

	intent := &models.TradeIntent{
		Ticker:       "RELIANCE",
		Side:         models.SideLong,
		CurrentPrice: 100,
		SignalReason: "VWAP breakout",
	}
	if err := agent.PromoteRaw(intent); err != nil {
		t.Fatalf("PromoteRaw: %v", err)
	}

	select {
	case exec := <-received:
		if exec.Ticker != "RELIANCE" {
			t.Fatalf("execution ticker = %q, want RELIANCE", exec.Ticker)
		}
		if exec.Side != models.SideLong {
			t.Fatalf("execution side = %q, want LONG", exec.Side)
		}
		if exec.Sector != "ENERGY" {
			t.Fatalf("execution sector = %q, want ENERGY", exec.Sector)
		}
		if exec.Quantity <= 0 {
			t.Fatalf("execution quantity = %d, want > 0", exec.Quantity)
		}
		if exec.ExecutionRef == "" {
			t.Fatal("expected non-empty execution_ref")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for execution message")
	}
}

func TestExecuteSubject_Format(t *testing.T) {
	if got := riskmanager.ExecuteSubject("RELIANCE"); got != "signal.execute.RELIANCE" {
		t.Fatalf("ExecuteSubject = %q, want signal.execute.RELIANCE", got)
	}
}
