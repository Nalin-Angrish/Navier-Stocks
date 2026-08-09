package sentiment_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Nalin-Angrish/Navier-Stocks/src/analysts/sentiment"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
)

const rssFixture = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Google News</title>
    <item>
      <title>Reliance posts strong quarterly results</title>
      <link>https://news.google.com/rss/articles/ABC</link>
      <pubDate>Sun, 09 Aug 2026 09:00:00 GMT</pubDate>
      <source url="https://example.com">Example News</source>
      <description>&lt;a href="https://example.com/rel/1"&gt;Reliance beat estimates&lt;/a&gt;</description>
    </item>
    <item>
      <title>Reliance expands retail footprint</title>
      <link>https://news.google.com/rss/articles/DEF</link>
      <pubDate>Mon, 10 Aug 2026 05:30:00 +0530</pubDate>
      <source url="https://example2.com">Example2 News</source>
      <description>Second story description</description>
    </item>
  </channel>
</rss>`

func TestGoogleNewsRSS_Fetch_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "q=RELIANCE+stock+news") {
			t.Fatalf("missing ticker query, got %q", r.URL.RawQuery)
		}
		if !strings.Contains(r.URL.RawQuery, "hl=en-IN") || !strings.Contains(r.URL.RawQuery, "gl=IN") {
			t.Fatalf("missing India params, got %q", r.URL.RawQuery)
		}
		if ua := r.Header.Get("User-Agent"); ua == "" {
			t.Fatal("expected a User-Agent header")
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(rssFixture))
	}))
	defer srv.Close()

	src := sentiment.NewGoogleNewsRSS(srv.URL, 0, nil)
	articles, err := src.Fetch(context.Background(), "RELIANCE")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if len(articles) != 2 {
		t.Fatalf("got %d articles, want 2", len(articles))
	}

	first := articles[0]
	if first.Ticker != "RELIANCE" {
		t.Fatalf("Ticker = %q, want RELIANCE", first.Ticker)
	}
	if first.Title != "Reliance posts strong quarterly results" {
		t.Fatalf("Title = %q", first.Title)
	}
	if first.URL != "https://news.google.com/rss/articles/ABC" {
		t.Fatalf("URL = %q", first.URL)
	}
	if first.Source != "Example News" {
		t.Fatalf("Source = %q, want Example News", first.Source)
	}
	if first.Description != "Reliance beat estimates" {
		t.Fatalf("Description = %q, want stripped HTML", first.Description)
	}
	wantPub := time.Date(2026, 8, 9, 9, 0, 0, 0, time.UTC)
	if !first.PublishedAt.Equal(wantPub) {
		t.Fatalf("PublishedAt = %v, want %v", first.PublishedAt, wantPub)
	}

	second := articles[1]
	if second.Title != "Reliance expands retail footprint" {
		t.Fatalf("Title = %q", second.Title)
	}
	if !second.PublishedAt.Equal(time.Date(2026, 8, 10, 5, 30, 0, 0, time.FixedZone("IST", 5*3600+30*60))) {
		t.Fatalf("PublishedAt = %v", second.PublishedAt)
	}
}

func TestGoogleNewsRSS_Fetch_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	src := sentiment.NewGoogleNewsRSS(srv.URL, 0, nil)
	_, err := src.Fetch(context.Background(), "TCS")
	if err == nil {
		t.Fatal("expected error for HTTP 502")
	}
	if !strings.Contains(err.Error(), "502") {
		t.Fatalf("error = %v, want status 502", err)
	}
}

func TestGoogleNewsRSS_Fetch_MalformedXML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<rss><channel>oops"))
	}))
	defer srv.Close()

	src := sentiment.NewGoogleNewsRSS(srv.URL, 0, nil)
	_, err := src.Fetch(context.Background(), "TCS")
	if err == nil {
		t.Fatal("expected error for malformed XML")
	}
}

func TestGoogleNewsRSS_Fetch_NetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	src := sentiment.NewGoogleNewsRSS(srv.URL, 0, nil)
	srv.Close()

	_, err := src.Fetch(context.Background(), "TCS")
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
}

func TestGoogleNewsRSS_DefaultBaseURL(t *testing.T) {
	src := sentiment.NewGoogleNewsRSS("", 0, nil)
	if src == nil {
		t.Fatal("NewGoogleNewsRSS returned nil")
	}
}

// mockNewsSource returns a canned article list, optionally failing.
type mockNewsSource struct {
	mu       sync.Mutex
	articles []models.Article
	fail     bool
}

func (m *mockNewsSource) Fetch(_ context.Context, _ string) ([]models.Article, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail {
		return nil, fmt.Errorf("source unavailable")
	}
	out := make([]models.Article, len(m.articles))
	copy(out, m.articles)
	return out, nil
}

func fixedArticles() []models.Article {
	return []models.Article{
		{Ticker: "RELIANCE", Title: "Story one", URL: "https://example.com/1"},
		{Ticker: "RELIANCE", Title: "Story two", URL: "https://example.com/2"},
	}
}

func TestScraper_FetchFresh_Dedup(t *testing.T) {
	mock := &mockNewsSource{articles: fixedArticles()}
	scraper := sentiment.NewScraper(mock, time.Hour)

	first, err := scraper.FetchFresh(context.Background(), "RELIANCE")
	if err != nil {
		t.Fatalf("first FetchFresh: %v", err)
	}
	if len(first) != 2 {
		t.Fatalf("first fetch got %d articles, want 2", len(first))
	}

	second, err := scraper.FetchFresh(context.Background(), "RELIANCE")
	if err != nil {
		t.Fatalf("second FetchFresh: %v", err)
	}
	if len(second) != 0 {
		t.Fatalf("second fetch got %d articles, want 0 (deduped)", len(second))
	}
}

func TestScraper_FetchFresh_ExpiredTTL(t *testing.T) {
	mock := &mockNewsSource{articles: fixedArticles()}
	scraper := sentiment.NewScraper(mock, 20*time.Millisecond)

	if _, err := scraper.FetchFresh(context.Background(), "RELIANCE"); err != nil {
		t.Fatalf("first FetchFresh: %v", err)
	}

	time.Sleep(60 * time.Millisecond)

	second, err := scraper.FetchFresh(context.Background(), "RELIANCE")
	if err != nil {
		t.Fatalf("second FetchFresh: %v", err)
	}
	if len(second) != 2 {
		t.Fatalf("after TTL expiry got %d articles, want 2", len(second))
	}
}

func TestScraper_FetchFresh_DedupByTitleWhenNoURL(t *testing.T) {
	mock := &mockNewsSource{
		articles: []models.Article{
			{Ticker: "TCS", Title: "Same story", URL: ""},
			{Ticker: "TCS", Title: "Same story", URL: ""},
		},
	}
	scraper := sentiment.NewScraper(mock, time.Hour)

	fresh, err := scraper.FetchFresh(context.Background(), "TCS")
	if err != nil {
		t.Fatalf("FetchFresh: %v", err)
	}
	// Both rows share the same title hash; only the first should surface.
	if len(fresh) != 1 {
		t.Fatalf("got %d fresh articles, want 1", len(fresh))
	}
}

func TestScraper_FetchFresh_SourceErrorPropagates(t *testing.T) {
	mock := &mockNewsSource{fail: true}
	scraper := sentiment.NewScraper(mock, time.Hour)

	_, err := scraper.FetchFresh(context.Background(), "RELIANCE")
	if err == nil {
		t.Fatal("expected error from failing source")
	}
}

func TestScheduler_TicksAndStops(t *testing.T) {
	sched := sentiment.NewScheduler(10 * time.Millisecond)
	ticks := sched.Start()
	defer sched.Stop()

	select {
	case <-ticks:
	case <-time.After(time.Second):
		t.Fatal("expected a tick within 1s")
	}

	done := make(chan struct{})
	go func() {
		sched.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Stop did not complete within 1s")
	}
}

func TestScheduler_StopBeforeStart(t *testing.T) {
	sched := sentiment.NewScheduler(time.Minute)
	sched.Stop()
}
