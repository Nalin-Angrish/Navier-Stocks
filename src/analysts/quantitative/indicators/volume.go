package indicators

import "github.com/Nalin-Angrish/Navier-Stocks/src/pkg/math"

// DefaultVolumeWindow is the standard rolling window (in ticks or candles)
// for computing the volume SMA that serves as the breakout baseline.
const DefaultVolumeWindow = 10

// DefaultVolumeThreshold is the minimum ratio of current volume to volume
// SMA required to trigger a breakout signal.  A value of 2.0 means the
// current volume must be at least double the average.
const DefaultVolumeThreshold = 2.0

// VolumeResult summarises a volume-breakout check for a single tick or
// candle.
type VolumeResult struct {
	IsBreakout    bool    // true when current volume ≥ threshold × SMA
	CurrentVolume int64   // the most recent volume value checked
	VolumeSMA     float64 // SMA of the rolling window (excluding current)
	Ratio         float64 // current volume / volume SMA (0 when SMA is 0)
	Window        int     // the window size used
}

// VolumeBreakout checks whether the most recent volume value constitutes
// a breakout by comparing it against the simple moving average of the
// preceding window volumes.
//
//   - volumes: ordered oldest to newest; the LAST element is treated as the
//     "current" volume to test.
//   - window:  number of prior observations used for the SMA baseline.
//     Must be ≥ 1.
//   - threshold:  the minimum ratio of current / SMA required for a breakout
//     (e.g. 2.0 means 2× the average).
//
// Returns a VolumeResult with IsBreakout=false when:
//   - volumes has fewer than window+1 entries (not enough history)
//   - window < 1
//   - volume SMA is zero (can't compute a meaningful ratio)
//
// The SMA is computed over volumes[0 : len-1] (all but the last element)
// when window exceeds the available history, the entire history is used.
func VolumeBreakout(volumes []int64, window int, threshold float64) VolumeResult {
	if len(volumes) < 2 || window < 1 || threshold <= 0 {
		return VolumeResult{Window: window}
	}

	current := volumes[len(volumes)-1]

	// use up to window prior values for the SMA baseline
	lookback := len(volumes) - 1
	if lookback > window {
		lookback = window
	}

	baseline := make([]float64, lookback)
	for i := 0; i < lookback; i++ {
		baseline[i] = float64(volumes[len(volumes)-1-lookback+i])
	}

	sma := math.Mean(baseline)
	if sma == 0 {
		return VolumeResult{
			CurrentVolume: current,
			VolumeSMA:     0,
			Ratio:         0,
			Window:        window,
		}
	}

	ratio := float64(current) / sma
	return VolumeResult{
		IsBreakout:    ratio >= threshold,
		CurrentVolume: current,
		VolumeSMA:     sma,
		Ratio:         ratio,
		Window:        window,
	}
}
