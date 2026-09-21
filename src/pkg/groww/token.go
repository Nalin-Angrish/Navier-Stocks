package groww

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// fetchAccessToken exchanges API key+secret for an access token.
// Shared by REST Client and FeedClient to avoid duplicated logic.
// It mirrors Groww's spec: POST {baseURL}/token/api/access with
// Authorization: Bearer <apiKey> and JSON {key_type, checksum, timestamp}
// where checksum = hex(sha256(secret + timestamp)).
func fetchAccessToken(apiKey, apiSecret, baseURL string, httpClient *http.Client) (string, error) {
	tokenURL := resolveTokenURL(baseURL)

	ts := fmt.Sprintf("%d", time.Now().Unix())
	sum := sha256.Sum256([]byte(apiSecret + ts))
	checksum := fmt.Sprintf("%x", sum)

	payload := map[string]string{
		"key_type":  "approval",
		"checksum":  checksum,
		"timestamp": ts,
	}
	body, _ := json.Marshal(payload)

	log.Printf("[Groww] token refresh POST %s (key %s)", tokenURL, maskKey(apiKey))

	req, err := http.NewRequest("POST", tokenURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("token refresh request %s: %w", tokenURL, err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token refresh request %s: %w", tokenURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("token refresh HTTP %d at %s: %s", resp.StatusCode, tokenURL, string(raw))
	}

	var result struct {
		Token      string `json:"token"`
		TokenRefID string `json:"tokenRefId"`
		Expiry     string `json:"expiry"`
		IsActive   bool   `json:"isActive"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("token refresh decode: %w", err)
	}
	if result.Token == "" {
		return "", fmt.Errorf("token refresh: empty token in response")
	}
	return result.Token, nil
}

// hasGrowwKeys reports whether both API credentials are present.
func hasGrowwKeys(apiKey, apiSecret string) bool {
	return apiKey != "" && apiSecret != ""
}

// resolveTokenURL returns the token endpoint, respecting GROWW_TOKEN_URL
// override. If GROWW_TOKEN_URL is unset it builds {baseURL}/token/api/access
// but normalizes baseURL to include /v1 so that a bare
// https://api.groww.in (which would give /token/api/access → 404) still
// resolves to /v1/token/api/access.
func resolveTokenURL(baseURL string) string {
	if v := os.Getenv("GROWW_TOKEN_URL"); v != "" {
		return v
	}
	base := strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	if !strings.HasSuffix(base, "/v1") {
		base += "/v1"
	}
	return base + "/token/api/access"
}

// maskKey returns a safe, truncated representation for logs.
func maskKey(k string) string {
	if len(k) <= 4 {
		return "****"
	}
	return k[:4] + "****"
}
