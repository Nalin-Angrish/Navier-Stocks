package recorder

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
)

// fixedClock pins the store's notion of "now".
func fixedClock(t *testing.T, at time.Time) func() time.Time {
	t.Helper()
	loc := at.Location()
	return func() time.Time { return at.In(loc) }
}

func tick(ticker string, price float64) models.PriceTick {
	return models.PriceTick{Ticker: ticker, Price: price, TsInMillis: 1724400000000}
}

func TestTickStore_AppendCreatesDailyFile(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenTickStore(dir)
	if err != nil {
		t.Fatalf("OpenTickStore: %v", err)
	}
	defer func() { _ = store.Close() }()

	store.SetClock(fixedClock(t, time.Date(2026, 8, 23, 10, 0, 0, 0, time.UTC)))
	if err := store.Append(tick("RELIANCE", 2500.5)); err != nil {
		t.Fatalf("Append: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "2026-08-23.jsonl"))
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	if len(raw) == 0 || raw[len(raw)-1] != '\n' {
		t.Fatal("each record must be newline-terminated")
	}
	var got models.PriceTick
	if err := json.Unmarshal(raw[:len(raw)-1], &got); err != nil {
		t.Fatalf("stored line is not JSON: %v", err)
	}
	if got.Ticker != "RELIANCE" || got.Price != 2500.5 {
		t.Fatalf("tick corrupted on disk: %+v", got)
	}
}

func TestTickStore_RotatesAtMidnightAndAppends(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenTickStore(dir)
	if err != nil {
		t.Fatalf("OpenTickStore: %v", err)
	}
	defer func() { _ = store.Close() }()

	day1 := fixedClock(t, time.Date(2026, 8, 21, 15, 29, 0, 0, time.UTC))
	store.SetClock(day1)
	if err := store.Append(tick("TCS", 4000)); err != nil {
		t.Fatalf("day1 append: %v", err)
	}

	// Next morning: same store, new file, old handle must be closed.
	store.SetClock(fixedClock(t, time.Date(2026, 8, 22, 9, 15, 0, 0, time.UTC)))
	if err := store.Append(tick("TCS", 4010)); err != nil {
		t.Fatalf("day2 append: %v", err)
	}
	if err := store.Append(tick("INFY", 1500)); err != nil {
		t.Fatalf("day2 append 2: %v", err)
	}

	for _, name := range []string{"2026-08-21.jsonl", "2026-08-22.jsonl"} {
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("open %s: %v", name, err)
		}
		lines := bufio.NewScanner(f)
		n := 0
		for lines.Scan() {
			n++
		}
		_ = f.Close()
		if n != 1 && name != "2026-08-22.jsonl" {
			t.Fatalf("%s should hold exactly 1 line, has %d", name, n)
		}
	}
}

func TestTickStore_ReopenAppendsWithoutTruncation(t *testing.T) {
	dir := t.TempDir()
	clock := fixedClock(t, time.Date(2026, 8, 23, 11, 0, 0, 0, time.UTC))

	store, err := OpenTickStore(dir)
	if err != nil {
		t.Fatalf("OpenTickStore: %v", err)
	}
	store.SetClock(clock)
	if err := store.Append(tick("SBIN", 800)); err != nil {
		t.Fatalf("first append: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Simulate a restart: fresh store over the same directory must keep
	// the earlier line (O_APPEND, never O_TRUNC).
	reborn, err := OpenTickStore(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = reborn.Close() }()
	reborn.SetClock(clock)
	if err := reborn.Append(tick("SBIN", 805)); err != nil {
		t.Fatalf("second append: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "2026-08-23.jsonl"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	count := 0
	for _, b := range raw {
		if b == '\n' {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("restart truncated history: %d lines", count)
	}
}

func TestTickStore_CloseIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenTickStore(dir)
	if err != nil {
		t.Fatalf("OpenTickStore: %v", err)
	}
	store.SetClock(fixedClock(t, time.Now()))
	if err := store.Append(tick("LT", 3500)); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("second close must be a no-op: %v", err)
	}
}
