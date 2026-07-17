package trader

import (
	"fmt"
	"time"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/database"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
)

// Compile-time assertion that PaperTrader satisfies TraderInterface.
var _ TraderInterface = (*PaperTrader)(nil)

// PaperTrader is a simulation backend that implements TraderInterface. It
// fills every order at the requested limit price (no slippage), writes a
// row to the positions table with status OPEN, and logs the fill to
// trade_log with status SIMULATED.  No external broker is contacted.
type PaperTrader struct {
	positions *database.PositionStore
	tradeLog  *database.TradeLogStore
}

// NewPaperTrader returns a PaperTrader backed by the given database stores.
func NewPaperTrader(positions *database.PositionStore, tradeLog *database.TradeLogStore) *PaperTrader {
	return &PaperTrader{positions: positions, tradeLog: tradeLog}
}

// Execute simulates a fill at the requested limit price and persists the
// resulting position and trade-log entry.  It returns an OrderResult with a
// synthetic broker order ID of the form PAPER-{ref}-{timestamp_ms}.
func (p *PaperTrader) Execute(order *models.TradeExecution) (*OrderResult, error) {
	pos := posFromExec(order)
	if err := p.positions.Insert(pos); err != nil {
		return nil, fmt.Errorf("papertrader insert position: %w", err)
	}

	if err := p.tradeLog.Insert(order, models.StatusSimulated); err != nil {
		return nil, fmt.Errorf("papertrader insert trade_log: %w", err)
	}

	brokerID := fmt.Sprintf("PAPER-%s-%d", order.ExecutionRef, time.Now().UnixMilli())
	return &OrderResult{
		BrokerOrderID: brokerID,
		ExecutedPrice: order.Price,
		ExecutedQty:   order.Quantity,
	}, nil
}

// posFromExec converts a TradeExecution into a Position, applying sensible
// defaults for fields the caller may have left empty:
//   - Sector defaults to "GENERAL".
//   - StopLoss defaults to 95 % of the entry price.
//   - TakeProfit defaults to 105 % of the entry price.
func posFromExec(order *models.TradeExecution) *models.Position {
	sector := order.Sector
	if sector == "" {
		sector = "GENERAL"
	}
	stopLoss := order.StopLoss
	if stopLoss == 0 {
		stopLoss = order.Price * 0.95
	}
	takeProfit := order.TakeProfit
	if takeProfit == 0 {
		takeProfit = order.Price * 1.05
	}
	return &models.Position{
		Ticker:       order.Ticker,
		Side:         order.Side,
		Quantity:     order.Quantity,
		EntryPrice:   order.Price,
		StopLoss:     stopLoss,
		TakeProfit:   takeProfit,
		Sector:       sector,
		Status:       models.PositionOpen,
		ExecutionRef: order.ExecutionRef,
	}
}
