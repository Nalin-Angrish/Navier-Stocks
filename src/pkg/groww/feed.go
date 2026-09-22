package groww

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nats-io/nkeys"
	"google.golang.org/protobuf/encoding/protowire"
)

// DefaultFeedURL is the NATS WebSocket endpoint for the Groww Feed.
const DefaultFeedURL = "wss://socket-api.groww.in"

// DefaultSocketTokenURL is the endpoint to exchange a NKey public key for a NATS JWT + subscriptionId.
const DefaultSocketTokenURL = "https://api.groww.in/v1/api/apex/v1/socket/token/create/"

type FeedCallback func(meta FeedMetadata)
type LTPCallback func(ltp LTPData, meta FeedMetadata)
type MarketDepthCallback func(depth MarketDepthData, meta FeedMetadata)

type FeedClient struct {
	token      string
	feedURL    string
	baseURL    string
	httpClient *http.Client
	mu         sync.RWMutex
	connected  bool
	done       chan struct{}
	apiKey     string
	apiSecret  string
	// NATS over WebSocket
	wsConn          *websocket.Conn
	wsSubscriptions map[string]int // subject -> sid
	nextSid         int
	natsToken       string
	natsSeed        string
	subscriptionId  string
	tokenFile       string
	seedFile        string
	ltpSnapshot     map[string]map[string]map[string]LTPData
	indexSnapshot   map[string]map[string]map[string]IndexData
	depthSnapshot   map[string]map[string]map[string]MarketDepthData
	onData          FeedCallback
	onLTP           LTPCallback
	onDepth         MarketDepthCallback
}

func NewFeedClient(accessToken string) *FeedClient {
	if accessToken == "" {
		accessToken = os.Getenv("GROWW_ACCESS_TOKEN")
	}
	apiKey := os.Getenv("GROWW_API_KEY")
	apiSecret := os.Getenv("GROWW_API_SECRET")
	return newFeedClient(accessToken, apiKey, apiSecret)
}

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
	feedURL := envOrDefault("GROWW_FEED_URL", DefaultFeedURL)
	// Legacy migration: the old WebSocket endpoint wss://api.groww.in/v1/feed
	// now returns 404. If the env still points there, transparently upgrade
	// to the current NATS endpoint so existing deployments self-heal without
	// manual .env edits.
	if feedURL == "wss://api.groww.in/v1/feed" {
		log.Printf("[Groww Feed] legacy GROWW_FEED_URL %s is deprecated, using %s", feedURL, DefaultFeedURL)
		feedURL = DefaultFeedURL
	}
	return &FeedClient{
		token:           token,
		apiKey:          apiKey,
		apiSecret:       apiSecret,
		feedURL:         feedURL,
		baseURL:         envOrDefault("GROWW_BASE_URL", DefaultBaseURL),
		httpClient:      &http.Client{Timeout: DefaultHTTPTimeout},
		done:            make(chan struct{}),
		wsSubscriptions: make(map[string]int),
		nextSid:         1,
	}
}

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

func (f *FeedClient) refreshToken() error {
	token, err := fetchAccessToken(f.apiKey, f.apiSecret, f.baseURL, f.httpClient)
	if err != nil {
		return err
	}
	f.SetToken(token)
	log.Printf("[Groww Feed] token refreshed (expiry unknown, key %s)", maskKey(f.apiKey))
	return nil
}

func (f *FeedClient) SetFeedCallback(cb FeedCallback) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onData = cb
}

func (f *FeedClient) SetLTPCallback(cb LTPCallback) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onLTP = cb
}

func (f *FeedClient) SetDepthCallback(cb MarketDepthCallback) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onDepth = cb
}

func (f *FeedClient) Connect() error {
	f.mu.Lock()
	if f.done == nil {
		f.done = make(chan struct{})
	} else {
		select {
		case <-f.done:
			f.done = make(chan struct{})
		default:
		}
	}
	f.mu.Unlock()

	f.mu.RLock()
	apiKey := f.apiKey
	hasKeys := hasGrowwKeys(f.apiKey, f.apiSecret)
	f.mu.RUnlock()

	authToken := apiKey
	if authToken == "" {
		if err := f.ensureToken(); err != nil {
			return fmt.Errorf("groww feed ensure token: %w", err)
		}
		authToken = f.getToken()
	}

	kp, err := nkeys.CreateUser()
	if err != nil {
		return fmt.Errorf("groww feed nkeys create: %w", err)
	}
	pub, err := kp.PublicKey()
	if err != nil {
		return fmt.Errorf("groww feed nkeys public: %w", err)
	}
	seed, err := kp.Seed()
	if err != nil {
		return fmt.Errorf("groww feed nkeys seed: %w", err)
	}

	socketTokenURL := envOrDefault("GROWW_SOCKET_TOKEN_URL", DefaultSocketTokenURL)
	natsJWT, subID, err := f.fetchSocketToken(authToken, pub, socketTokenURL)
	if err != nil {
		if hasKeys && authToken == apiKey {
			log.Printf("[Groww Feed] socket token with apiKey failed (%v), trying refreshed access token", err)
			if refreshErr := f.refreshToken(); refreshErr == nil {
				natsJWT, subID, err = f.fetchSocketToken(f.getToken(), pub, socketTokenURL)
			}
		}
		if err != nil {
			return fmt.Errorf("groww feed socket token: %w", err)
		}
	}

	// Dial WebSocket
	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 10 * time.Second
	wsConn, _, err := dialer.Dial(f.feedURL, nil)
	if err != nil {
		return fmt.Errorf("groww feed NATS dial %s: %w", f.feedURL, err)
	}

	// NATS handshake: read INFO, send CONNECT with JWT/nkey/sig, handle PING/PONG
	_ = wsConn.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, infoMsg, err := wsConn.ReadMessage()
	if err != nil {
		_ = wsConn.Close()
		return fmt.Errorf("groww feed NATS INFO: %w", err)
	}
	var info struct {
		Nonce string `json:"nonce"`
	}
	start := bytes.Index(infoMsg, []byte("{"))
	end := bytes.LastIndex(infoMsg, []byte("}"))
	if start < 0 || end < 0 {
		_ = wsConn.Close()
		return fmt.Errorf("groww feed NATS INFO parse: %s", string(infoMsg))
	}
	if err := json.Unmarshal(infoMsg[start:end+1], &info); err != nil {
		_ = wsConn.Close()
		return fmt.Errorf("groww feed NATS INFO json: %w", err)
	}
	kp2, err := nkeys.FromSeed(seed)
	if err != nil {
		_ = wsConn.Close()
		return fmt.Errorf("groww feed nkeys from seed: %w", err)
	}
	sig, err := kp2.Sign([]byte(info.Nonce))
	if err != nil {
		_ = wsConn.Close()
		return fmt.Errorf("groww feed sign nonce: %w", err)
	}
	sigEnc := base64.StdEncoding.EncodeToString(sig)
	connectJSON := fmt.Sprintf(`{"jwt":"%s","nkey":"%s","sig":"%s","name":"navier-stocks","lang":"go","version":"1.0.0","protocol":1,"echo":true,"headers":true}`, natsJWT, pub, sigEnc)
	if err := wsConn.WriteMessage(websocket.TextMessage, []byte("CONNECT "+connectJSON+"\r\n")); err != nil {
		_ = wsConn.Close()
		return fmt.Errorf("groww feed CONNECT: %w", err)
	}
	// Wait for PING from server and reply with PONG
	_ = wsConn.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, pingMsg, err := wsConn.ReadMessage()
	if err != nil {
		_ = wsConn.Close()
		return fmt.Errorf("groww feed PING: %w", err)
	}
	if string(pingMsg) != "PING\r\n" {
		// Some servers send INFO again, handle
		if !bytes.HasPrefix(pingMsg, []byte("PING")) {
			_ = wsConn.Close()
			return fmt.Errorf("groww feed expected PING, got %q", string(pingMsg))
		}
	}
	if err := wsConn.WriteMessage(websocket.TextMessage, []byte("PONG\r\n")); err != nil {
		_ = wsConn.Close()
		return fmt.Errorf("groww feed PONG: %w", err)
	}

	f.mu.Lock()
	if f.tokenFile != "" {
		_ = os.Remove(f.tokenFile)
	}
	if f.seedFile != "" {
		_ = os.Remove(f.seedFile)
	}
	// Store temp files for cleanup (even though we use memory now, keep for compat)
	// We don't actually need files for manual NATS, but keep fields
	f.natsToken = natsJWT
	f.natsSeed = string(seed)
	f.subscriptionId = subID
	f.wsConn = wsConn
	f.connected = true
	f.wsSubscriptions = make(map[string]int)
	f.nextSid = 1
	f.mu.Unlock()

	log.Printf("[Groww Feed] NATS connected to %s (subId %s)", f.feedURL, maskKey(subID))
	// Start background read loop for PING/PONG and MSG
	go f.natsReadLoop(wsConn)
	return nil
}

func (f *FeedClient) fetchSocketToken(authToken, pubKey, tokenURL string) (string, string, error) {
	body := map[string]string{"socketKey": pubKey}
	b, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", tokenURL, bytes.NewReader(b))
	if err != nil {
		return "", "", fmt.Errorf("socket token request %s: %w", tokenURL, err)
	}
	req.Header.Set("Authorization", "Bearer "+authToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Client-Id", "growwapi")
	req.Header.Set("X-Client-Platform", "growwapi-python-client")
	req.Header.Set("X-Client-Platform-Version", "1.5.0")
	req.Header.Set("X-Api-Version", "1.0")
	resp, err := f.httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("socket token request %s: %w", tokenURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("socket token read %s: %w", tokenURL, err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("socket token HTTP %d at %s: %s", resp.StatusCode, tokenURL, string(raw))
	}
	var direct struct {
		Token          string `json:"token"`
		SubscriptionId string `json:"subscriptionId"`
		Expiry         string `json:"expiry"`
	}
	if err := json.Unmarshal(raw, &direct); err == nil && direct.Token != "" {
		return direct.Token, direct.SubscriptionId, nil
	}
	var env struct {
		Status  string `json:"status"`
		Payload *struct {
			Token          string `json:"token"`
			SubscriptionId string `json:"subscriptionId"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(raw, &env); err == nil && env.Payload != nil && env.Payload.Token != "" {
		return env.Payload.Token, env.Payload.SubscriptionId, nil
	}
	return "", "", fmt.Errorf("socket token decode: empty token body %s", string(raw))
}

func (f *FeedClient) natsReadLoop(conn *websocket.Conn) {
	defer func() {
		f.mu.Lock()
		f.connected = false
		f.mu.Unlock()
	}()
	for {
		select {
		case <-f.done:
			return
		default:
		}
		_ = conn.SetReadDeadline(time.Now().Add(70 * time.Second))
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure) {
				log.Printf("[Groww Feed] NATS read error: %v", err)
			}
			return
		}
		// Handle PING
		if bytes.Equal(msg, []byte("PING\r\n")) {
			_ = conn.WriteMessage(websocket.TextMessage, []byte("PONG\r\n"))
			continue
		}
		if bytes.HasPrefix(msg, []byte("PING")) {
			_ = conn.WriteMessage(websocket.TextMessage, []byte("PONG\r\n"))
			continue
		}
		if bytes.HasPrefix(msg, []byte("PONG")) {
			continue
		}
		if bytes.HasPrefix(msg, []byte("INFO")) {
			continue
		}
		if bytes.HasPrefix(msg, []byte("+OK")) {
			continue
		}
		if bytes.HasPrefix(msg, []byte("-ERR")) {
			log.Printf("[Groww Feed] NATS -ERR: %s", string(msg))
			continue
		}
		if bytes.HasPrefix(msg, []byte("MSG")) {
			f.handleNATSMsg(msg)
		}
	}
}

func (f *FeedClient) handleNATSMsg(raw []byte) {
	// MSG <subject> <sid> [reply] <len>\r\n<payload>\r\n
	// Example: MSG /ld/eq/nse/price.2885 1 42\r\n<proto bytes>\r\n
	parts := bytes.Split(raw, []byte(" "))
	if len(parts) < 3 {
		return
	}
	subject := string(parts[1])
	// Find payload after \r\n
	headerEnd := bytes.Index(raw, []byte("\r\n"))
	if headerEnd < 0 {
		return
	}
	// The header contains MSG, subject, sid, [reply], len
	// We need to parse len is last token before \r\n
	headerLine := raw[:headerEnd]
	tokens := bytes.Fields(headerLine)
	if len(tokens) < 3 {
		return
	}
	// Last token is length
	lenStr := string(tokens[len(tokens)-1])
	payloadStart := headerEnd + 2
	payloadEnd := len(raw) - 2 // trailing \r\n
	if payloadEnd < payloadStart {
		return
	}
	// Use lenStr to slice correctly in case payload contains \r\n
	// But for now use payloadStart:payloadEnd
	payload := raw[payloadStart:payloadEnd]
	_ = lenStr
	f.dispatchBySubject(subject, payload)
}

func (f *FeedClient) dispatchBySubject(subject string, payload []byte) {
	if len(subject) >= 13 && subject[:13] == "/ld/eq/nse/pri" || len(subject) >= 13 && subject[:11] == "/ld/eq/nse/" {
		// Could be price or book
		if len(subject) >= 16 && subject[11:15] == "book" || (len(subject) >= 16 && bytes.Contains([]byte(subject), []byte("/book."))) {
			f.handleDepthMessage(&natsMsg{Subject: subject, Data: payload})
		} else {
			f.handleLTPMessage(&natsMsg{Subject: subject, Data: payload})
		}
		return
	}
	if len(subject) >= 11 && (subject[:11] == "/ld/eq/bse/" || subject[:11] == "/ld/fo/nse/" || subject[:11] == "/ld/fo/bse/") {
		if bytes.Contains([]byte(subject), []byte("/book.")) {
			f.handleDepthMessage(&natsMsg{Subject: subject, Data: payload})
		} else {
			f.handleLTPMessage(&natsMsg{Subject: subject, Data: payload})
		}
		return
	}
	if len(subject) >= 15 && subject[:15] == "/ld/indices/" {
		f.handleIndexMessage(&natsMsg{Subject: subject, Data: payload})
		return
	}
	if subject == "stocks/order/updates.apex."+f.subscriptionId || subject == "stocks_fo/order/updates.apex."+f.subscriptionId {
		f.handleOrderMessage(&natsMsg{Subject: subject, Data: payload})
		return
	}
	if subject == "stocks_fo/position/updates.apex."+f.subscriptionId {
		f.handlePositionMessage(&natsMsg{Subject: subject, Data: payload})
		return
	}
	// Fallback: try LTP
	f.handleLTPMessage(&natsMsg{Subject: subject, Data: payload})
}

// Minimal nats.Msg replacement for handlers
type natsMsg struct {
	Subject string
	Data    []byte
}

func (f *FeedClient) SubscribeLTP(instruments []FeedInstrument, cb ...LTPCallback) error {
	if len(cb) > 0 {
		f.mu.Lock()
		f.onLTP = cb[0]
		f.mu.Unlock()
	}
	return f.subscribeLTP(instruments)
}

func (f *FeedClient) subscribeLTP(instruments []FeedInstrument) error {
	f.mu.RLock()
	conn := f.wsConn
	f.mu.RUnlock()
	if conn == nil {
		return fmt.Errorf("groww feed: not connected")
	}
	for _, inst := range instruments {
		subject := livePriceTopic(inst.Exchange, inst.Segment, inst.ExchangeToken)
		if err := f.natsSubscribe(subject); err != nil {
			return fmt.Errorf("subscribe %s: %w", subject, err)
		}
		log.Printf("[Groww Feed] subscribed LTP %s %s %s -> %s", inst.Exchange, inst.Segment, inst.ExchangeToken, subject)
	}
	return nil
}

func (f *FeedClient) SubscribeMarketDepth(instruments []FeedInstrument, cb ...MarketDepthCallback) error {
	if len(cb) > 0 {
		f.mu.Lock()
		f.onDepth = cb[0]
		f.mu.Unlock()
	}
	f.mu.RLock()
	conn := f.wsConn
	f.mu.RUnlock()
	if conn == nil {
		return fmt.Errorf("groww feed: not connected")
	}
	for _, inst := range instruments {
		subject := marketDepthTopic(inst.Exchange, inst.Segment, inst.ExchangeToken)
		if err := f.natsSubscribe(subject); err != nil {
			return fmt.Errorf("subscribe depth %s: %w", subject, err)
		}
		log.Printf("[Groww Feed] subscribed depth %s -> %s", inst.ExchangeToken, subject)
	}
	return nil
}

func (f *FeedClient) SubscribeIndexValue(instruments []FeedInstrument) error {
	f.mu.RLock()
	conn := f.wsConn
	f.mu.RUnlock()
	if conn == nil {
		return fmt.Errorf("groww feed: not connected")
	}
	for _, inst := range instruments {
		subject := indexPriceTopic(inst.Exchange, inst.ExchangeToken)
		if err := f.natsSubscribe(subject); err != nil {
			return fmt.Errorf("subscribe index %s: %w", subject, err)
		}
		log.Printf("[Groww Feed] subscribed index %s -> %s", inst.ExchangeToken, subject)
	}
	return nil
}

func (f *FeedClient) SubscribeEquityOrderUpdates(cb ...FeedCallback) error {
	if len(cb) > 0 {
		f.mu.Lock()
		f.onData = cb[0]
		f.mu.Unlock()
	}
	f.mu.RLock()
	conn := f.wsConn
	subID := f.subscriptionId
	f.mu.RUnlock()
	if conn == nil {
		return fmt.Errorf("groww feed: not connected")
	}
	subject := "stocks/order/updates.apex." + subID
	if err := f.natsSubscribe(subject); err != nil {
		return err
	}
	log.Printf("[Groww Feed] subscribed equity order updates %s", subject)
	return nil
}

func (f *FeedClient) SubscribeFNOOrderUpdates(cb ...FeedCallback) error {
	if len(cb) > 0 {
		f.mu.Lock()
		f.onData = cb[0]
		f.mu.Unlock()
	}
	f.mu.RLock()
	conn := f.wsConn
	subID := f.subscriptionId
	f.mu.RUnlock()
	if conn == nil {
		return fmt.Errorf("groww feed: not connected")
	}
	subject := "stocks_fo/order/updates.apex." + subID
	if err := f.natsSubscribe(subject); err != nil {
		return err
	}
	log.Printf("[Groww Feed] subscribed FNO order updates %s", subject)
	return nil
}

func (f *FeedClient) SubscribeFNOPositionUpdates(cb ...FeedCallback) error {
	if len(cb) > 0 {
		f.mu.Lock()
		f.onData = cb[0]
		f.mu.Unlock()
	}
	f.mu.RLock()
	conn := f.wsConn
	subID := f.subscriptionId
	f.mu.RUnlock()
	if conn == nil {
		return fmt.Errorf("groww feed: not connected")
	}
	subject := "stocks_fo/position/updates.apex." + subID
	if err := f.natsSubscribe(subject); err != nil {
		return err
	}
	log.Printf("[Groww Feed] subscribed FNO position updates %s", subject)
	return nil
}

func (f *FeedClient) natsSubscribe(subject string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.wsConn == nil {
		return fmt.Errorf("groww feed: not connected")
	}
	if _, exists := f.wsSubscriptions[subject]; exists {
		return nil
	}
	sid := f.nextSid
	f.nextSid++
	f.wsSubscriptions[subject] = sid
	// NATS SUB protocol: SUB <subject> <sid>\r\n
	msg := fmt.Sprintf("SUB %s %d\r\n", subject, sid)
	if err := f.wsConn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
		return err
	}
	return nil
}

func (f *FeedClient) UnsubscribeLTP(instruments []FeedInstrument) error {
	for _, inst := range instruments {
		subject := livePriceTopic(inst.Exchange, inst.Segment, inst.ExchangeToken)
		if err := f.natsUnsubscribe(subject); err != nil {
			return err
		}
	}
	return nil
}

func (f *FeedClient) UnsubscribeMarketDepth(instruments []FeedInstrument) error {
	for _, inst := range instruments {
		subject := marketDepthTopic(inst.Exchange, inst.Segment, inst.ExchangeToken)
		if err := f.natsUnsubscribe(subject); err != nil {
			return err
		}
	}
	return nil
}

func (f *FeedClient) UnsubscribeIndexValue(instruments []FeedInstrument) error {
	for _, inst := range instruments {
		subject := indexPriceTopic(inst.Exchange, inst.ExchangeToken)
		if err := f.natsUnsubscribe(subject); err != nil {
			return err
		}
	}
	return nil
}

func (f *FeedClient) natsUnsubscribe(subject string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	sid, ok := f.wsSubscriptions[subject]
	if !ok {
		return nil
	}
	msg := fmt.Sprintf("UNSUB %d\r\n", sid)
	if err := f.wsConn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
		return err
	}
	delete(f.wsSubscriptions, subject)
	return nil
}

func (f *FeedClient) Consume() {
	f.mu.RLock()
	conn := f.wsConn
	f.mu.RUnlock()
	if conn == nil {
		return
	}
	select {
	case <-f.done:
		return
	case <-time.After(24 * time.Hour):
		return
	}
}

func (f *FeedClient) GetLTP() map[string]map[string]map[string]LTPData {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.ltpSnapshot == nil {
		return nil
	}
	out := make(map[string]map[string]map[string]LTPData, len(f.ltpSnapshot))
	for ex, segs := range f.ltpSnapshot {
		out[ex] = make(map[string]map[string]LTPData, len(segs))
		for seg, toks := range segs {
			out[ex][seg] = make(map[string]LTPData, len(toks))
			for tok, v := range toks {
				out[ex][seg][tok] = v
			}
		}
	}
	return out
}

func (f *FeedClient) GetMarketDepth() map[string]map[string]map[string]MarketDepthData {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.depthSnapshot == nil {
		return nil
	}
	out := make(map[string]map[string]map[string]MarketDepthData, len(f.depthSnapshot))
	for ex, segs := range f.depthSnapshot {
		out[ex] = make(map[string]map[string]MarketDepthData, len(segs))
		for seg, toks := range segs {
			out[ex][seg] = make(map[string]MarketDepthData, len(toks))
			for tok, v := range toks {
				out[ex][seg][tok] = v
			}
		}
	}
	return out
}

func (f *FeedClient) GetIndexValue() map[string]map[string]map[string]IndexData {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.indexSnapshot == nil {
		return nil
	}
	out := make(map[string]map[string]map[string]IndexData, len(f.indexSnapshot))
	for ex, segs := range f.indexSnapshot {
		out[ex] = make(map[string]map[string]IndexData, len(segs))
		for seg, toks := range segs {
			out[ex][seg] = make(map[string]IndexData, len(toks))
			for tok, v := range toks {
				out[ex][seg][tok] = v
			}
		}
	}
	return out
}

func (f *FeedClient) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	select {
	case <-f.done:
	default:
		close(f.done)
	}
	if f.wsConn != nil {
		_ = f.wsConn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		_ = f.wsConn.Close()
		f.wsConn = nil
	}
	f.connected = false
	if f.tokenFile != "" {
		_ = os.Remove(f.tokenFile)
		f.tokenFile = ""
	}
	if f.seedFile != "" {
		_ = os.Remove(f.seedFile)
		f.seedFile = ""
	}
	f.wsSubscriptions = make(map[string]int)
	return nil
}

func (f *FeedClient) IsConnected() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.wsConn != nil {
		return true
	}
	return f.connected
}

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

func (f *FeedClient) handleLTPMessage(m *natsMsg) {
	subject := m.Subject
	token := subjectToken(subject)
	exchange, segment := subjectExchangeSegment(subject)
	if token == "" {
		return
	}
	ltp, err := parseLTPProto(m.Data)
	if err != nil {
		var fallback LTPData
		if json.Unmarshal(m.Data, &fallback) == nil && fallback.LTP != 0 {
			ltp = fallback
		} else {
			log.Printf("[Groww Feed] LTP parse failed %s: %v", subject, err)
			return
		}
	}
	f.mu.Lock()
	if f.ltpSnapshot == nil {
		f.ltpSnapshot = make(map[string]map[string]map[string]LTPData)
	}
	if _, ok := f.ltpSnapshot[string(exchange)]; !ok {
		f.ltpSnapshot[string(exchange)] = make(map[string]map[string]LTPData)
	}
	if _, ok := f.ltpSnapshot[string(exchange)][string(segment)]; !ok {
		f.ltpSnapshot[string(exchange)][string(segment)] = make(map[string]LTPData)
	}
	f.ltpSnapshot[string(exchange)][string(segment)][token] = ltp
	onLTP := f.onLTP
	onData := f.onData
	f.mu.Unlock()
	meta := FeedMetadata{Exchange: exchange, Segment: segment, FeedType: "ltp", FeedKey: token}
	if onData != nil {
		onData(meta)
	}
	if onLTP != nil {
		onLTP(ltp, meta)
	}
}

func (f *FeedClient) handleDepthMessage(m *natsMsg) {
	subject := m.Subject
	token := subjectToken(subject)
	exchange, segment := subjectExchangeSegment(subject)
	if token == "" {
		return
	}
	var depth MarketDepthData
	if ltp, err := parseLTPProto(m.Data); err == nil {
		depth.TsInMillis = ltp.TsInMillis
	} else {
		depth.TsInMillis = time.Now().UnixMilli()
	}
	f.mu.Lock()
	if f.depthSnapshot == nil {
		f.depthSnapshot = make(map[string]map[string]map[string]MarketDepthData)
	}
	if _, ok := f.depthSnapshot[string(exchange)]; !ok {
		f.depthSnapshot[string(exchange)] = make(map[string]map[string]MarketDepthData)
	}
	if _, ok := f.depthSnapshot[string(exchange)][string(segment)]; !ok {
		f.depthSnapshot[string(exchange)][string(segment)] = make(map[string]MarketDepthData)
	}
	f.depthSnapshot[string(exchange)][string(segment)][token] = depth
	onDepth := f.onDepth
	onData := f.onData
	f.mu.Unlock()
	meta := FeedMetadata{Exchange: exchange, Segment: segment, FeedType: "market_depth", FeedKey: token}
	if onData != nil {
		onData(meta)
	}
	if onDepth != nil {
		onDepth(depth, meta)
	}
}

func (f *FeedClient) handleIndexMessage(m *natsMsg) {
	subject := m.Subject
	token := subjectToken(subject)
	exchange, _ := subjectExchangeSegment(subject)
	if token == "" {
		return
	}
	var idx IndexData
	if ltp, err := parseLTPProto(m.Data); err == nil {
		idx.TsInMillis = ltp.TsInMillis
		idx.Value = ltp.LTP
	} else {
		_ = json.Unmarshal(m.Data, &idx)
		if idx.Value == 0 {
			return
		}
	}
	f.mu.Lock()
	if f.indexSnapshot == nil {
		f.indexSnapshot = make(map[string]map[string]map[string]IndexData)
	}
	if _, ok := f.indexSnapshot[string(exchange)]; !ok {
		f.indexSnapshot[string(exchange)] = make(map[string]map[string]IndexData)
	}
	seg := string(SegmentCash)
	if _, ok := f.indexSnapshot[string(exchange)][seg]; !ok {
		f.indexSnapshot[string(exchange)][seg] = make(map[string]IndexData)
	}
	f.indexSnapshot[string(exchange)][seg][token] = idx
	onData := f.onData
	f.mu.Unlock()
	meta := FeedMetadata{Exchange: exchange, Segment: SegmentCash, FeedType: "index_value", FeedKey: token}
	if onData != nil {
		onData(meta)
	}
}

func (f *FeedClient) handleOrderMessage(m *natsMsg) {
	f.mu.RLock()
	onData := f.onData
	f.mu.RUnlock()
	if onData != nil {
		meta := FeedMetadata{FeedType: "order_updates", FeedKey: m.Subject}
		onData(meta)
	}
	log.Printf("[Groww Feed] order update %s (%d bytes)", m.Subject, len(m.Data))
}

func (f *FeedClient) handlePositionMessage(m *natsMsg) {
	f.mu.RLock()
	onData := f.onData
	f.mu.RUnlock()
	if onData != nil {
		meta := FeedMetadata{FeedType: "position_updates", FeedKey: m.Subject}
		onData(meta)
	}
	log.Printf("[Groww Feed] position update %s (%d bytes)", m.Subject, len(m.Data))
}

func livePriceTopic(ex Exchange, seg Segment, token string) string {
	switch seg {
	case SegmentFNO:
		if ex == ExchangeBSE {
			return "/ld/fo/bse/price." + token
		}
		return "/ld/fo/nse/price." + token
	case SegmentCash:
		fallthrough
	default:
		if ex == ExchangeBSE {
			return "/ld/eq/bse/price." + token
		}
		return "/ld/eq/nse/price." + token
	}
}

func marketDepthTopic(ex Exchange, seg Segment, token string) string {
	switch seg {
	case SegmentFNO:
		if ex == ExchangeBSE {
			return "/ld/fo/bse/book." + token
		}
		return "/ld/fo/nse/book." + token
	default:
		if ex == ExchangeBSE {
			return "/ld/eq/bse/book." + token
		}
		return "/ld/eq/nse/book." + token
	}
}

func indexPriceTopic(ex Exchange, token string) string {
	if ex == ExchangeBSE {
		return "/ld/indices/bse/price." + token
	}
	return "/ld/indices/nse/price." + token
}

func subjectToken(subject string) string {
	for i := len(subject) - 1; i >= 0; i-- {
		if subject[i] == '.' {
			return subject[i+1:]
		}
	}
	return ""
}

func subjectExchangeSegment(subject string) (Exchange, Segment) {
	if len(subject) >= 11 && subject[:11] == "/ld/eq/nse/" {
		if len(subject) >= 16 && subject[:16] == "/ld/eq/nse/book" {
			return ExchangeNSE, SegmentCash
		}
		return ExchangeNSE, SegmentCash
	}
	if len(subject) >= 11 && subject[:11] == "/ld/eq/bse/" {
		return ExchangeBSE, SegmentCash
	}
	if len(subject) >= 11 && subject[:11] == "/ld/fo/nse/" {
		return ExchangeNSE, SegmentFNO
	}
	if len(subject) >= 11 && subject[:11] == "/ld/fo/bse/" {
		return ExchangeBSE, SegmentFNO
	}
	if len(subject) >= 15 && subject[:15] == "/ld/indices/nse" {
		return ExchangeNSE, SegmentCash
	}
	if len(subject) >= 15 && subject[:15] == "/ld/indices/bse" {
		return ExchangeBSE, SegmentCash
	}
	return ExchangeNSE, SegmentCash
}

func parseLTPProto(data []byte) (LTPData, error) {
	outer := data
	for len(outer) > 0 {
		num, wtype, n := protowire.ConsumeTag(outer)
		if n < 0 {
			return LTPData{}, fmt.Errorf("protowire tag: %w", protowire.ParseError(n))
		}
		outer = outer[n:]
		if num == 4 && wtype == protowire.BytesType {
			inner, m := protowire.ConsumeBytes(outer)
			if m < 0 {
				return LTPData{}, fmt.Errorf("protowire bytes: %w", protowire.ParseError(m))
			}
			return parseStocksLivePriceProto(inner)
		}
		m := protowire.ConsumeFieldValue(num, wtype, outer)
		if m < 0 {
			return LTPData{}, fmt.Errorf("protowire skip: %w", protowire.ParseError(m))
		}
		outer = outer[m:]
	}
	if ltp, err := parseStocksLivePriceProto(data); err == nil && ltp.LTP != 0 {
		return ltp, nil
	}
	return LTPData{}, fmt.Errorf("stockLivePrice not found")
}

func parseStocksLivePriceProto(data []byte) (LTPData, error) {
	var out LTPData
	remaining := data
	for len(remaining) > 0 {
		num, wtype, n := protowire.ConsumeTag(remaining)
		if n < 0 {
			return out, fmt.Errorf("protowire tag: %w", protowire.ParseError(n))
		}
		remaining = remaining[n:]
		switch num {
		case 1:
			if wtype != protowire.Fixed64Type {
				m := protowire.ConsumeFieldValue(num, wtype, remaining)
				if m < 0 {
					return out, protowire.ParseError(m)
				}
				remaining = remaining[m:]
				continue
			}
			v, m := protowire.ConsumeFixed64(remaining)
			if m < 0 {
				return out, protowire.ParseError(m)
			}
			remaining = remaining[m:]
			bits := uint64(v)
			f := math.Float64frombits(bits)
			out.TsInMillis = int64(f)
		case 13:
			if wtype != protowire.Fixed64Type {
				m := protowire.ConsumeFieldValue(num, wtype, remaining)
				if m < 0 {
					return out, protowire.ParseError(m)
				}
				remaining = remaining[m:]
				continue
			}
			v, m := protowire.ConsumeFixed64(remaining)
			if m < 0 {
				return out, protowire.ParseError(m)
			}
			remaining = remaining[m:]
			out.LTP = math.Float64frombits(uint64(v))
		default:
			m := protowire.ConsumeFieldValue(num, wtype, remaining)
			if m < 0 {
				return out, protowire.ParseError(m)
			}
			remaining = remaining[m:]
		}
	}
	return out, nil
}

//nolint:unused
func wrapDialError(prefix string, err error, resp *http.Response) error {
	if resp != nil {
		return fmt.Errorf("%s: %w (HTTP %d)", prefix, err, resp.StatusCode)
	}
	return fmt.Errorf("%s: %w", prefix, err)
}

//nolint:unused
func httpStatus(resp *http.Response) string {
	if resp == nil {
		return "-"
	}
	return fmt.Sprintf("%d", resp.StatusCode)
}
