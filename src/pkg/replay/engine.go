// Package replay re-drives the signal.price.* pipeline from the Market
// Recorder's daily JSONL archives (story 8.4).
//
// The engine loads one archive, restores the original tick order, and
// republishes each tick onto its per-ticker subject while preserving
// inter-tick pacing divided by a speed factor: 1.0 replays in real time,
// 10.0 ten times faster, and 0 (or negative) publishes as fast as
// possible with no delays at all.
package replay

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"time"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/nats"
)

// Publisher is the outbound surface the engine needs; satisfied by
// *nats.JetStream and by tests.
type Publisher interface {
	Publish(subj string, data []byte) error
}

// Step is one scheduled replay action: publish Tick after waiting Delay
// since the previous step.
type Step struct {
	Tick  models.PriceTick
	Delay time.Duration // wait BEFORE publishing this tick
}

// LoadTicks parses a JSONL archive into ticks sorted chronologically.
// Malformed lines are skipped with a warning so a truncated tail from an
// unclean shutdown still replays.
func LoadTicks(path string) ([]models.PriceTick, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("replay: open archive: %w", err)
	}
	defer func() { _ = f.Close() }()

	var ticks []models.PriceTick
	sc := bufio.NewScanner(f)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		var tick models.PriceTick
		if err := json.Unmarshal(sc.Bytes(), &tick); err != nil {
			log.Printf("[Replay] %s:%d skipping malformed line: %v", path, lineNo, err)
			continue
		}
		ticks = append(ticks, tick)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("replay: scan %s: %w", path, err)
	}
	sort.SliceStable(ticks, func(i, j int) bool {
		return ticks[i].TsInMillis < ticks[j].TsInMillis
	})
	return ticks, nil
}

// Plan converts ordered ticks into the schedule the engine executes:
// each step carries the gap to the previous tick, scaled down by speed.
// A non-positive speed collapses all gaps to zero (maximum-speed replay).
func Plan(ticks []models.PriceTick, speed float64) []Step {
	steps := make([]Step, 0, len(ticks))
	var prevMillis int64
	for i, tick := range ticks {
		delay := time.Duration(0)
		if i > 0 && speed > 0 {
			gap := tick.TsInMillis - prevMillis
			if gap > 0 {
				delay = time.Duration(float64(gap) / speed * float64(time.Millisecond))
			}
		}
		prevMillis = tick.TsInMillis
		steps = append(steps, Step{Tick: tick, Delay: delay})
	}
	return steps
}

// Engine republishes archived ticks through Publisher.
type Engine struct {
	pub Publisher
}

// NewEngine returns an Engine publishing through pub.
func NewEngine(pub Publisher) *Engine {
	return &Engine{pub: pub}
}

// Run executes the schedule until completion or ctx cancellation.  It
// returns the number of successfully published ticks.
func (e *Engine) Run(ctx context.Context, steps []Step) (int, error) {
	published := 0
	for _, step := range steps {
		if step.Delay > 0 {
			select {
			case <-ctx.Done():
				return published, ctx.Err()
			case <-time.After(step.Delay):
			}
		}

		data, err := json.Marshal(step.Tick)
		if err != nil {
			return published, fmt.Errorf("replay: marshal tick: %w", err)
		}
		if err := e.pub.Publish(nats.PriceSubject(step.Tick.Ticker), data); err != nil {
			return published, fmt.Errorf("replay: publish %s: %w", step.Tick.Ticker, err)
		}
		published++
	}
	return published, nil
}
