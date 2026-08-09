package database_test

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/database"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
)

func TestNewSentimentStore(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()

	store := database.NewSentimentStore(db)
	if store == nil {
		t.Fatal("NewSentimentStore returned nil")
	}
}

func TestInsertSentimentScore_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()

	store := database.NewSentimentStore(db)

	mock.ExpectQuery(`INSERT INTO sentiment_scores`).
		WithArgs("RELIANCE", 0.85, "BULLISH", "Strong results", "google-news",
			"https://news.example.com/rel/1", `{"bias":"BULLISH","confidence":0.85}`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(11))

	score := &models.SentimentScore{
		Ticker:      "RELIANCE",
		Bias:        models.BiasBullish,
		Confidence:  0.85,
		Summary:     "Strong results",
		Source:      "google-news",
		SourceURL:   "https://news.example.com/rel/1",
		RawResponse: `{"bias":"BULLISH","confidence":0.85}`,
	}

	if err := store.Insert(score); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if score.ID != 11 {
		t.Fatalf("expected ID=11, got %d", score.ID)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestInsertSentimentScore_DBError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()

	store := database.NewSentimentStore(db)

	mock.ExpectQuery(`INSERT INTO sentiment_scores`).
		WithArgs("TCS", 0.4, "NEUTRAL", "", "", "https://news.example.com/tcs/1", "").
		WillReturnError(errStoreFailure)

	score := &models.SentimentScore{
		Ticker:     "TCS",
		Bias:       models.BiasNeutral,
		Confidence: 0.4,
		SourceURL:  "https://news.example.com/tcs/1",
	}

	err = store.Insert(score)
	if err == nil {
		t.Fatal("expected error from Insert")
	}
	if err.Error() != "insert sentiment score: store unavailable" {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestLatestScore_Found(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()

	store := database.NewSentimentStore(db)

	recorded := time.Date(2026, 8, 9, 10, 30, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{
		"id", "ticker", "confidence", "bias", "summary", "source", "source_url", "raw_response", "recorded_at",
	}).AddRow(5, "SBIN", 0.72, "BEARISH", "NPA concerns", "google-news",
		"https://news.example.com/sbin/9", `{"bias":"BEARISH","confidence":0.72}`, recorded)

	mock.ExpectQuery(`SELECT id, ticker, confidence, bias`).
		WithArgs("SBIN").
		WillReturnRows(rows)

	score, err := store.LatestScore("SBIN")
	if err != nil {
		t.Fatalf("LatestScore: %v", err)
	}
	if score == nil {
		t.Fatalf("expected a score, got nil")
		return
	}
	if score.ID != 5 {
		t.Fatalf("ID = %d, want 5", score.ID)
	}
	if score.Ticker != "SBIN" {
		t.Fatalf("Ticker = %q, want SBIN", score.Ticker)
	}
	if score.Bias != models.BiasBearish {
		t.Fatalf("Bias = %q, want BEARISH", score.Bias)
	}
	if score.Confidence != 0.72 {
		t.Fatalf("Confidence = %f, want 0.72", score.Confidence)
	}
	if !score.RecordedAt.Equal(recorded) {
		t.Fatalf("RecordedAt = %v, want %v", score.RecordedAt, recorded)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestLatestScore_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()

	store := database.NewSentimentStore(db)

	mock.ExpectQuery(`SELECT id, ticker, confidence, bias`).
		WithArgs("UNLISTED").
		WillReturnError(sql.ErrNoRows)

	score, err := store.LatestScore("UNLISTED")
	if err != nil {
		t.Fatalf("LatestScore: %v", err)
	}
	if score != nil {
		t.Fatalf("expected nil score for missing ticker, got %+v", score)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestLatestScore_DBError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()

	store := database.NewSentimentStore(db)

	mock.ExpectQuery(`SELECT id, ticker, confidence, bias`).
		WithArgs("TCS").
		WillReturnError(errStoreFailure)

	_, err = store.LatestScore("TCS")
	if err == nil {
		t.Fatal("expected error from LatestScore")
	}
	if !errors.Is(err, errStoreFailure) {
		t.Fatalf("expected wrapped errStoreFailure, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
