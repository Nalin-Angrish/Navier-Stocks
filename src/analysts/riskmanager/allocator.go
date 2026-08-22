package riskmanager

import (
	"log"
	"math"
	"os"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
)

// DefaultRiskPercent is the fraction of total capital risked per trade under
// the fixed-fractional sizing rule (2% per the Risk-Guardrails HLD).
const DefaultRiskPercent = 0.02

// totalCapitalEnvKey is the environment variable that supplies the platform's
// deployable capital for the allocator.
const totalCapitalEnvKey = "TOTAL_CAPITAL"

// DefaultTotalCapital is used when TOTAL_CAPITAL is absent, so the allocator
// degrades gracefully instead of denying every trade.
const DefaultTotalCapital = 1_000_000.0

// CalculateQuantity implements the fixed-fractional capital allocator:
//
//	quantity = (totalCapital × riskPercent) / (entryPrice − stopLossPrice)
//
// The risk distance is the absolute gap between entry and stop, making the
// formula direction-agnostic (longs use entry−stop, shorts stop−entry).  The
// result is floored to a whole lot; zero shares mean the per-share risk
// exceeds the platform's per-trade budget, so no position should be opened.
func CalculateQuantity(totalCapital, entryPrice, stopLoss float64) int {
	return CalculateQuantityPct(totalCapital, entryPrice, stopLoss, DefaultRiskPercent)
}

// CalculateQuantityPct is CalculateQuantity with an explicit risk fraction,
// kept internal so callers (and tests) can tune the percentage.
func CalculateQuantityPct(totalCapital, entryPrice, stopLoss, riskPercent float64) int {
	if totalCapital <= 0 || entryPrice <= 0 || stopLoss <= 0 {
		return 0
	}
	riskPerShare := math.Abs(entryPrice - stopLoss)
	if riskPerShare <= 0 {
		return 0
	}
	return int((totalCapital * riskPercent) / riskPerShare)
}

// LoadTotalCapital reads TOTAL_CAPITAL from the environment, falling back to
// the documented default when absent or malformed.
func LoadTotalCapital() float64 {
	if raw := os.Getenv(totalCapitalEnvKey); raw != "" {
		if v := parseFloat(raw); v > 0 {
			return v
		}
		log.Printf("[Risk Manager] invalid %s=%q, using %.0f", totalCapitalEnvKey, raw, DefaultTotalCapital)
	}
	return DefaultTotalCapital
}

// DefaultStopPct is how far below (long) or above (short) the entry price the
// default stop-loss sits, mirroring the PaperTrader's 5% structural stop.
const DefaultStopPct = 0.05

// stopLossFor derives a directional stop-loss from the entry price, matching
// the PaperTrader's defaults so the allocator always has a risk distance to
// size against.
func (g *Analyst) stopLossFor(intent *models.TradeIntent) float64 {
	if intent.Side == models.SideShort {
		return intent.CurrentPrice * (1 + DefaultStopPct)
	}
	return intent.CurrentPrice * (1 - DefaultStopPct)
}
