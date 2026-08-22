// Command replay re-drives the signal.price.* pipeline from a Market
// Recorder daily archive, letting the full downstream stack be tested
// against recorded market data.
//
// Usage:
//
//	replay -date 2026-08-21                  # real-time replay of that day
//	replay -date 2026-08-21 -speed 10        # 10x faster
//	replay -date 2026-08-21 -speed 0         # as fast as possible
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/nats"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/replay"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/utils"
)

func main() {
	utils.LoadEnv()

	defaultDate := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	date := flag.String("date", defaultDate, "archive date YYYY-MM-DD")
	dir := flag.String("dir", "data/ticks", "directory containing the recorder archives")
	speed := flag.Float64("speed", 1.0, "replay speed multiplier (<=0 replays at maximum speed)")
	flag.Parse()

	js, err := nats.ConnectJetStream()
	if err != nil {
		fatal("nats: %v", err)
	}
	defer js.Close()

	if err := js.EnsureStream(nats.StreamTrading); err != nil {
		fatal("ensure stream: %v", err)
	}

	ticks, err := replay.LoadTicks(filepath.Join(*dir, *date+".jsonl"))
	if err != nil {
		fatal("%v", err)
	}
	if len(ticks) == 0 {
		log.Printf("[Replay] archive %s/%s.jsonl is empty — nothing to do", *dir, *date)
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	steps := replay.Plan(ticks, *speed)
	log.Printf("[Replay] publishing %d ticks from %s at speed %.2fx (Ctrl-C aborts)", len(steps), *date, *speed)

	start := time.Now()
	published, err := replay.NewEngine(js).Run(ctx, steps)
	if err != nil && ctx.Err() == nil {
		fatal("%v", err)
	}

	note := ""
	if ctx.Err() != nil {
		note = " (aborted)"
	}
	log.Printf("[Replay] done%s: %d/%d ticks in %s", note, published, len(steps), time.Since(start))
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "replay: "+format+"\n", args...)
	os.Exit(1)
}
