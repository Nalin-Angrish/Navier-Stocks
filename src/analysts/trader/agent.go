package trader

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

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
	sub      *nats.Subscription // active signal.execute.* subscription
	priceSub *nats.Subscription // active signal.price.* subscription (exit monitor)
	exit     *ExitMonitor       // intraday SL/TP liquidation engine; nil disables
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
	js.SetDurablePrefix("navier-trader")
	db, err := database.Connect()
	if err != nil {
		js.Close()
		return nil, fmt.Errorf("trader: db: %w", err)
	}
	positionStore := database.NewPositionStore(db)
	tradeLogStore := database.NewTradeLogStore(db)

	a := newAgent(js, NewPaperTrader(positionStore, tradeLogStore), db)
	// The intraday exit monitor shares the position store with the
	// PaperTrader and publishes closing executions back onto the same
	// signal.execute.* stream this agent consumes.
	a.exit = NewExitMonitor(positionStore, js)
	return a, nil
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

	// Exit monitor: subscribe to the price stream and keep the open-position
	// cache warm.  A nil monitor (unit tests) skips this entirely.
	if g.exit != nil {
		priceSub, err := g.js.Subscribe(nats.SubjectPricePrefix+"*", g.exit.HandlePrice)
		if err != nil {
			log.Printf("[Trader] price stream subscribe failed: %v", err)
			return
		}
		g.priceSub = priceSub

		if err := g.exit.Refresh(); err != nil {
			log.Printf("[Trader] exit cache initial refresh: %v", err)
		}
		go g.refreshLoop()
		log.Println("[Trader] Exit monitor armed on signal.price.*")
	}

	<-g.stopChan

	for _, s := range []*nats.Subscription{g.sub, g.priceSub} {
		if s != nil {
			if err := s.Unsubscribe(); err != nil {
				log.Printf("[Trader] Unsubscribe error: %v", err)
			}
		}
	}
}

// refreshLoop periodically re-reads open positions into the exit monitor's
// cache so positions opened/closed outside the price path stay consistent.
func (g *Analyst) refreshLoop() {
	t := time.NewTicker(ExitRefreshInterval)
	defer t.Stop()
	for {
		select {
		case <-g.stopChan:
			return
		case <-t.C:
			if err := g.exit.Refresh(); err != nil {
				log.Printf("[Trader] exit cache refresh: %v", err)
			}
		}
	}
}

// Stop signals the agent to shut down: it closes the stop channel (which
// unblocks Run()), closes the NATS connection, and if a database handle
// is present, closes it as well.
func (g *Analyst) Stop() {
	select {
	case <-g.stopChan:
	default:
		close(g.stopChan)
	}
	g.js.Close()
	if g.db != nil {
		err := g.db.Close()
		if err != nil {
			log.Printf("[Trader] Error closing database: %v", err)
		}
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
