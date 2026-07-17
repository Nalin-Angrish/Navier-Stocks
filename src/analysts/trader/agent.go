package trader

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/database"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/nats"
)

type Analyst struct {
	js       *nats.JetStream
	trader   TraderInterface
	sub      *nats.Subscription
	db       *sql.DB
	stopChan chan struct{}
}

func NewAgent() (*Analyst, error) {
	js, err := nats.ConnectJetStream()
	if err != nil {
		return nil, fmt.Errorf("trader: nats: %w", err)
	}
	db, err := database.Connect()
	if err != nil {
		js.Close()
		return nil, fmt.Errorf("trader: db: %w", err)
	}
	trader := NewPaperTrader(
		database.NewPositionStore(db),
		database.NewTradeLogStore(db),
	)
	return newAgent(js, trader, db), nil
}

func newAgent(js *nats.JetStream, trader TraderInterface, db ...*sql.DB) *Analyst {
	var dbPtr *sql.DB
	if len(db) > 0 {
		dbPtr = db[0]
	}
	return &Analyst{
		js:       js,
		trader:   trader,
		db:       dbPtr,
		stopChan: make(chan struct{}),
	}
}

func NewAgentForTest(js *nats.JetStream, trader TraderInterface) *Analyst {
	return newAgent(js, trader)
}

func (g *Analyst) Run() {
	if err := g.js.EnsureStream(nats.StreamTrading); err != nil {
		log.Printf("[Trader] Stream ensure failed: %v", err)
		return
	}

	sub, err := g.js.Subscribe("signal.execute.*", g.handleExecution)
	if err != nil {
		log.Printf("[Trader] Subscribe failed: %v", err)
		return
	}
	g.sub = sub
	log.Println("[Trader] Subscribed to signal.execute.*")

	<-g.stopChan

	if g.sub != nil {
		if err := g.sub.Unsubscribe(); err != nil {
			log.Printf("[Trader] Unsubscribe error: %v", err)
		}
	}
}

func (g *Analyst) Stop() {
	close(g.stopChan)
	g.js.Close()
	if g.db != nil {
		g.db.Close()
	}
}

func (g *Analyst) handleExecution(m *nats.Msg) {
	var exec models.TradeExecution
	if err := json.Unmarshal(m.Data, &exec); err != nil {
		log.Printf("[Trader] Invalid execution JSON: %v", err)
		ackOrLog(m)
		return
	}

	if err := exec.Validate(); err != nil {
		log.Printf("[Trader] Invalid execution: %v", err)
		ackOrLog(m)
		return
	}

	log.Printf("[Trader] Executing %s | side=%s qty=%d price=%.2f ref=%s",
		exec.Ticker, exec.Side, exec.Quantity, exec.Price, exec.ExecutionRef)

	result, err := g.trader.Execute(&exec)
	if err != nil {
		log.Printf("[Trader] Execute failed: %v", err)
		if nakErr := m.Nak(); nakErr != nil {
			log.Printf("[Trader] Nak error: %v", nakErr)
		}
		return
	}

	log.Printf("[Trader] Executed %s | id=%s fill_price=%.2f fill_qty=%d",
		exec.Ticker, result.BrokerOrderID, result.ExecutedPrice, result.ExecutedQty)

	if err := m.Ack(); err != nil {
		log.Printf("[Trader] Ack error: %v", err)
	}
}

func ackOrLog(m *nats.Msg) {
	if err := m.Ack(); err != nil {
		log.Printf("[Trader] Ack error on invalid message: %v", err)
	}
}
