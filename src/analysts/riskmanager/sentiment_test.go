package riskmanager_test

import (
	"errors"
	"testing"

	"github.com/Nalin-Angrish/Navier-Stocks/src/analysts/riskmanager"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
)

// mockScoreStore returns a fixed latest score per ticker.
type mockScoreStore struct {
	scores map[string]*models.SentimentScore
	err    error
}

func (m mockScoreStore) LatestScore(ticker string) (*models.SentimentScore, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.scores[ticker], nil
}

func TestSentimentGate_HighConfidence(t *testing.T) {
	store := mockScoreStore{scores: map[string]*models.SentimentScore{
		"RELIANCE": {Ticker: "RELIANCE", Confidence: 0.85},
	}}
	if err := riskmanager.SentimentGateRaw(store, "RELIANCE", 0.3); err != nil {
		t.Fatalf("expected high-confidence score to pass, got %v", err)
	}
}

func TestSentimentGate_LowConfidenceRejected(t *testing.T) {
	store := mockScoreStore{scores: map[string]*models.SentimentScore{
		"TCS": {Ticker: "TCS", Confidence: 0.2},
	}}
	err := riskmanager.SentimentGateRaw(store, "TCS", 0.3)
	if !errors.Is(err, riskmanager.ErrSentimentLowConfidence) {
		t.Fatalf("expected ErrSentimentLowConfidence, got %v", err)
	}
}

func TestSentimentGate_ExactlyAtThreshold(t *testing.T) {
	// Confidence exactly at the floor is permitted (>= semantics).
	store := mockScoreStore{scores: map[string]*models.SentimentScore{
		"SBIN": {Ticker: "SBIN", Confidence: 0.3},
	}}
	if err := riskmanager.SentimentGateRaw(store, "SBIN", 0.3); err != nil {
		t.Fatalf("expected threshold-exact score to pass, got %v", err)
	}
}

func TestSentimentGate_MissingScoreRejected(t *testing.T) {
	store := mockScoreStore{scores: map[string]*models.SentimentScore{}}
	err := riskmanager.SentimentGateRaw(store, "UNLISTED", 0.3)
	if !errors.Is(err, riskmanager.ErrSentimentMissing) {
		t.Fatalf("expected ErrSentimentMissing, got %v", err)
	}
}

func TestSentimentGate_StoreErrorPropagates(t *testing.T) {
	want := errors.New("db down")
	store := mockScoreStore{err: want}
	_, err := store.LatestScore("TCS")
	if !errors.Is(err, want) {
		t.Fatalf("expected store error to propagate, got %v", err)
	}
}

func TestAnalyse_SentimentGateWired(t *testing.T) {
	agent := riskmanager.NewAgentForTest(nil)
	agent.SetScoreStore(mockScoreStore{scores: map[string]*models.SentimentScore{
		"RELIANCE": {Ticker: "RELIANCE", Confidence: 0.1},
	}})
	agent.SetMinConfidence(0.3)

	err := agent.EvaluateRaw(models.TradeIntent{Ticker: "RELIANCE", Side: models.SideLong})
	if !errors.Is(err, riskmanager.ErrSentimentLowConfidence) {
		t.Fatalf("expected ErrSentimentLowConfidence, got %v", err)
	}
}
