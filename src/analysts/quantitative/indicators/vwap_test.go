// Package indicators_test exercises the VWAP, Bollinger Band, and volume
// breakout indicator functions against known values and degenerate inputs.
package indicators_test

import (
	"testing"

	"github.com/Nalin-Angrish/Navier-Stocks/src/analysts/quantitative/indicators"
)

// ---------------------------------------------------------------------------
// VWAP
// ---------------------------------------------------------------------------

func TestVWAP_Normal(t *testing.T) {
	prices := []float64{10, 20, 30}
	volumes := []int64{100, 200, 300}
	r := indicators.VWAP(prices, volumes)
	if !r.Valid {
		t.Fatal("VWAP returned Valid=false")
	}
	// (10*100 + 20*200 + 30*300) / (100+200+300) = 14000/600 ≈ 23.3333
	want := 14000.0 / 600.0
	if r.VWAP != want {
		t.Fatalf("VWAP = %f, want %f", r.VWAP, want)
	}
	if r.TotalVolume != 600 {
		t.Fatalf("TotalVolume = %f, want 600", r.TotalVolume)
	}
	if r.PriceVolume != 14000 {
		t.Fatalf("PriceVolume = %f, want 14000", r.PriceVolume)
	}
}

func TestVWAP_SingleTick(t *testing.T) {
	r := indicators.VWAP([]float64{100.5}, []int64{1000})
	if !r.Valid || r.VWAP != 100.5 {
		t.Fatalf("VWAP(single) = %f, want 100.5", r.VWAP)
	}
}

func TestVWAP_EmptyPrices(t *testing.T) {
	r := indicators.VWAP(nil, []int64{100})
	if r.Valid {
		t.Fatal("VWAP(nil prices) returned Valid=true")
	}
}

func TestVWAP_EmptyVolumes(t *testing.T) {
	r := indicators.VWAP([]float64{10}, nil)
	if r.Valid {
		t.Fatal("VWAP(nil volumes) returned Valid=true")
	}
}

func TestVWAP_BothEmpty(t *testing.T) {
	r := indicators.VWAP(nil, nil)
	if r.Valid {
		t.Fatal("VWAP(both nil) returned Valid=true")
	}
}

func TestVWAP_MismatchedLength(t *testing.T) {
	r := indicators.VWAP([]float64{10, 20}, []int64{100})
	if r.Valid {
		t.Fatal("VWAP(mismatched) returned Valid=true")
	}
}

func TestVWAP_ZeroVolume(t *testing.T) {
	r := indicators.VWAP([]float64{10, 20}, []int64{0, 0})
	if r.Valid {
		t.Fatal("VWAP(zero volume) returned Valid=true")
	}
}

func TestVWAP_PartialZeroVolume(t *testing.T) {
	prices := []float64{10, 20, 30}
	volumes := []int64{100, 0, 300}
	r := indicators.VWAP(prices, volumes)
	if !r.Valid {
		t.Fatal("VWAP(partial zero) returned Valid=false")
	}
	// (10*100 + 20*0 + 30*300) / (100+0+300) = (1000+0+9000)/400 = 10000/400 = 25
	if r.VWAP != 25 {
		t.Fatalf("VWAP = %f, want 25", r.VWAP)
	}
}

func TestVWAP_IdenticalPrices(t *testing.T) {
	prices := []float64{50, 50, 50, 50}
	volumes := []int64{100, 200, 300, 400}
	r := indicators.VWAP(prices, volumes)
	if !r.Valid || r.VWAP != 50 {
		t.Fatalf("VWAP(constant) = %f, want 50", r.VWAP)
	}
}

// ---------------------------------------------------------------------------
// VWAPFromCandles
// ---------------------------------------------------------------------------

func TestVWAPFromCandles_Normal(t *testing.T) {
	candles := []indicators.Candle{
		{Close: 10, Volume: 100},
		{Close: 20, Volume: 200},
		{Close: 30, Volume: 300},
	}
	r := indicators.VWAPFromCandles(candles)
	if !r.Valid {
		t.Fatal("VWAPFromCandles returned Valid=false")
	}
	want := 14000.0 / 600.0
	if r.VWAP != want {
		t.Fatalf("VWAP = %f, want %f", r.VWAP, want)
	}
}

func TestVWAPFromCandles_Empty(t *testing.T) {
	r := indicators.VWAPFromCandles(nil)
	if r.Valid {
		t.Fatal("VWAPFromCandles(nil) returned Valid=true")
	}
}

// ---------------------------------------------------------------------------
// VWAPFromCandlesTypical
// ---------------------------------------------------------------------------

func TestVWAPFromCandlesTypical_Normal(t *testing.T) {
	candles := []indicators.Candle{
		{High: 12, Low: 8, Close: 10, Volume: 100},
		{High: 25, Low: 15, Close: 20, Volume: 200},
	}
	r := indicators.VWAPFromCandlesTypical(candles)
	if !r.Valid {
		t.Fatal("VWAPFromCandlesTypical returned Valid=false")
	}
	// typical prices: (12+8+10)/3 = 10, (25+15+20)/3 = 20
	// VWAP = (10*100 + 20*200) / 300 = 5000/300 ≈ 16.666...
	want := 5000.0 / 300.0
	if r.VWAP != want {
		t.Fatalf("VWAP = %f, want %f", r.VWAP, want)
	}
}

func TestVWAPFromCandlesTypical_Empty(t *testing.T) {
	r := indicators.VWAPFromCandlesTypical(nil)
	if r.Valid {
		t.Fatal("VWAPFromCandlesTypical(nil) returned Valid=true")
	}
}

// ---------------------------------------------------------------------------
// CumulativeVWAP
// ---------------------------------------------------------------------------

func TestCumulativeVWAP_Add(t *testing.T) {
	cv := indicators.NewCumulativeVWAP()
	if cv.Value() != 0 {
		t.Fatalf("initial Value = %f, want 0", cv.Value())
	}

	v1 := cv.Add(10, 100)
	// 10*100 / 100 = 10
	if v1 != 10 {
		t.Fatalf("after first Add = %f, want 10", v1)
	}

	v2 := cv.Add(20, 200)
	// (10*100 + 20*200) / (100+200) = 5000/300 ≈ 16.666...
	want := 5000.0 / 300.0
	if v2 != want {
		t.Fatalf("after second Add = %f, want %f", v2, want)
	}

	if cv.Value() != want {
		t.Fatalf("Value = %f, want %f", cv.Value(), want)
	}
}

func TestCumulativeVWAP_ZeroVolume(t *testing.T) {
	cv := indicators.NewCumulativeVWAP()
	cv.Add(100, 0)
	if cv.Value() != 0 {
		t.Fatalf("Value after zero volume = %f, want 0", cv.Value())
	}
}

func TestCumulativeVWAP_MixedZeroAndNonZero(t *testing.T) {
	cv := indicators.NewCumulativeVWAP()
	cv.Add(10, 0)        // no-op
	v := cv.Add(20, 100) // 20*100 / 100 = 20
	if v != 20 {
		t.Fatalf("Value = %f, want 20", v)
	}
}

func TestCumulativeVWAP_FreshState(t *testing.T) {
	cv := indicators.NewCumulativeVWAP()
	pv, vs := cv.Observations()
	if pv != 0 || vs != 0 {
		t.Fatalf("fresh Observations = (%f, %f), want (0, 0)", pv, vs)
	}
}

func TestCumulativeVWAP_Observations(t *testing.T) {
	cv := indicators.NewCumulativeVWAP()
	cv.Add(10, 100)
	cv.Add(20, 200)
	pv, vs := cv.Observations()
	if pv != 5000 || vs != 300 {
		t.Fatalf("Observations = (%f, %f), want (5000, 300)", pv, vs)
	}
}

// ---------------------------------------------------------------------------
// VWAPBand
// ---------------------------------------------------------------------------

func TestVWAPBand_Above(t *testing.T) {
	dev, pct := indicators.VWAPBand(110, 100)
	if abs(dev-10) > 1e-9 || abs(pct-10) > 1e-9 {
		t.Fatalf("above = (%g, %g), want (10, 10)", dev, pct)
	}
}

func TestVWAPBand_Below(t *testing.T) {
	dev, pct := indicators.VWAPBand(90, 100)
	if abs(dev+10) > 1e-9 || abs(pct+10) > 1e-9 {
		t.Fatalf("below = (%g, %g), want (-10, -10)", dev, pct)
	}
}

func TestVWAPBand_AtVWAP(t *testing.T) {
	dev, pct := indicators.VWAPBand(100, 100)
	if dev != 0 || pct != 0 {
		t.Fatalf("at vwap = (%f, %f), want (0, 0)", dev, pct)
	}
}

func TestVWAPBand_ZeroVWAP(t *testing.T) {
	dev, pct := indicators.VWAPBand(50, 0)
	if dev != 0 || pct != 0 {
		t.Fatalf("zero vwap = (%f, %f), want (0, 0)", dev, pct)
	}
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkVWAP_1k(b *testing.B) {
	prices := make([]float64, 1000)
	volumes := make([]int64, 1000)
	for i := range prices {
		prices[i] = 100 + float64(i)
		volumes[i] = int64(1000 + i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		indicators.VWAP(prices, volumes)
	}
}

func BenchmarkCumulativeVWAP_Add(b *testing.B) {
	cv := indicators.NewCumulativeVWAP()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cv.Add(100.5, 1000)
	}
}
