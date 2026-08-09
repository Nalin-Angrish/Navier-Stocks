package models_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
)

func TestSentimentScore_Validate_Valid(t *testing.T) {
	score := models.SentimentScore{
		Ticker:     "RELIANCE",
		Bias:       models.BiasBullish,
		Confidence: 0.8,
		Summary:    "Strong Q2 results beat estimates",
		Source:     "google-news",
		SourceURL:  "https://news.example.com/rel/1",
	}
	if err := score.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSentimentScore_Validate_MissingTicker(t *testing.T) {
	score := models.SentimentScore{
		Bias:       models.BiasNeutral,
		Confidence: 0.5,
		SourceURL:  "https://news.example.com/x",
	}
	if err := score.Validate(); err == nil {
		t.Fatal("expected error for empty ticker")
	}
}

func TestSentimentScore_Validate_InvalidBias(t *testing.T) {
	score := models.SentimentScore{
		Ticker:     "TCS",
		Bias:       "LUNAR",
		Confidence: 0.5,
		SourceURL:  "https://news.example.com/x",
	}
	if err := score.Validate(); err == nil {
		t.Fatal("expected error for invalid bias")
	}
}

func TestSentimentScore_Validate_ConfidenceBounds(t *testing.T) {
	tooLow := models.SentimentScore{
		Ticker:     "TCS",
		Bias:       models.BiasBullish,
		Confidence: -0.1,
		SourceURL:  "https://news.example.com/x",
	}
	if err := tooLow.Validate(); err == nil {
		t.Fatal("expected error for negative confidence")
	}

	tooHigh := models.SentimentScore{
		Ticker:     "TCS",
		Bias:       models.BiasBullish,
		Confidence: 1.5,
		SourceURL:  "https://news.example.com/x",
	}
	if err := tooHigh.Validate(); err == nil {
		t.Fatal("expected error for confidence > 1")
	}
}

func TestSentimentScore_Validate_MissingSourceURL(t *testing.T) {
	score := models.SentimentScore{
		Ticker:     "TCS",
		Bias:       models.BiasBearish,
		Confidence: 0.6,
	}
	if err := score.Validate(); err == nil {
		t.Fatal("expected error for empty source_url")
	}
}

func TestSentimentScore_JSONRoundTrip(t *testing.T) {
	now := time.Date(2026, 8, 9, 10, 30, 0, 0, time.UTC)
	score := models.SentimentScore{
		ID:          7,
		Ticker:      "SBIN",
		Bias:        models.BiasBearish,
		Confidence:  0.72,
		Summary:     "NPA concerns weigh on the stock",
		Source:      "google-news",
		SourceURL:   "https://news.example.com/sbin/9",
		RawResponse: `{"bias":"BEARISH","confidence":0.72,"summary":"NPA concerns weigh on the stock"}`,
		RecordedAt:  now,
	}

	data, err := json.Marshal(&score)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded models.SentimentScore
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Ticker != score.Ticker {
		t.Fatalf("Ticker = %q, want %q", decoded.Ticker, score.Ticker)
	}
	if decoded.Bias != score.Bias {
		t.Fatalf("Bias = %q, want %q", decoded.Bias, score.Bias)
	}
	if decoded.Confidence != score.Confidence {
		t.Fatalf("Confidence = %f, want %f", decoded.Confidence, score.Confidence)
	}
	if decoded.SourceURL != score.SourceURL {
		t.Fatalf("SourceURL = %q, want %q", decoded.SourceURL, score.SourceURL)
	}
	if !decoded.RecordedAt.Equal(score.RecordedAt) {
		t.Fatalf("RecordedAt = %v, want %v", decoded.RecordedAt, score.RecordedAt)
	}
}

func TestArticle_JSONRoundTrip(t *testing.T) {
	now := time.Date(2026, 8, 9, 9, 0, 0, 0, time.UTC)
	article := models.Article{
		Ticker:      "INFY",
		Title:       "Infosys wins large deal",
		URL:         "https://news.example.com/infy/1",
		Source:      "google-news",
		Description: "Infosys announced a new deal.",
		PublishedAt: now,
	}

	data, err := json.Marshal(&article)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded models.Article
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Title != article.Title {
		t.Fatalf("Title = %q, want %q", decoded.Title, article.Title)
	}
	if decoded.URL != article.URL {
		t.Fatalf("URL = %q, want %q", decoded.URL, article.URL)
	}
	if !decoded.PublishedAt.Equal(article.PublishedAt) {
		t.Fatalf("PublishedAt = %v, want %v", decoded.PublishedAt, article.PublishedAt)
	}
}
