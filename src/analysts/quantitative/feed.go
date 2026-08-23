package quantitative

import (
	"encoding/json"
	"log"
	"time"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/groww"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/nats"
)

// DefaultPollInterval is how often the feed connector reads the latest LTP
// snapshot and pushes ticks into the ticker store.
const DefaultPollInterval = 100 * time.Millisecond

// QuotePollInterval is how often the REST quote endpoint is polled to enrich
// LTP ticks with real volume data.  Must be longer than PricePublishInterval
// to avoid excessive API usage.
const QuotePollInterval = 5 * time.Second

// PricePublishInterval throttles the signal.price.<TICKER> stream: at most
// one price tick per ticker per interval is published, enough for the exit
// monitor without flooding JetStream at the raw poll rate.
const PricePublishInterval = time.Second

// FeedConnector wraps a groww.FeedClient, subscribes to LTP for every symbol
// in the trading universe, and pushes incoming price ticks into the shared
// TickerStore.  It handles reconnection with exponential backoff via the
// underlying FeedClient.
type FeedConnector struct {
	client       *groww.FeedClient
	restClient   *groww.Client // REST client for volume enrichment (OBS-01)
	ts           *TickerStore
	universe     *Universe
	js           *nats.JetStream      // optional price-stream publisher
	lastPricePub map[string]time.Time // per-ticker throttle state
	pollInterval time.Duration
	stopChan     chan struct{}
}

// NewFeedConnector creates a FeedConnector that pushes LTP ticks into the
// given TickerStore for all symbols in the universe.  An optional JetStream
// handle enables publishing the signal.price.<TICKER> stream consumed by
// the Trader Gateway's intraday exit monitor.
func NewFeedConnector(client *groww.FeedClient, ts *TickerStore, u *Universe, js ...*nats.JetStream) *FeedConnector {
	var jsPtr *nats.JetStream
	if len(js) > 0 {
		jsPtr = js[0]
	}
	return &FeedConnector{
		client:       client,
		ts:           ts,
		universe:     u,
		js:           jsPtr,
		lastPricePub: make(map[string]time.Time),
		pollInterval: DefaultPollInterval,
		stopChan:     make(chan struct{}),
	}
}

// SetRESTClient injects a Groww REST client used to poll volume data.
// Must be called before Start().
func (fc *FeedConnector) SetRESTClient(c *groww.Client) {
	fc.restClient = c
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

	// Volume enrichment: poll REST quote endpoint periodically to fill
	// in volume data that the LTP WebSocket does not carry (OBS-01).
	if fc.restClient != nil {
		go fc.quoteLoop()
	}

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
				ts := time.UnixMilli(data.TsInMillis)
				store.Append(Tick{
					Price:     data.LTP,
					Volume:    0, // LTP snapshot does not carry volume
					Timestamp: ts,
				})
				fc.publishPrice(symbol, data.LTP, data.TsInMillis)
			}
		}
	}
}

// publishPrice emits a throttled signal.price.<TICKER> message for the exit
// monitor.  Best-effort: publishing never disturbs the ingest path, and the
// per-ticker throttle keeps JetStream traffic bounded when the poll rate is
// high.  A nil JetStream handle (unit tests, offline replay) disables it.
// Uses wall-clock time for throttling (OBS-14) to avoid flood/starvation
// when exchange timestamps are degenerate.
func (fc *FeedConnector) publishPrice(symbol string, price float64, tsMillis int64) {
	if fc.js == nil {
		return
	}
	now := time.Now()
	if last, ok := fc.lastPricePub[symbol]; ok && now.Sub(last) < PricePublishInterval {
		return
	}
	fc.lastPricePub[symbol] = now

	data, err := json.Marshal(models.PriceTick{
		Ticker:     symbol,
		Price:      price,
		TsInMillis: tsMillis,
	})
	if err != nil {
		return // malformed tick cannot happen with numeric fields; skip
	}
	if err := fc.js.Publish(nats.PriceSubject(symbol), data); err != nil {
		log.Printf("[Scout] price publish %s: %v", symbol, err)
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

// quoteLoop polls the Groww REST quote endpoint at QuotePollInterval to
// enrich LTP ticks with real volume data.  The LTP WebSocket does not
// carry volume (OBS-01); this loop fills the gap.
func (fc *FeedConnector) quoteLoop() {
	ticker := time.NewTicker(QuotePollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			fc.pollQuotes()
		case <-fc.stopChan:
			return
		}
	}
}

// pollQuotes fetches a live quote for every tracked symbol and patches the
// volume into the most recent tick.  Errors are logged and skipped — volume
// enrichment is best-effort and never blocks the LTP path.
func (fc *FeedConnector) pollQuotes() {
	for _, e := range fc.universe.Entries {
		symbol := e.Symbol
		store := fc.ts.Get(symbol)
		if store == nil || store.Len() == 0 {
			continue
		}

		quote, err := fc.restClient.GetQuote(e.Exchange, e.Segment, symbol)
		if err != nil {
			log.Printf("[Quantitative Feed] quote %s: %v", symbol, err)
			continue
		}

		store.UpdateLatestVolume(int64(quote.Volume))
	}
}
