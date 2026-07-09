<div align="center">
  
# 🌊 Navier-Stocks
**A multi-agent approach to solving financial turbulence.**

[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![NATS JetStream](https://img.shields.io/badge/NATS-JetStream-27AE60?style=flat&logo=nats)](https://nats.io/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16.0-4169E1?style=flat&logo=postgresql)](https://www.postgresql.org/)
[![Docker Compose](https://img.shields.io/badge/Docker-Compose-2496ED?style=flat&logo=docker)](https://www.docker.com/)
[![License](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

</div>

**Navier-Stocks** is a high-performance, heterogeneous multi-agent algorithmic trading system engineered specifically for the Indian Stock Exchanges (**NSE & BSE**). 

Built from the ground up in Go, the system treats market liquidity, order book momentum, and price velocity as structural fluid vectors. By treating volatile market movements as a bounded computational problem governed by predictable mechanical thresholds—much like navigating the immersed boundaries of a complex fluid simulation—Navier-Stocks isolates high-probability intraday momentum breakouts while strictly managing capital correlation. 

Designed for extreme operational efficiency, the entire distributed architecture is optimized to run flawlessly on constrained, hobby-tier cloud infrastructure with sub-5 millisecond internal routing latency.

## 🏛️ System Architecture

Navier-Stocks achieves its speed and resilience by strictly decoupling the execution pipeline into two distinct streams: a **Fast Path** for deterministic mathematical execution, and a **Slow Path** for asynchronous cognitive context. 

Agents communicate via **NATS JetStream** for low-latency, fault-tolerant message passing, while **PostgreSQL** anchors the system's long-term state and operational memory.

### The 4-Agent Cohort

| Agent | Paradigm | Role & Responsibility |
| :--- | :--- | :--- |
| 📡 **Quantitative Scout** | Deterministic / Go | The real-time ingestor. Subscribes to live WebSocket ticks, maintaining hyper-efficient in-memory ring buffers. It solves for pressure differentials in the market, hunting for VWAP + Bollinger Band momentum breakouts. |
| 🧠 **Sentiment Analyst** | Cognitive / LLM | The asynchronous macro observer. It continuously scrapes and processes financial news and corporate filings, writing contextual confidence scores to the database without ever blocking the fast execution path. |
| 🛡️ **Risk Manager** | Deterministic / Go | The structural gatekeeper. It intercepts trade intents, validating them against rigid intraday time windows, lock-free sector concentration maps, and overall portfolio limits before calculating precise capital allocation. |
| ⚡ **Trader Gateway** | Interface | The "dumb pipe" execution layer. Seamlessly hot-swappable between a `PaperTrader` for database-only simulation and a `GrowwTrader` for live REST API broker interactions. |

## ⚙️ Core Technical Features

### 1. Ultra-Low Latency "Fast Path" Processing
Standard multi-agent systems bottleneck when forcing LLMs to make real-time decisions. Navier-Stocks sidesteps this by keeping the core execution loop (Scout -> Risk Manager -> Trader) strictly mathematical. By leveraging Go's lightweight goroutines and NATS byte-payloads, a live market tick is ingested, validated, and translated into an execution signal in **under 5 milliseconds**.

### 2. Algorithmic Exposure & Diversity Control
Intraday diversity is defined by correlation control. The Risk Manager utilizes a `sync.RWMutex` state map to instantly enforce:
* **Sector Concentration Caps:** Preventing exposure to systemic shocks by limiting active trades per structural sector (e.g., maximum one position in Nifty IT or Nifty Bank at a time).
* **Concurrency Ceilings:** Capping the total number of simultaneous open positions to preserve capital liquidity.
* **Directional Equilibrium:** Ensuring the ratio of Long vs. Short positions remains balanced against broader index momentum.

### 3. Dynamic Capital Allocation (The 2% Rule)
Capital preservation is hardcoded into the platform's DNA. Every execution boundary is calculated dynamically using a fixed-fractional allocation matrix. The system ensures that a technical stop-loss trigger will never jeopardize more than a predefined fraction of the total deployable capital:

$$Quantity = \frac{Total\ Capital \times 0.02}{Entry\ Price - Stop\ Loss\ Price}$$

### 4. Operational Timing Guardrails
Built for the specific mechanics of the Indian markets, the system programmatically sidesteps opening volatility and broker-enforced liquidation penalties:
* **Opening Buffer:** Execution suppression from **09:15 to 09:30 AM IST** to allow the boundary layer of the market to stabilize.
* **Closing Cutoff & Auto-Square-Off:** Halts new entries post-15:00 IST and automatically triggers market-liquidation for all open MIS positions precisely at **15:15 PM IST**.

## 🛠️ Technology Stack

* **Language:** Go (Golang) — Chosen for unparalleled concurrency (`goroutines`), minimal memory footprint, and rapid binary execution.
* **Message Broker:** NATS JetStream — Provides persistent, at-least-once delivery pub/sub messaging for agent communication, outperforming Redis in low-memory environments.
* **Database:** PostgreSQL — Handles ACID-compliant state recovery, historical transaction auditing, and asynchronous sentiment parameter storage.
* **Orchestration:** Docker & Docker Compose — Containerized deployments ensuring exact parity between local development and cloud production environments.

## 🚀 Getting Started

### Prerequisites
* [Docker](https://docs.docker.com/get-docker/) and [Docker Compose](https://docs.docker.com/compose/install/)
* [Go 1.26+](https://golang.org/dl/) (For local development outside of containers)

### Quickstart Installation

1. **Clone the repository:**
```bash
git clone https://github.com/Nalin-Angrish/Navier-Stocks.git
cd navier-stocks
```

2. **Configure the environment:**
```bash
cp .env.example .env
# Edit .env with your broker API keys, database credentials, and risk parameters
```


3. **Deploy the infrastructure stack:**
```bash
docker-compose up -d --build
```


*This single command provisions the Go agent microservices, initializes the PostgreSQL schemas, and spins up the NATS JetStream broker.*

4. **Verify System Health:**
```bash
docker-compose logs -f risk-manager
```

## 📚 Documentation & Wiki

For a deep dive into the underlying quantitative math, NATS JSON payloads, or operational runbooks, please consult the official **[Navier-Stocks Engineering Wiki](https://github.com/Nalin-Angrish/Navier-Stocks/wiki)**.

* [High-Level Design (HLD)](https://github.com/Nalin-Angrish/Navier-Stocks/wiki/High-Level-Design)
* [Risk Guardrails & Timing Controls](https://github.com/Nalin-Angrish/Navier-Stocks/wiki/Risk-Guardrails)

## ⚠️ Disclaimer

**Navier-Stocks is strictly an educational engineering project and a Proof-of-Concept for distributed multi-agent systems.** This repository does not constitute financial advice. Algorithmic trading on live financial markets carries a high risk of catastrophic capital loss. The creators of this repository are not responsible for any financial losses incurred through the deployment of this software. Always test thoroughly using the `PaperTrader` sandbox interface before supplying live broker credentials.