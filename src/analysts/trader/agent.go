package trader

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/database"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/nats"
)

type ExecutionStore interface {
	InsertTradeLogEntry(*models.TradeExecution) error
}

type Analyst struct {
	js       *nats.JetStream
	store    ExecutionStore
	sub      *nats.Subscription
	stopChan chan struct{}
	errChan  chan error
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
	store := database.NewTradeLogStore(db)
	return newAgent(js, store), nil
}

func newAgent(js *nats.JetStream, store ExecutionStore) *Analyst {
	return &Analyst{
		js:       js,
		store:    store,
		stopChan: make(chan struct{}),
		errChan:  make(chan error, 1),
	}
}

func NewAgentForTest(js *nats.JetStream, store ExecutionStore) *Analyst {
	return newAgent(js, store)
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

	log.Printf("[Trader] Received execution for %s | side=%s qty=%d price=%.2f ref=%s",
		exec.Ticker, exec.Side, exec.Quantity, exec.Price, exec.ExecutionRef)

	if err := g.store.InsertTradeLogEntry(&exec); err != nil {
		log.Printf("[Trader] Failed to persist execution: %v", err)
		if nakErr := m.Nak(); nakErr != nil {
			log.Printf("[Trader] Nak error: %v", nakErr)
		}
		return
	}

	if err := m.Ack(); err != nil {
		log.Printf("[Trader] Ack error: %v", err)
	}
}

func ackOrLog(m *nats.Msg) {
	if err := m.Ack(); err != nil {
		log.Printf("[Trader] Ack error on invalid message: %v", err)
	}
}
