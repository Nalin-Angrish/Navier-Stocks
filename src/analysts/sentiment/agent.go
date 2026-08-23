package sentiment

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Nalin-Angrish/Navier-Stocks/src/analysts/quantitative"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/database"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/nats"
)

// ScoreStore is the persistence surface the Sentiment Analyst needs from the
// database, kept as an interface so tests can inject mocks.
type ScoreStore interface {
	Insert(score *models.SentimentScore) error
	LatestScore(ticker string) (*models.SentimentScore, error)
}

// NewsScraper abstracts the deduplicating news fetcher used by the agent.
type NewsScraper interface {
	FetchFresh(ctx context.Context, ticker string) ([]models.Article, error)
}

// Analyst is the Sentiment Analyst agent (the "slow path").  On a configurable
// cadence it scrapes fresh news for every watched ticker, scores new articles
// through an LLM, and persists the resulting SentimentScore rows.  A staleness
// guard skips the LLM whenever a score already exists within the staleness
// window, and each new score is announced on the sentiment stream.
type Analyst struct {
	js       *nats.JetStream
	scraper  NewsScraper
	llm      LLMClient
	store    ScoreStore
	db       *sql.DB
	universe []string

	interval  time.Duration // cadence of scrape-and-score cycles
	staleness time.Duration // skip LLM while the latest score is younger than this

	stopChan chan struct{}
	ticks    <-chan time.Time // injectable in tests; nil in production
}

// NewAgent creates a fully-wired Sentiment Analyst: NATS JetStream, PostgreSQL,
// an LLM client from environment configuration, a Google News RSS scraper, and
// the resolved ticker universe.  The caller must call Stop() to release
// resources.
func NewAgent() (*Analyst, error) {
	js, err := nats.ConnectJetStream()
	if err != nil {
		return nil, fmt.Errorf("sentiment: nats: %w", err)
	}
	db, err := database.Connect()
	if err != nil {
		js.Close()
		return nil, fmt.Errorf("sentiment: db: %w", err)
	}

	llm, err := NewLLMClient(LLMConfigFromEnv())
	if err != nil {
		js.Close()
		_ = db.Close()
		return nil, fmt.Errorf("sentiment: llm: %w", err)
	}

	scraper := NewScraper(
		NewGoogleNewsRSS(DefaultGoogleNewsURL, 2*time.Second, nil),
		24*time.Hour,
	)

	return newAnalyst(
		js,
		scraper,
		llm,
		database.NewSentimentStore(db),
		db,
		resolveUniverse(),
		parseDurationEnv("SENTIMENT_INTERVAL", 15*time.Minute),
		parseDurationEnv("SENTIMENT_STALENESS", 60*time.Minute),
	), nil
}

// newAnalyst is the shared constructor used by NewAgent and NewAgentForTest.
func newAnalyst(js *nats.JetStream, scraper NewsScraper, llm LLMClient, store ScoreStore, db *sql.DB, universe []string, interval, staleness time.Duration) *Analyst {
	return &Analyst{
		js:        js,
		scraper:   scraper,
		llm:       llm,
		store:     store,
		db:        db,
		universe:  universe,
		interval:  interval,
		staleness: staleness,
		stopChan:  make(chan struct{}),
	}
}

// NewAgentForTest returns an Analyst wired to the supplied dependencies
// without opening any connections, so tests can inject mocks.
func NewAgentForTest(js *nats.JetStream, scraper NewsScraper, llm LLMClient, store ScoreStore, universe []string, interval, staleness time.Duration) *Analyst {
	return newAnalyst(js, scraper, llm, store, nil, universe, interval, staleness)
}

// Run ensures the sentiment stream exists and then loops over scheduler ticks,
// processing the full universe each cycle, until Stop() is called.
func (g *Analyst) Run() {
	if g.js != nil {
		if err := g.js.EnsureStream(nats.StreamSentiment); err != nil {
			log.Printf("[Sentiment] Stream ensure failed: %v", err)
			return
		}
	}

	var sched *Scheduler
	ticks := g.ticks
	if ticks == nil {
		sched = NewScheduler(g.interval)
		ticks = sched.Start()
		defer sched.Stop()
	}

	log.Printf("[Sentiment] Ready: interval=%s staleness=%s universe=%v",
		g.interval, g.staleness, g.universe)

	ctx := context.Background()
	for {
		select {
		case <-ticks:
			g.runCycle(ctx)
		case <-g.stopChan:
			return
		}
	}
}

// Stop signals the agent to shut down: it closes the stop channel (unblocking
// Run()), closes NATS, and closes the database handle if present.
func (g *Analyst) Stop() {
	select {
	case <-g.stopChan:
	default:
		close(g.stopChan)
	}
	if g.js != nil {
		g.js.Close()
	}
	if g.db != nil {
		if err := g.db.Close(); err != nil {
			log.Printf("[Sentiment] Error closing database: %v", err)
		}
	}
}

// runCycle processes every ticker in the universe once.
func (g *Analyst) runCycle(ctx context.Context) {
	for _, ticker := range g.universe {
		if err := g.processTicker(ctx, ticker); err != nil {
			log.Printf("[Sentiment] %s: %v", ticker, err)
		}
	}
}

// processTicker is the per-ticker pipeline: staleness guard → scrape fresh
// news → score each new article → persist and announce the score.  LLM and
// persistence failures are logged and skipped so one bad article does not
// stall the rest of the cycle.
func (g *Analyst) processTicker(ctx context.Context, ticker string) error {
	fresh, err := g.hasFreshScore(ticker)
	if err != nil {
		return fmt.Errorf("staleness check: %w", err)
	}
	if fresh {
		log.Printf("[Sentiment] %s: fresh score, skipping LLM", ticker)
		return nil
	}

	articles, err := g.scraper.FetchFresh(ctx, ticker)
	if err != nil {
		return fmt.Errorf("scrape: %w", err)
	}
	if len(articles) == 0 {
		log.Printf("[Sentiment] %s: no fresh articles", ticker)
		return nil
	}

	for _, article := range articles {
		score, err := g.llm.Score(ctx, article)
		if err != nil {
			log.Printf("[Sentiment] %s: LLM error: %v", ticker, err)
			continue
		}

		// The ticker being processed is authoritative regardless of what the
		// LLM implementation attached to the score.
		score.Ticker = ticker
		score.RecordedAt = time.Now()
		if err := g.store.Insert(score); err != nil {
			log.Printf("[Sentiment] %s: persist error: %v", ticker, err)
			continue
		}

		g.publishUpdate(score)
	}
	return nil
}

// hasFreshScore reports whether a SentimentScore written within the staleness
// window already exists for the ticker, in which case the LLM is skipped.
func (g *Analyst) hasFreshScore(ticker string) (bool, error) {
	latest, err := g.store.LatestScore(ticker)
	if err != nil {
		return false, err
	}
	if latest == nil {
		return false, nil
	}
	return time.Since(latest.RecordedAt) < g.staleness, nil
}

// publishUpdate announces a newly persisted score on the sentiment stream so
// downstream consumers (e.g. a future dashboard) can react without polling.
func (g *Analyst) publishUpdate(score *models.SentimentScore) {
	if g.js == nil {
		return
	}
	data, err := json.Marshal(score)
	if err != nil {
		log.Printf("[Sentiment] publish marshal error: %v", err)
		return
	}
	if err := g.js.Publish(nats.SubjectSentimentUpd, data); err != nil {
		log.Printf("[Sentiment] publish update error: %v", err)
	}
}

// parseDurationEnv reads a Go duration from the environment, falling back to
// the provided default on absence or parse failure.
func parseDurationEnv(key string, fallback time.Duration) time.Duration {
	if raw := os.Getenv(key); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil {
			return d
		}
		log.Printf("[Sentiment] invalid %s=%q, using %v", key, raw, fallback)
	}
	return fallback
}

// resolveUniverse returns the tickers the Sentiment Analyst watches: a
// SENTIMENT_UNIVERSE override (comma-separated) if set, otherwise the same
// resolved universe as the Quantitative Scout so both agents agree on scope.
func resolveUniverse() []string {
	raw := os.Getenv("SENTIMENT_UNIVERSE")
	if raw == "" {
		return quantitative.ResolveUniverse().Symbols
	}

	var symbols []string
	for _, part := range strings.Split(raw, ",") {
		if part = strings.TrimSpace(part); part != "" {
			symbols = append(symbols, part)
		}
	}
	if len(symbols) == 0 {
		return quantitative.ResolveUniverse().Symbols
	}
	return symbols
}
