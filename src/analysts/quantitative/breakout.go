package quantitative

import (
	"encoding/json"
	"log"
	"time"

	"github.com/Nalin-Angrish/Navier-Stocks/src/analysts/quantitative/indicators"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/nats"
)

// DefaultEvalInterval is how often the breakout detector evaluates every
// tracked symbol for signal conditions.
const DefaultEvalInterval = 60 * time.Second

// BreakoutConfig tunes the sensitivity of the breakout detector.
type BreakoutConfig struct {
	// BollingerPeriod is the rolling window for Bollinger Bands (default 20).
	BollingerPeriod int
	// BollingerMultiplier is the number of standard deviations (default 2.0).
	BollingerMultiplier float64
	// VolumeWindow is the rolling window for volume SMA (default 10).
	VolumeWindow int
	// VolumeThreshold is the minimum ratio of current/SMA (default 2.0).
	VolumeThreshold float64
	// EvalInterval is how often the detector runs (default 60s).
	EvalInterval time.Duration
	// CandleDuration is the aggregation window for OHLC candles (default 1min).
	CandleDuration time.Duration
}

// DefaultBreakoutConfig returns a BreakoutConfig with standard parameters.
func DefaultBreakoutConfig() BreakoutConfig {
	return BreakoutConfig{
		BollingerPeriod:     20,
		BollingerMultiplier: 2.0,
		VolumeWindow:        10,
		VolumeThreshold:     2.0,
		EvalInterval:        DefaultEvalInterval,
		CandleDuration:      time.Minute,
	}
}

// BreakoutDetector periodically evaluates every symbol in the universe
// against VWAP / Bollinger Band / volume-breakout conditions and publishes
// TradeIntent messages to NATS when a setup is detected.
type BreakoutDetector struct {
	js       *nats.JetStream
	ts       *TickerStore
	universe *Universe
	cfg      BreakoutConfig
	stopChan chan struct{}
}

// NewBreakoutDetector creates a detector that reads from the shared ticker
// store and publishes intents via the given JetStream context.
func NewBreakoutDetector(js *nats.JetStream, ts *TickerStore, u *Universe, cfg BreakoutConfig) *BreakoutDetector {
	return &BreakoutDetector{
		js:       js,
		ts:       ts,
		universe: u,
		cfg:      cfg,
		stopChan: make(chan struct{}),
	}
}

// Start launches the periodic evaluation loop in a background goroutine.
func (bd *BreakoutDetector) Start() {
	go bd.evalLoop()
}

// Stop signals the evaluation loop to exit.
func (bd *BreakoutDetector) Stop() {
	close(bd.stopChan)
}

// evalLoop ticks on the configured interval and evaluates all symbols.
func (bd *BreakoutDetector) evalLoop() {
	ticker := time.NewTicker(bd.cfg.EvalInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			bd.evaluateAll()
		case <-bd.stopChan:
			return
		}
	}
}

// evaluateAll iterates every tracked symbol and attempts to generate intents.
func (bd *BreakoutDetector) evaluateAll() {
	for _, symbol := range bd.universe.Symbols {
		store := bd.ts.Get(symbol)
		if store == nil || store.Len() < 2 {
			continue
		}
		intent := bd.evaluate(symbol, store)
		if intent == nil {
			continue
		}
		bd.publish(intent)
	}
}

// evaluate checks whether the given symbol's latest data triggers a breakout
// signal.  Returns nil when conditions are not met or data is insufficient.
func (bd *BreakoutDetector) evaluate(symbol string, store *TickStore) *models.TradeIntent {
	candles := store.Candles(bd.cfg.CandleDuration)
	if len(candles) < 2 {
		return nil
	}
	last := candles[len(candles)-1]

	// -- VWAP from candles ---------------------------------------------------
	vwapRes := indicators.VWAPFromCandles(toIndicatorCandles(candles))
	if !vwapRes.Valid {
		return nil
	}

	// -- Bollinger Bands -----------------------------------------------------
	recentPrices := store.RecentPrices(bd.cfg.BollingerPeriod)
	if len(recentPrices) < bd.cfg.BollingerPeriod {
		return nil
	}
	bands, ok := indicators.BollingerBands(recentPrices, bd.cfg.BollingerPeriod, bd.cfg.BollingerMultiplier)
	if !ok {
		return nil
	}

	// -- Volume Breakout -----------------------------------------------------
	recentVolumes := store.RecentVolumes(bd.cfg.VolumeWindow + 1)
	if len(recentVolumes) < bd.cfg.VolumeWindow+1 {
		return nil
	}
	volResult := indicators.VolumeBreakout(recentVolumes, bd.cfg.VolumeWindow, bd.cfg.VolumeThreshold)

	// -- Signal logic --------------------------------------------------------
	closePrice := last.Close
	aboveVWAP := closePrice > vwapRes.VWAP
	belowVWAP := closePrice < vwapRes.VWAP
	aboveUpper := closePrice > bands.Upper
	belowLower := closePrice < bands.Lower

	// Long: close decisively above VWAP + volume confirmation + above upper BB.
	if aboveVWAP && volResult.IsBreakout && aboveUpper {
		return &models.TradeIntent{
			Ticker:       symbol,
			Side:         models.SideLong,
			CurrentPrice: closePrice,
			VWAP:         vwapRes.VWAP,
			VolumeRatio:  volResult.Ratio,
			SignalReason: "VWAP_breakout_plus_volume_plus_BB",
			Timestamp:    time.Now(),
		}
	}

	// Short: close decisively below VWAP + volume confirmation + below lower BB.
	if belowVWAP && volResult.IsBreakout && belowLower {
		return &models.TradeIntent{
			Ticker:       symbol,
			Side:         models.SideShort,
			CurrentPrice: closePrice,
			VWAP:         vwapRes.VWAP,
			VolumeRatio:  volResult.Ratio,
			SignalReason: "VWAP_breakout_plus_volume_plus_BB",
			Timestamp:    time.Now(),
		}
	}

	return nil
}

// toIndicatorCandles converts the quantitative.Candle slice to the local
// indicators.Candle type so the indicators package can be used without
// importing the indicators package's Candle type directly.
func toIndicatorCandles(src []Candle) []indicators.Candle {
	dst := make([]indicators.Candle, len(src))
	for i, c := range src {
		dst[i] = indicators.Candle{
			Open:   c.Open,
			High:   c.High,
			Low:    c.Low,
			Close:  c.Close,
			Volume: c.Volume,
		}
	}
	return dst
}

// publish serialises a TradeIntent and sends it on signal.intent.new.
func (bd *BreakoutDetector) publish(intent *models.TradeIntent) {
	data, err := json.Marshal(intent)
	if err != nil {
		log.Printf("[Quantitative Breakout] marshal error: %v", err)
		return
	}
	if err := bd.js.Publish(nats.SubjectIntentNew, data); err != nil {
		log.Printf("[Quantitative Breakout] publish error: %v", err)
		return
	}
	log.Printf("[Quantitative Breakout] %s %s @ %.2f (VWAP=%.2f, volRatio=%.2f)",
		intent.Side, intent.Ticker, intent.CurrentPrice, intent.VWAP, intent.VolumeRatio)
}
