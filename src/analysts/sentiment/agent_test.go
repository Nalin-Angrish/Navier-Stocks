package sentiment

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	natsserver "github.com/nats-io/nats-server/v2/test"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/nats"
)

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

type mockScoreStore struct {
	mu        sync.Mutex
	inserted  []*models.SentimentScore
	latest    *models.SentimentScore
	latestErr error
	insertErr error
}

func (m *mockScoreStore) Insert(s *models.SentimentScore) error {
	if m.insertErr != nil {
		return m.insertErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inserted = append(m.inserted, s)
	return nil
}

func (m *mockScoreStore) LatestScore(_ string) (*models.SentimentScore, error) {
	if m.latestErr != nil {
		return nil, m.latestErr
	}
	return m.latest, nil
}

func (m *mockScoreStore) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.inserted)
}

var errLLMDown = fmt.Errorf("llm down")

type mockLLM struct {
	mu        sync.Mutex
	called    int
	failFirst int
	score     *models.SentimentScore
}

func (m *mockLLM) Score(_ context.Context, _ models.Article) (*models.SentimentScore, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.called++
	if m.failFirst > 0 {
		m.failFirst--
		return nil, errLLMDown
	}
	if m.score == nil {
		return nil, errLLMDown
	}
	return m.score, nil
}

func (m *mockLLM) calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.called
}

type mockScraper struct {
	articles []models.Article
	err      error
}

func (m *mockScraper) FetchFresh(_ context.Context, _ string) ([]models.Article, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.articles, nil
}

func validScore() *models.SentimentScore {
	return &models.SentimentScore{
		Bias:       models.BiasBullish,
		Confidence: 0.8,
		Summary:    "Strong results",
		Source:     "google-news",
		SourceURL:  "https://example.com/1",
	}
}

func newTestAnalyst(js *nats.JetStream, scraper NewsScraper, llm LLMClient, store ScoreStore) *Analyst {
	return newAnalyst(js, scraper, llm, store, nil, []string{"RELIANCE"}, time.Minute, time.Minute)
}

// ---------------------------------------------------------------------------
// NATS test helpers
// ---------------------------------------------------------------------------

func startJetStreamServer(t *testing.T) *server.Server {
	t.Helper()
	opts := &server.Options{
		Port:      -1,
		JetStream: true,
		StoreDir:  t.TempDir(),
	}
	s := natsserver.RunServer(opts)
	t.Cleanup(func() { s.Shutdown() })
	return s
}

func connectJS(t *testing.T, s *server.Server) *nats.JetStream {
	t.Helper()
	t.Setenv("NATS_URL", s.ClientURL())
	js, err := nats.ConnectJetStream()
	if err != nil {
		t.Fatalf("ConnectJetStream(): %v", err)
	}
	t.Cleanup(func() { js.Close() })
	return js
}

func ensureStream(t *testing.T, js *nats.JetStream, name string, subjects ...string) {
	t.Helper()
	if err := js.EnsureStream(nats.StreamConfig{Name: name, Subjects: subjects}); err != nil {
		t.Fatalf("EnsureStream(%q): %v", name, err)
	}
}

// ---------------------------------------------------------------------------
// NewAgent
// ---------------------------------------------------------------------------

func TestNewAgent_NATSConnectFailure(t *testing.T) {
	t.Setenv("NATS_URL", "nats://localhost:1")
	_, err := NewAgent()
	if err == nil {
		t.Fatal("expected error from NewAgent with invalid NATS URL")
	}
}

// ---------------------------------------------------------------------------
// processTicker
// ---------------------------------------------------------------------------

func TestProcessTicker_FreshScoreSkipsLLM(t *testing.T) {
	store := &mockScoreStore{latest: &models.SentimentScore{RecordedAt: time.Now()}}
	llm := &mockLLM{score: validScore()}
	a := newTestAnalyst(nil, &mockScraper{}, llm, store)

	if err := a.processTicker(context.Background(), "RELIANCE"); err != nil {
		t.Fatalf("processTicker: %v", err)
	}
	if llm.calls() != 0 {
		t.Fatalf("LLM called %d times, want 0", llm.calls())
	}
	if store.count() != 0 {
		t.Fatalf("store got %d inserts, want 0", store.count())
	}
}

func TestProcessTicker_StaleScoreStillProcesses(t *testing.T) {
	store := &mockScoreStore{latest: &models.SentimentScore{RecordedAt: time.Now().Add(-2 * time.Hour)}}
	llm := &mockLLM{score: validScore()}
	scraper := &mockScraper{articles: []models.Article{
		{Ticker: "RELIANCE", Title: "A", URL: "https://example.com/1"},
		{Ticker: "RELIANCE", Title: "B", URL: "https://example.com/2"},
	}}
	a := newTestAnalyst(nil, scraper, llm, store)

	if err := a.processTicker(context.Background(), "RELIANCE"); err != nil {
		t.Fatalf("processTicker: %v", err)
	}
	if llm.calls() != 2 {
		t.Fatalf("LLM called %d times, want 2", llm.calls())
	}
	if store.count() != 2 {
		t.Fatalf("store got %d inserts, want 2", store.count())
	}
}

func TestProcessTicker_NoFreshArticlesSkipsLLM(t *testing.T) {
	store := &mockScoreStore{}
	llm := &mockLLM{score: validScore()}
	a := newTestAnalyst(nil, &mockScraper{}, llm, store)

	if err := a.processTicker(context.Background(), "RELIANCE"); err != nil {
		t.Fatalf("processTicker: %v", err)
	}
	if llm.calls() != 0 {
		t.Fatalf("LLM called %d times, want 0", llm.calls())
	}
}

func TestProcessTicker_SetsTickerOnScore(t *testing.T) {
	store := &mockScoreStore{}
	llm := &mockLLM{score: validScore()}
	scraper := &mockScraper{articles: []models.Article{
		{Ticker: "RELIANCE", Title: "A", URL: "https://example.com/1"},
	}}
	a := newTestAnalyst(nil, scraper, llm, store)

	if err := a.processTicker(context.Background(), "RELIANCE"); err != nil {
		t.Fatalf("processTicker: %v", err)
	}
	store.mu.Lock()
	inserted := store.inserted[0]
	store.mu.Unlock()
	if inserted.Ticker != "RELIANCE" {
		t.Fatalf("inserted ticker = %q, want RELIANCE", inserted.Ticker)
	}
	if inserted.SourceURL != "https://example.com/1" {
		t.Fatalf("inserted source_url = %q, want article URL", inserted.SourceURL)
	}
}

func TestProcessTicker_LatestScoreError(t *testing.T) {
	store := &mockScoreStore{latestErr: fmt.Errorf("db down")}
	llm := &mockLLM{score: validScore()}
	a := newTestAnalyst(nil, &mockScraper{}, llm, store)

	err := a.processTicker(context.Background(), "RELIANCE")
	if err == nil {
		t.Fatal("expected error from processTicker")
	}
	if !strings.Contains(err.Error(), "staleness check") {
		t.Fatalf("error = %v, want staleness-check context", err)
	}
	if llm.calls() != 0 {
		t.Fatalf("LLM called %d times, want 0", llm.calls())
	}
}

func TestProcessTicker_ScrapeError(t *testing.T) {
	store := &mockScoreStore{}
	llm := &mockLLM{score: validScore()}
	scraper := &mockScraper{err: fmt.Errorf("feed down")}
	a := newTestAnalyst(nil, scraper, llm, store)

	err := a.processTicker(context.Background(), "RELIANCE")
	if err == nil {
		t.Fatal("expected error from processTicker")
	}
	if llm.calls() != 0 {
		t.Fatalf("LLM called %d times, want 0", llm.calls())
	}
}

func TestProcessTicker_LLMErrorContinues(t *testing.T) {
	store := &mockScoreStore{}
	llm := &mockLLM{score: validScore(), failFirst: 1}
	scraper := &mockScraper{articles: []models.Article{
		{Ticker: "RELIANCE", Title: "A", URL: "https://example.com/1"},
		{Ticker: "RELIANCE", Title: "B", URL: "https://example.com/2"},
	}}
	a := newTestAnalyst(nil, scraper, llm, store)

	if err := a.processTicker(context.Background(), "RELIANCE"); err != nil {
		t.Fatalf("processTicker: %v", err)
	}
	if llm.calls() != 2 {
		t.Fatalf("LLM called %d times, want 2", llm.calls())
	}
	if store.count() != 1 {
		t.Fatalf("store got %d inserts, want 1 (failed article skipped)", store.count())
	}
}

func TestProcessTicker_PersistErrorContinues(t *testing.T) {
	store := &mockScoreStore{insertErr: fmt.Errorf("db down")}
	llm := &mockLLM{score: validScore()}
	scraper := &mockScraper{articles: []models.Article{
		{Ticker: "RELIANCE", Title: "A", URL: "https://example.com/1"},
	}}
	a := newTestAnalyst(nil, scraper, llm, store)

	if err := a.processTicker(context.Background(), "RELIANCE"); err != nil {
		t.Fatalf("processTicker: %v", err)
	}
	if store.count() != 0 {
		t.Fatalf("store got %d inserts, want 0", store.count())
	}
}

func TestProcessTicker_PublishesUpdate(t *testing.T) {
	s := startJetStreamServer(t)
	js := connectJS(t, s)
	ensureStream(t, js, "sentiment", "sentiment.>")

	received := make(chan *models.SentimentScore, 1)
	sub, err := js.Subscribe("sentiment.update", func(m *nats.Msg) {
		var score models.SentimentScore
		if err := json.Unmarshal(m.Data, &score); err == nil {
			received <- &score
		}
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	store := &mockScoreStore{}
	llm := &mockLLM{score: validScore()}
	scraper := &mockScraper{articles: []models.Article{
		{Ticker: "RELIANCE", Title: "A", URL: "https://example.com/1"},
	}}
	a := newTestAnalyst(js, scraper, llm, store)

	if err := a.processTicker(context.Background(), "RELIANCE"); err != nil {
		t.Fatalf("processTicker: %v", err)
	}

	select {
	case score := <-received:
		if score.Ticker != "RELIANCE" {
			t.Fatalf("published ticker = %q, want RELIANCE", score.Ticker)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for sentiment.update")
	}
}

// ---------------------------------------------------------------------------
// Run
// ---------------------------------------------------------------------------

func TestRun_ProcessesOnTick(t *testing.T) {
	store := &mockScoreStore{}
	llm := &mockLLM{score: validScore()}
	scraper := &mockScraper{articles: []models.Article{
		{Ticker: "RELIANCE", Title: "A", URL: "https://example.com/1"},
	}}
	a := newAnalyst(nil, scraper, llm, store, nil, []string{"RELIANCE"}, time.Minute, time.Minute)

	ticks := make(chan time.Time, 1)
	a.ticks = ticks

	go a.Run()
	t.Cleanup(a.Stop)

	ticks <- time.Now()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if llm.calls() >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if llm.calls() != 1 {
		t.Fatalf("LLM called %d times, want 1", llm.calls())
	}
}

func TestRun_Stops(t *testing.T) {
	a := newAnalyst(nil, &mockScraper{}, &mockLLM{}, &mockScoreStore{}, nil, []string{"RELIANCE"}, time.Millisecond, time.Minute)

	done := make(chan struct{})
	go func() {
		a.Run()
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	a.Stop()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit within 2s of Stop")
	}
}

func TestRun_EnsureStreamFailure(t *testing.T) {
	srvOpts := &server.Options{
		Port:      -1,
		JetStream: false,
	}
	s := natsserver.RunServer(srvOpts)
	t.Cleanup(func() { s.Shutdown() })

	t.Setenv("NATS_URL", s.ClientURL())
	js, err := nats.ConnectJetStream()
	if err != nil {
		t.Skipf("ConnectJetStream: %v", err)
	}
	t.Cleanup(func() { js.Close() })

	a := newAnalyst(js, &mockScraper{}, &mockLLM{}, &mockScoreStore{}, nil, []string{"RELIANCE"}, time.Minute, time.Minute)
	a.Run()
}

// ---------------------------------------------------------------------------
// Universe resolution
// ---------------------------------------------------------------------------

func TestResolveUniverse_Custom(t *testing.T) {
	t.Setenv("SENTIMENT_UNIVERSE", " TCS, INFY ,,")

	u := resolveUniverse()
	if len(u) != 2 || u[0] != "TCS" || u[1] != "INFY" {
		t.Fatalf("universe = %v, want [TCS INFY]", u)
	}
}

func TestResolveUniverse_DefaultFallsBackToScout(t *testing.T) {
	t.Setenv("SENTIMENT_UNIVERSE", "")

	u := resolveUniverse()
	if len(u) != 25 {
		t.Fatalf("default universe got %d symbols, want 25", len(u))
	}
}

func TestResolveUniverse_AllWhitespaceFallsBack(t *testing.T) {
	t.Setenv("SENTIMENT_UNIVERSE", "  , ,  ")

	u := resolveUniverse()
	if len(u) != 25 {
		t.Fatalf("all-whitespace universe got %d symbols, want 25", len(u))
	}
}
