package quantitative

import (
	"log"
	"time"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/groww"
)

// DefaultPollInterval is how often the feed connector reads the latest LTP
// snapshot and pushes ticks into the ticker store.
const DefaultPollInterval = 100 * time.Millisecond

// FeedConnector wraps a groww.FeedClient, subscribes to LTP for every symbol
// in the trading universe, and pushes incoming price ticks into the shared
// TickerStore.  It handles reconnection with exponential backoff via the
// underlying FeedClient.
type FeedConnector struct {
	client       *groww.FeedClient
	ts           *TickerStore
	universe     *Universe
	pollInterval time.Duration
	stopChan     chan struct{}
}

// NewFeedConnector creates a FeedConnector that pushes LTP ticks into the
// given TickerStore for all symbols in the universe.
func NewFeedConnector(client *groww.FeedClient, ts *TickerStore, u *Universe) *FeedConnector {
	return &FeedConnector{
		client:       client,
		ts:           ts,
		universe:     u,
		pollInterval: DefaultPollInterval,
		stopChan:     make(chan struct{}),
	}
}

// Start connects to the Groww feed, subscribes to LTP for the full universe,
// and launches the polling loop.  It is non-blocking; call Stop() to tear
// down.
func (fc *FeedConnector) Start() error {
	if err := fc.client.Connect(); err != nil {
		return err
	}

	instruments := fc.feedInstruments()
	if err := fc.client.SubscribeLTP(instruments); err != nil {
		if closeErr := fc.client.Close(); closeErr != nil {
			log.Printf("[Quantitative Feed] close error after subscribe failure: %v", closeErr)
		}
		return err
	}

	// Consume runs the blocking read loop in a background goroutine.
	go fc.client.Consume()

	// Polling goroutine reads LTP snapshots on a ticker.
	go fc.pollLoop()

	log.Printf("[Quantitative Feed] subscribed to %d instruments", len(instruments))
	return nil
}

// Stop shuts down the WebSocket connection and terminates the polling loop.
func (fc *FeedConnector) Stop() {
	close(fc.stopChan)
	if err := fc.client.Close(); err != nil {
		log.Printf("[Quantitative Feed] close error: %v", err)
	}
}

// pollLoop reads LTP snapshots on every tick and pushes ticks into the store.
func (fc *FeedConnector) pollLoop() {
	ticker := time.NewTicker(fc.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			fc.processSnapshot()
		case <-fc.stopChan:
			return
		}
	}
}

// processSnapshot reads the latest LTP snapshot from the feed client and
// pushes a Tick into the TickerStore for each updated instrument.
func (fc *FeedConnector) processSnapshot() {
	ltp := fc.client.GetLTP()
	if ltp == nil {
		return
	}

	for exchangeKey, exchanges := range ltp {
		for segmentKey, segments := range exchanges {
			for tokenKey, data := range segments {
				symbol := fc.tokenSymbol(exchangeKey, segmentKey, tokenKey)
				if symbol == "" {
					continue
				}
				store := fc.ts.Store(symbol)
				if store == nil {
					continue
				}
				store.Append(Tick{
					Price:     data.LTP,
					Volume:    0, // LTP snapshot does not carry volume
					Timestamp: time.UnixMilli(data.TsInMillis),
				})
			}
		}
	}
}

// tokenSymbol resolves the exchange+segment+token triple to a trading symbol.
// The Groww LTP snapshot keys use exchange/segment names that may differ from
// the standard constants; we do a best-effort match.
func (fc *FeedConnector) tokenSymbol(exchangeKey, segmentKey, tokenKey string) string {
	// Fast path: check the token→symbol map first.
	if s, ok := fc.universe.TokenToSymbol[tokenKey]; ok {
		return s
	}

	// Fallback: scan all universe entries for a token match (for instruments
	// that were resolved with tokens later).
	for _, e := range fc.universe.Entries {
		if e.ExchangeToken == tokenKey {
			return e.Symbol
		}
	}
	return ""
}

// feedInstruments converts the universe entries to Groww FeedInstrument
// values for WebSocket subscription.
func (fc *FeedConnector) feedInstruments() []groww.FeedInstrument {
	insts := make([]groww.FeedInstrument, 0, len(fc.universe.Entries))
	for _, e := range fc.universe.Entries {
		insts = append(insts, groww.FeedInstrument{
			Exchange:      e.Exchange,
			Segment:       e.Segment,
			ExchangeToken: e.ExchangeToken,
		})
	}
	return insts
}
