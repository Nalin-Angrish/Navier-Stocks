package groww

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// fetchAccessToken exchanges API key+secret for an access token.
// Shared by REST Client and FeedClient to avoid duplicated logic.
// It mirrors Groww's spec: POST {baseURL}/token/api/access with
// Authorization: Bearer <apiKey> and JSON {key_type, checksum, timestamp}
// where checksum = hex(sha256(secret + timestamp)).
func fetchAccessToken(apiKey, apiSecret, baseURL string, httpClient *http.Client) (string, error) {
	tokenURL := envOrDefault("GROWW_TOKEN_URL", baseURL+"/token/api/access")

	ts := fmt.Sprintf("%d", time.Now().Unix())
	sum := sha256.Sum256([]byte(apiSecret + ts))
	checksum := fmt.Sprintf("%x", sum)

	payload := map[string]string{
		"key_type":  "approval",
		"checksum":  checksum,
		"timestamp": ts,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", tokenURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("token refresh request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token refresh request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("token refresh HTTP %d: %s", resp.StatusCode, string(raw))
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

// maskKey returns a safe, truncated representation for logs.
func maskKey(k string) string {
	if len(k) <= 4 {
		return "****"
	}
	return k[:4] + "****"
}
