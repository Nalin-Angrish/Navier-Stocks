package trader

import (
	"fmt"
	"time"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/database"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
)

var _ TraderInterface = (*PaperTrader)(nil)

type PaperTrader struct {
	positions *database.PositionStore
	tradeLog  *database.TradeLogStore
}

func NewPaperTrader(positions *database.PositionStore, tradeLog *database.TradeLogStore) *PaperTrader {
	return &PaperTrader{positions: positions, tradeLog: tradeLog}
}

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
