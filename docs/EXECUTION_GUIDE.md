# Navier-Stocks — End-to-End Execution Guide

Complete instructions to go from zero to a running trading bot with
sentiment analysis (Ollama) and tick recording.

---

## Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| Go | 1.27+ | Build the binary |
| Docker + Docker Compose | 24+ | NATS + PostgreSQL |
| Ollama | latest | LLM for sentiment analysis |
| Groww account | — | API access token |

Check what you have:
```bash
go version        # need 1.27+
docker --version  # need 24+
docker compose version
ollama --version  # if not installed, see Step 1
```

---

## Step 1: Install Ollama (if not installed)

```bash
# Linux / macOS
curl -fsSL https://ollama.ai/install.sh | sh

# Verify
ollama --version
```

Pull the model:
```bash
# Recommended for your hardware (i7 + RTX 4050 6GB VRAM)
ollama pull llama3

# Lighter alternative (if you want faster responses / less VRAM)
# ollama pull phi3
```

Start the Ollama server:
```bash
ollama serve
```

Verify it's running:
```bash
curl http://localhost:11434/api/tags
# Should return a JSON list with llama3 (or phi3) in it
```

---

## Step 2: Clone and configure

```bash
git clone https://github.com/Nalin-Angrish/Navier-Stocks.git
cd Navier-Stocks
cp .env.example .env
```

Edit `.env` with your values:

```bash
# ── Groww API ──────────────────────────────────────────────
# Option A: Direct token (expires daily at 6 AM IST)
GROWW_ACCESS_TOKEN=your_token_here

# Option B: API key + secret (auto-refreshes, recommended for long runs)
# GROWW_API_KEY=your_key
# GROWW_API_SECRET=your_secret

# ── Infrastructure ─────────────────────────────────────────
NATS_URL=nats://localhost:4222
PG_HOST=localhost
PG_PORT=5432
PG_USER=navier
PG_PASSWORD=devpassword
PG_DATABASE=navier_stocks
PG_SSLMODE=disable

# ── LLM (Sentiment Analyst) ───────────────────────────────
LLM_PROVIDER=ollama
LLM_BASE_URL=http://localhost:11434
LLM_MODEL=llama3          # or phi3
LLM_TIMEOUT=60s

# ── Risk Parameters ────────────────────────────────────────
TOTAL_CAPITAL=1000000     # ₹10 lakhs deployable capital
RISK_MIN_CONFIDENCE=0.30  # sentiment confidence floor (0.0–1.0)

# ── Recording ──────────────────────────────────────────────
TICKS_DIR=data/ticks
```

---

## Step 3: Start infrastructure (NATS + PostgreSQL)

```bash
# Start both in the background
docker compose up -d nats postgres

# Wait ~5 seconds for PostgreSQL to initialize, then apply migrations
sleep 5
make db/migrate
```

Verify both are running:
```bash
docker compose ps
# Should show nats and postgres as "Up"

# Quick health check
curl -s http://localhost:8222/varz | head -1   # NATS monitoring
PGPASSWORD=devpassword psql -h localhost -U navier -d navier_stocks -c "\dt"  # DB tables
```

You should see these tables: `positions`, `sentiment_scores`, `trade_log`.

---

## Step 4: Build and run

```bash
# Build the binary
make build

# Run it (foreground — logs print to terminal)
make run
```

Or use Docker Compose for the full stack:
```bash
docker compose up -d --build
```

---

## Step 5: Verify it's working

You should see boot logs like:
```
[Boot] Starting all agents...
[Boot] QuantitativeScout running
[Boot] RiskManager running
[Boot] TraderGateway running
[Boot] SentimentAnalyst running
[Boot] MarketRecorder running
[Quantitative Analyst] tracking 11 symbols
[Quantitative Feed] subscribed to 11 instruments
[Quantitative Analyst] Ready...
```

During market hours (9:15 AM – 3:30 PM IST), look for:
```
[Scout] quote RELIANCE: ...           # volume enrichment working
[Risk Manager] risk config: ...       # RM started
[Sentiment] INFY: fresh score ...     # LLM scoring news
[Risk Manager] square-off ...         # positions closed at 3:15 PM
```

---

## Step 6: Run in the background (for all-day unattended)

```bash
# Using tmux (recommended)
tmux new -s bot
make run
# Press Ctrl+B, then D to detach

# Re-attach later
tmux attach -t bot

# Or using nohup
nohup make run > bot.log 2>&1 &
# Check logs: tail -f bot.log
```

---

## Step 7: Check results

### At market close (after 3:30 PM IST):
```bash
# View live logs
tmux attach -t bot
# or
tail -f bot.log
```

### P&L report (after positions are closed):
```bash
# Today's P&L
go run ./src/tools/report --from 2026-08-24 --to 2026-08-24

# With breakdowns
go run ./src/tools/report --from 2026-08-24 --to 2026-08-24 --json
```

### Recorded ticks (for replay/backtesting):
```bash
ls data/ticks/
# 2026-08-24.jsonl

# Replay to test exit logic
go run ./src/tools/replay --date 2026-08-24 --speed 10x
```

---

## Quick Reference: All Commands

```bash
# ── Setup (one-time) ──────────────────────────────────────
ollama pull llama3
ollama serve                              # keep running in a separate terminal
cp .env.example .env                      # edit with your keys
docker compose up -d nats postgres
sleep 5 && make db/migrate

# ── Daily run ──────────────────────────────────────────────
make build
tmux new -s bot
make run
# Ctrl+B, D to detach

# ── Check status ───────────────────────────────────────────
tmux attach -t bot                        # view live logs
docker compose ps                         # infra health
curl -s http://localhost:11434/api/tags   # ollama models

# ── Results ────────────────────────────────────────────────
go run ./src/tools/report --from YYYY-MM-DD --to YYYY-MM-DD
ls data/ticks/                            # recorded ticks

# ── Shutdown ───────────────────────────────────────────────
# In the bot terminal: Ctrl+C
# Or from another terminal:
kill -SIGTERM $(pgrep navier-stocks)
```

---

## Troubleshooting

| Problem | Fix |
|---------|-----|
| `ollama: connection refused` | Start `ollama serve` in another terminal |
| `NATS: no responders` | `docker compose up -d nats` |
| `relation "positions" does not exist` | `make db/migrate` |
| `GROWW_ACCESS_TOKEN expired` | Get a new token from Groww dashboard; tokens expire daily at 6 AM IST |
| `price for X is stale` warning | WebSocket dropped; it should auto-reconnect. If persistent, restart the bot |
| `out of memory` (Ollama) | Switch to `phi3` or `qwen2:1.5b` |
| Bot does nothing before 9:15 AM | Normal — market is closed. Entries start after 9:30 AM IST |
