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

// Analyst is the Trader Gateway agent.  It subscribes to signal.execute.*
// via NATS JetStream, deserialises each message into a TradeExecution,
// validates it, and passes it to the configured TraderInterface backend
// (PaperTrader in simulation, GrowwTrader for live trading).
//
// At-least-once delivery is guaranteed: the NATS message is Acked only
// after the backend returns successfully.  On failure the message is
// Naked (negative-acknowledged) so JetStream redelivers it.
type Analyst struct {
	js       *nats.JetStream    // NATS JetStream connection
	trader   TraderInterface    // pluggable execution backend
	sub      *nats.Subscription // active subscription, closed on Stop
	db       *sql.DB            // PostgreSQL handle, closed on Stop
	stopChan chan struct{}      // closed by Stop() to unblock Run()
}

// NewAgent creates a fully-wired Analyst: it connects to NATS JetStream,
// opens a PostgreSQL connection, and initialises a PaperTrader backend.
// The caller must call Stop() to release resources.
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

// newAgent is the shared constructor used by NewAgent and NewAgentForTest.
// The variadic db parameter allows tests to omit the database handle.
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

// NewAgentForTest returns an Analyst wired to the supplied JetStream and
// TraderInterface, skipping database setup so tests can inject mocks.
func NewAgentForTest(js *nats.JetStream, trader TraderInterface) *Analyst {
	return newAgent(js, trader)
}

// Run ensures the trading stream exists, subscribes to signal.execute.*,
// and blocks until Stop() is called.  The subscription is torn down when
// the method returns.
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

// Stop signals the agent to shut down: it closes the stop channel (which
// unblocks Run()), closes the NATS connection, and if a database handle
// is present, closes it as well.
func (g *Analyst) Stop() {
	close(g.stopChan)
	g.js.Close()
	if g.db != nil {
		g.db.Close()
	}
}

// handleExecution is the NATS message handler for signal.execute.*.  It
// deserialises the JSON payload, validates the TradeExecution, forwards it
// to the TraderInterface backend, and then Acks the message (or Naks on
// failure to trigger redelivery).
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

// ackOrLog attempts a best-effort ACK on a message that was determined to
// be unprocessable (invalid JSON or validation failure).  These errors are
// terminal — there is no point redelivering — so we ack to move past them
// and log if the ack itself fails.
func ackOrLog(m *nats.Msg) {
	if err := m.Ack(); err != nil {
		log.Printf("[Trader] Ack error on invalid message: %v", err)
	}
}
