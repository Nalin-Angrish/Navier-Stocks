// Package quantitative implements the Quantitative Scout agent — the
// real-time market-data ingestor that maintains per-ticker in-memory ring
// buffers, computes VWAP / Bollinger Band / volume-breakout signals, and
// publishes trade intents to NATS.  This file provides the lock-free-ish
// ring-buffer storage that sits at the heart of the fast path.
package quantitative

import (
	"sync"
	"time"
)

// Tick represents a single market-data point pushed from the exchange
// WebSocket feed.  It carries the price, volume, and exchange timestamp
// for one ticker at one moment in time.
type Tick struct {
	Price     float64   `json:"price"`
	Volume    int64     `json:"volume"`
	Timestamp time.Time `json:"timestamp"`
}

// Candle is an OHLCV aggregation produced by TickStore.Candles.  It
// summarises the open, high, low, close prices and cumulative volume
// over a fixed time window.
type Candle struct {
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume int64   `json:"volume"`
}

// TickStore is a fixed-size ring buffer for market ticks.  Once the buffer
// is full each new Append evicts the oldest tick.  The store is safe for
// concurrent use by one writer (the WebSocket goroutine) and any number of
// readers (indicator engines, query code).
type TickStore struct {
	mu    sync.RWMutex
	buf   []Tick
	size  int
	write int  // next slot to write; points past the last written slot
	full  bool // true once the buffer has wrapped at least once
}

// NewTickStore creates a ring buffer that retains the last n ticks.  If n is
// less than 1 it defaults to 1 so the buffer is never degenerate.
func NewTickStore(n int) *TickStore {
	if n < 1 {
		n = 1
	}
	return &TickStore{
		buf:  make([]Tick, n),
		size: n,
	}
}

// Append inserts a tick into the ring buffer.  If the buffer is full the
// oldest tick is silently evicted.  The caller must not modify t after
// Append returns.
func (ts *TickStore) Append(t Tick) {
	ts.mu.Lock()
	ts.buf[ts.write] = t
	ts.write++
	if ts.write == ts.size {
		ts.write = 0
		ts.full = true
	}
	ts.mu.Unlock()
}

// Len returns the number of ticks currently stored.  Once the buffer has
// filled and wrapped this is always equal to Cap.
func (ts *TickStore) Len() int {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	if ts.full {
		return ts.size
	}
	return ts.write
}

// Cap returns the maximum number of ticks the ring buffer can hold.
func (ts *TickStore) Cap() int { return ts.size }

// RecentPrices returns the last n closing prices in chronological order
// (oldest first).  If fewer than n ticks are available all of them are
// returned.  Returns nil when n <= 0.
func (ts *TickStore) RecentPrices(n int) []float64 {
	if n <= 0 {
		return nil
	}
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	count := ts.count()
	if n > count {
		n = count
	}
	result := make([]float64, n)
	for i := 0; i < n; i++ {
		result[i] = ts.at(count - n + i).Price
	}
	return result
}

// RecentVolumes returns the last n volume values in chronological order.
// Semantics are identical to RecentPrices.
func (ts *TickStore) RecentVolumes(n int) []int64 {
	if n <= 0 {
		return nil
	}
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	count := ts.count()
	if n > count {
		n = count
	}
	result := make([]int64, n)
	for i := 0; i < n; i++ {
		result[i] = ts.at(count - n + i).Volume
	}
	return result
}

// Candles aggregates the stored ticks into OHLCV candles of the given
// duration.  Each tick is placed into the bucket floor(t.UnixNano() / dur).
// Candles are returned in chronological order.  Returns nil when there are
// no ticks.
func (ts *TickStore) Candles(dur time.Duration) []Candle {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	count := ts.count()
	if count == 0 {
		return nil
	}

	// acc accumulates the OHLCV state for a single time bucket.
	type acc struct {
		candle  Candle
		id      int64
		openSet bool
	}
	var buckets []acc

	for i := 0; i < count; i++ {
		t := ts.at(i)
		bid := t.Timestamp.UnixNano() / int64(dur)
		if len(buckets) == 0 || buckets[len(buckets)-1].id != bid {
			buckets = append(buckets, acc{
				candle: Candle{
					Open:   t.Price,
					High:   t.Price,
					Low:    t.Price,
					Close:  t.Price,
					Volume: t.Volume,
				},
				id:      bid,
				openSet: true,
			})
			continue
		}
		cur := &buckets[len(buckets)-1]
		if t.Price > cur.candle.High {
			cur.candle.High = t.Price
		}
		if t.Price < cur.candle.Low {
			cur.candle.Low = t.Price
		}
		cur.candle.Close = t.Price
		cur.candle.Volume += t.Volume
	}

	result := make([]Candle, len(buckets))
	for i, b := range buckets {
		result[i] = b.candle
	}
	return result
}

// count returns the number of ticks available.  The caller must hold at
// least an RLock.
func (ts *TickStore) count() int {
	if ts.full {
		return ts.size
	}
	return ts.write
}

// at returns the i-th tick in insertion order (0 = oldest).  The caller
// must hold at least an RLock.
func (ts *TickStore) at(i int) Tick {
	if ts.full {
		return ts.buf[(ts.write+i)%ts.size]
	}
	return ts.buf[i]
}

// UpdateLatestVolume overwrites the volume of the most recent tick in the
// ring buffer.  This is used by the REST quote poller to enrich LTP-only
// ticks with real volume data without appending a duplicate price point.
// It is a no-op when the buffer is empty.
func (ts *TickStore) UpdateLatestVolume(vol int64) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	count := ts.count()
	if count == 0 {
		return
	}
	idx := ts.write - 1
	if idx < 0 {
		idx = ts.size - 1
	}
	ts.buf[idx].Volume = vol
}

// DefaultMaxTickers is the maximum number of distinct ticker symbols the
// TickerStore will track unless overridden.  This caps per-process memory
// usage from ring buffers.
const DefaultMaxTickers = 25

// TickerStore manages one TickStore per symbol, enforcing a configurable
// maximum number of tracked symbols so that memory usage is bounded.
type TickerStore struct {
	mu     sync.RWMutex
	stores map[string]*TickStore
	max    int // soft limit on distinct symbols
	bufCap int // TickStore capacity handed to each new store
}

// NewTickerStore creates a TickerStore that tracks at most max tickers.
// Each per-ticker ring buffer is created with the given bufCap.  When max
// is zero or negative DefaultMaxTickers is used; when bufCap is zero or
// negative 256 is used.
func NewTickerStore(max, bufCap int) *TickerStore {
	if max < 1 {
		max = DefaultMaxTickers
	}
	if bufCap < 1 {
		bufCap = 256
	}
	return &TickerStore{
		stores: make(map[string]*TickStore, max),
		max:    max,
		bufCap: bufCap,
	}
}

// Store returns the TickStore for the given symbol, creating one if it does
// not yet exist.  It returns nil when the maximum number of tickers has
// already been reached.
func (ts *TickerStore) Store(symbol string) *TickStore {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if s, ok := ts.stores[symbol]; ok {
		return s
	}
	if len(ts.stores) >= ts.max {
		return nil
	}
	s := NewTickStore(ts.bufCap)
	ts.stores[symbol] = s
	return s
}

// Get returns the TickStore for the given symbol, or nil if the symbol is
// not currently being tracked.
func (ts *TickerStore) Get(symbol string) *TickStore {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.stores[symbol]
}

// Symbols returns a snapshot of all currently tracked ticker symbols in
// no particular order.
func (ts *TickerStore) Symbols() []string {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	syms := make([]string, 0, len(ts.stores))
	for s := range ts.stores {
		syms = append(syms, s)
	}
	return syms
}

// Len returns the number of ticker symbols currently being tracked.
func (ts *TickerStore) Len() int {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return len(ts.stores)
}

// Cap returns the maximum number of tickers this store will track.
func (ts *TickerStore) Cap() int { return ts.max }
