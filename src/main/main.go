// Navier-Stocks entrypoint — boots all four trading agents in parallel.
//
// Each analyst package exposes NewAgent() returning (Agent, error).
// main() iterates a constructor table, starts every agent in its own
// goroutine, then blocks on SIGINT/SIGTERM to orchestrate a graceful
// shutdown.
package main

import (
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/Nalin-Angrish/Navier-Stocks/src/analysts/quantitative"
	"github.com/Nalin-Angrish/Navier-Stocks/src/analysts/recorder"
	"github.com/Nalin-Angrish/Navier-Stocks/src/analysts/riskmanager"
	"github.com/Nalin-Angrish/Navier-Stocks/src/analysts/sentiment"
	"github.com/Nalin-Angrish/Navier-Stocks/src/analysts/trader"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/database"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/utils"
)

// Agent is the lifecycle contract every analyst must satisfy.
// Packages define their own concrete type; Go's structural typing
// wires them into this interface automatically.
type Agent interface {
	Run()
	Stop()
}

// agentDef pairs a human-readable name with a constructor so the
// bootstrap loop can log failures without caring about the concrete type.
type agentDef struct {
	name string
	new  func() (Agent, error)
}

func main() {
	utils.LoadEnv()

	// -- Database migrations --------------------------------------------------
	db, err := database.Connect()
	if err != nil {
		log.Fatalf("[Boot] Database connect failed: %v", err)
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			log.Printf("[Boot] Database close: %v", cerr)
		}
	}()

	if err := db.Ping(); err != nil {
		log.Fatalf("[Boot] Database ping failed: %v", err)
	}
	log.Println("[Boot] Database connected")

	if err := database.Migrate(db); err != nil {
		log.Fatalf("[Boot] Migration failed: %v", err)
	}

	log.Println("[Boot] Starting all agents...")

	// -- Constructor table ---------------------------------------------------
	// Add a new analyst here and it will be started / stopped automatically.
	agents := []agentDef{
		{"QuantitativeScout", func() (Agent, error) { return quantitative.NewAgent() }},
		{"RiskManager", func() (Agent, error) { return riskmanager.NewAgent() }},
		{"TraderGateway", func() (Agent, error) { return trader.NewAgent() }},
		{"SentimentAnalyst", func() (Agent, error) { return sentiment.NewAgent() }},
		{"MarketRecorder", func() (Agent, error) { return recorder.NewAgent() }},
	}

	// -- Bootstrap -----------------------------------------------------------
	// Construct every agent sequentially so a single init failure aborts
	// early before any goroutine is launched.
	var running []Agent
	var wg sync.WaitGroup

	for _, def := range agents {
		a, err := def.new()
		if err != nil {
			log.Fatalf("[Boot] %s init failed: %v", def.name, err)
		}
		running = append(running, a)
		wg.Add(1)
		go func(name string, a Agent) {
			defer wg.Done()
			log.Printf("[Boot] %s running", name)
			a.Run()
		}(def.name, a)
	}

	// -- Wait for shutdown signal --------------------------------------------
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	// -- Graceful shutdown ---------------------------------------------------
	// Signal every agent to stop, then wait for each goroutine to finish.
	log.Println("[Boot] Shutting down all agents...")
	for _, a := range running {
		a.Stop()
	}
	wg.Wait()
}
