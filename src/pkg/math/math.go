// Package math provides common statistical and arithmetic primitives used
// across the Navier-Stocks indicator and risk engines.  All functions work
// on pre-allocated float64 slices and return zero-valued results when input
// is degenerate (nil, empty, or below minimum cardinality).
package math

import stdmath "math"

// Sum returns the sum of vals.  Returns 0 for a nil or empty slice.
func Sum(vals []float64) float64 {
	var s float64
	for _, v := range vals {
		s += v
	}
	return s
}

// Mean returns the arithmetic mean of vals.  Returns 0 when len(vals) == 0.
func Mean(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	return Sum(vals) / float64(len(vals))
}

// Variance returns the population variance of vals.
// Returns 0 when len(vals) < 2 since variance is undefined for fewer
// than two observations.
func Variance(vals []float64) float64 {
	n := len(vals)
	if n < 2 {
		return 0
	}
	m := Mean(vals)
	var sumSq float64
	for _, v := range vals {
		d := v - m
		sumSq += d * d
	}
	return sumSq / float64(n)
}

// StdDev returns the population standard deviation (sqrt of Variance).
// Returns 0 when len(vals) < 2.
func StdDev(vals []float64) float64 {
	return stdmath.Sqrt(Variance(vals))
}

// SMA computes the simple moving average of vals over the given window.
// The result contains len(vals)-window+1 values where result[i] is the
// average of vals[i .. i+window-1].
// Returns nil when window < 1 or len(vals) < window.
func SMA(vals []float64, window int) []float64 {
	if window < 1 || len(vals) < window {
		return nil
	}
	result := make([]float64, len(vals)-window+1)

	var sum float64
	for i := 0; i < window; i++ {
		sum += vals[i]
	}
	result[0] = sum / float64(window)

	for i := window; i < len(vals); i++ {
		sum += vals[i] - vals[i-window]
		result[i-window+1] = sum / float64(window)
	}
	return result
}

// WeightedAverage computes Σ(vals[i] × weights[i]) / Σ(weights).
// Returns false when the slices have different lengths or when the
// total weight is zero.  When valid, the weighted average is returned
// with ok=true.
func WeightedAverage(vals, weights []float64) (avg float64, ok bool) {
	if len(vals) == 0 || len(vals) != len(weights) {
		return 0, false
	}
	var wSum, vwSum float64
	for i := range vals {
		wSum += weights[i]
		vwSum += vals[i] * weights[i]
	}
	if wSum == 0 {
		return 0, false
	}
	return vwSum / wSum, true
}
