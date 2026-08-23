package riskmanager

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/nats"
)

// ErrInsufficientCapital is returned when the allocator sizes the intent at
// zero shares, meaning a single share's risk distance exceeds the platform's
// per-trade budget.
var ErrInsufficientCapital = errors.New("insufficient capital for 2% risk budget")

// DefaultRewardRiskRatio is the target reward-to-risk multiple for take-profit
// calculation.  A ratio of 2.0 means the target profit is twice the risk
// distance (entry → stop-loss).
const DefaultRewardRiskRatio = 2.0

// ExecutionPublisher abstracts the JetStream publish target so the promoting
// logic can be tested without a live NATS connection.
type ExecutionPublisher interface {
	Publish(subj string, data []byte) error
}

// promote turns an approved intent into an execution signal.  It sizes the
// position with the 2% allocator, assembles a validated TradeExecution, and
// publishes it to signal.execute.<TICKER>.  On success the sector exposure
// map is updated so the concentration gates see the new position.
//
// The ExcecutionRef is derived per-publish so downstream reconciliation can
// correlate the execution back to its originating intent.
func (g *Analyst) promote(intent *models.TradeIntent) error {
	exec, err := g.buildExecution(intent)
	if err != nil {
		return err
	}

	if err := g.publishExecution(exec); err != nil {
		return err
	}

	// Update the sector exposure map only after a successful publish, so the
	// gates never count an execution that did not leave the agent.
	if g.exposure != nil {
		g.exposure.Register(exec.Side, exec.Sector)
	}
	log.Printf("[Risk Manager] Published execution %s %s x%d @ %.2f (ref=%s)",
		exec.Side, exec.Ticker, exec.Quantity, exec.Price, exec.ExecutionRef)
	return nil
}

// buildExecution assembles a TradeExecution from an approved intent: it sizes
// the quantity with the 2% allocator, derives the default stop-loss, resolves
// the sector, and stamps a correlation reference.
func (g *Analyst) buildExecution(intent *models.TradeIntent) (*models.TradeExecution, error) {
	stopLoss := g.stopLossFor(intent)
	quantity := CalculateQuantity(g.capital, intent.CurrentPrice, stopLoss)
	if quantity <= 0 {
		return nil, ErrInsufficientCapital
	}

	takeProfit := calculateTakeProfit(intent.Side, intent.CurrentPrice, stopLoss)

	return &models.TradeExecution{
		Ticker:       intent.Ticker,
		Side:         intent.Side,
		Quantity:     quantity,
		Price:        intent.CurrentPrice,
		StopLoss:     stopLoss,
		TakeProfit:   takeProfit,
		Sector:       g.sectorOf(intent.Ticker),
		SignalReason: intent.SignalReason,
		ExecutionRef: fmt.Sprintf("risk-%s-%d", intent.Ticker, time.Now().UnixNano()),
	}, nil
}

// calculateTakeProfit derives the target exit price from the entry, stop-loss,
// and the reward-to-risk ratio.  For LONG: TP = entry + ratio × (entry − stop).
// For SHORT: TP = entry − ratio × (stop − entry).
func calculateTakeProfit(side models.Side, entry, stopLoss float64) float64 {
	risk := entry - stopLoss
	if side == models.SideShort {
		risk = stopLoss - entry
	}
	if risk <= 0 {
		return 0
	}
	target := DefaultRewardRiskRatio * risk
	if side == models.SideShort {
		return entry - target
	}
	return entry + target
}

// publishExecution serialises and publishes the execution to
// signal.execute.<TICKER> via JetStream.
func (g *Analyst) publishExecution(exec *models.TradeExecution) error {
	if g.js == nil {
		return nil
	}
	data, err := json.Marshal(exec)
	if err != nil {
		return fmt.Errorf("marshal execution: %w", err)
	}
	return g.js.Publish(executeSubject(exec.Ticker), data)
}

// ExecuteSubject builds the per-ticker execution subject, e.g.
// signal.execute.RELIANCE.  Exported for tests and cross-package callers.
func ExecuteSubject(ticker string) string {
	return "signal.execute." + ticker
}

// executeSubject is the internal alias used by the publisher.
func executeSubject(ticker string) string {
	return ExecuteSubject(ticker)
}

// compile-time assertion that *nats.JetStream satisfies ExecutionPublisher.
var _ ExecutionPublisher = (*nats.JetStream)(nil)
