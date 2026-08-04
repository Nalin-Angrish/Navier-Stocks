package quantitative

import (
	"log"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/nats"
)

// Analyst is the Quantitative Scout agent — the real-time market-data
// ingestor and breakout-signal generator.  It owns the per-ticker ring
// buffers, the WebSocket feed connector, the indicator evaluation engine,
// and publishes trade intents to the NATS signal.intent.new subject.
type Analyst struct {
	js          *nats.JetStream
	tickerStore *TickerStore
	universe    *Universe
	stopChan    chan struct{}
}

// NewAgent creates a fully-wired Quantitative Scout.  It resolves the
// trading universe and initialises the ring-buffer store — without starting
// any goroutine (that happens in Run()).
func NewAgent() (*Analyst, error) {
	js, err := nats.ConnectJetStream()
	if err != nil {
		return nil, err
	}

	universe := ResolveUniverse()
	tickerStore := NewTickerStore(DefaultMaxTickers, 256)

	return &Analyst{
		js:          js,
		tickerStore: tickerStore,
		universe:    universe,
		stopChan:    make(chan struct{}),
	}, nil
}

// Run ensures the required NATS stream exists and blocks until Stop() is
// called.
func (g *Analyst) Run() {
	// Ensure the trading stream exists so published intents are persisted.
	if err := g.js.EnsureStream(nats.StreamTrading); err != nil {
		log.Printf("[Quantitative Analyst] stream setup error: %v", err)
	}

	// 1.7 — Universe resolved in NewAgent already.
	log.Printf("[Quantitative Analyst] tracking %d symbols", len(g.universe.Symbols))

	log.Println("[Quantitative Analyst] Ready...")
	<-g.stopChan
}

// Stop performs a graceful shutdown of all sub-systems in reverse order of
// their start.
func (g *Analyst) Stop() {
	g.js.Close()
	close(g.stopChan)
}
