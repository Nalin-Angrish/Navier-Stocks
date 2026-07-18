package indicators

import "github.com/Nalin-Angrish/Navier-Stocks/src/pkg/math"

// DefaultBollingerPeriod is the standard 20-period window used by most
// trading platforms for Bollinger Bands.
const DefaultBollingerPeriod = 20

// DefaultBollingerMultiplier is the standard 2 standard-deviation width.
const DefaultBollingerMultiplier = 2.0

// Bands holds the three Bollinger Band lines for a single point in time,
// typically computed over the most recent N closing prices.
type Bands struct {
	Upper  float64 // middle + multiplier × σ
	Middle float64 // SMA of the window
	Lower  float64 // middle − multiplier × σ
}

// BollingerBands computes the upper, middle, and lower Bollinger Bands
// from the most recent closing prices using the given window period and
// standard-deviation multiplier.
//
//   - period:     rolling window length (e.g. 20).  Must be ≥ 2.
//   - multiplier: number of standard deviations from the mean (e.g. 2.0).
//
// Returns a Bands value and ok=true on success.  ok=false when there are
// fewer than period prices or period < 2.
//
// Standard interpretation:
//   - Upper = SMA(period) + multiplier × σ_population(period)
//   - Lower = SMA(period) − multiplier × σ_population(period)
func BollingerBands(prices []float64, period int, multiplier float64) (Bands, bool) {
	if period < 2 || len(prices) < period {
		return Bands{}, false
	}

	window := prices[len(prices)-period:]
	middle := math.Mean(window)
	sd := math.StdDev(window)

	return Bands{
		Upper:  middle + multiplier*sd,
		Middle: middle,
		Lower:  middle - multiplier*sd,
	}, true
}

// BollingerBandsSeries computes a full series of Bollinger Bands values
// over the entire price slice, producing one entry per valid window.
// The i-th result corresponds to prices[i : i+period].
//
// Returns nil when period < 2 or len(prices) < period.
func BollingerBandsSeries(prices []float64, period int, multiplier float64) []Bands {
	if period < 2 || len(prices) < period {
		return nil
	}

	result := make([]Bands, len(prices)-period+1)
	for i := 0; i <= len(prices)-period; i++ {
		window := prices[i : i+period]
		middle := math.Mean(window)
		sd := math.StdDev(window)
		result[i] = Bands{
			Upper:  middle + multiplier*sd,
			Middle: middle,
			Lower:  middle - multiplier*sd,
		}
	}
	return result
}

// BandWidth returns the relative width of the Bollinger Bands as a
// percentage of the middle band:
//
//	width = (upper − lower) / middle × 100
//
// Returns 0 when middle is 0 to avoid division by zero.
func BandWidth(b Bands) float64 {
	if b.Middle == 0 {
		return 0
	}
	return (b.Upper - b.Lower) / b.Middle * 100
}

// PercentB computes the %b indicator, which shows the position of a price
// relative to the Bollinger Bands:
//
//	%b = (price − lower) / (upper − lower)
//
// A value of 1.0 means price is at the upper band, 0.0 at the lower band,
// < 0 below the lower band, and > 1 above the upper band.
// Returns 0 when upper == lower (bands are collapsed).
func PercentB(price float64, b Bands) float64 {
	denom := b.Upper - b.Lower
	if denom == 0 {
		return 0
	}
	return (price - b.Lower) / denom
}
