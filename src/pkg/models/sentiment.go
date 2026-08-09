package models

import (
	"errors"
	"time"
)

// SentimentBias constrains the directional verdict produced by the Sentiment
// Analyst's LLM inference for a given news article.
type SentimentBias string

const (
	BiasBullish SentimentBias = "BULLISH"
	BiasBearish SentimentBias = "BEARISH"
	BiasNeutral SentimentBias = "NEUTRAL"
)

// SentimentScore is the persisted output of the Sentiment Analyst's LLM
// pipeline.  One row is written per scored article and stored in the
// sentiment_scores table.  The Risk Manager reads the most recent row per
// ticker (via SentimentStore.LatestScore) when evaluating an intent.
//
// SourceURL and RawResponse form the citation metadata: the originating
// article URL and the exact LLM output that produced the verdict.
type SentimentScore struct {
	ID          int64         `json:"id"`
	Ticker      string        `json:"ticker"`
	Bias        SentimentBias `json:"bias"`
	Confidence  float64       `json:"confidence"`
	Summary     string        `json:"summary,omitempty"`
	Source      string        `json:"source,omitempty"`
	SourceURL   string        `json:"source_url,omitempty"`
	RawResponse string        `json:"raw_response,omitempty"`
	RecordedAt  time.Time     `json:"recorded_at,omitempty"`
}

// Validate checks that a SentimentScore is well-formed enough to persist:
// a ticker, a valid bias, a confidence inside [0,1], and a citation URL.
func (s *SentimentScore) Validate() error {
	if s.Ticker == "" {
		return errors.New("ticker is required")
	}
	if s.Bias != BiasBullish && s.Bias != BiasBearish && s.Bias != BiasNeutral {
		return errors.New("bias must be BULLISH, BEARISH or NEUTRAL")
	}
	if s.Confidence < 0 || s.Confidence > 1 {
		return errors.New("confidence must be within [0,1]")
	}
	if s.SourceURL == "" {
		return errors.New("source_url is required")
	}
	return nil
}

// Article is a single news item fetched by the news-scraper framework and
// handed to the LLM client for scoring.  The Ticker field records which
// security the article was fetched for so the pipeline can attribute the
// resulting score.
type Article struct {
	Ticker      string    `json:"ticker"`
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	Source      string    `json:"source"`
	Description string    `json:"description,omitempty"`
	PublishedAt time.Time `json:"published_at,omitempty"`
}
