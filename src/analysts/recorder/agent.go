// Package recorder archives the raw signal.price.* stream to daily JSONL
// files for offline replay (story 8.3b).
package recorder

import (
	"fmt"
	"log"
	"os"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/nats"
)

// ticksDirEnvKey selects where daily tick archives are written.
const ticksDirEnvKey = "TICKS_DIR"

// DefaultTicksDir is used when TICKS_DIR is absent, keeping the repo
// self-contained for development runs.
const DefaultTicksDir = "data/ticks"

// envTicksDir reads TICKS_DIR, falling back to DefaultTicksDir.
func envTicksDir() string {
	if dir := os.Getenv(ticksDirEnvKey); dir != "" {
		return dir
	}
	return DefaultTicksDir
}

// Analyst is the Market Recorder agent.  It mirrors the other analysts'
// lifecycle contract (Run/Stop) but performs no analysis: every tick on
// signal.price.* is appended verbatim to a daily JSONL file.
type Analyst struct {
	js       *nats.JetStream
	sub      *nats.Subscription
	store    *TickStore
	stopChan chan struct{}
	stopped  chan struct{} // closed once Run has fully torn down
}

// NewAgent creates a Recorder writing ticks into TICKS_DIR (default
// ./data/ticks).  The caller must call Stop().
func NewAgent() (*Analyst, error) {
	dir := envTicksDir()
	store, err := OpenTickStore(dir)
	if err != nil {
		return nil, fmt.Errorf("recorder: %w", err)
	}
	js, err := nats.ConnectJetStream()
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("recorder: nats: %w", err)
	}
	return newAnalyst(js, store), nil
}

// newAnalyst is the shared constructor used by NewAgent and tests.
func newAnalyst(js *nats.JetStream, store *TickStore) *Analyst {
	return &Analyst{
		js:       js,
		store:    store,
		stopChan: make(chan struct{}),
		stopped:  make(chan struct{}),
	}
}

// Run subscribes to signal.price.* and appends every tick until Stop is
// called.  The price stream rides the existing trading JetStream; if it
// cannot be ensured the agent exits immediately so boot logs surface it.
func (a *Analyst) Run() {
	defer close(a.stopped)

	if err := a.js.EnsureStream(nats.StreamTrading); err != nil {
		log.Printf("[Recorder] Stream ensure failed: %v\n", err)
		return
	}

	sub, err := a.js.Subscribe(nats.SubjectPricePrefix+"*", a.handlePrice)
	if err != nil {
		log.Printf("[Recorder] Subscribe failed: %v\n", err)
		return
	}
	a.sub = sub
	fmt.Println("[Recorder] Recording signal.price.*")

	<-a.stopChan

	if a.sub != nil {
		if err := a.sub.Unsubscribe(); err != nil {
			log.Printf("[Recorder] Unsubscribe error: %v\n", err)
		}
	}
	if err := a.store.Close(); err != nil {
		log.Printf("[Recorder] close error: %v\n", err)
	}
}

// Stop signals Run to tear down and waits for teardown to finish.
func (a *Analyst) Stop() {
	select {
	case <-a.stopChan:
		// already stopping
	default:
		close(a.stopChan)
	}
	<-a.stopped
}
