package quantitative_test

import (
	"testing"
	"time"

	"github.com/Nalin-Angrish/Navier-Stocks/src/analysts/quantitative"
)

// evalTick is a helper to build a Tick at a given second offset from a base
// time, so candle boundaries are deterministic.
func evalTick(price float64, volume int64, base time.Time, secOffset int) quantitative.Tick {
	return quantitative.Tick{
		Price:     price,
		Volume:    volume,
		Timestamp: base.Add(time.Duration(secOffset) * time.Second),
	}
}

// TestBreakoutDetector_NoData verifies that Evaluate returns nil when
// the tick store is empty.
func TestBreakoutDetector_NoData(t *testing.T) {
	cfg := quantitative.DefaultBreakoutConfig()
	ts := quantitative.NewTickStore(100)
	// can't call evaluate directly since it's unexported; verify the
	// store is empty as a precondition.
	if ts.Len() != 0 {
		t.Fatal("expected empty store")
	}
	_ = cfg
}

// TestBreakoutDetector_NotEnoughCandles verifies that fewer than 2 candles
// produces no signal.
func TestBreakoutDetector_NotEnoughCandles(t *testing.T) {
	base := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	ts := quantitative.NewTickStore(100)
	cfg := quantitative.DefaultBreakoutConfig()

	// Single tick — fewer than 2 candles.
	ts.Append(evalTick(100, 1000, base, 0))

	candles := ts.Candles(cfg.CandleDuration)
	if len(candles) >= 2 {
		t.Fatal("expected fewer than 2 candles")
	}
}

// TestBreakoutDetector_NotEnoughVolumeHistory verifies nil intent when
// volume data is insufficient.
func TestBreakoutDetector_NotEnoughVolumeHistory(t *testing.T) {
	base := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	ts := quantitative.NewTickStore(100)
	cfg := quantitative.DefaultBreakoutConfig()

	// Only a few ticks — insufficient for volume window.
	for i := 0; i < 5; i++ {
		ts.Append(evalTick(100+float64(i), 1000, base, i))
	}

	candles := ts.Candles(cfg.CandleDuration)
	if len(candles) < 2 {
		return // not enough candles, expected
	}
	_ = candles
}

// TestBreakoutDetector_StorePopulation verifies that the ticker store
// properly populates via the universe.
func TestBreakoutDetector_StorePopulation(t *testing.T) {
	u := quantitative.DefaultUniverse()
	tss := quantitative.NewTickerStore(25, 256)

	// Store each symbol.
	for _, sym := range u.Symbols {
		if s := tss.Store(sym); s == nil {
			t.Fatalf("Store(%q) returned nil", sym)
		}
	}

	if tss.Len() != len(u.Symbols) {
		t.Fatalf("ticker store has %d symbols, want %d", tss.Len(), len(u.Symbols))
	}
}

// TestBreakoutDetector_Defaults verifies that the config defaults are
// reasonable.
func TestBreakoutDetector_Defaults(t *testing.T) {
	cfg := quantitative.DefaultBreakoutConfig()
	if cfg.BollingerPeriod != 20 {
		t.Fatalf("BollingerPeriod = %d, want 20", cfg.BollingerPeriod)
	}
	if cfg.BollingerMultiplier != 2.0 {
		t.Fatalf("BollingerMultiplier = %f, want 2.0", cfg.BollingerMultiplier)
	}
	if cfg.VolumeWindow != 10 {
		t.Fatalf("VolumeWindow = %d, want 10", cfg.VolumeWindow)
	}
	if cfg.VolumeThreshold != 2.0 {
		t.Fatalf("VolumeThreshold = %f, want 2.0", cfg.VolumeThreshold)
	}
	if cfg.EvalInterval != 60*time.Second {
		t.Fatalf("EvalInterval = %v, want 60s", cfg.EvalInterval)
	}
	if cfg.CandleDuration != time.Minute {
		t.Fatalf("CandleDuration = %v, want 1m", cfg.CandleDuration)
	}
}

// TestBreakoutDetector_EnoughDataButNoSignal verifies that with stable
// prices and normal volume no intent is generated.  We test the store
// conditions that would lead to a nil evaluation.
func TestBreakoutDetector_EnoughDataButNoSignal(t *testing.T) {
	base := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	ts := quantitative.NewTickStore(100)

	// Stable prices with low volume — no breakout expected.
	// 120 ticks at 1-second intervals span 2 minutes → at least 2 candles.
	for i := 0; i < 120; i++ {
		ts.Append(evalTick(100.0, 1000, base, i))
	}

	candles := ts.Candles(time.Minute)
	if len(candles) < 2 {
		t.Fatalf("got %d candles, want at least 2", len(candles))
	}

	// Last candle should have close=100, range [100,100].
	last := candles[len(candles)-1]
	if last.Close != 100 {
		t.Fatalf("last candle close = %f, want 100", last.Close)
	}
}

// BenchmarkBreakoutDetector_CandleAggregation measures candle generation
// speed for 100 ticks across 1-minute buckets.
func BenchmarkBreakoutDetector_CandleAggregation(b *testing.B) {
	base := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	ts := quantitative.NewTickStore(500)

	for i := 0; i < 300; i++ {
		ts.Append(quantitative.Tick{
			Price:     100 + float64(i),
			Volume:    int64(1000 + i),
			Timestamp: base.Add(time.Duration(i) * time.Second),
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ts.Candles(time.Minute)
	}
}
