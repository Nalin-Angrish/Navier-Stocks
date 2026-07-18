package groww

import (
	"fmt"
	"net/url"
	"strings"
)

// GetQuote fetches the full live-data snapshot for an instrument.
func (c *Client) GetQuote(exchange Exchange, seg Segment, symbol string) (*Quote, error) {
	path := fmt.Sprintf("/live-data/quote?exchange=%s&segment=%s&trading_symbol=%s",
		string(exchange), string(seg), url.PathEscape(symbol))
	resp, err := c.do("GET", path, nil, c.liveDataRL)
	if err != nil {
		return nil, err
	}
	var out Quote
	if err := decodeResponse(resp, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetLTP fetches the last traded price for one or more instruments.
// The symbols should be in "EXCHANGE_SYMBOL" format (e.g. "NSE_RELIANCE").
func (c *Client) GetLTP(seg Segment, symbols []string) (LTPResponse, error) {
	path := fmt.Sprintf("/live-data/ltp?segment=%s&exchange_symbols=%s",
		string(seg), url.QueryEscape(strings.Join(symbols, ",")))
	resp, err := c.do("GET", path, nil, c.liveDataRL)
	if err != nil {
		return nil, err
	}
	var out LTPResponse
	if err := decodeResponse(resp, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetOHLC fetches the OHLC data for one or more instruments.
func (c *Client) GetOHLC(seg Segment, symbols []string) (OHLCResponse, error) {
	path := fmt.Sprintf("/live-data/ohlc?segment=%s&exchange_symbols=%s",
		string(seg), url.QueryEscape(strings.Join(symbols, ",")))
	resp, err := c.do("GET", path, nil, c.liveDataRL)
	if err != nil {
		return nil, err
	}
	var out OHLCResponse
	if err := decodeResponse(resp, &out); err != nil {
		return nil, err
	}
	return out, nil
}
