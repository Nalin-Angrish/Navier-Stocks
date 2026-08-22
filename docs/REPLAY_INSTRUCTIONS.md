# Replay Testing Guide

How to re-drive the trading pipeline from recorded market data and
evaluate the bot's realized performance.

The loop works end to end without touching a broker:

```
recorder archive ──▶ replay CLI ──▶ signal.price.*
                                        │
                    Quantitative Scout ◀┘  (indicators, signals)
                                        │
                              Risk Manager (gate + size)
                                        │
                             Trader Gateway (paper fills)
                                        │
                     exit monitor (SL/TP) ──▶ positions table
                                        │
                          report CLI ──▶ P&L summary
```

## Prerequisites

1. **NATS** and **PostgreSQL** running locally (`NATS_URL`, `PG_*` in
   `.env`; see `.env.example`).
2. Database schema up to date — all migrations in `src/migrations`
   applied.
3. A tick archive recorded by the Market Recorder (see next section).
   Archives land in `$TICKS_DIR` (default `data/ticks`) as one JSONL
   file per day, e.g. `data/ticks/2026-08-21.jsonl`.

## Step 1 — Record a day of ticks

The recorder ships inside the normal boot table, so any live session
produces an archive automatically:

```sh
go run ./src/main        # boots all agents incl. MarketRecorder
```

Let it run through a session (or as long as you like). Stop with
Ctrl-C; the current day's file is safe to reuse — reopening appends,
it never truncates.

No archive yet? Any JSONL file of `{"ticker":..., "price":...,
"ts_millis":...}` lines named `YYYY-MM-DD.jsonl` works.

## Step 2 — Boot the stack under test

In a second terminal, start the agents that will consume the replay:
the full boot is fine, or just the downstream half if you want to skip
live scraping noise:

```sh
go run ./src/main
```

> **Caution:** replay publishes onto the *live* `signal.price.*`
> subjects. Run replays only while the Trader Gateway is in paper mode;
> never against a funded broker session.

## Step 3 — Replay the archive

```sh
go run ./src/tools/replay -date 2026-08-21            # real time
go run ./src/tools/replay -date 2026-08-21 -speed 10  # 10x faster
go run ./src/tools/replay -date 2026-08-21 -speed 0   # maximum speed
```

| Flag    | Default        | Meaning                                            |
|---------|----------------|----------------------------------------------------|
| `-date` | yesterday      | Archive day (`YYYY-MM-DD.jsonl` inside `-dir`)     |
| `-dir`  | `data/ticks`   | Directory containing recorder archives             |
| `-speed`| `1.0`          | Pace multiplier; `<=0` collapses all inter-tick gaps|

Notes:

- Original tick order is restored before publishing, so indicator warm-up
  and intraday sequences behave exactly as they did live.
- Ctrl-C aborts cleanly mid-stream; the log line reports how many ticks
  made it out.
- An empty or missing archive is reported and the tool exits — check the
  date spelling first when this happens.

## Step 4 — Evaluate P&L and metrics

Once the replay has driven some round trips (stop-loss, take-profit,
or the 15:00 IST square-off close positions), summarize results:

```sh
go run ./src/tools/report                       # human-readable table
go run ./src/tools/report -json                 # machine-readable
go run ./src/tools/report -from 2026-08-21 -to 2026-08-21
```

Date ranges are inclusive and default to the trailing 30 days. Scoping
`-from/-to` to the replayed day isolates that run's numbers.

### Metric reference

| Metric         | Meaning                                                            |
|----------------|--------------------------------------------------------------------|
| `closed_trades`| Round trips completed in the window                                |
| `total_pnl`    | Sum of realized P&L across closes                                  |
| `wins/losses`  | Count of positive / negative closes                                |
| `win_rate`     | Wins ÷ (wins + losses); flat closes excluded                       |
| `avg_win`      | Mean profit per winning trade                                      |
| `avg_loss`     | Mean loss per losing trade (negative)                              |
| `max_drawdown` | Worst peak-to-trough dip of the cumulative equity curve            |
| `by_day`       | Realized P&L per calendar day                                      |
| `by_ticker`    | Realized P&L per symbol — spot the bleeding symbols                |
| `by_exit_reason`| P&L split by `stop_loss` / `take_profit` / `square_off`           |

### Reading the numbers

- `by_exit_reason` is the strategy health check: sustained losses under
  `stop_loss` with thin `take_profit` gains means boundaries need work.
- Compare `avg_win` against `|avg_loss|`: with a 1:1 win rate you want
  wins at least as large as losses.
- `max_drawdown` sizes tail risk; rerun the same day at different
  speeds to confirm results are pace-invariant.

## Tips

- **Clean slate per experiment:** truncate the book so metrics reflect
  exactly one run:
  ```sql
  TRUNCATE positions RESTART IDENTITY;
  ```
- **Fast iteration:** `-speed 0` plus a small universe gets a full day
  through the pipeline in seconds.
- **Determinism:** LLM sentiment scores can vary between runs; expect
  Gate-4 outcomes to differ slightly unless the sentiment cache is warm
  and unchanged.
