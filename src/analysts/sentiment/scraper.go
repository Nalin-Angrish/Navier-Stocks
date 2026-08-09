package sentiment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
)

// NewsSource fetches the latest news articles for a single ticker.  Concrete
// adapters (e.g. GoogleNewsRSS) wrap one upstream feed; the Scraper adds
// cross-cycle deduplication on top.
type NewsSource interface {
	Fetch(ctx context.Context, ticker string) ([]models.Article, error)
}

// DefaultGoogleNewsURL is the Google News RSS search endpoint configured for
// the Indian market (English, India region).
const DefaultGoogleNewsURL = "https://news.google.com/rss/search"

// GoogleNewsRSS is a NewsSource backed by the Google News RSS search feed.
// Requests are spaced by a minimum interval (rate limiting) and any non-200
// response or transport failure surfaces as a descriptive error.
type GoogleNewsRSS struct {
	baseURL string
	client  *http.Client
	limiter *rateLimiter
}

// NewGoogleNewsRSS builds a Google News RSS adapter.  An empty baseURL falls
// back to DefaultGoogleNewsURL.  A minInterval <= 0 disables rate limiting
// and a nil client falls back to http.DefaultClient.
func NewGoogleNewsRSS(baseURL string, minInterval time.Duration, client *http.Client) *GoogleNewsRSS {
	if baseURL == "" {
		baseURL = DefaultGoogleNewsURL
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &GoogleNewsRSS{
		baseURL: baseURL,
		client:  client,
		limiter: newRateLimiter(minInterval),
	}
}

// Fetch queries the Google News RSS search feed for the given ticker and
// parses the returned items into Article values.  It returns an error on
// transport failures, non-200 responses, or malformed XML.
func (g *GoogleNewsRSS) Fetch(ctx context.Context, ticker string) ([]models.Article, error) {
	if err := g.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("google news: rate limit wait: %w", err)
	}

	query := url.QueryEscape(ticker + " stock news")
	endpoint := fmt.Sprintf("%s?q=%s&hl=en-IN&gl=IN&ceid=IN:en", g.baseURL, query)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("google news: build request: %w", err)
	}
	req.Header.Set("User-Agent", "Navier-Stocks-Sentiment-Analyst/1.0")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google news: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google news: http %d", resp.StatusCode)
	}

	return parseRSS(resp.Body, ticker)
}

// rssFeed / rssItem mirror the subset of the RSS 2.0 document Google News
// emits that we care about.
type rssFeed struct {
	Channel struct {
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
}

type rssItem struct {
	Title   string `xml:"title"`
	Link    string `xml:"link"`
	PubDate string `xml:"pubDate"`
	Source  string `xml:"source"`
	Desc    string `xml:"description"`
}

// parseRSS decodes an RSS feed into Article values tagged with the ticker
// they were fetched for.
func parseRSS(r io.Reader, ticker string) ([]models.Article, error) {
	var feed rssFeed
	if err := xml.NewDecoder(r).Decode(&feed); err != nil {
		return nil, fmt.Errorf("google news: parse rss: %w", err)
	}

	articles := make([]models.Article, 0, len(feed.Channel.Items))
	for _, item := range feed.Channel.Items {
		articles = append(articles, models.Article{
			Ticker:      ticker,
			Title:       strings.TrimSpace(item.Title),
			URL:         strings.TrimSpace(item.Link),
			Source:      strings.TrimSpace(item.Source),
			Description: cleanDescription(item.Desc),
			PublishedAt: parsePubDate(item.PubDate),
		})
	}
	return articles, nil
}

// cleanDescription strips HTML tags and decodes entities from the RSS
// description, which Google News emits as markup.
func cleanDescription(desc string) string {
	return strings.TrimSpace(html.UnescapeString(htmlTagRE.ReplaceAllString(desc, " ")))
}

var htmlTagRE = regexp.MustCompile(`<[^>]*>`)

// parsePubDate parses a Google News pubDate across the handful of layouts
// feeds actually emit, returning the zero time when nothing matches.
func parsePubDate(raw string) time.Time {
	for _, layout := range []string{
		time.RFC1123Z,
		time.RFC1123,
		time.RFC3339,
		"Mon, 2 Jan 2006 15:04:05 -0700",
		"Mon, 02 Jan 2006 15:04:05 MST",
	} {
		if t, err := time.Parse(layout, strings.TrimSpace(raw)); err == nil {
			return t
		}
	}
	return time.Time{}
}

// Scraper wraps a NewsSource and filters out articles whose URL/title hash
// was already seen within the dedup TTL, so repeated polling cycles do not
// re-score the same story.
type Scraper struct {
	source NewsSource
	ttl    time.Duration

	mu   sync.Mutex
	seen map[string]time.Time // hash -> first seen
}

// NewScraper builds a deduplicating scraper over the given source.
func NewScraper(source NewsSource, ttl time.Duration) *Scraper {
	return &Scraper{
		source: source,
		ttl:    ttl,
		seen:   make(map[string]time.Time),
	}
}

// FetchFresh returns only articles not seen within the dedup TTL.  Already
// seen (or still within the window) articles are dropped, and the seen set is
// pruned of expired entries to keep memory bounded.
func (s *Scraper) FetchFresh(ctx context.Context, ticker string) ([]models.Article, error) {
	articles, err := s.source.Fetch(ctx, ticker)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	fresh := make([]models.Article, 0, len(articles))
	for _, a := range articles {
		h := articleHash(&a)

		s.mu.Lock()
		firstSeen, ok := s.seen[h]
		if ok && now.Sub(firstSeen) < s.ttl {
			s.mu.Unlock()
			continue
		}
		s.seen[h] = now
		s.mu.Unlock()

		fresh = append(fresh, a)
	}

	s.prune(now)
	return fresh, nil
}

// articleHash returns a short content-addressed identifier derived from the
// article URL, falling back to its title when the URL is empty.
func articleHash(a *models.Article) string {
	key := a.URL
	if key == "" {
		key = a.Title
	}
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:8])
}

// prune removes entries whose dedup window has elapsed so the same story can
// be re-scored after the TTL and the map does not grow unbounded.
func (s *Scraper) prune(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for h, firstSeen := range s.seen {
		if now.Sub(firstSeen) >= s.ttl {
			delete(s.seen, h)
		}
	}
}

// Scheduler emits a tick every interval so the agent can run its
// scrape-and-score cycle on a configurable cadence.  It is safe to Stop once;
// Stop before Start is a no-op.
type Scheduler struct {
	interval time.Duration
	ch       chan time.Time
	stopCh   chan struct{}
	doneCh   chan struct{}
	once     sync.Once
	started  atomic.Bool
}

// NewScheduler returns a stopped scheduler; call Start to begin ticking.
// Non-positive intervals are clamped to one minute.
func NewScheduler(interval time.Duration) *Scheduler {
	if interval <= 0 {
		interval = time.Minute
	}
	return &Scheduler{
		interval: interval,
		ch:       make(chan time.Time, 1),
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
}

// Start launches the ticker goroutine and returns the tick channel.
func (s *Scheduler) Start() <-chan time.Time {
	s.started.Store(true)
	go s.run()
	return s.ch
}

// Stop halts the ticker goroutine and waits for it to exit.  Calling Stop
// before Start returns immediately.
func (s *Scheduler) Stop() {
	s.once.Do(func() {
		close(s.stopCh)
		if s.started.Load() {
			<-s.doneCh
		}
	})
}

func (s *Scheduler) run() {
	defer close(s.doneCh)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case t := <-ticker.C:
			select {
			case s.ch <- t:
			case <-s.stopCh:
				return
			}
		case <-s.stopCh:
			return
		}
	}
}

// rateLimiter spaces successive Wait calls at least minInterval apart.  A
// zero minInterval disables waiting entirely.
type rateLimiter struct {
	mu          sync.Mutex
	minInterval time.Duration
	last        time.Time
}

func newRateLimiter(minInterval time.Duration) *rateLimiter {
	return &rateLimiter{minInterval: minInterval}
}

// Wait blocks until the minimum interval since the previous Wait has elapsed,
// honouring context cancellation.
func (r *rateLimiter) Wait(ctx context.Context) error {
	if r.minInterval <= 0 {
		return nil
	}

	r.mu.Lock()
	wait := r.minInterval - time.Since(r.last)
	if wait < 0 {
		wait = 0
	}
	r.mu.Unlock()

	if wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	r.mu.Lock()
	r.last = time.Now()
	r.mu.Unlock()
	return nil
}
