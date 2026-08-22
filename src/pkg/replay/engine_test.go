package replay

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/nats"
)

// stubPub captures publishes and can fail on demand.
type stubPub struct {
	sent   []pubRecord
	err    error
	failAt int // index at which err is returned; -1 disables
}

type pubRecord struct {
	subject string
	tick    models.PriceTick
}

func (p *stubPub) Publish(subj string, data []byte) error {
	if p.failAt >= 0 && len(p.sent) == p.failAt {
		return p.err
	}
	var tick models.PriceTick
	_ = json.Unmarshal(data, &tick)
	p.sent = append(p.sent, pubRecord{subj, tick})
	return nil
}

func writeArchive(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "2026-08-21.jsonl")
	if err := os.WriteFile(path, []byte(joinLines(lines)), 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	return path
}

func joinLines(lines []string) string {
	out := ""
	for _, l := range lines {
		out += l + "\n"
	}
	return out
}

func line(ticker string, price float64, millis int64) string {
	b, _ := json.Marshal(models.PriceTick{Ticker: ticker, Price: price, TsInMillis: millis})
	return string(b)
}

func TestLoadTicks_SortsAndSkipsMalformed(t *testing.T) {
	path := writeArchive(t,
		line("TCS", 4000, 3000), // deliberately out of order
		line("RELIANCE", 2500, 1000),
		"{broken",
		line("INFY", 1500, 2000),
	)

	ticks, err := LoadTicks(path)
	if err != nil {
		t.Fatalf("LoadTicks: %v", err)
	}
	if len(ticks) != 3 {
		t.Fatalf("expected 3 good ticks, got %d", len(ticks))
	}
	for i, want := range []int64{1000, 2000, 3000} {
		if ticks[i].TsInMillis != want {
			t.Fatalf("tick %d out of order: %d", i, ticks[i].TsInMillis)
		}
	}
}

func TestLoadTicks_MissingFileErrors(t *testing.T) {
	if _, err := LoadTicks(filepath.Join(t.TempDir(), "nope.jsonl")); err == nil {
		t.Fatal("expected error for missing archive")
	}
}

func TestPlan_PreservesGapsScaledBySpeed(t *testing.T) {
	ticks := []models.PriceTick{
		{Ticker: "A", TsInMillis: 0},
		{Ticker: "B", TsInMillis: 500},  // gap 500ms
		{Ticker: "C", TsInMillis: 1250}, // gap 750ms
	}

	at1x := Plan(ticks, 1.0)
	if at1x[0].Delay != 0 || at1x[1].Delay != 500*time.Millisecond || at1x[2].Delay != 750*time.Millisecond {
		t.Fatalf("1x plan wrong: %+v", at1x)
	}

	at10x := Plan(ticks, 10.0)
	if at10x[1].Delay != 50*time.Millisecond || at10x[2].Delay != 75*time.Millisecond {
		t.Fatalf("10x plan wrong: %+v", at10x)
	}

	maxSpeed := Plan(ticks, 0) // non-positive → no delays
	for i, s := range maxSpeed {
		if s.Delay != 0 {
			t.Fatalf("max-speed step %d carries delay %v", i, s.Delay)
		}
	}
}

func TestEngine_PublishesOnPerTickerSubjectsInOrder(t *testing.T) {
	pub := &stubPub{failAt: -1}
	e := NewEngine(pub)

	steps := Plan([]models.PriceTick{
		{Ticker: "RELIANCE", Price: 100, TsInMillis: 1},
		{Ticker: "TCS", Price: 4000, TsInMillis: 2},
	}, 0)

	n, err := e.Run(context.Background(), steps)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if n != 2 || len(pub.sent) != 2 {
		t.Fatalf("published=%d recorded=%d", n, len(pub.sent))
	}
	if pub.sent[0].subject != nats.PriceSubject("RELIANCE") ||
		pub.sent[1].subject != nats.PriceSubject("TCS") {
		t.Fatalf("subjects wrong: %s, %s", pub.sent[0].subject, pub.sent[1].subject)
	}
	if pub.sent[1].tick.Price != 4000 {
		t.Fatalf("payload corrupted: %+v", pub.sent[1].tick)
	}
}

func TestEngine_PublishFailureStopsWithCount(t *testing.T) {
	pub := &stubPub{failAt: 1, err: errors.New("nats down")}
	e := NewEngine(pub)

	steps := Plan([]models.PriceTick{
		{TsInMillis: 1}, {TsInMillis: 2}, {TsInMillis: 3},
	}, 0)

	n, err := e.Run(context.Background(), steps)
	if err == nil || n != 1 {
		t.Fatalf("expected stop after first failure with n=1, got n=%d err=%v", n, err)
	}
}

func TestEngine_ContextCancelDuringGap(t *testing.T) {
	pub := &stubPub{failAt: -1}
	e := NewEngine(pub)

	steps := Plan([]models.PriceTick{
		{TsInMillis: 1},
		{TsInMillis: 1 + 60_000}, // 60s gap at 1x — far beyond the test budget
	}, 1.0)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	n, err := e.Run(ctx, steps)
	if err == nil || n != 1 {
		t.Fatalf("expected cancellation after first tick, got n=%d err=%v", n, err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("cancellation took too long: %v", elapsed)
	}
}
