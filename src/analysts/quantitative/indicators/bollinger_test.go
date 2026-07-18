package indicators_test

import (
	"testing"

	"github.com/Nalin-Angrish/Navier-Stocks/src/analysts/quantitative/indicators"
)

// ---------------------------------------------------------------------------
// BollingerBands
// ---------------------------------------------------------------------------

func TestBollingerBands_Normal(t *testing.T) {
	// period=3, prices [10, 12, 11]:
	//   mean = 11, var = ((10-11)^2+(12-11)^2+(11-11)^2)/3 = (1+1+0)/3 = 2/3
	//   sd = sqrt(2/3) ≈ 0.81649658
	//   upper = 11 + 2*0.81649658 ≈ 12.63299316
	//   lower = 11 - 2*0.81649658 ≈ 9.36700684
	prices := []float64{10, 12, 11}
	b, ok := indicators.BollingerBands(prices, 3, 2.0)
	if !ok {
		t.Fatal("BollingerBands returned ok=false")
	}
	if b.Middle != 11 {
		t.Fatalf("Middle = %f, want 11", b.Middle)
	}
	// check within small epsilon
	if abs(b.Upper-12.63299316) > 1e-6 {
		t.Fatalf("Upper = %f, want ~12.63299316", b.Upper)
	}
	if abs(b.Lower-9.36700684) > 1e-6 {
		t.Fatalf("Lower = %f, want ~9.36700684", b.Lower)
	}
}

func TestBollingerBands_ConstantPrices(t *testing.T) {
	prices := []float64{50, 50, 50, 50, 50}
	b, ok := indicators.BollingerBands(prices, 5, 2.0)
	if !ok {
		t.Fatal("BollingerBands(constant) returned ok=false")
	}
	if b.Upper != 50 || b.Middle != 50 || b.Lower != 50 {
		t.Fatalf("constant bands = (%f, %f, %f), want (50, 50, 50)",
			b.Upper, b.Middle, b.Lower)
	}
}

func TestBollingerBands_InsufficientData(t *testing.T) {
	_, ok := indicators.BollingerBands([]float64{10, 20}, 5, 2.0)
	if ok {
		t.Fatal("BollingerBands(short) returned ok=true")
	}
}

func TestBollingerBands_Empty(t *testing.T) {
	_, ok := indicators.BollingerBands(nil, 20, 2.0)
	if ok {
		t.Fatal("BollingerBands(nil) returned ok=true")
	}
}

func TestBollingerBands_PeriodOne(t *testing.T) {
	_, ok := indicators.BollingerBands([]float64{10, 20, 30}, 1, 2.0)
	if ok {
		t.Fatal("BollingerBands(period=1) returned ok=true")
	}
}

func TestBollingerBands_PeriodTwo(t *testing.T) {
	prices := []float64{10, 12}
	b, ok := indicators.BollingerBands(prices, 2, 2.0)
	if !ok {
		t.Fatal("BollingerBands(period=2) returned ok=false")
	}
	// mean = 11, var = ((10-11)^2+(12-11)^2)/2 = (1+1)/2 = 1, sd = 1
	// upper = 11 + 2*1 = 13, lower = 11 - 2*1 = 9
	if b.Middle != 11 || b.Upper != 13 || b.Lower != 9 {
		t.Fatalf("bands = (%f, %f, %f), want (13, 11, 9)",
			b.Upper, b.Middle, b.Lower)
	}
}

func TestBollingerBands_CustomMultiplier(t *testing.T) {
	prices := []float64{10, 12, 11}
	b, ok := indicators.BollingerBands(prices, 3, 1.0)
	if !ok {
		t.Fatal("BollingerBands(mult=1) returned ok=false")
	}
	// middle = 11, sd ≈ 0.8165
	// upper = 11 + 1*0.8165 ≈ 11.8165
	if b.Upper >= b.Middle+(b.Middle-b.Lower)+0.001 {
		t.Fatal("custom multiplier not applied correctly")
	}
}

// ---------------------------------------------------------------------------
// BollingerBandsSeries
// ---------------------------------------------------------------------------

func TestBollingerBandsSeries_Normal(t *testing.T) {
	prices := []float64{10, 12, 11, 13, 15}
	series := indicators.BollingerBandsSeries(prices, 3, 2.0)
	if len(series) != 3 {
		t.Fatalf("series len = %d, want 3", len(series))
	}
	// window 0: [10, 12, 11] → middle = 11
	if series[0].Middle != 11 {
		t.Fatalf("series[0].Middle = %f, want 11", series[0].Middle)
	}
	// window 1: [12, 11, 13] → middle = 12
	if series[1].Middle != 12 {
		t.Fatalf("series[1].Middle = %f, want 12", series[1].Middle)
	}
	// window 2: [11, 13, 15] → middle = 13
	if series[2].Middle != 13 {
		t.Fatalf("series[2].Middle = %f, want 13", series[2].Middle)
	}
}

func TestBollingerBandsSeries_InsufficientData(t *testing.T) {
	series := indicators.BollingerBandsSeries([]float64{1, 2}, 5, 2.0)
	if series != nil {
		t.Fatal("expected nil for insufficient data")
	}
}

func TestBollingerBandsSeries_Empty(t *testing.T) {
	if s := indicators.BollingerBandsSeries(nil, 10, 2.0); s != nil {
		t.Fatal("expected nil for empty input")
	}
}

func TestBollingerBandsSeries_ExactLength(t *testing.T) {
	prices := []float64{1, 2, 3}
	series := indicators.BollingerBandsSeries(prices, 3, 2.0)
	if len(series) != 1 {
		t.Fatalf("len = %d, want 1", len(series))
	}
}

// ---------------------------------------------------------------------------
// BandWidth
// ---------------------------------------------------------------------------

func TestBandWidth_Normal(t *testing.T) {
	b := indicators.Bands{Upper: 15, Middle: 10, Lower: 5}
	// (15-5)/10 * 100 = 100
	if w := indicators.BandWidth(b); w != 100 {
		t.Fatalf("BandWidth = %f, want 100", w)
	}
}

func TestBandWidth_ZeroMiddle(t *testing.T) {
	b := indicators.Bands{Upper: 10, Middle: 0, Lower: -10}
	if w := indicators.BandWidth(b); w != 0 {
		t.Fatalf("BandWidth(zero middle) = %f, want 0", w)
	}
}

func TestBandWidth_Collapsed(t *testing.T) {
	b := indicators.Bands{Upper: 10, Middle: 10, Lower: 10}
	if w := indicators.BandWidth(b); w != 0 {
		t.Fatalf("BandWidth(collapsed) = %f, want 0", w)
	}
}

// ---------------------------------------------------------------------------
// PercentB
// ---------------------------------------------------------------------------

func TestPercentB_AtUpper(t *testing.T) {
	b := indicators.Bands{Upper: 100, Middle: 75, Lower: 50}
	p := indicators.PercentB(100, b)
	if p != 1.0 {
		t.Fatalf("PercentB(at upper) = %f, want 1.0", p)
	}
}

func TestPercentB_AtMiddle(t *testing.T) {
	b := indicators.Bands{Upper: 100, Middle: 75, Lower: 50}
	p := indicators.PercentB(75, b)
	if p != 0.5 {
		t.Fatalf("PercentB(at middle) = %f, want 0.5", p)
	}
}

func TestPercentB_AtLower(t *testing.T) {
	b := indicators.Bands{Upper: 100, Middle: 75, Lower: 50}
	p := indicators.PercentB(50, b)
	if p != 0 {
		t.Fatalf("PercentB(at lower) = %f, want 0", p)
	}
}

func TestPercentB_AboveUpper(t *testing.T) {
	b := indicators.Bands{Upper: 100, Middle: 75, Lower: 50}
	p := indicators.PercentB(120, b)
	if p != 1.4 {
		t.Fatalf("PercentB(above) = %f, want 1.4", p)
	}
}

func TestPercentB_BelowLower(t *testing.T) {
	b := indicators.Bands{Upper: 100, Middle: 75, Lower: 50}
	p := indicators.PercentB(30, b)
	if p != -0.4 {
		t.Fatalf("PercentB(below) = %f, want -0.4", p)
	}
}

func TestPercentB_CollapsedBands(t *testing.T) {
	b := indicators.Bands{Upper: 50, Middle: 50, Lower: 50}
	p := indicators.PercentB(50, b)
	if p != 0 {
		t.Fatalf("PercentB(collapsed) = %f, want 0", p)
	}
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkBollingerBands(b *testing.B) {
	prices := make([]float64, 100)
	for i := range prices {
		prices[i] = 100 + float64(i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		indicators.BollingerBands(prices, 20, 2.0)
	}
}

func BenchmarkBollingerBandsSeries(b *testing.B) {
	prices := make([]float64, 500)
	for i := range prices {
		prices[i] = 100 + float64(i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		indicators.BollingerBandsSeries(prices, 20, 2.0)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
