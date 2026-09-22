// Command feedcheck verifies the Groww WebSocket feed end-to-end locally.
//
// It connects via the real FeedClient (API key/secret auto-refresh) and via
// the higher-level FeedConnector, subscribes to LTP for a small universe, and
// checks that ticks arrive within the requested window. It is intentionally
// a standalone tool (not a go test) so it never runs on CI without creds.
//
// Usage:
//
//	go run ./src/tools/feedcheck --api-key KEY --api-secret SECRET
//	go run ./src/tools/feedcheck --duration 15s --symbols RELIANCE,TCS,INFY
//	GROWW_API_KEY=xxx GROWW_API_SECRET=yyy go run ./src/tools/feedcheck
//	GROWW_API_KEY=xxx GROWW_API_SECRET=yyy go run ./src/tools/feedcheck --feed-url wss://api.groww.in/v1/feed
//
// Exit code 0 = feed connected and (if market open) ticks received.
// Exit code 1 = connect/subscribe failed or no ticks during open market.
// Outside market hours the tool warns and exits 0 if connect succeeds but no ticks.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Nalin-Angrish/Navier-Stocks/src/analysts/quantitative"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/groww"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/utils"
)

func main() {
	// Flags override env so the tool works both with and without a .env file.
	apiKeyFlag := flag.String("api-key", "", "Groww API key (fallback: GROWW_API_KEY)")
	apiSecretFlag := flag.String("api-secret", "", "Groww API secret (fallback: GROWW_API_SECRET)")
	tokenFlag := flag.String("token", "", "static Groww access token (fallback: GROWW_ACCESS_TOKEN)")
	durationFlag := flag.Duration("duration", 10*time.Second, "how long to follow the feed for live ticks")
	symbolsFlag := flag.String("symbols", "", "comma-separated symbols to subscribe (default: 3-symbol smoke set; use 'universe' for full 25)")
	feedURLFlag := flag.String("feed-url", "", "override Groww feed URL (fallback: GROWW_FEED_URL)")
	baseURLFlag := flag.String("base-url", "", "override Groww REST base URL (fallback: GROWW_BASE_URL)")
	flag.Parse()

	utils.LoadEnv()

	// Resolve credentials with flag > env priority.
	apiKey := firstNonEmpty(*apiKeyFlag, os.Getenv("GROWW_API_KEY"))
	apiSecret := firstNonEmpty(*apiSecretFlag, os.Getenv("GROWW_API_SECRET"))
	token := firstNonEmpty(*tokenFlag, os.Getenv("GROWW_ACCESS_TOKEN"))
	if *feedURLFlag != "" {
		_ = os.Setenv("GROWW_FEED_URL", *feedURLFlag)
	}
	if *baseURLFlag != "" {
		_ = os.Setenv("GROWW_BASE_URL", *baseURLFlag)
	}
	if *feedURLFlag != "" {
		log.Printf("[feedcheck] using feed URL: %s", os.Getenv("GROWW_FEED_URL"))
	}
	if *baseURLFlag != "" {
		log.Printf("[feedcheck] using base URL: %s", os.Getenv("GROWW_BASE_URL"))
	}

	hasKeys := apiKey != "" && apiSecret != ""
	hasToken := token != ""
	if !hasKeys && !hasToken {
		fatal("no credentials: set GROWW_API_KEY+GROWW_API_SECRET or GROWW_ACCESS_TOKEN (or pass --api-key/--api-secret/--token)")
	}
	if hasKeys {
		log.Printf("[feedcheck] auth: API key %s (auto-refresh)", mask(apiKey))
	} else {
		log.Printf("[feedcheck] auth: static token (%d chars)", len(token))
	}

	// Market-hours hint — outside hours the feed may connect but deliver no ticks.
	if !utils.IsMarketOpen(time.Now()) {
		log.Printf("[feedcheck] warning: market is currently CLOSED (IsMarketOpen=false, IsOpeningBuffer=%v IsClosingCutoff=%v). Connection should still succeed but ticks may be empty.",
			utils.IsOpeningBuffer(time.Now()), utils.IsClosingCutoff(time.Now()))
	} else {
		log.Printf("[feedcheck] market is OPEN — ticks expected within %s", durationFlag.String())
	}

	// Resolve symbols to subscribe.
	universe := buildTestUniverse(*symbolsFlag)
	log.Printf("[feedcheck] symbols: %s (%d)", strings.Join(universe.Symbols, ","), len(universe.Symbols))

	// Resolve exchange tokens from the public instrument CSV (no auth needed).
	// This is required because quantitative.DefaultUniverse has empty tokens.
	if err := resolveTokens(universe); err != nil {
		log.Printf("[feedcheck] warning: token resolve failed: %v (will try with empty tokens — may fail to produce ticks)", err)
	} else {
		missing := 0
		for _, e := range universe.Entries {
			if e.ExchangeToken == "" {
				missing++
			}
		}
		if missing > 0 {
			log.Printf("[feedcheck] warning: %d/%d symbols missing exchange_token after resolve", missing, len(universe.Entries))
		} else {
			log.Printf("[feedcheck] resolved exchange tokens for all %d symbols", len(universe.Entries))
		}
	}

	// Phase 1: direct FeedClient probe (single round-trip to verify handshake).
	if err := probeRawFeed(apiKey, apiSecret, token, universe, *durationFlag); err != nil {
		fatal("raw FeedClient probe failed: %v", err)
	}

	// Phase 2: FeedConnector probe (the path Quantitative Scout actually uses).
	if err := probeFeedConnector(apiKey, apiSecret, token, universe, *durationFlag); err != nil {
		fatal("FeedConnector probe failed: %v", err)
	}

	log.Printf("[feedcheck] SUCCESS — feed connected and live data path verified")
}

func probeRawFeed(apiKey, apiSecret, token string, universe *quantitative.Universe, duration time.Duration) error {
	log.Printf("[feedcheck] --- raw FeedClient: Connect + SubscribeLTP + Consume %s ---", duration)

	var fc *groww.FeedClient
	if apiKey != "" && apiSecret != "" {
		fc = groww.NewFeedClientFromKeys(apiKey, apiSecret)
	} else {
		fc = groww.NewFeedClient(token)
	}

	if err := fc.Connect(); err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer func() { _ = fc.Close() }()

	log.Printf("[feedcheck] raw FeedClient: WebSocket connected")

	instruments := toInstruments(universe)
	if err := fc.SubscribeLTP(instruments); err != nil {
		return fmt.Errorf("SubscribeLTP: %w", err)
	}
	log.Printf("[feedcheck] raw FeedClient: subscribed LTP for %d instruments", len(instruments))

	// Track callbacks so we can report arrival without relying on poll snapshot timing.
	tickCount := 0
	fc.SetLTPCallback(func(ltp groww.LTPData, meta groww.FeedMetadata) {
		tickCount++
		if tickCount <= 5 {
			log.Printf("[feedcheck] raw LTP tick %d: LTP=%.2f ts=%d", tickCount, ltp.LTP, ltp.TsInMillis)
		}
	})

	go fc.Consume()
	// Poll snapshot for 10s to confirm data path; outside market hours this may stay nil.
	deadline := time.Now().Add(duration)
	seen := false
	for time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		snap := fc.GetLTP()
		if len(snap) > 0 {
			seen = true
			// Count leaf entries.
			n := 0
			for _, ex := range snap {
				for _, seg := range ex {
					n += len(seg)
				}
			}
			// Snapshot arrived but callback may not yet have fired (rare race).
			_ = tickCount
			log.Printf("[feedcheck] raw FeedClient: snapshot has %d symbols (ticks via callback: %d)", n, tickCount)
			break
		}
	}
	if !seen && tickCount == 0 {
		if !utils.IsMarketOpen(time.Now()) {
			log.Printf("[feedcheck] raw FeedClient: no ticks in %s — expected outside market hours, connection was successful", duration)
			return nil
		}
		return fmt.Errorf("no LTP ticks/snapshot within %s (market appears open)", duration)
	}
	log.Printf("[feedcheck] raw FeedClient: OK (callback ticks=%d)", tickCount)
	return nil
}

func probeFeedConnector(apiKey, apiSecret, token string, universe *quantitative.Universe, duration time.Duration) error {
	log.Printf("[feedcheck] --- FeedConnector: Start + TickerStore poll %s ---", duration)

	var feedClient *groww.FeedClient
	var restClient *groww.Client
	if apiKey != "" && apiSecret != "" {
		feedClient = groww.NewFeedClientFromKeys(apiKey, apiSecret)
		restClient = groww.NewClientFromKeys(apiKey, apiSecret)
	} else {
		feedClient = groww.NewFeedClient(token)
		restClient = groww.NewClient(token)
	}

	ts := quantitative.NewTickerStore(len(universe.Symbols)+5, 256)
	fc := quantitative.NewFeedConnector(feedClient, ts, universe)
	fc.SetRESTClient(restClient)

	if err := fc.Start(); err != nil {
		return fmt.Errorf("FeedConnector.Start: %w", err)
	}
	defer fc.Stop()

	log.Printf("[feedcheck] FeedConnector: subscribed, polling store every 100ms")

	deadline := time.Now().Add(duration)
	totalTicks := 0
	for time.Now().Before(deadline) {
		time.Sleep(1 * time.Second)
		// Aggregate store sizes.
		count := 0
		for _, sym := range universe.Symbols {
			if st := ts.Store(sym); st != nil {
				count += st.Len()
			}
		}
		if count > totalTicks {
			totalTicks = count
			// Log one sample price.
			for _, sym := range universe.Symbols {
				if st := ts.Get(sym); st != nil && st.Len() > 0 {
					prices := st.RecentPrices(1)
					vols := st.RecentVolumes(1)
					price := 0.0
					vol := int64(0)
					if len(prices) > 0 {
						price = prices[0]
					}
					if len(vols) > 0 {
						vol = vols[0]
					}
					log.Printf("[feedcheck] FeedConnector: %s ticks=%d latest %s=%.2f vol=%d", sym, st.Len(), sym, price, vol)
					break
				}
			}
		}
	}
	if totalTicks == 0 {
		if !utils.IsMarketOpen(time.Now()) {
			log.Printf("[feedcheck] FeedConnector: no ticks in %s — expected outside market hours, connection/start was successful", duration)
			return nil
		}
		return fmt.Errorf("no ticks landed in TickerStore within %s (market open, expected >0)", duration)
	}
	log.Printf("[feedcheck] FeedConnector: OK totalTicks=%d", totalTicks)
	return nil
}

func buildTestUniverse(symbolsFlag string) *quantitative.Universe {
	trimmed := strings.TrimSpace(symbolsFlag)
	if trimmed == "" {
		// Small smoke set: cheaper to resolve and subscribe, fast feedback.
		// These are all in the instrument CSV and liquid.
		return quantitative.NewUniverseFromSymbols([]string{"RELIANCE", "TCS", "INFY"})
	}
	if strings.EqualFold(trimmed, "universe") || strings.EqualFold(trimmed, "all") || strings.EqualFold(trimmed, "default") {
		return quantitative.DefaultUniverse()
	}
	parts := strings.Split(trimmed, ",")
	var syms []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			syms = append(syms, strings.ToUpper(p))
		}
	}
	if len(syms) == 0 {
		return quantitative.NewUniverseFromSymbols([]string{"RELIANCE", "TCS", "INFY"})
	}
	return quantitative.NewUniverseFromSymbols(syms)
}

func resolveTokens(u *quantitative.Universe) error {
	raw, err := groww.DownloadInstruments()
	if err != nil {
		return err
	}
	instruments, err := groww.ParseInstruments(raw)
	if err != nil {
		return err
	}
	// Map trading_symbol -> exchange_token for NSE CASH.
	tokenBySymbol := make(map[string]string)
	for _, inst := range instruments {
		if inst.Exchange != groww.ExchangeNSE || inst.Segment != groww.SegmentCash {
			continue
		}
		// TradingSymbol is already upper-case in CSV.
		if _, ok := tokenBySymbol[inst.TradingSymbol]; !ok && inst.ExchangeToken != "" {
			tokenBySymbol[inst.TradingSymbol] = inst.ExchangeToken
		}
	}
	for i, e := range u.Entries {
		if tok, ok := tokenBySymbol[e.Symbol]; ok {
			u.Entries[i].ExchangeToken = tok
			// Keep lookup maps in sync.
			u.TokenToSymbol[tok] = e.Symbol
		}
	}
	return nil
}

func toInstruments(u *quantitative.Universe) []groww.FeedInstrument {
	out := make([]groww.FeedInstrument, 0, len(u.Entries))
	for _, e := range u.Entries {
		out = append(out, groww.FeedInstrument{
			Exchange:      e.Exchange,
			Segment:       e.Segment,
			ExchangeToken: e.ExchangeToken,
		})
	}
	return out
}

func mask(s string) string {
	if len(s) <= 4 {
		return "****"
	}
	return s[:4] + "****"
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func fatal(format string, args ...any) {
	log.Printf("feedcheck: "+format, args...)
	fmt.Fprintf(os.Stderr, "feedcheck: "+format+"\n", args...)
	os.Exit(1)
}
