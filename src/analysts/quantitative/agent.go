package quantitative

import (
	"log"
	"os"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/groww"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/nats"
)

// Analyst is the Quantitative Scout agent — the real-time market-data
// ingestor and breakout-signal generator.  It owns the per-ticker ring
// buffers, the WebSocket feed connector, the indicator evaluation engine,
// and publishes trade intents to the NATS signal.intent.new subject.
type Analyst struct {
	js               *nats.JetStream
	tickerStore      *TickerStore
	universe         *Universe
	feedConnector    *FeedConnector
	breakoutDetector *BreakoutDetector
	stopChan         chan struct{}
}

// NewAgent creates a fully-wired Quantitative Scout.  It resolves the
// trading universe, initialises the ring-buffer store, connects to the
// Groww WebSocket feed, and prepares the breakout-detection loop —
// all without starting any goroutine (that happens in Run()).
func NewAgent() (*Analyst, error) {
	js, err := nats.ConnectJetStream()
	if err != nil {
		return nil, err
	}
	js.SetDurablePrefix("navier-scout")

	universe := ResolveUniverse()

	tickerStore := NewTickerStore(DefaultMaxTickers, 256)

	feedClient := groww.NewFeedClient(os.Getenv("GROWW_ACCESS_TOKEN"))
	feedConnector := NewFeedConnector(feedClient, tickerStore, universe, js)

	breakoutCfg := DefaultBreakoutConfig()
	breakoutDetector := NewBreakoutDetector(js, tickerStore, universe, breakoutCfg)

	return &Analyst{
		js:               js,
		tickerStore:      tickerStore,
		universe:         universe,
		feedConnector:    feedConnector,
		breakoutDetector: breakoutDetector,
		stopChan:         make(chan struct{}),
	}, nil
}

// Run ensures the required NATS stream exists, starts the feed connector
// and breakout detector, and blocks until Stop() is called.
func (g *Analyst) Run() {
	// Ensure the trading stream exists so published intents are persisted.
	if err := g.js.EnsureStream(nats.StreamTrading); err != nil {
		log.Printf("[Quantitative Analyst] stream setup error: %v", err)
		return
	}

	// 1.7 — Universe resolved in NewAgent already.
	log.Printf("[Quantitative Analyst] tracking %d symbols", len(g.universe.Symbols))

	// 1.2 — Start the WebSocket feed connector.
	if err := g.feedConnector.Start(); err != nil {
		log.Printf("[Quantitative Analyst] feed start error: %v", err)
	}

	// 1.6 — Start the breakout detector.
	g.breakoutDetector.Start()

	log.Println("[Quantitative Analyst] Ready...")
	<-g.stopChan
}

// Stop performs a graceful shutdown of all sub-systems in reverse order of
// their start.
func (g *Analyst) Stop() {
	select {
	case <-g.stopChan:
	default:
		close(g.stopChan)
	}
	g.breakoutDetector.Stop()
	g.feedConnector.Stop()
	g.js.Close()
}
