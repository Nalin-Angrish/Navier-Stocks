package groww

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// DefaultFeedURL is the default WebSocket endpoint for the Groww Feed.
const DefaultFeedURL = "wss://api.groww.in/v1/feed"

// FeedCallback is invoked by the FeedClient when new data arrives.
// The metadata parameter identifies the source of the data.
type FeedCallback func(meta FeedMetadata)

// LTPCallback is a convenience callback that receives parsed LTP data.
type LTPCallback func(ltp LTPData, meta FeedMetadata)

// MarketDepthCallback is a convenience callback for market depth updates.
type MarketDepthCallback func(depth MarketDepthData, meta FeedMetadata)

// FeedClient manages a WebSocket connection to the Groww Feed for
// real-time market data, order updates, and position updates.
//
// The client supports up to 1000 simultaneous instrument subscriptions
// and can be used in two modes:
//   - Asynchronous: callbacks are invoked when data arrives
//   - Synchronous: call GetLTP() / GetMarketDepth() / GetIndexValue()
//     after a poll interval
type FeedClient struct {
	token      string
	feedURL    string
	baseURL    string
	httpClient *http.Client
	dialer     *websocket.Dialer
	conn       *websocket.Conn
	mu         sync.RWMutex
	connected  bool
	done       chan struct{}

	// Optional API key + secret for token refresh (same flow as REST Client).
	apiKey    string
	apiSecret string

	// Latest snapshots (synchronous access).
	ltpSnapshot   map[string]map[string]map[string]LTPData
	indexSnapshot map[string]map[string]map[string]IndexData
	depthSnapshot map[string]map[string]map[string]MarketDepthData

	// Callbacks.
	onData  FeedCallback
	onLTP   LTPCallback
	onDepth MarketDepthCallback
}

// NewFeedClient creates a FeedClient authenticated with the given token.
// The token is read from GROWW_ACCESS_TOKEN if empty. If no token is
// available but GROWW_API_KEY/GROWW_API_SECRET are set, the client will
// obtain a token automatically on Connect() (and refresh it when the
// WebSocket handshake is rejected with 401).
func NewFeedClient(accessToken string) *FeedClient {
	if accessToken == "" {
		accessToken = os.Getenv("GROWW_ACCESS_TOKEN")
	}
	apiKey := os.Getenv("GROWW_API_KEY")
	apiSecret := os.Getenv("GROWW_API_SECRET")
	return newFeedClient(accessToken, apiKey, apiSecret)
}

// NewFeedClientFromKeys creates a FeedClient that authenticates via the
// Groww API key + secret pair. The key/secret are read from
// GROWW_API_KEY/GROWW_API_SECRET if empty. The client will exchange them
// for an access token on first Connect() and automatically refresh on
// authentication failures.
func NewFeedClientFromKeys(apiKey, apiSecret string) *FeedClient {
	if apiKey == "" {
		apiKey = os.Getenv("GROWW_API_KEY")
	}
	if apiSecret == "" {
		apiSecret = os.Getenv("GROWW_API_SECRET")
	}
	return newFeedClient("", apiKey, apiSecret)
}

func newFeedClient(token, apiKey, apiSecret string) *FeedClient {
	return &FeedClient{
		token:      token,
		apiKey:     apiKey,
		apiSecret:  apiSecret,
		feedURL:    envOrDefault("GROWW_FEED_URL", DefaultFeedURL),
		baseURL:    envOrDefault("GROWW_BASE_URL", DefaultBaseURL),
		httpClient: &http.Client{Timeout: DefaultHTTPTimeout},
		dialer:     websocket.DefaultDialer,
		done:       make(chan struct{}),
	}
}

// SetToken replaces the bearer token at runtime (used after a refresh).
func (f *FeedClient) SetToken(token string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.token = token
}

func (f *FeedClient) getToken() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.token
}

// ensureToken obtains an access token via the API key/secret if none is
// currently set. It logs the action for debuggability.
func (f *FeedClient) ensureToken() error {
	f.mu.RLock()
	hasToken := f.token != ""
	hasKeys := hasGrowwKeys(f.apiKey, f.apiSecret)
	f.mu.RUnlock()
	if hasToken {
		return nil
	}
	if hasKeys {
		log.Printf("[Groww Feed] no token, fetching via API key %s", maskKey(f.apiKey))
		if err := f.refreshToken(); err != nil {
			return fmt.Errorf("%w: %v", ErrAuthExpired, err)
		}
		return nil
	}
	return fmt.Errorf("%w: set GROWW_ACCESS_TOKEN or GROWW_API_KEY/GROWW_API_SECRET", ErrAuthExpired)
}

// refreshToken exchanges the API key + secret for a new access token via
// the shared fetchAccessToken helper.
func (f *FeedClient) refreshToken() error {
	token, err := fetchAccessToken(f.apiKey, f.apiSecret, f.baseURL, f.httpClient)
	if err != nil {
		return err
	}
	f.SetToken(token)
	log.Printf("[Groww Feed] token refreshed (expiry unknown, key %s)", maskKey(f.apiKey))
	return nil
}

// SetFeedCallback registers a general-purpose callback for all feed data.
func (f *FeedClient) SetFeedCallback(cb FeedCallback) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onData = cb
}

// SetLTPCallback registers a callback for parsed LTP updates.
func (f *FeedClient) SetLTPCallback(cb LTPCallback) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onLTP = cb
}

// SetDepthCallback registers a callback for parsed market depth updates.
func (f *FeedClient) SetDepthCallback(cb MarketDepthCallback) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onDepth = cb
}

// Connect establishes the WebSocket connection and authenticates.
// It fetches a token via API key/secret if needed, then dials. On
// handshake failure it refreshes once when credentials are available.
func (f *FeedClient) Connect() error {
	if err := f.ensureToken(); err != nil {
		return fmt.Errorf("groww feed ensure token: %w", err)
	}

	conn, resp, err := f.dial(f.getToken())
	if err == nil {
		return f.setupConn(conn)
	}

	// Only retry with a fresh token when we have keys; otherwise surface the dial error.
	if !hasGrowwKeys(f.apiKey, f.apiSecret) {
		return wrapDialError("groww feed dial", err, resp)
	}

	log.Printf("[Groww Feed] dial failed (%v, HTTP %v), retrying with refreshed token (key %s)", err, httpStatus(resp), maskKey(f.apiKey))
	if refreshErr := f.refreshToken(); refreshErr != nil {
		return fmt.Errorf("groww feed dial: %w (HTTP %v, refresh failed: %v)", err, httpStatus(resp), refreshErr)
	}

	conn, resp, err = f.dial(f.getToken())
	if err != nil {
		return wrapDialError("groww feed dial (after refresh)", err, resp)
	}
	return f.setupConn(conn)
}

// dial performs the websocket handshake with the given token.
func (f *FeedClient) dial(token string) (*websocket.Conn, *http.Response, error) {
	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)
	return f.dialer.Dial(f.feedURL, header)
}

// setupConn installs keepalive handlers and marks the client connected.
func (f *FeedClient) setupConn(conn *websocket.Conn) error {
	// Set up ping/pong keepalive so the server doesn't drop idle connections.
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	})
	// Send an initial pong deadline so the first read has a timeout.
	if err := conn.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
		_ = conn.Close()
		return fmt.Errorf("groww feed read deadline: %w", err)
	}
	go f.pingLoop(conn)

	f.mu.Lock()
	f.conn = conn
	f.connected = true
	f.mu.Unlock()
	return nil
}

func wrapDialError(prefix string, err error, resp *http.Response) error {
	if resp != nil {
		return fmt.Errorf("%s: %w (HTTP %d)", prefix, err, resp.StatusCode)
	}
	return fmt.Errorf("%s: %w", prefix, err)
}

func httpStatus(resp *http.Response) string {
	if resp == nil {
		return "-"
	}
	return fmt.Sprintf("%d", resp.StatusCode)
}

// SubscribeLTP subscribes to live LTP updates for a list of instruments.
// The optional callback is invoked each time new data arrives.
func (f *FeedClient) SubscribeLTP(instruments []FeedInstrument, cb ...LTPCallback) error {
	if len(cb) > 0 {
		f.mu.Lock()
		f.onLTP = cb[0]
		f.mu.Unlock()
	}
	return f.subscribe("ltp", instruments)
}

// SubscribeMarketDepth subscribes to live market depth for a list of
// instruments.
func (f *FeedClient) SubscribeMarketDepth(instruments []FeedInstrument, cb ...MarketDepthCallback) error {
	if len(cb) > 0 {
		f.mu.Lock()
		f.onDepth = cb[0]
		f.mu.Unlock()
	}
	return f.subscribe("market_depth", instruments)
}

// SubscribeIndexValue subscribes to live index value updates.
func (f *FeedClient) SubscribeIndexValue(instruments []FeedInstrument) error {
	return f.subscribe("index_value", instruments)
}

// SubscribeEquityOrderUpdates subscribes to real-time equity order updates.
func (f *FeedClient) SubscribeEquityOrderUpdates(cb ...FeedCallback) error {
	if len(cb) > 0 {
		f.mu.Lock()
		f.onData = cb[0]
		f.mu.Unlock()
	}
	return f.subscribe("order_updates_equity", nil)
}

// SubscribeFNOOrderUpdates subscribes to real-time FNO order updates.
func (f *FeedClient) SubscribeFNOOrderUpdates(cb ...FeedCallback) error {
	if len(cb) > 0 {
		f.mu.Lock()
		f.onData = cb[0]
		f.mu.Unlock()
	}
	return f.subscribe("order_updates_fno", nil)
}

// SubscribeFNOPositionUpdates subscribes to real-time FNO position updates.
func (f *FeedClient) SubscribeFNOPositionUpdates(cb ...FeedCallback) error {
	if len(cb) > 0 {
		f.mu.Lock()
		f.onData = cb[0]
		f.mu.Unlock()
	}
	return f.subscribe("position_updates_fno", nil)
}

// subscribe sends a subscription message over the WebSocket.
func (f *FeedClient) subscribe(feedType string, instruments []FeedInstrument) error {
	f.mu.RLock()
	conn := f.conn
	f.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("groww feed: not connected")
	}

	msg := map[string]any{
		"action":      "subscribe",
		"feed_type":   feedType,
		"instruments": instruments,
	}
	return conn.WriteJSON(msg)
}

// UnsubscribeLTP unsubscribes from LTP updates for the given instruments.
func (f *FeedClient) UnsubscribeLTP(instruments []FeedInstrument) error {
	return f.unsubscribe("ltp", instruments)
}

// UnsubscribeMarketDepth unsubscribes from market depth updates.
func (f *FeedClient) UnsubscribeMarketDepth(instruments []FeedInstrument) error {
	return f.unsubscribe("market_depth", instruments)
}

// UnsubscribeIndexValue unsubscribes from index value updates.
func (f *FeedClient) UnsubscribeIndexValue(instruments []FeedInstrument) error {
	return f.unsubscribe("index_value", instruments)
}

func (f *FeedClient) unsubscribe(feedType string, instruments []FeedInstrument) error {
	f.mu.RLock()
	conn := f.conn
	f.mu.RUnlock()
	if conn == nil {
		return fmt.Errorf("groww feed: not connected")
	}
	msg := map[string]any{
		"action":      "unsubscribe",
		"feed_type":   feedType,
		"instruments": instruments,
	}
	return conn.WriteJSON(msg)
}

// Consume enters the blocking read loop, calling registered callbacks as
// messages arrive.  It returns when Close() is called or the connection
// is lost.
func (f *FeedClient) Consume() {
	defer func() {
		f.mu.Lock()
		f.connected = false
		f.mu.Unlock()
	}()

	for {
		f.mu.RLock()
		conn := f.conn
		f.mu.RUnlock()

		if conn == nil {
			return
		}

		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure) {
				log.Printf("[Groww Feed] read error: %v", err)
			}
			return
		}

		f.dispatch(message)
	}
}

// dispatch parses a raw JSON feed message and invokes registered callbacks.
func (f *FeedClient) dispatch(data []byte) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return
	}

	// Try to extract metadata.
	var meta FeedMetadata
	if metaRaw, ok := raw["meta"]; ok {
		_ = json.Unmarshal(metaRaw, &meta) // best-effort
	}

	// Dispatch by top-level key.
	f.mu.RLock()
	onData := f.onData
	onLTP := f.onLTP
	onDepth := f.onDepth
	f.mu.RUnlock()

	if onData != nil {
		onData(meta)
	}

	if ltpRaw, ok := raw["ltp"]; ok {
		f.handleLTP(ltpRaw, meta, onLTP)
	}
	if depthRaw, ok := raw["depth"]; ok {
		f.handleDepth(depthRaw, meta, onDepth)
	}
	if idxRaw, ok := raw["index"]; ok {
		f.handleIndex(idxRaw, meta)
	}
}

// handleLTP parses the LTP payload and updates the snapshot + callback.
func (f *FeedClient) handleLTP(data json.RawMessage, meta FeedMetadata, cb LTPCallback) {
	var parsed map[string]map[string]map[string]LTPData
	if err := json.Unmarshal(data, &parsed); err != nil {
		return
	}
	f.mu.Lock()
	f.ltpSnapshot = parsed
	f.mu.Unlock()

	if cb == nil {
		return
	}
	for _, exchanges := range parsed {
		for _, segs := range exchanges {
			for _, ltp := range segs {
				cb(ltp, meta)
			}
		}
	}
}

// handleDepth parses the market depth payload.
func (f *FeedClient) handleDepth(data json.RawMessage, meta FeedMetadata, cb MarketDepthCallback) {
	var parsed map[string]map[string]map[string]MarketDepthData
	if err := json.Unmarshal(data, &parsed); err != nil {
		return
	}
	f.mu.Lock()
	f.depthSnapshot = parsed
	f.mu.Unlock()

	if cb == nil {
		return
	}
	for _, exchanges := range parsed {
		for _, segs := range exchanges {
			for _, depth := range segs {
				cb(depth, meta)
			}
		}
	}
}

// handleIndex parses the index value payload.
func (f *FeedClient) handleIndex(data json.RawMessage, meta FeedMetadata) {
	var parsed map[string]map[string]map[string]IndexData
	if err := json.Unmarshal(data, &parsed); err != nil {
		return
	}
	f.mu.Lock()
	f.indexSnapshot = parsed
	f.mu.Unlock()
}

// GetLTP returns a snapshot of the latest LTP values keyed by
// exchange → segment → exchange_token → LTPData.
func (f *FeedClient) GetLTP() map[string]map[string]map[string]LTPData {
	f.mu.RLock()
	defer f.mu.RUnlock()
	snap := f.ltpSnapshot
	return snap
}

// GetMarketDepth returns a snapshot of the latest market depth data.
func (f *FeedClient) GetMarketDepth() map[string]map[string]map[string]MarketDepthData {
	f.mu.RLock()
	defer f.mu.RUnlock()
	snap := f.depthSnapshot
	return snap
}

// GetIndexValue returns a snapshot of the latest index values.
func (f *FeedClient) GetIndexValue() map[string]map[string]map[string]IndexData {
	f.mu.RLock()
	defer f.mu.RUnlock()
	snap := f.indexSnapshot
	return snap
}

// Close gracefully shuts down the WebSocket connection.
func (f *FeedClient) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.conn != nil {
		err := f.conn.WriteMessage(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		)
		closeErr := f.conn.Close()
		f.conn = nil
		f.connected = false
		if err != nil {
			return err
		}
		return closeErr
	}
	return nil
}

// IsConnected reports whether the WebSocket is currently connected.
func (f *FeedClient) IsConnected() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.connected
}

// pingLoop sends WebSocket pings at a fixed interval to keep the connection
// alive.  It runs until the connection is closed.
func (f *FeedClient) pingLoop(conn *websocket.Conn) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second)); err != nil {
				return // connection is dead; Consume will detect the read error
			}
		case <-f.done:
			return
		}
	}
}

// Reconnect attempts to reconnect after a disconnection with backoff.
func (f *FeedClient) Reconnect(maxRetries int) error {
	backoff := 100 * time.Millisecond
	for i := 0; i < maxRetries; i++ {
		if err := f.Connect(); err != nil {
			log.Printf("[Groww Feed] reconnect attempt %d failed: %v", i+1, err)
			time.Sleep(backoff)
			backoff *= 2
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
			continue
		}
		return nil
	}
	return fmt.Errorf("groww feed: reconnect failed after %d attempts", maxRetries)
}
