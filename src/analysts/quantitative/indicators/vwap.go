// Package indicators implements the technical-analysis engines used by the
// Quantitative Scout to detect momentum, volatility, and volume signals from
// real-time tick data.  Each indicator is self-contained, stateless, and
// operates on pre-extracted float64 slices so that callers can feed data
// directly from TickStore queries without coupling to the storage layer.
package indicators

// VWAPResult carries the Volume-Weighted Average Price and supporting
// aggregates so callers can inspect cumulative volume and the constituent
// sums that produced the result.
type VWAPResult struct {
	VWAP        float64 // the volume-weighted average price
	TotalVolume float64 // sum of all volume (as float64 for division)
	PriceVolume float64 // sum of price × volume
	Valid       bool    // true when enough data was provided
}

// VWAP computes the Volume-Weighted Average Price from parallel slices of
// prices and volumes (e.g. Tick.Price and Tick.Volume extracted from a
// TickStore).
//
//	VWAP = Σ(price_i × volume_i) / Σ(volume_i)
//
// Returns a VWAPResult with Valid=false when:
//   - either slice is empty
//   - lengths differ
//   - total volume is zero (no meaningful average)
func VWAP(prices []float64, volumes []int64) VWAPResult {
	if len(prices) == 0 || len(volumes) == 0 || len(prices) != len(volumes) {
		return VWAPResult{Valid: false}
	}

	var pvSum float64
	var volSum float64
	for i := range prices {
		v := float64(volumes[i])
		pvSum += prices[i] * v
		volSum += v
	}

	if volSum == 0 {
		return VWAPResult{Valid: false}
	}

	return VWAPResult{
		VWAP:        pvSum / volSum,
		TotalVolume: volSum,
		PriceVolume: pvSum,
		Valid:       true,
	}
}

// VWAPFromCandles computes VWAP from a slice of OHLCV candles.  It uses
// the closing price as the representative price for each candle.
//
//	VWAP = Σ(close_i × volume_i) / Σ(volume_i)
//
// Returns a VWAPResult with Valid=false when the input is empty or all
// volumes are zero.
func VWAPFromCandles(candles []Candle) VWAPResult {
	if len(candles) == 0 {
		return VWAPResult{Valid: false}
	}

	prices := make([]float64, len(candles))
	volumes := make([]int64, len(candles))
	for i, c := range candles {
		prices[i] = c.Close
		volumes[i] = c.Volume
	}
	return VWAP(prices, volumes)
}

// VWAPFromCandlesTypical uses the typical price per candle instead of
// closing price alone:
//
//	typical_price = (high + low + close) / 3
//
// This is a common alternative that better reflects the candle's overall
// price action.
func VWAPFromCandlesTypical(candles []Candle) VWAPResult {
	if len(candles) == 0 {
		return VWAPResult{Valid: false}
	}

	prices := make([]float64, len(candles))
	volumes := make([]int64, len(candles))
	for i, c := range candles {
		prices[i] = (c.High + c.Low + c.Close) / 3.0
		volumes[i] = c.Volume
	}
	return VWAP(prices, volumes)
}

// CumulativeVWAP tracks a running VWAP calculation over a trading session.
// Each call to Add incorporates a new price-volume observation and returns
// the updated VWAP.  This is useful for real-time feeds where ticks arrive
// incrementally.
type CumulativeVWAP struct {
	pvSum float64
	vSum  float64
}

// NewCumulativeVWAP returns an initialised CumulativeVWAP with no
// observations.
func NewCumulativeVWAP() *CumulativeVWAP {
	return &CumulativeVWAP{}
}

// Add incorporates one price-volume observation and returns the updated
// VWAP.  The result is valid (non-NaN) whenever at least one observation
// with positive volume has been added.
func (cv *CumulativeVWAP) Add(price float64, volume int64) float64 {
	v := float64(volume)
	cv.pvSum += price * v
	cv.vSum += v
	if cv.vSum == 0 {
		return 0
	}
	return cv.pvSum / cv.vSum
}

// Value returns the current VWAP without modifying state.  Returns 0 when
// no observations have been recorded.
func (cv *CumulativeVWAP) Value() float64 {
	if cv.vSum == 0 {
		return 0
	}
	return cv.pvSum / cv.vSum
}

// Observations returns the cumulative price-volume sum and volume sum for
// inspection or serialisation.
func (cv *CumulativeVWAP) Observations() (priceVolumeSum, volumeSum float64) {
	return cv.pvSum, cv.vSum
}

// VWAPBand returns the distance of the current price from the VWAP line
// as both an absolute value and a percentage.
//
//	deviation = price - vwap
//	deviationPct = (price / vwap - 1) × 100
//
// Both are zero when vwap is zero to avoid division by zero.
func VWAPBand(price, vwap float64) (deviation, deviationPct float64) {
	if vwap == 0 {
		return 0, 0
	}
	deviation = price - vwap
	deviationPct = (price/vwap - 1) * 100
	return
}

// Candle is a copy of the quantitative.Candle type used by indicator
// functions so that the indicators package does not depend on the
// quantitative package (avoiding circular imports).
type Candle struct {
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume int64
}

// CandleFromTick is a helper comment documenting that tick-level data
// should first be aggregated via TickStore.Candles() before passing to
// VWAPFromCandles or VWAPFromCandlesTypical.
//
//lint:ignore U1000 this type alias documents the expected input contract.
type _ = Candle
