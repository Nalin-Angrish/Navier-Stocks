// Command report prints realized trading performance from the positions
// table: totals, win rate, average win/loss, max drawdown, and breakdowns
// by day, ticker, and exit reason.
//
// Usage:
//
//	report                          # last 30 days, human-readable table
//	report -from 2026-08-01 -to 2026-08-31
//	report -json                    # machine-readable Summary JSON
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/database"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/utils"
)

func main() {
	utils.LoadEnv()

	defaultFrom := time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	fromStr := flag.String("from", defaultFrom, "start date YYYY-MM-DD (inclusive)")
	toStr := flag.String("to", time.Now().Format("2006-01-02"), "end date YYYY-MM-DD (inclusive)")
	asJSON := flag.Bool("json", false, "emit machine-readable JSON instead of a table")
	flag.Parse()

	from, err := time.ParseInLocation("2006-01-02", *fromStr, time.Local)
	if err != nil {
		fatal("invalid -from: %v", err)
	}
	to, err := time.ParseInLocation("2006-01-02", *toStr, time.Local)
	if err != nil {
		fatal("invalid -to: %v", err)
	}
	to = to.AddDate(0, 0, 1) // half-open upper bound → inclusive end date

	db, err := database.Connect()
	if err != nil {
		fatal("connect: %v", err)
	}
	defer func() { _ = db.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	positions, err := database.NewReportStore(db).ClosedPositions(ctx, from, to)
	if err != nil {
		fatal("load closed positions: %v", err)
	}

	summary := database.ComputeSummary(positions)

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(summary); err != nil {
			fatal("encode summary: %v", err)
		}
		return
	}
	printTable(summary)
}

// row writes one table line, discarding per-call write errors: the
// underlying destination is stdout and Flush reports any real failure.
func row(w *tabwriter.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

func printTable(s database.Summary) {
	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	defer func() { _ = w.Flush() }()

	row(w, "Closed trades:\t%d\n", s.ClosedTrades)
	row(w, "Total P&L:\t%.2f\n", s.TotalPnL)
	row(w, "Wins / Losses:\t%d / %d\n", s.Wins, s.Losses)
	row(w, "Win rate:\t%.1f%%\n", s.WinRate*100)
	row(w, "Avg win / Avg loss:\t%.2f / %.2f\n", s.AvgWin, s.AvgLoss)
	row(w, "Max drawdown:\t%.2f\n", s.MaxDrawdown)

	_, _ = fmt.Fprintln(w, "\nBy day:")
	for _, line := range rankedBreakdown(s.ByDay) {
		row(w, "  %s\t%.2f\n", line.key, line.pnl)
	}

	_, _ = fmt.Fprintln(w, "\nBy ticker:")
	for _, line := range rankedBreakdown(s.ByTicker) {
		row(w, "  %s\t%.2f\n", line.key, line.pnl)
	}

	_, _ = fmt.Fprintln(w, "\nBy exit reason:")
	for _, line := range rankedBreakdown(s.ByExitReason) {
		row(w, "  %s\t%.2f\n", line.key, line.pnl)
	}
}

// breakdownLine is one row of a grouped P&L breakdown.
type breakdownLine struct {
	key string
	pnl float64
}

// rankedBreakdown sorts a grouping map by absolute P&L contribution so the
// most impactful rows surface first regardless of sign.
func rankedBreakdown(m map[string]float64) []breakdownLine {
	lines := make([]breakdownLine, 0, len(m))
	for k, v := range m {
		lines = append(lines, breakdownLine{k, v})
	}
	abs := func(f float64) float64 {
		if f < 0 {
			return -f
		}
		return f
	}
	for i := 1; i < len(lines); i++ {
		for j := i; j > 0 && abs(lines[j-1].pnl) < abs(lines[j].pnl); j-- {
			lines[j-1], lines[j] = lines[j], lines[j-1]
		}
	}
	return lines
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "report: "+format+"\n", args...)
	os.Exit(1)
}
