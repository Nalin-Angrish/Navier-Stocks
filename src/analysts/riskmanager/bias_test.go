package riskmanager_test

import (
	"errors"
	"testing"

	"github.com/Nalin-Angrish/Navier-Stocks/src/analysts/riskmanager"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
)

func TestBiasGate_FirstPositionAllowed(t *testing.T) {
	// An empty book has no directional skew; either side opens freely.
	if err := riskmanager.CheckBias(0, 0, models.SideLong, 3.0); err != nil {
		t.Fatalf("expected first long to pass, got %v", err)
	}
	if err := riskmanager.CheckBias(0, 0, models.SideShort, 3.0); err != nil {
		t.Fatalf("expected first short to pass, got %v", err)
	}
}

func TestBiasGate_OneSidedBookAllowsSameSide(t *testing.T) {
	// 2 longs, 0 shorts: adding a 3rd long keeps a purely-long book (no ratio
	// constraint applies when one side is empty — the concurrency ceiling
	// bounds the total footprint).
	if err := riskmanager.CheckBias(2, 0, models.SideLong, 3.0); err != nil {
		t.Fatalf("expected third long to pass, got %v", err)
	}
}

func TestBiasGate_LongDominanceRejected(t *testing.T) {
	// 3 longs + 1 short already sits exactly at 3:1.  Adding a 4th long would
	// push the ratio to 4:1, so Gate 3 must block it.
	if err := riskmanager.CheckBias(3, 1, models.SideLong, 3.0); !errors.Is(err, riskmanager.ErrBiasLimit) {
		t.Fatalf("expected ErrBiasLimit on 4th long vs 1 short, got %v", err)
	}
}

func TestBiasGate_ShortDominanceRejected(t *testing.T) {
	// 1 long + 3 shorts at exactly 3:1; a 4th short would exceed it.
	if err := riskmanager.CheckBias(1, 3, models.SideShort, 3.0); !errors.Is(err, riskmanager.ErrBiasLimit) {
		t.Fatalf("expected ErrBiasLimit on 4th short vs 1 long, got %v", err)
	}
}

func TestBiasGate_BalancedBookAllows(t *testing.T) {
	// 2 longs, 1 short = 2:1; a long keeps it 3:1 ≤ 3:1 → permitted.
	if err := riskmanager.CheckBias(2, 1, models.SideLong, 3.0); err != nil {
		t.Fatalf("expected balanced long to pass, got %v", err)
	}
}

func TestBiasGate_CounterbalanceAllowed(t *testing.T) {
	// 3 longs, 1 short; adding a short rebalances toward the minority side,
	// which never inflates the ratio — permitted.
	if err := riskmanager.CheckBias(3, 1, models.SideShort, 3.0); err != nil {
		t.Fatalf("expected counterbalancing short to pass, got %v", err)
	}
}

func TestAnalyse_BiasGateWired(t *testing.T) {
	agent := riskmanager.NewAgentForTest(nil)
	// High ceilings so only the bias gate can reject the 4th long while
	// the book holds 3 longs : 1 short.
	exp := riskmanager.NewExposure(4, 100)
	agent.SetExposure(exp)
	exp.Register(models.SideLong, "IT")
	exp.Register(models.SideLong, "AUTO")
	exp.Register(models.SideLong, "METAL")
	exp.Register(models.SideShort, "BANKING")

	// 3 longs : 1 short; a 4th long crosses the 3:1 ratio and is rejected.
	err := agent.EvaluateRaw(models.TradeIntent{Ticker: "TCS", Side: models.SideLong})
	if !errors.Is(err, riskmanager.ErrBiasLimit) {
		t.Fatalf("expected ErrBiasLimit, got %v", err)
	}
}
