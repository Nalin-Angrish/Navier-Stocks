package riskmanager

import (
	"errors"
	"log"
	"os"
	"strconv"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
)

// DefaultMinConfidence is the Gate floor on the latest SentimentScore
// confidence below which an intent is rejected.  Override with the
// RISK_MIN_CONFIDENCE environment variable (0.3 per the Sentinel story).
const DefaultMinConfidence = 0.3

// minConfidenceEnvKey is the environment variable that tunes the sentiment
// gate's confidence floor.
const minConfidenceEnvKey = "RISK_MIN_CONFIDENCE"

// ErrSentimentLowConfidence is returned by the sentiment gate when the latest
// score for a ticker sits below the confidence floor.
var ErrSentimentLowConfidence = errors.New("sentiment confidence below threshold")

// ErrSentimentMissing is returned when no SentimentScore exists yet for the
// ticker, so the gate cannot attest to its news-driven confidence.
var ErrSentimentMissing = errors.New("no sentiment score for ticker")

// ScoreStore is the persistence surface the Risk Manager needs from the
// Sentiment Analyst's database, kept as an interface so tests can inject
// mocks.
type ScoreStore interface {
	LatestScore(ticker string) (*models.SentimentScore, error)
}

// minConfidenceFromEnv reads the confidence floor, falling back to the
// documented default when the variable is absent or unparseable.
func minConfidenceFromEnv() float64 {
	if raw := os.Getenv(minConfidenceEnvKey); raw != "" {
		if v := parseFloat(raw); v >= 0 && v <= 1 {
			return v
		}
		log.Printf("[Risk Manager] invalid %s=%q, using %.2f", minConfidenceEnvKey, raw, DefaultMinConfidence)
	}
	return DefaultMinConfidence
}

// sentimentGate queries the latest SentimentScore for the ticker and permits
// the intent only when a score exists with confidence at or above the floor.
// A missing row is treated as a rejection: without news-driven evidence the
// gate cannot pass the intent through.
func sentimentGate(store ScoreStore, ticker string, minConfidence float64) error {
	return SentimentGateRaw(store, ticker, minConfidence)
}

// SentimentGateRaw is the exported form of the sentiment confidence gate so
// tests can exercise it without a full Analyst.  A nil store disables the
// gate (unit tests / degraded mode).
func SentimentGateRaw(store ScoreStore, ticker string, minConfidence float64) error {
	if store == nil {
		// No backing store wired (unit tests / degraded mode): skip the gate
		// rather than blocking the entire pipeline.
		return nil
	}
	score, err := store.LatestScore(ticker)
	if err != nil {
		return err
	}
	if score == nil {
		return ErrSentimentMissing
	}
	if score.Confidence < minConfidence {
		return ErrSentimentLowConfidence
	}
	return nil
}

// parseFloat is a tiny helper separating env parsing from the gate logic so
// tests do not need to touch os.Getenv.
func parseFloat(s string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return -1
	}
	return v
}
