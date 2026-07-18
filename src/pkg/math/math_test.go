// Package math_test exercises the statistical primitives used by indicator
// engines.  Tests cover normal cases, edge cases, and degenerate inputs
// (nil, empty, single-element, constant, negative values).
package math_test

import (
	"testing"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/math"
)

// ---------------------------------------------------------------------------
// Sum
// ---------------------------------------------------------------------------

func TestSum_Normal(t *testing.T) {
	if got := math.Sum([]float64{1, 2, 3, 4, 5}); got != 15 {
		t.Fatalf("Sum = %f, want 15", got)
	}
}

func TestSum_Negative(t *testing.T) {
	if got := math.Sum([]float64{-1, -2, 3}); got != 0 {
		t.Fatalf("Sum = %f, want 0", got)
	}
}

func TestSum_Empty(t *testing.T) {
	if got := math.Sum(nil); got != 0 {
		t.Fatalf("Sum(nil) = %f, want 0", got)
	}
	if got := math.Sum([]float64{}); got != 0 {
		t.Fatalf("Sum(empty) = %f, want 0", got)
	}
}

func TestSum_Single(t *testing.T) {
	if got := math.Sum([]float64{42}); got != 42 {
		t.Fatalf("Sum = %f, want 42", got)
	}
}

// ---------------------------------------------------------------------------
// Mean
// ---------------------------------------------------------------------------

func TestMean_Normal(t *testing.T) {
	if got := math.Mean([]float64{2, 4, 6, 8}); got != 5 {
		t.Fatalf("Mean = %f, want 5", got)
	}
}

func TestMean_Empty(t *testing.T) {
	if got := math.Mean(nil); got != 0 {
		t.Fatalf("Mean(nil) = %f, want 0", got)
	}
}

func TestMean_Single(t *testing.T) {
	if got := math.Mean([]float64{3.14}); got != 3.14 {
		t.Fatalf("Mean = %f, want 3.14", got)
	}
}

func TestMean_Fractional(t *testing.T) {
	if got := math.Mean([]float64{1, 2}); got != 1.5 {
		t.Fatalf("Mean = %f, want 1.5", got)
	}
}

// ---------------------------------------------------------------------------
// Variance
// ---------------------------------------------------------------------------

func TestVariance_Normal(t *testing.T) {
	// population variance of {2, 4, 4, 4, 5, 5, 7, 9}: mean = 5
	// ((3^2)+(1^2)+(1^2)+(1^2)+(0^2)+(0^2)+(2^2)+(4^2))/8 = 32/8 = 4
	vals := []float64{2, 4, 4, 4, 5, 5, 7, 9}
	if got := math.Variance(vals); got != 4 {
		t.Fatalf("Variance = %f, want 4", got)
	}
}

func TestVariance_Zero(t *testing.T) {
	vals := []float64{5, 5, 5, 5}
	if got := math.Variance(vals); got != 0 {
		t.Fatalf("constant Variance = %f, want 0", got)
	}
}

func TestVariance_Empty(t *testing.T) {
	if got := math.Variance(nil); got != 0 {
		t.Fatalf("Variance(nil) = %f, want 0", got)
	}
}

func TestVariance_Single(t *testing.T) {
	if got := math.Variance([]float64{42}); got != 0 {
		t.Fatalf("Variance(single) = %f, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// StdDev
// ---------------------------------------------------------------------------

func TestStdDev_Normal(t *testing.T) {
	// variance = 4 => stddev = 2
	vals := []float64{2, 4, 4, 4, 5, 5, 7, 9}
	if got := math.StdDev(vals); got != 2 {
		t.Fatalf("StdDev = %f, want 2", got)
	}
}

func TestStdDev_Zero(t *testing.T) {
	if got := math.StdDev([]float64{3, 3, 3}); got != 0 {
		t.Fatalf("StdDev(constant) = %f, want 0", got)
	}
}

func TestStdDev_Empty(t *testing.T) {
	if got := math.StdDev(nil); got != 0 {
		t.Fatalf("StdDev(nil) = %f, want 0", got)
	}
}

func TestStdDev_Single(t *testing.T) {
	if got := math.StdDev([]float64{100}); got != 0 {
		t.Fatalf("StdDev(single) = %f, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// SMA
// ---------------------------------------------------------------------------

func TestSMA_Normal(t *testing.T) {
	vals := []float64{1, 2, 3, 4, 5}
	got := math.SMA(vals, 3)
	expected := []float64{2, 3, 4}
	if len(got) != len(expected) {
		t.Fatalf("SMA len = %d, want %d", len(got), len(expected))
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Fatalf("SMA[%d] = %f, want %f", i, got[i], expected[i])
		}
	}
}

func TestSMA_WindowEqualsLength(t *testing.T) {
	vals := []float64{1, 2, 3}
	got := math.SMA(vals, 3)
	if len(got) != 1 || got[0] != 2 {
		t.Fatalf("SMA(full window) = %v, want [2]", got)
	}
}

func TestSMA_WindowLargerThanLength(t *testing.T) {
	if got := math.SMA([]float64{1, 2}, 5); got != nil {
		t.Fatalf("SMA(short) = %v, want nil", got)
	}
}

func TestSMA_Empty(t *testing.T) {
	if got := math.SMA(nil, 10); got != nil {
		t.Fatalf("SMA(nil) = %v, want nil", got)
	}
	if got := math.SMA([]float64{}, 5); got != nil {
		t.Fatalf("SMA(empty) = %v, want nil", got)
	}
}

func TestSMA_ZeroWindow(t *testing.T) {
	if got := math.SMA([]float64{1, 2, 3}, 0); got != nil {
		t.Fatalf("SMA(window=0) = %v, want nil", got)
	}
	if got := math.SMA([]float64{1, 2, 3}, -1); got != nil {
		t.Fatalf("SMA(window=-1) = %v, want nil", got)
	}
}

func TestSMA_WindowOne(t *testing.T) {
	vals := []float64{1, 2, 3, 4}
	got := math.SMA(vals, 1)
	if len(got) != 4 {
		t.Fatalf("SMA(window=1) len = %d, want 4", len(got))
	}
	for i := range vals {
		if got[i] != vals[i] {
			t.Fatalf("SMA(window=1)[%d] = %f, want %f", i, got[i], vals[i])
		}
	}
}

func TestSMA_Constant(t *testing.T) {
	vals := []float64{5, 5, 5, 5, 5}
	got := math.SMA(vals, 3)
	for i, v := range got {
		if v != 5 {
			t.Fatalf("SMA(constant)[%d] = %f, want 5", i, v)
		}
	}
}

// ---------------------------------------------------------------------------
// WeightedAverage
// ---------------------------------------------------------------------------

func TestWeightedAverage_Normal(t *testing.T) {
	// (10*1 + 20*2 + 30*3) / (1+2+3) = (10+40+90)/6 = 140/6 ≈ 23.333
	vals := []float64{10, 20, 30}
	wts := []float64{1, 2, 3}
	got, ok := math.WeightedAverage(vals, wts)
	if !ok {
		t.Fatal("WeightedAverage returned ok=false")
	}
	if got != 140.0/6.0 {
		t.Fatalf("WeightedAverage = %f, want %f", got, 140.0/6.0)
	}
}

func TestWeightedAverage_EqualWeights(t *testing.T) {
	vals := []float64{10, 20, 30}
	wts := []float64{1, 1, 1}
	got, ok := math.WeightedAverage(vals, wts)
	if !ok {
		t.Fatal("WeightedAverage returned ok=false")
	}
	if got != 20 {
		t.Fatalf("WeightedAverage(equal) = %f, want 20", got)
	}
}

func TestWeightedAverage_ZeroWeights(t *testing.T) {
	vals := []float64{10, 20}
	wts := []float64{0, 0}
	_, ok := math.WeightedAverage(vals, wts)
	if ok {
		t.Fatal("WeightedAverage(zero weights) returned ok=true")
	}
}

func TestWeightedAverage_MismatchedLength(t *testing.T) {
	_, ok := math.WeightedAverage([]float64{1, 2}, []float64{1})
	if ok {
		t.Fatal("WeightedAverage(mismatched) returned ok=true")
	}
}

func TestWeightedAverage_Empty(t *testing.T) {
	_, ok := math.WeightedAverage(nil, nil)
	if ok {
		t.Fatal("WeightedAverage(nil) returned ok=true")
	}
	_, ok = math.WeightedAverage([]float64{}, []float64{})
	if ok {
		t.Fatal("WeightedAverage(empty) returned ok=true")
	}
}

func TestWeightedAverage_Single(t *testing.T) {
	got, ok := math.WeightedAverage([]float64{42}, []float64{1})
	if !ok || got != 42 {
		t.Fatalf("WeightedAverage(single) = %f, want 42", got)
	}
}

func TestWeightedAverage_NegativeWeight(t *testing.T) {
	// negative weights are allowed mathematically but unusual
	vals := []float64{10, 20}
	wts := []float64{5, -2}
	got, ok := math.WeightedAverage(vals, wts)
	// (10*5 + 20*(-2)) / (5-2) = (50-40)/3 = 10/3 ≈ 3.333
	if !ok {
		t.Fatal("WeightedAverage returned ok=false")
	}
	if got != 10.0/3.0 {
		t.Fatalf("WeightedAverage(negative w) = %f, want %f", got, 10.0/3.0)
	}
}
