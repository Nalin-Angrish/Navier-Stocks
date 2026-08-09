package sentiment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
)

// LLMClient scores a single news article, returning a SentimentScore with a
// BULLISH/BEARISH/NEUTRAL verdict and a confidence in [0,1].  Implementations
// talk to an LLM provider over HTTP and parse the structured response.
type LLMClient interface {
	Score(ctx context.Context, article models.Article) (*models.SentimentScore, error)
}

// LLMConfig holds the provider-agnostic settings used to build an LLMClient.
type LLMConfig struct {
	Provider string        // "openai" (OpenAI-compatible) or "ollama"
	BaseURL  string        // server root, e.g. https://api.openai.com/v1
	APIKey   string        // bearer token (ignored by local providers)
	Model    string        // model identifier, e.g. gpt-4o-mini or llama3
	Timeout  time.Duration // per-request timeout
}

// NewLLMClient validates the config and returns a client for the requested
// provider.  Both supported providers speak a chat-completions style API; only
// the endpoint path and response envelope differ.
func NewLLMClient(cfg LLMConfig) (LLMClient, error) {
	if cfg.Model == "" {
		return nil, fmt.Errorf("llm: model is required")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	hc := &http.Client{Timeout: cfg.Timeout}

	switch strings.ToLower(cfg.Provider) {
	case "", "openai":
		return &openAIClient{
			baseURL: strings.TrimSuffix(cfg.BaseURL, "/"),
			apiKey:  cfg.APIKey,
			model:   cfg.Model,
			client:  hc,
		}, nil
	case "ollama":
		return &ollamaClient{
			baseURL: strings.TrimSuffix(cfg.BaseURL, "/"),
			model:   cfg.Model,
			client:  hc,
		}, nil
	default:
		return nil, fmt.Errorf("llm: unsupported provider %q (want openai or ollama)", cfg.Provider)
	}
}

// LLMConfigFromEnv builds an LLMConfig from environment variables, applying
// sensible defaults for a local Ollama deployment:
//
//	LLM_PROVIDER  (default "ollama")
//	LLM_BASE_URL  (default https://api.openai.com/v1 for openai, else http://localhost:11434)
//	LLM_API_KEY   (default "")
//	LLM_MODEL     (default "gpt-4o-mini" for openai, else "llama3")
//	LLM_TIMEOUT   (Go duration, default "60s")
func LLMConfigFromEnv() LLMConfig {
	provider := strings.ToLower(os.Getenv("LLM_PROVIDER"))
	if provider == "" {
		provider = "ollama"
	}

	var baseURL, model string
	switch provider {
	case "openai":
		baseURL = envOr("LLM_BASE_URL", "https://api.openai.com/v1")
		model = envOr("LLM_MODEL", "gpt-4o-mini")
	default:
		baseURL = envOr("LLM_BASE_URL", "http://localhost:11434")
		model = envOr("LLM_MODEL", "llama3")
	}

	timeout := 60 * time.Second
	if raw := os.Getenv("LLM_TIMEOUT"); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil {
			timeout = d
		}
	}

	return LLMConfig{
		Provider: provider,
		BaseURL:  baseURL,
		APIKey:   os.Getenv("LLM_API_KEY"),
		Model:    model,
		Timeout:  timeout,
	}
}

// envOr returns the environment variable value if set, otherwise fallback.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// systemPrompt instructs the model to return a single, machine-parseable JSON
// object so the response can be unmarshalled into a SentimentScore.
const systemPrompt = `You are a financial sentiment analyst for Indian equity markets. Given a news headline about a listed company, classify the sentiment it conveys as BULLISH, BEARISH, or NEUTRAL and rate your confidence between 0 and 1. Respond with a single JSON object only, using this exact shape: {"bias":"BULLISH","confidence":0.0,"summary":"one-sentence rationale"}`

// userPrompt renders the per-article user message fed to the model.
func userPrompt(a models.Article) string {
	published := ""
	if !a.PublishedAt.IsZero() {
		published = a.PublishedAt.Format(time.RFC3339)
	}
	return fmt.Sprintf(
		"Ticker: %s\nHeadline: %s\nPublished: %s\nSource: %s\nURL: %s\n\nClassify the sentiment of this headline for the ticker.",
		a.Ticker, a.Title, published, a.Source, a.URL,
	)
}

// chatMessage mirrors the chat-completions message envelope.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatRequest mirrors the chat-completions request envelope.
type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []chatMessage   `json:"messages"`
	Stream         bool            `json:"stream,omitempty"`
	Format         string          `json:"format,omitempty"` // Ollama JSON-mode
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
	Temperature    float64         `json:"temperature,omitempty"`
}

type responseFormat struct {
	Type string `json:"type"`
}

// openAIClient talks to any OpenAI-compatible /chat/completions endpoint
// (OpenAI, Azure, vLLM, and others) using a bearer-token style API key.
type openAIClient struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// Score sends the article to the provider and parses the assistant content.
func (c *openAIClient) Score(ctx context.Context, article models.Article) (*models.SentimentScore, error) {
	body, err := json.Marshal(chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt(article)},
		},
		ResponseFormat: &responseFormat{Type: "json_object"},
		Temperature:    0,
	})
	if err != nil {
		return nil, fmt.Errorf("llm: marshal request: %w", err)
	}

	resp, err := c.do(ctx, c.baseURL+"/chat/completions", body)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, httpStatusError(resp, "llm")
	}

	var out openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("llm: decode response: %w", err)
	}
	if len(out.Choices) == 0 || strings.TrimSpace(out.Choices[0].Message.Content) == "" {
		return nil, fmt.Errorf("llm: empty response from provider")
	}

	return buildScore(out.Choices[0].Message.Content, article)
}

// ollamaClient talks to a local Ollama server via its /api/chat endpoint.
// JSON mode is requested so the model emits a single parseable object.
type ollamaClient struct {
	baseURL string
	model   string
	client  *http.Client
}

type ollamaResponse struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
}

// Score sends the article to Ollama and parses the assistant content.
func (c *ollamaClient) Score(ctx context.Context, article models.Article) (*models.SentimentScore, error) {
	body, err := json.Marshal(chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt(article)},
		},
		Stream:      false,
		Format:      "json",
		Temperature: 0,
	})
	if err != nil {
		return nil, fmt.Errorf("llm: marshal request: %w", err)
	}

	resp, err := c.do(ctx, c.baseURL+"/api/chat", body)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, httpStatusError(resp, "llm")
	}

	var out ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("llm: decode response: %w", err)
	}
	if strings.TrimSpace(out.Message.Content) == "" {
		return nil, fmt.Errorf("llm: empty response from provider")
	}

	return buildScore(out.Message.Content, article)
}

// do performs the POST request with the caller's context.
func (c *openAIClient) do(ctx context.Context, url string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("llm: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("llm: %w", err)
	}
	return resp, nil
}

// do performs the POST request with the caller's context.
func (c *ollamaClient) do(ctx context.Context, url string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("llm: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("llm: %w", err)
	}
	return resp, nil
}

// buildScore converts the raw assistant content into a SentimentScore,
// attaching the article's citation metadata so the caller can persist it
// directly.
func buildScore(content string, article models.Article) (*models.SentimentScore, error) {
	content = stripCodeFences(content)

	var raw struct {
		Bias       string  `json:"bias"`
		Confidence float64 `json:"confidence"`
		Summary    string  `json:"summary"`
	}
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, fmt.Errorf("llm: parse content: %w", err)
	}

	score := &models.SentimentScore{
		Ticker:      article.Ticker,
		Bias:        models.SentimentBias(strings.ToUpper(strings.TrimSpace(raw.Bias))),
		Confidence:  raw.Confidence,
		Summary:     raw.Summary,
		Source:      article.Source,
		SourceURL:   article.URL,
		RawResponse: content,
	}
	if err := score.Validate(); err != nil {
		return nil, fmt.Errorf("llm: invalid score: %w", err)
	}
	return score, nil
}

// stripCodeFences removes any ```json ... ``` markers a model may wrap its
// output in before JSON parsing.
func stripCodeFences(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if idx := strings.IndexByte(s, '\n'); idx >= 0 {
			s = s[idx+1:]
		} else {
			s = ""
		}
	}
	if idx := strings.LastIndex(s, "```"); idx >= 0 {
		s = s[:idx]
	}
	return strings.TrimSpace(s)
}

// httpStatusError builds a descriptive error for a non-2xx LLM response.
func httpStatusError(resp *http.Response, caller string) error {
	msg := ""
	if data, err := io.ReadAll(resp.Body); err == nil && len(data) > 0 {
		msg = ": " + strings.TrimSpace(string(data))
	}
	return fmt.Errorf("%s: provider returned HTTP %d%s", caller, resp.StatusCode, msg)
}
