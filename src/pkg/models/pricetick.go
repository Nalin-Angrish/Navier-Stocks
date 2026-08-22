package models

// PriceTick is the lightweight last-traded-price stream published by the
// Quantitative Scout on signal.price.<TICKER>.  It exists so downstream
// agents (the Trader Gateway's intraday exit monitor) can react to price
// movement without scraping the scout's in-memory ring buffers.
type PriceTick struct {
	Ticker     string  `json:"ticker"`
	Price      float64 `json:"price"`
	TsInMillis int64   `json:"ts_millis"`
}
