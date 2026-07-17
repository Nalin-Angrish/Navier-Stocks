// Package trader implements the Trader Gateway — the execution layer of the
// multi-agent system. It subscribes to signal.execute.* via NATS JetStream,
// deserialises incoming TradeExecution messages, and routes them through a
// swappable TraderInterface backend (PaperTrader or GrowwTrader) that
// persists the resulting position and trade log to PostgreSQL.
package trader

import "github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"

// OrderResult holds the broker-level response after a trade execution.
// BrokerOrderID is the unique identifier assigned by the broker (or simulated
// by the PaperTrader). ExecutedPrice and ExecutedQty reflect the actual fill,
// which may differ from the requested values in live trading.
type OrderResult struct {
	BrokerOrderID string
	ExecutedPrice float64
	ExecutedQty   int
}

// TraderInterface is the pluggable contract that the Analyst calls when a
// validated execution signal arrives. Implementations must persist the order
// and return a filled OrderResult or an error that triggers a NATS Nak.
type TraderInterface interface {
	Execute(order *models.TradeExecution) (*OrderResult, error)
}
