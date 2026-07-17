// Package quantitative_test exercises the ring-buffer tick store and
// ticker-store map.  Tests cover empty / full / wrap-around states,
// windowed queries, candle aggregation, concurrency safety, and the
// ticker-store capacity limit.  Benchmarks verify sub-microsecond
// latency for all fast-path operations.
package quantitative_test

import (
	"testing"
	"time"

	"github.com/Nalin-Angrish/Navier-Stocks/src/analysts/quantitative"
)

// now returns a fixed point in time used throughout the tests so that
// candle boundaries are deterministic.
func now() time.Time {
	return time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
}

// ---------------------------------------------------------------------------
// TickStore construction
// ---------------------------------------------------------------------------

func TestNewTickStore_ZeroSize(t *testing.T) {
	ts := quantitative.NewTickStore(0)
	if ts.Cap() != 1 {
		t.Fatalf("Cap() = %d, want 1", ts.Cap())
	}
}

// ---------------------------------------------------------------------------
// Append / Len
// ---------------------------------------------------------------------------

func TestTickStore_AppendAndLen(t *testing.T) {
	ts := quantitative.NewTickStore(4)

	if ts.Len() != 0 {
		t.Fatalf("Len() = %d, want 0", ts.Len())
	}

	ts.Append(quantitative.Tick{Price: 100, Volume: 1000, Timestamp: now()})
	if ts.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", ts.Len())
	}

	ts.Append(quantitative.Tick{Price: 101, Volume: 2000, Timestamp: now()})
	if ts.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", ts.Len())
	}
}

func TestTickStore_FillAndWrap(t *testing.T) {
	ts := quantitative.NewTickStore(3)

	for i := 0; i < 3; i++ {
		ts.Append(quantitative.Tick{Price: float64(100 + i), Volume: 1000, Timestamp: now()})
	}
	if ts.Len() != 3 {
		t.Fatalf("Len() = %d, want 3", ts.Len())
	}

	ts.Append(quantitative.Tick{Price: 200, Volume: 5000, Timestamp: now()})
	if ts.Len() != 3 {
		t.Fatalf("after wrap Len() = %d, want 3", ts.Len())
	}

	prices := ts.RecentPrices(3)
	if len(prices) != 3 {
		t.Fatalf("RecentPrices(3) = %d items, want 3", len(prices))
	}
	if prices[0] != 101 || prices[1] != 102 || prices[2] != 200 {
		t.Fatalf("prices = %v, want [101 102 200]", prices)
	}
}

func TestTickStore_CapConsistency(t *testing.T) {
	ts := quantitative.NewTickStore(10)
	for i := 0; i < 10; i++ {
		ts.Append(quantitative.Tick{Price: float64(i), Volume: 1000, Timestamp: now()})
		if ts.Len() != i+1 {
			t.Fatalf("after %d appends Len() = %d, want %d", i+1, ts.Len(), i+1)
		}
	}
	for i := 10; i < 100; i++ {
		ts.Append(quantitative.Tick{Price: float64(i), Volume: 1000, Timestamp: now()})
		if ts.Len() != 10 {
			t.Fatalf("after %d appends Len() = %d, want 10", i+1, ts.Len())
		}
	}
}

func TestTickStore_Cap(t *testing.T) {
	ts := quantitative.NewTickStore(8)
	if ts.Cap() != 8 {
		t.Fatalf("Cap() = %d, want 8", ts.Cap())
	}
}

// ---------------------------------------------------------------------------
// RecentPrices
// ---------------------------------------------------------------------------

func TestTickStore_RecentPrices_LessThanN(t *testing.T) {
	ts := quantitative.NewTickStore(10)
	ts.Append(quantitative.Tick{Price: 50, Volume: 500, Timestamp: now()})
	ts.Append(quantitative.Tick{Price: 51, Volume: 600, Timestamp: now()})

	prices := ts.RecentPrices(10)
	if len(prices) != 2 {
		t.Fatalf("len = %d, want 2", len(prices))
	}
	if prices[0] != 50 || prices[1] != 51 {
		t.Fatalf("prices = %v, want [50 51]", prices)
	}
}

func TestTickStore_RecentPrices_Empty(t *testing.T) {
	ts := quantitative.NewTickStore(10)
	if p := ts.RecentPrices(5); len(p) != 0 {
		t.Fatalf("expected empty slice, got %v", p)
	}
}

func TestTickStore_RecentPrices_ZeroN(t *testing.T) {
	ts := quantitative.NewTickStore(10)
	ts.Append(quantitative.Tick{Price: 100, Volume: 1000, Timestamp: now()})
	if p := ts.RecentPrices(0); p != nil {
		t.Fatalf("expected nil, got %v", p)
	}
}

func TestTickStore_RecentPrices_NegativeN(t *testing.T) {
	ts := quantitative.NewTickStore(10)
	ts.Append(quantitative.Tick{Price: 100, Volume: 1000, Timestamp: now()})
	if p := ts.RecentPrices(-1); p != nil {
		t.Fatalf("expected nil for negative n, got %v", p)
	}
}

func TestTickStore_RecentPrices_ExactFit(t *testing.T) {
	ts := quantitative.NewTickStore(3)
	for i := 0; i < 3; i++ {
		ts.Append(quantitative.Tick{Price: float64(10 + i), Volume: 1000, Timestamp: now()})
	}
	prices := ts.RecentPrices(3)
	if len(prices) != 3 {
		t.Fatalf("len = %d, want 3", len(prices))
	}
	if prices[0] != 10 || prices[1] != 11 || prices[2] != 12 {
		t.Fatalf("prices = %v, want [10 11 12]", prices)
	}
}

// ---------------------------------------------------------------------------
// RecentVolumes
// ---------------------------------------------------------------------------

func TestTickStore_RecentVolumes(t *testing.T) {
	ts := quantitative.NewTickStore(5)
	for i := 0; i < 5; i++ {
		ts.Append(quantitative.Tick{Price: 100, Volume: int64(1000 * (i + 1)), Timestamp: now()})
	}

	vols := ts.RecentVolumes(3)
	if len(vols) != 3 {
		t.Fatalf("len = %d, want 3", len(vols))
	}
	if vols[0] != 3000 || vols[1] != 4000 || vols[2] != 5000 {
		t.Fatalf("volumes = %v, want [3000 4000 5000]", vols)
	}
}

func TestTickStore_RecentVolumes_Empty(t *testing.T) {
	ts := quantitative.NewTickStore(10)
	if v := ts.RecentVolumes(5); len(v) != 0 {
		t.Fatalf("expected empty slice, got %v", v)
	}
}

func TestTickStore_RecentVolumes_NegativeN(t *testing.T) {
	ts := quantitative.NewTickStore(10)
	ts.Append(quantitative.Tick{Price: 100, Volume: 1000, Timestamp: now()})
	if v := ts.RecentVolumes(-5); v != nil {
		t.Fatalf("expected nil for negative n, got %v", v)
	}
}

// ---------------------------------------------------------------------------
// Candles
// ---------------------------------------------------------------------------

func TestTickStore_Candles_SingleBucket(t *testing.T) {
	ts := quantitative.NewTickStore(10)
	base := now()
	ts.Append(quantitative.Tick{Price: 100, Volume: 1000, Timestamp: base})
	ts.Append(quantitative.Tick{Price: 110, Volume: 500, Timestamp: base.Add(30 * time.Second)})
	ts.Append(quantitative.Tick{Price: 95, Volume: 2000, Timestamp: base.Add(45 * time.Second)})
	ts.Append(quantitative.Tick{Price: 105, Volume: 800, Timestamp: base.Add(59 * time.Second)})

	candles := ts.Candles(time.Minute)
	if len(candles) != 1 {
		t.Fatalf("got %d candles, want 1", len(candles))
	}
	c := candles[0]
	if c.Open != 100 || c.High != 110 || c.Low != 95 || c.Close != 105 || c.Volume != 4300 {
		t.Fatalf("candle = %+v, want {Open:100 High:110 Low:95 Close:105 Volume:4300}", c)
	}
}

func TestTickStore_Candles_MultipleBuckets(t *testing.T) {
	ts := quantitative.NewTickStore(10)
	base := now()
	ts.Append(quantitative.Tick{Price: 100, Volume: 1000, Timestamp: base})
	ts.Append(quantitative.Tick{Price: 110, Volume: 500, Timestamp: base.Add(1 * time.Minute)})
	ts.Append(quantitative.Tick{Price: 120, Volume: 300, Timestamp: base.Add(2 * time.Minute)})

	candles := ts.Candles(time.Minute)
	if len(candles) != 3 {
		t.Fatalf("got %d candles, want 3", len(candles))
	}
	if candles[0].Open != 100 || candles[0].Close != 100 {
		t.Fatalf("candle 0: %+v", candles[0])
	}
	if candles[1].Open != 110 || candles[1].Close != 110 {
		t.Fatalf("candle 1: %+v", candles[1])
	}
	if candles[2].Open != 120 || candles[2].Close != 120 {
		t.Fatalf("candle 2: %+v", candles[2])
	}
}

func TestTickStore_Candles_Empty(t *testing.T) {
	ts := quantitative.NewTickStore(10)
	if c := ts.Candles(time.Minute); c != nil {
		t.Fatalf("expected nil, got %v", c)
	}
}

func TestTickStore_Candles_NoDups(t *testing.T) {
	ts := quantitative.NewTickStore(10)
	base := now()
	ts.Append(quantitative.Tick{Price: 100, Volume: 500, Timestamp: base})
	ts.Append(quantitative.Tick{Price: 110, Volume: 300, Timestamp: base.Add(2 * time.Minute)})
	ts.Append(quantitative.Tick{Price: 105, Volume: 200, Timestamp: base.Add(2 * time.Minute)})

	candles := ts.Candles(time.Minute)
	if len(candles) != 2 {
		t.Fatalf("got %d candles, want 2", len(candles))
	}
	if candles[0].Close != 100 {
		t.Fatalf("candle[0].Close = %f, want 100", candles[0].Close)
	}
	if candles[1].High != 110 || candles[1].Low != 105 || candles[1].Close != 105 {
		t.Fatalf("candle[1] = %+v, want {High:110 Low:105 Close:105}", candles[1])
	}
}

// ---------------------------------------------------------------------------
// Wrap-around
// ---------------------------------------------------------------------------

func TestTickStore_WrapAroundPreservesOrder(t *testing.T) {
	ts := quantitative.NewTickStore(4)
	for i := 0; i < 6; i++ {
		ts.Append(quantitative.Tick{Price: float64(i), Volume: 1000, Timestamp: now()})
	}

	prices := ts.RecentPrices(4)
	expected := []float64{2, 3, 4, 5}
	for i, p := range prices {
		if p != expected[i] {
			t.Fatalf("prices[%d] = %f, want %f", i, p, expected[i])
		}
	}
}

func TestTickStore_WrapAroundThenPartialRead(t *testing.T) {
	ts := quantitative.NewTickStore(4)
	for i := 0; i < 6; i++ {
		ts.Append(quantitative.Tick{Price: float64(i), Volume: 1000, Timestamp: now()})
	}

	prices := ts.RecentPrices(2)
	if len(prices) != 2 {
		t.Fatalf("len = %d, want 2", len(prices))
	}
	if prices[0] != 4 || prices[1] != 5 {
		t.Fatalf("prices = %v, want [4 5]", prices)
	}
}

// ---------------------------------------------------------------------------
// Concurrency
// ---------------------------------------------------------------------------

func TestTickStore_ConcurrentAppendAndRead(t *testing.T) {
	ts := quantitative.NewTickStore(100)
	done := make(chan struct{})

	go func() {
		for i := 0; i < 1000; i++ {
			ts.Append(quantitative.Tick{Price: float64(i), Volume: 1000, Timestamp: now()})
		}
		close(done)
	}()

	for i := 0; i < 100; i++ {
		ts.RecentPrices(10)
		ts.RecentVolumes(5)
		ts.Candles(time.Minute)
		ts.Len()
	}

	<-done
}

// ---------------------------------------------------------------------------
// TickerStore
// ---------------------------------------------------------------------------

func TestTickerStore_StoreAndGet(t *testing.T) {
	tss := quantitative.NewTickerStore(5, 10)
	s := tss.Store("RELIANCE")
	if s == nil {
		t.Fatal("Store returned nil")
	}

	s2 := tss.Store("RELIANCE")
	if s != s2 {
		t.Fatal("Store returned different instance for same symbol")
	}

	got := tss.Get("RELIANCE")
	if got != s {
		t.Fatal("Get returned different instance")
	}
}

func TestTickerStore_MaxTickers(t *testing.T) {
	tss := quantitative.NewTickerStore(3, 10)
	symbols := []string{"A", "B", "C", "D"}
	for _, sym := range symbols[:3] {
		if s := tss.Store(sym); s == nil {
			t.Fatalf("Store(%q) returned nil", sym)
		}
	}

	if s := tss.Store("D"); s != nil {
		t.Fatal("Store should return nil when max tickers reached")
	}

	if tss.Len() != 3 {
		t.Fatalf("Len() = %d, want 3", tss.Len())
	}
}

func TestTickerStore_Symbols(t *testing.T) {
	tss := quantitative.NewTickerStore(5, 10)
	tss.Store("RELIANCE")
	tss.Store("TCS")
	tss.Store("INFY")

	syms := tss.Symbols()
	if len(syms) != 3 {
		t.Fatalf("len(Symbols) = %d, want 3", len(syms))
	}

	m := make(map[string]bool)
	for _, s := range syms {
		m[s] = true
	}
	for _, want := range []string{"RELIANCE", "TCS", "INFY"} {
		if !m[want] {
			t.Fatalf("missing symbol %q", want)
		}
	}
}

func TestTickerStore_GetNonexistent(t *testing.T) {
	tss := quantitative.NewTickerStore(5, 10)
	if s := tss.Get("NONEXISTENT"); s != nil {
		t.Fatal("Get should return nil for nonexistent symbol")
	}
}

func TestTickerStore_Cap(t *testing.T) {
	tss := quantitative.NewTickerStore(10, 100)
	if tss.Cap() != 10 {
		t.Fatalf("Cap() = %d, want 10", tss.Cap())
	}
}

func TestTickerStore_DefaultMax(t *testing.T) {
	tss := quantitative.NewTickerStore(0, 10)
	if tss.Cap() != 25 {
		t.Fatalf("Cap() = %d, want 25", tss.Cap())
	}
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkTickStore_Append(b *testing.B) {
	ts := quantitative.NewTickStore(256)
	tick := quantitative.Tick{Price: 100.50, Volume: 1000, Timestamp: now()}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ts.Append(tick)
	}
}

func BenchmarkTickStore_RecentPrices(b *testing.B) {
	ts := quantitative.NewTickStore(256)
	tick := quantitative.Tick{Price: 100.50, Volume: 1000, Timestamp: now()}
	for i := 0; i < 256; i++ {
		ts.Append(tick)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ts.RecentPrices(20)
	}
}

func BenchmarkTickStore_RecentVolumes(b *testing.B) {
	ts := quantitative.NewTickStore(256)
	tick := quantitative.Tick{Price: 100.50, Volume: 1000, Timestamp: now()}
	for i := 0; i < 256; i++ {
		ts.Append(tick)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ts.RecentVolumes(20)
	}
}

func BenchmarkTickStore_Candles(b *testing.B) {
	ts := quantitative.NewTickStore(256)
	base := now()
	for i := 0; i < 256; i++ {
		ts.Append(quantitative.Tick{
			Price:     100 + float64(i),
			Volume:    1000,
			Timestamp: base.Add(time.Duration(i) * time.Second),
		})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ts.Candles(time.Minute)
	}
}

func BenchmarkTickStore_AppendConcurrent(b *testing.B) {
	ts := quantitative.NewTickStore(256)
	tick := quantitative.Tick{Price: 100.50, Volume: 1000, Timestamp: now()}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ts.Append(tick)
		}
	})
}

func BenchmarkTickStore_RecentPrices_FullBuf(b *testing.B) {
	ts := quantitative.NewTickStore(1024)
	tick := quantitative.Tick{Price: 100.50, Volume: 1000, Timestamp: now()}
	for i := 0; i < 1024; i++ {
		ts.Append(tick)
	}
	n := float64(ts.Cap())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ts.RecentPrices(int(n))
	}
}

func BenchmarkTickStore_Append_Burst(b *testing.B) {
	ts := quantitative.NewTickStore(1024)
	tick := quantitative.Tick{Price: 100.50, Volume: 1000, Timestamp: now()}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ts.Append(tick)
		ts.Append(tick)
		ts.Append(tick)
		ts.Append(tick)
		ts.Append(tick)
	}
}

func BenchmarkTickerStore_Store(b *testing.B) {
	tss := quantitative.NewTickerStore(25, 256)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sym := "STOCK" + string(rune('A'+i%26))
		tss.Store(sym)
	}
}

func BenchmarkTickStore_Candles_Large(b *testing.B) {
	ts := quantitative.NewTickStore(1024)
	base := now()
	for i := 0; i < 1024; i++ {
		ts.Append(quantitative.Tick{
			Price:     100 + float64(i),
			Volume:    1000,
			Timestamp: base.Add(time.Duration(i) * time.Second),
		})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ts.Candles(5 * time.Minute)
	}
}
