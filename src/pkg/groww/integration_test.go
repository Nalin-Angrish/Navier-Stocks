//go:build integration

package groww_test

import (
	"os"
	"testing"
	"time"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/groww"
)

func skipIfNoCreds(t *testing.T) {
	t.Helper()
	if os.Getenv("GROWW_API_KEY") == "" && os.Getenv("GROWW_ACCESS_TOKEN") == "" {
		t.Skip("Skipping live API test: set GROWW_API_KEY+GROWW_API_SECRET or GROWW_ACCESS_TOKEN")
	}
}

func liveClient(t *testing.T) *groww.Client {
	t.Helper()
	if token := os.Getenv("GROWW_ACCESS_TOKEN"); token != "" {
		return groww.NewClient(token)
	}
	return groww.NewClientFromKeys("", "")
}

func TestLiveConnectivity(t *testing.T) {
	skipIfNoCreds(t)
	c := liveClient(t)

	t.Run("GetQuote_RELIANCE", func(t *testing.T) {
		q, err := c.GetQuote(groww.ExchangeNSE, groww.SegmentCash, "RELIANCE")
		if err != nil {
			t.Fatalf("GetQuote failed: %v", err)
		}
		t.Logf("RELIANCE  LTP=%.2f  change=%.2f%%  high=%.2f  low=%.2f  vol=%d",
			q.LastPrice, q.DayChangePerc, q.HighTradeRange, q.LowTradeRange, q.Volume)
		if q.LastPrice <= 0 {
			t.Error("LastPrice should be > 0")
		}
	})

	t.Run("GetLTP_multi", func(t *testing.T) {
		ltp, err := c.GetLTP(groww.SegmentCash, []string{"NSE_RELIANCE", "NSE_TCS", "NSE_INFY"})
		if err != nil {
			t.Fatalf("GetLTP failed: %v", err)
		}
		for sym, price := range ltp {
			t.Logf("  %s = %.2f", sym, price)
			if price <= 0 {
				t.Errorf("%s: LTP should be > 0, got %.2f", sym, price)
			}
		}
		if len(ltp) == 0 {
			t.Error("expected at least one LTP result")
		}
	})

	t.Run("GetOHLC_RELIANCE", func(t *testing.T) {
		ohlc, err := c.GetOHLC(groww.SegmentCash, []string{"NSE_RELIANCE"})
		if err != nil {
			t.Fatalf("GetOHLC failed: %v", err)
		}
		for sym, o := range ohlc {
			t.Logf("  %s = O=%.2f H=%.2f L=%.2f C=%.2f", sym, o.Open, o.High, o.Low, o.Close)
		}
		if len(ohlc) == 0 {
			t.Error("expected OHLC data")
		}
	})
}

func TestLiveMargin(t *testing.T) {
	skipIfNoCreds(t)
	c := liveClient(t)

	m, err := c.GetUserMargin()
	if err != nil {
		t.Fatalf("GetUserMargin failed: %v", err)
	}
	t.Logf("ClearCash=%.2f  NetMarginUsed=%.2f  MISBalance=%.2f",
		m.ClearCash, m.NetMarginUsed, misBalance(m))
}

func misBalance(m *groww.MarginResponse) float64 {
	if m.EquityMarginDetails != nil {
		return m.EquityMarginDetails.MISBalanceAvail
	}
	return 0
}

func TestLiveHistorical(t *testing.T) {
	skipIfNoCreds(t)
	c := liveClient(t)

	end := time.Now()
	start := end.Add(-7 * 24 * time.Hour)

	candles, err := c.GetHistoricalCandles(
		groww.ExchangeNSE, groww.SegmentCash, "RELIANCE",
		start, end, groww.CandleInterval1Day,
	)
	if err != nil {
		t.Fatalf("GetHistoricalCandles failed: %v", err)
	}
	if len(candles) == 0 {
		t.Fatal("expected at least one candle")
	}
	last := candles[len(candles)-1]
	t.Logf("RELIANCE 1-day candles: %d returned", len(candles))
	t.Logf("  latest: O=%.2f H=%.2f L=%.2f C=%.2f V=%d",
		last.Open, last.High, last.Low, last.Close, last.Volume)
}

func TestLiveInstrumentMaster(t *testing.T) {
	skipIfNoCreds(t)

	raw, err := groww.DownloadInstruments()
	if err != nil {
		t.Fatalf("DownloadInstruments failed: %v", err)
	}
	instruments, err := groww.ParseInstruments(raw)
	if err != nil {
		t.Fatalf("ParseInstruments failed: %v", err)
	}
	if len(instruments) == 0 {
		t.Fatal("expected at least one instrument")
	}
	t.Logf("Downloaded %d instruments", len(instruments))

	var countNSE int
	for _, inst := range instruments {
		if inst.Exchange == groww.ExchangeNSE {
			countNSE++
		}
	}
	t.Logf("NSE instruments: %d", countNSE)
	if countNSE == 0 {
		t.Error("expected at least one NSE instrument")
	}
}
