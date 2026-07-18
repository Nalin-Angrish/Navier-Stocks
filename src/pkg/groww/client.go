package groww

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

// Defaults for the Groww API client.
const (
	DefaultBaseURL    = "https://api.groww.in/v1"
	DefaultAPIVersion = "1.0"
	DefaultHTTPTimeout = 10 * time.Second

	// Rate-limit defaults (conservative safety margins applied).
	ordersMaxPerSec    = 8
	liveDataMaxPerSec  = 8
	nonTradingMaxPerSec = 15
)

// Client is an HTTP client for the Groww Trading REST API.  It manages
// authentication, rate limiting, and the standard JSON envelope.
type Client struct {
	baseURL    string
	apiVersion string
	httpClient *http.Client
	token      string

	// Optional API key + secret for daily token refresh.
	apiKey    string
	apiSecret string

	mu            sync.Mutex
	ordersRL      *rateLimiter
	liveDataRL    *rateLimiter
	nonTradingRL  *rateLimiter
}

// NewClient creates a Client authenticated with a static access token.
// The token is read from GROWW_ACCESS_TOKEN if the argument is empty.
func NewClient(accessToken string) *Client {
	if accessToken == "" {
		accessToken = os.Getenv("GROWW_ACCESS_TOKEN")
	}
	return newClient(accessToken, "", "")
}

// NewClientFromKeys creates a Client that uses an API key + secret pair and
// will attempt to refresh the access token daily.  The key and secret are
// read from GROWW_API_KEY and GROWW_API_SECRET if empty.
func NewClientFromKeys(apiKey, apiSecret string) *Client {
	if apiKey == "" {
		apiKey = os.Getenv("GROWW_API_KEY")
	}
	if apiSecret == "" {
		apiSecret = os.Getenv("GROWW_API_SECRET")
	}
	return newClient("", apiKey, apiSecret)
}

func newClient(token, apiKey, apiSecret string) *Client {
	c := &Client{
		baseURL:     envOrDefault("GROWW_BASE_URL", DefaultBaseURL),
		apiVersion:  DefaultAPIVersion,
		httpClient:  &http.Client{Timeout: DefaultHTTPTimeout},
		token:       token,
		apiKey:      apiKey,
		apiSecret:   apiSecret,
		ordersRL:    newRateLimiter(ordersMaxPerSec),
		liveDataRL:  newRateLimiter(liveDataMaxPerSec),
		nonTradingRL: newRateLimiter(nonTradingMaxPerSec),
	}
	return c
}

// SetToken replaces the bearer token at runtime (used after a refresh).
func (c *Client) SetToken(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.token = token
}

// do sends an HTTP request, applies rate limiting, handles auth errors,
// and decodes the standard Groww envelope.
func (c *Client) do(method, path string, body any, rateBucket *rateLimiter) (*http.Response, error) {
	if rateBucket != nil {
		rateBucket.Wait()
	}

	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("groww marshal: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, c.baseURL+path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("groww request: %w", err)
	}
	c.mu.Lock()
	token := c.token
	c.mu.Unlock()

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-API-VERSION", c.apiVersion)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("groww: %w", err)
	}

	// Handle 401 — attempt token refresh once.
	if resp.StatusCode == http.StatusUnauthorized && c.apiKey != "" && c.apiSecret != "" {
		resp.Body.Close()
		if refreshErr := c.refreshToken(); refreshErr != nil {
			return nil, fmt.Errorf("%w: %v", ErrAuthExpired, refreshErr)
		}
		// Retry with new token.
		return c.do(method, path, body, rateBucket)
	}

	return resp, nil
}

// refreshToken exchanges the API key + secret for a new access token.
// Uses the Groww Cloud token endpoint:
//
//	POST /v1/token/api/access
//	Authorization: Bearer <API_KEY>
//	{"key_type":"approval","checksum":"<sha256(secret+timestamp)>","timestamp":"<epoch_seconds>"}
func (c *Client) refreshToken() error {
	tokenURL := envOrDefault("GROWW_TOKEN_URL", c.baseURL+"/token/api/access")

	ts := fmt.Sprintf("%d", time.Now().Unix())
	h := sha256.Sum256([]byte(c.apiSecret + ts))
	checksum := fmt.Sprintf("%x", h)

	payload := map[string]string{
		"key_type":  "approval",
		"checksum":  checksum,
		"timestamp": ts,
	}
	data, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", tokenURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("token refresh request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("token refresh request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token refresh HTTP %d", resp.StatusCode)
	}

	var result struct {
		Token      string `json:"token"`
		TokenRefID string `json:"tokenRefId"`
		Expiry     string `json:"expiry"`
		IsActive   bool   `json:"isActive"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("token refresh decode: %w", err)
	}
	if result.Token == "" {
		return fmt.Errorf("token refresh: empty token in response")
	}

	c.SetToken(result.Token)
	return nil
}

// decodeResponse reads the HTTP body, checks the envelope, and unmarshals
// the payload into dst.
func decodeResponse(resp *http.Response, dst any) error {
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("groww read: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeError(resp.StatusCode, raw)
	}

	var envelope struct {
		Status  string          `json:"status"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("groww decode: %w", err)
	}
	if envelope.Status != APIStatusSuccess {
		return decodeError(resp.StatusCode, raw)
	}
	if dst != nil && envelope.Payload != nil {
		if err := json.Unmarshal(envelope.Payload, dst); err != nil {
			return fmt.Errorf("groww payload decode: %w", err)
		}
	}
	return nil
}

// decodeError constructs an APIError from a non-2xx response.
func decodeError(status int, body []byte) error {
	var envelope struct {
		Status  string `json:"status"`
		Payload struct {
			Code    string `json:"code,omitempty"`
			Message string `json:"message,omitempty"`
			Remark  string `json:"remark,omitempty"`
		} `json:"payload"`
	}
	msg := fmt.Sprintf("HTTP %d", status)
	code := ""
	if err := json.Unmarshal(body, &envelope); err == nil && envelope.Payload.Message != "" {
		msg = envelope.Payload.Message
		code = envelope.Payload.Code
	}
	if msg == "" {
		msg = string(body)
	}
	return &APIError{HTTPStatus: status, Code: code, Message: msg}
}

// ---------------------------------------------------------------------------
// Rate limiter (token bucket)
// ---------------------------------------------------------------------------

type rateLimiter struct {
	mu       sync.Mutex
	tokens   float64
	maxRate  float64
	lastTick time.Time
}

func newRateLimiter(perSec float64) *rateLimiter {
	return &rateLimiter{
		tokens:   perSec,
		maxRate:  perSec,
		lastTick: time.Now(),
	}
}

func (rl *rateLimiter) Wait() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastTick)
	rl.lastTick = now

	rl.tokens += elapsed.Seconds() * rl.maxRate
	if rl.tokens > rl.maxRate {
		rl.tokens = rl.maxRate
	}
	if rl.tokens < 1 {
		sleep := time.Duration((1 - rl.tokens) / rl.maxRate * float64(time.Second))
		rl.mu.Unlock()
		time.Sleep(sleep)
		rl.mu.Lock()
		rl.lastTick = time.Now()
		rl.tokens = rl.maxRate - 1
		return
	}
	rl.tokens--
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
