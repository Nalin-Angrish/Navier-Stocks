package trader

import "github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"

type OrderResult struct {
	BrokerOrderID string
	ExecutedPrice float64
	ExecutedQty   int
}

type TraderInterface interface {
	Execute(order *models.TradeExecution) (*OrderResult, error)
}
