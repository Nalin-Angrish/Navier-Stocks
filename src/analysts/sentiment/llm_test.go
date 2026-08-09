package sentiment_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Nalin-Angrish/Navier-Stocks/src/analysts/sentiment"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
)

func TestNewLLMClient_RequiresModel(t *testing.T) {
	_, err := sentiment.NewLLMClient(sentiment.LLMConfig{Provider: "openai"})
	if err == nil {
		t.Fatal("expected error for missing model")
	}
}

func TestNewLLMClient_UnsupportedProvider(t *testing.T) {
	_, err := sentiment.NewLLMClient(sentiment.LLMConfig{Provider: "gemini", Model: "x"})
	if err == nil {
		t.Fatal("expected error for unsupported provider")
	}
}

func TestNewLLMClient_DefaultsProviderToOpenAI(t *testing.T) {
	_, err := sentiment.NewLLMClient(sentiment.LLMConfig{Model: "gpt-4o-mini"})
	if err != nil {
		t.Fatalf("expected default openai provider to build: %v", err)
	}
}

func testArticle() models.Article {
	return models.Article{
		Ticker:      "RELIANCE",
		Title:       "Reliance posts strong quarterly results",
		URL:         "https://news.example.com/rel/1",
		Source:      "google-news",
		Description: "Reliance beat analyst estimates.",
		PublishedAt: time.Date(2026, 8, 9, 9, 0, 0, 0, time.UTC),
	}
}

func TestOpenAIScore_Success(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("Authorization = %q, want Bearer test-key", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"bias\":\"BULLISH\",\"confidence\":0.82,\"summary\":\"Strong results beat estimates\"}"}}]}`))
	}))
	defer srv.Close()

	client, err := sentiment.NewLLMClient(sentiment.LLMConfig{
		Provider: "openai",
		BaseURL:  srv.URL,
		APIKey:   "test-key",
		Model:    "gpt-4o-mini",
	})
	if err != nil {
		t.Fatalf("NewLLMClient: %v", err)
	}

	score, err := client.Score(context.Background(), testArticle())
	if err != nil {
		t.Fatalf("Score: %v", err)
	}

	if score.Bias != models.BiasBullish {
		t.Fatalf("Bias = %q, want BULLISH", score.Bias)
	}
	if score.Confidence != 0.82 {
		t.Fatalf("Confidence = %f, want 0.82", score.Confidence)
	}
	if score.Summary != "Strong results beat estimates" {
		t.Fatalf("Summary = %q", score.Summary)
	}
	if score.Ticker != "RELIANCE" {
		t.Fatalf("Ticker = %q, want RELIANCE", score.Ticker)
	}
	if score.SourceURL != "https://news.example.com/rel/1" {
		t.Fatalf("SourceURL = %q", score.SourceURL)
	}
	if !strings.Contains(score.RawResponse, `"bias":"BULLISH"`) {
		t.Fatalf("RawResponse = %q, want original LLM content", score.RawResponse)
	}

	model, _ := gotBody["model"].(string)
	if model != "gpt-4o-mini" {
		t.Fatalf("request model = %q, want gpt-4o-mini", model)
	}
}

func TestOpenAIScore_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("rate limited"))
	}))
	defer srv.Close()

	client, err := sentiment.NewLLMClient(sentiment.LLMConfig{
		Provider: "openai",
		BaseURL:  srv.URL,
		Model:    "gpt-4o-mini",
	})
	if err != nil {
		t.Fatalf("NewLLMClient: %v", err)
	}

	_, err = client.Score(context.Background(), testArticle())
	if err == nil {
		t.Fatal("expected error for HTTP 429")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Fatalf("error = %v, want status 429", err)
	}
}

func TestOpenAIScore_EmptyChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv.Close()

	client, err := sentiment.NewLLMClient(sentiment.LLMConfig{Provider: "openai", BaseURL: srv.URL, Model: "gpt-4o-mini"})
	if err != nil {
		t.Fatalf("NewLLMClient: %v", err)
	}

	_, err = client.Score(context.Background(), testArticle())
	if err == nil {
		t.Fatal("expected error for empty choices")
	}
}

func TestOpenAIScore_NetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	client, err := sentiment.NewLLMClient(sentiment.LLMConfig{Provider: "openai", BaseURL: srv.URL, Model: "gpt-4o-mini"})
	if err != nil {
		t.Fatalf("NewLLMClient: %v", err)
	}
	srv.Close()

	_, err = client.Score(context.Background(), testArticle())
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
}

func TestOllamaScore_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		var req struct {
			Model    string `json:"model"`
			Format   string `json:"format"`
			Stream   bool   `json:"stream"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Model != "llama3" {
			t.Fatalf("model = %q, want llama3", req.Model)
		}
		if req.Format != "json" {
			t.Fatalf("format = %q, want json", req.Format)
		}
		if req.Stream {
			t.Fatal("expected stream=false")
		}
		_, _ = w.Write([]byte(`{"message":{"content":"{\"bias\":\"BEARISH\",\"confidence\":0.63,\"summary\":\"NPA concerns weigh on the stock\"}"},"done":true}`))
	}))
	defer srv.Close()

	client, err := sentiment.NewLLMClient(sentiment.LLMConfig{Provider: "ollama", BaseURL: srv.URL, Model: "llama3"})
	if err != nil {
		t.Fatalf("NewLLMClient: %v", err)
	}

	score, err := client.Score(context.Background(), testArticle())
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if score.Bias != models.BiasBearish {
		t.Fatalf("Bias = %q, want BEARISH", score.Bias)
	}
	if score.Confidence != 0.63 {
		t.Fatalf("Confidence = %f, want 0.63", score.Confidence)
	}
}

func TestScore_StripsCodeFences(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("{\"message\":{\"content\":\"```json\\n{\\\"bias\\\":\\\"NEUTRAL\\\",\\\"confidence\\\":0.5,\\\"summary\\\":\\\"Mixed signals\\\"}\\n```\"},\"done\":true}"))
	}))
	defer srv.Close()

	client, err := sentiment.NewLLMClient(sentiment.LLMConfig{Provider: "ollama", BaseURL: srv.URL, Model: "llama3"})
	if err != nil {
		t.Fatalf("NewLLMClient: %v", err)
	}

	score, err := client.Score(context.Background(), testArticle())
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if score.Bias != models.BiasNeutral {
		t.Fatalf("Bias = %q, want NEUTRAL", score.Bias)
	}
	if score.Confidence != 0.5 {
		t.Fatalf("Confidence = %f, want 0.5", score.Confidence)
	}
}

func TestScore_InvalidJSONContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("{\"message\":{\"content\":\"not json at all\"},\"done\":true}"))
	}))
	defer srv.Close()

	client, err := sentiment.NewLLMClient(sentiment.LLMConfig{Provider: "ollama", BaseURL: srv.URL, Model: "llama3"})
	if err != nil {
		t.Fatalf("NewLLMClient: %v", err)
	}

	_, err = client.Score(context.Background(), testArticle())
	if err == nil {
		t.Fatal("expected error for unparseable content")
	}
}

func TestScore_InvalidBias(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"message":{"content":"{\"bias\":\"LUNAR\",\"confidence\":0.5,\"summary\":\"x\"}"},"done":true}`))
	}))
	defer srv.Close()

	client, err := sentiment.NewLLMClient(sentiment.LLMConfig{Provider: "ollama", BaseURL: srv.URL, Model: "llama3"})
	if err != nil {
		t.Fatalf("NewLLMClient: %v", err)
	}

	_, err = client.Score(context.Background(), testArticle())
	if err == nil {
		t.Fatal("expected error for invalid bias")
	}
}

func TestScore_InvalidConfidence(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"message":{"content":"{\"bias\":\"BULLISH\",\"confidence\":1.7,\"summary\":\"x\"}"},"done":true}`))
	}))
	defer srv.Close()

	client, err := sentiment.NewLLMClient(sentiment.LLMConfig{Provider: "ollama", BaseURL: srv.URL, Model: "llama3"})
	if err != nil {
		t.Fatalf("NewLLMClient: %v", err)
	}

	_, err = client.Score(context.Background(), testArticle())
	if err == nil {
		t.Fatal("expected error for confidence > 1")
	}
}

func TestLLMConfigFromEnv_Defaults(t *testing.T) {
	t.Setenv("LLM_PROVIDER", "")
	t.Setenv("LLM_BASE_URL", "")
	t.Setenv("LLM_API_KEY", "")
	t.Setenv("LLM_MODEL", "")
	t.Setenv("LLM_TIMEOUT", "")

	cfg := sentiment.LLMConfigFromEnv()
	if cfg.Provider != "ollama" {
		t.Fatalf("Provider = %q, want ollama", cfg.Provider)
	}
	if cfg.BaseURL != "http://localhost:11434" {
		t.Fatalf("BaseURL = %q, want http://localhost:11434", cfg.BaseURL)
	}
	if cfg.Model != "llama3" {
		t.Fatalf("Model = %q, want llama3", cfg.Model)
	}
	if cfg.Timeout != 60*time.Second {
		t.Fatalf("Timeout = %v, want 60s", cfg.Timeout)
	}
}

func TestLLMConfigFromEnv_Overrides(t *testing.T) {
	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("LLM_BASE_URL", "https://llm.example.com/v1")
	t.Setenv("LLM_API_KEY", "secret")
	t.Setenv("LLM_MODEL", "gpt-4o")
	t.Setenv("LLM_TIMEOUT", "10s")

	cfg := sentiment.LLMConfigFromEnv()
	if cfg.Provider != "openai" {
		t.Fatalf("Provider = %q, want openai", cfg.Provider)
	}
	if cfg.BaseURL != "https://llm.example.com/v1" {
		t.Fatalf("BaseURL = %q", cfg.BaseURL)
	}
	if cfg.APIKey != "secret" {
		t.Fatalf("APIKey = %q", cfg.APIKey)
	}
	if cfg.Model != "gpt-4o" {
		t.Fatalf("Model = %q", cfg.Model)
	}
	if cfg.Timeout != 10*time.Second {
		t.Fatalf("Timeout = %v, want 10s", cfg.Timeout)
	}
}
