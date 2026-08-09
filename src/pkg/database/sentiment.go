package database

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
)

// SentimentStore wraps the sentiment_scores table and provides persistence
// for the Sentiment Analyst's LLM output.  It also exposes LatestScore, which
// the Risk Manager uses to retrieve the most recent score for a ticker.
type SentimentStore struct {
	db *sql.DB
}

// NewSentimentStore returns a SentimentStore backed by the given database
// connection.
func NewSentimentStore(db *sql.DB) *SentimentStore {
	return &SentimentStore{db: db}
}

// Insert writes a SentimentScore row and populates score.ID with the
// auto-generated primary key.  recorded_at is left to the database default
// (NOW()).
func (s *SentimentStore) Insert(score *models.SentimentScore) error {
	query := `
		INSERT INTO sentiment_scores (ticker, confidence, bias, summary, source, source_url, raw_response)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id`

	err := s.db.QueryRow(query,
		score.Ticker,
		score.Confidence,
		string(score.Bias),
		score.Summary,
		score.Source,
		score.SourceURL,
		score.RawResponse,
	).Scan(&score.ID)
	if err != nil {
		return fmt.Errorf("insert sentiment score: %w", err)
	}
	return nil
}

// LatestScore returns the most recent SentimentScore row for the given
// ticker (ordered by recorded_at, then id, descending).  It returns
// (nil, nil) when no score exists yet for the ticker so callers can treat
// "never scored" the same as "stale".
func (s *SentimentStore) LatestScore(ticker string) (*models.SentimentScore, error) {
	query := `
		SELECT id, ticker, confidence, bias, summary, source, source_url, raw_response, recorded_at
		FROM sentiment_scores
		WHERE ticker = $1
		ORDER BY recorded_at DESC, id DESC
		LIMIT 1`

	var score models.SentimentScore
	err := s.db.QueryRow(query, ticker).Scan(
		&score.ID,
		&score.Ticker,
		&score.Confidence,
		&score.Bias,
		&score.Summary,
		&score.Source,
		&score.SourceURL,
		&score.RawResponse,
		&score.RecordedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("latest sentiment score: %w", err)
	}
	return &score, nil
}
