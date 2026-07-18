package groww

import (
	"fmt"
	"net/url"
	"time"
)

// GetHistoricalCandles fetches historical candle data for an instrument.
// The interval determines the candle width (see CandleInterval constants).
func (c *Client) GetHistoricalCandles(exchange Exchange, seg Segment, symbol string, start, end time.Time, interval CandleInterval) ([]HistoricalCandle, error) {
	path := fmt.Sprintf("/historical/candle/range?exchange=%s&segment=%s&trading_symbol=%s&start_time=%s&end_time=%s&interval_in_minutes=%s",
		string(exchange),
		string(seg),
		url.PathEscape(symbol),
		url.QueryEscape(start.Format("2006-01-02 15:04:05")),
		url.QueryEscape(end.Format("2006-01-02 15:04:05")),
		url.QueryEscape(string(interval)),
	)
	resp, err := c.do("GET", path, nil, c.nonTradingRL)
	if err != nil {
		return nil, err
	}

	var envelope struct {
		Candles [][]any `json:"candles"`
	}
	if err := decodeResponse(resp, &envelope); err != nil {
		return nil, err
	}

	candles := make([]HistoricalCandle, 0, len(envelope.Candles))
	for _, raw := range envelope.Candles {
		if len(raw) < 6 {
			continue
		}
		ts, _ := toInt64(raw[0])
		open, _ := toFloat64(raw[1])
		high, _ := toFloat64(raw[2])
		low, _ := toFloat64(raw[3])
		close_, _ := toFloat64(raw[4])
		vol, _ := toInt64(raw[5])
		candles = append(candles, HistoricalCandle{
			Timestamp: ts,
			Open:      open,
			High:      high,
			Low:       low,
			Close:     close_,
			Volume:    int(vol),
		})
	}
	return candles, nil
}

func toInt64(v any) (int64, bool) {
	switch x := v.(type) {
	case float64:
		return int64(x), true
	case int64:
		return x, true
	case int:
		return int64(x), true
	default:
		return 0, false
	}
}

func toFloat64(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int64:
		return float64(x), true
	case int:
		return float64(x), true
	default:
		return 0, false
	}
}
