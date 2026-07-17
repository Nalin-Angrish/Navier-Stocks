package trader_test

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	natsserver "github.com/nats-io/nats-server/v2/test"

	"github.com/Nalin-Angrish/Navier-Stocks/src/analysts/trader"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/nats"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func startJetStreamServer(t *testing.T) *server.Server {
	t.Helper()
	opts := &server.Options{
		Port:      -1,
		JetStream: true,
		StoreDir:  t.TempDir(),
	}
	s := natsserver.RunServer(opts)
	t.Cleanup(func() { s.Shutdown() })
	return s
}

func connectJS(t *testing.T, s *server.Server) *nats.JetStream {
	t.Helper()
	t.Setenv("NATS_URL", s.ClientURL())
	js, err := nats.ConnectJetStream()
	if err != nil {
		t.Fatalf("ConnectJetStream(): %v", err)
	}
	t.Cleanup(func() { js.Close() })
	return js
}

func ensureStream(t *testing.T, js *nats.JetStream, name string, subjects ...string) {
	t.Helper()
	if err := js.EnsureStream(nats.StreamConfig{Name: name, Subjects: subjects}); err != nil {
		t.Fatalf("EnsureStream(%q): %v", name, err)
	}
}

// ---------------------------------------------------------------------------
// Mock store
// ---------------------------------------------------------------------------

type mockStore struct {
	mu      sync.Mutex
	entries []*models.TradeExecution
	fail    atomic.Bool
}

func (m *mockStore) InsertTradeLogEntry(e *models.TradeExecution) error {
	if m.fail.Load() {
		return assertAnError
	}
	m.mu.Lock()
	m.entries = append(m.entries, e)
	m.mu.Unlock()
	return nil
}

var assertAnError = &mockError{"store unavailable"}

type mockError struct{ msg string }

func (e *mockError) Error() string { return e.msg }

func TestNewAgent_NATSConnectFailure(t *testing.T) {
	t.Setenv("NATS_URL", "nats://localhost:1")
	_, err := trader.NewAgent()
	if err == nil {
		t.Fatal("expected error from NewAgent with invalid NATS URL")
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestTrader_ReceivesAndPersistsExecution(t *testing.T) {
	s := startJetStreamServer(t)
	js := connectJS(t, s)
	ensureStream(t, js, "trading", "signal.>")

	store := &mockStore{}
	agent := trader.NewAgentForTest(js, store)

	go agent.Run()
	t.Cleanup(agent.Stop)

	time.Sleep(200 * time.Millisecond) // allow subscription to register

	exec := models.TradeExecution{
		Ticker:       "RELIANCE",
		Side:         models.SideLong,
		Quantity:     10,
		Price:        2500.50,
		StopLoss:     2400.00,
		TakeProfit:   2750.00,
		Sector:       "Energy",
		SignalReason: "VWAP breakout",
		ExecutionRef: "exec-001",
	}
	publishExec(t, js, exec)

	var got *models.TradeExecution
	select {
	case <-waitForEntry(store):
		store.mu.Lock()
		got = store.entries[0]
		store.mu.Unlock()
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for store entry")
	}

	if got.Ticker != "RELIANCE" {
		t.Fatalf("ticker = %q, want %q", got.Ticker, "RELIANCE")
	}
	if got.Side != models.SideLong {
		t.Fatalf("side = %q, want %q", got.Side, models.SideLong)
	}
	if got.Quantity != 10 {
		t.Fatalf("quantity = %d, want %d", got.Quantity, 10)
	}
	if got.Price != 2500.50 {
		t.Fatalf("price = %.2f, want %.2f", got.Price, 2500.50)
	}
	if got.ExecutionRef != "exec-001" {
		t.Fatalf("execution_ref = %q, want %q", got.ExecutionRef, "exec-001")
	}
}

func TestTrader_RejectsInvalidJSON(t *testing.T) {
	s := startJetStreamServer(t)
	js := connectJS(t, s)
	ensureStream(t, js, "trading", "signal.>")

	store := &mockStore{}
	agent := trader.NewAgentForTest(js, store)

	go agent.Run()
	t.Cleanup(agent.Stop)

	time.Sleep(200 * time.Millisecond)

	if err := js.Publish("signal.execute.trade", []byte("not json")); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	time.Sleep(500 * time.Millisecond)
	store.mu.Lock()
	count := len(store.entries)
	store.mu.Unlock()
	if count != 0 {
		t.Fatalf("expected 0 persisted entries for invalid JSON, got %d", count)
	}
}

func TestTrader_RejectsMissingTicker(t *testing.T) {
	s := startJetStreamServer(t)
	js := connectJS(t, s)
	ensureStream(t, js, "trading", "signal.>")

	store := &mockStore{}
	agent := trader.NewAgentForTest(js, store)

	go agent.Run()
	t.Cleanup(agent.Stop)

	time.Sleep(200 * time.Millisecond)

	exec := models.TradeExecution{
		Ticker:       "",
		Side:         models.SideLong,
		Quantity:     10,
		Price:        100,
		ExecutionRef: "exec-bad",
	}
	publishExec(t, js, exec)

	time.Sleep(500 * time.Millisecond)
	store.mu.Lock()
	count := len(store.entries)
	store.mu.Unlock()
	if count != 0 {
		t.Fatalf("expected 0 persisted entries for invalid execution, got %d", count)
	}
}

func TestTrader_RejectsNegativeQuantity(t *testing.T) {
	s := startJetStreamServer(t)
	js := connectJS(t, s)
	ensureStream(t, js, "trading", "signal.>")

	store := &mockStore{}
	agent := trader.NewAgentForTest(js, store)

	go agent.Run()
	t.Cleanup(agent.Stop)

	time.Sleep(200 * time.Millisecond)

	exec := models.TradeExecution{
		Ticker:       "TCS",
		Side:         models.SideShort,
		Quantity:     -5,
		Price:        3500,
		ExecutionRef: "exec-neg",
	}
	publishExec(t, js, exec)

	time.Sleep(500 * time.Millisecond)
	store.mu.Lock()
	count := len(store.entries)
	store.mu.Unlock()
	if count != 0 {
		t.Fatalf("expected 0 persisted entries for negative quantity, got %d", count)
	}
}

func TestTrader_NaksOnStoreFailure(t *testing.T) {
	s := startJetStreamServer(t)
	js := connectJS(t, s)
	ensureStream(t, js, "trading", "signal.>")

	store := &mockStore{}
	store.fail.Store(true)
	agent := trader.NewAgentForTest(js, store)

	go agent.Run()
	t.Cleanup(agent.Stop)

	time.Sleep(200 * time.Millisecond)

	exec := models.TradeExecution{
		Ticker:       "INFY",
		Side:         models.SideLong,
		Quantity:     5,
		Price:        1500,
		ExecutionRef: "exec-nak",
	}
	publishExec(t, js, exec)

	time.Sleep(500 * time.Millisecond)
	store.mu.Lock()
	count := len(store.entries)
	store.mu.Unlock()
	if count != 0 {
		t.Fatalf("expected 0 persisted entries for failing store, got %d", count)
	}
}

func TestTrader_SkipsEmptyExecutionRef(t *testing.T) {
	s := startJetStreamServer(t)
	js := connectJS(t, s)
	ensureStream(t, js, "trading", "signal.>")

	store := &mockStore{}
	agent := trader.NewAgentForTest(js, store)

	go agent.Run()
	t.Cleanup(agent.Stop)

	time.Sleep(200 * time.Millisecond)

	data, _ := json.Marshal(map[string]interface{}{
		"ticker":   "HDFC",
		"side":     "LONG",
		"quantity": 10,
		"price":    1600,
	})
	if err := js.Publish("signal.execute.trade", data); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	time.Sleep(500 * time.Millisecond)
	store.mu.Lock()
	count := len(store.entries)
	store.mu.Unlock()
	if count != 0 {
		t.Fatalf("expected 0 persisted entries for missing execution_ref, got %d", count)
	}
}

func TestTrader_ReceivesOnWildcardSubject(t *testing.T) {
	s := startJetStreamServer(t)
	js := connectJS(t, s)
	ensureStream(t, js, "trading", "signal.>")

	store := &mockStore{}
	agent := trader.NewAgentForTest(js, store)

	go agent.Run()
	t.Cleanup(agent.Stop)

	time.Sleep(200 * time.Millisecond)

	exec := models.TradeExecution{
		Ticker:       "WIPRO",
		Side:         models.SideShort,
		Quantity:     20,
		Price:        500,
		ExecutionRef: "exec-wild",
	}
	publishExec(t, js, exec, "signal.execute.trade")

	var got *models.TradeExecution
	select {
	case <-waitForEntry(store):
		store.mu.Lock()
		got = store.entries[0]
		store.mu.Unlock()
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for wildcard execution")
	}

	if got.Ticker != "WIPRO" {
		t.Fatalf("ticker = %q, want %q", got.Ticker, "WIPRO")
	}
}

func TestTrader_ShortSideRoundTrip(t *testing.T) {
	s := startJetStreamServer(t)
	js := connectJS(t, s)
	ensureStream(t, js, "trading", "signal.>")

	store := &mockStore{}
	agent := trader.NewAgentForTest(js, store)

	go agent.Run()
	t.Cleanup(agent.Stop)

	time.Sleep(200 * time.Millisecond)

	exec := models.TradeExecution{
		Ticker:       "TCS",
		Side:         models.SideShort,
		Quantity:     25,
		Price:        3450.00,
		Sector:       "IT",
		SignalReason: "Bearish sentiment",
		ExecutionRef: "exec-short-001",
	}
	publishExec(t, js, exec)

	var got *models.TradeExecution
	select {
	case <-waitForEntry(store):
		store.mu.Lock()
		got = store.entries[0]
		store.mu.Unlock()
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for store entry")
	}

	if got.Ticker != "TCS" {
		t.Fatalf("ticker = %q, want %q", got.Ticker, "TCS")
	}
	if got.Side != models.SideShort {
		t.Fatalf("side = %q, want %q", got.Side, models.SideShort)
	}
	if got.Quantity != 25 {
		t.Fatalf("quantity = %d, want %d", got.Quantity, 25)
	}
	if got.Price != 3450.00 {
		t.Fatalf("price = %.2f, want %.2f", got.Price, 3450.00)
	}
	if got.ExecutionRef != "exec-short-001" {
		t.Fatalf("execution_ref = %q, want %q", got.ExecutionRef, "exec-short-001")
	}
}

func TestTrader_ConcurrentMessages(t *testing.T) {
	s := startJetStreamServer(t)
	js := connectJS(t, s)
	ensureStream(t, js, "trading", "signal.>")

	store := &mockStore{}
	agent := trader.NewAgentForTest(js, store)

	go agent.Run()
	t.Cleanup(agent.Stop)

	time.Sleep(200 * time.Millisecond)

	const count = 50
	for i := 0; i < count; i++ {
		exec := models.TradeExecution{
			Ticker:       "STOCK",
			Side:         models.SideLong,
			Quantity:     i + 1,
			Price:        100.00 + float64(i),
			ExecutionRef: fmt.Sprintf("exec-concurrent-%03d", i),
		}
		publishExec(t, js, exec)
	}

	select {
	case <-waitForNEntries(store, count):
		store.mu.Lock()
		n := len(store.entries)
		store.mu.Unlock()
		if n != count {
			t.Fatalf("expected %d entries, got %d", count, n)
		}
	case <-time.After(5 * time.Second):
		store.mu.Lock()
		n := len(store.entries)
		store.mu.Unlock()
		t.Fatalf("timed out waiting for %d entries, got %d", count, n)
	}
}

func TestTrader_HandlesUnknownJSONFields(t *testing.T) {
	s := startJetStreamServer(t)
	js := connectJS(t, s)
	ensureStream(t, js, "trading", "signal.>")

	store := &mockStore{}
	agent := trader.NewAgentForTest(js, store)

	go agent.Run()
	t.Cleanup(agent.Stop)

	time.Sleep(200 * time.Millisecond)

	data := `{"ticker":"HDFC","side":"LONG","quantity":15,"price":1650,"execution_ref":"exec-unknown","extra_field":"ignored","nested":{"a":1}}`
	if err := js.Publish("signal.execute.trade", []byte(data)); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case <-waitForEntry(store):
		store.mu.Lock()
		got := store.entries[0]
		store.mu.Unlock()
		if got.Ticker != "HDFC" {
			t.Fatalf("ticker = %q, want %q", got.Ticker, "HDFC")
		}
		if got.Side != models.SideLong {
			t.Fatalf("side = %q, want %q", got.Side, models.SideLong)
		}
		if got.ExecutionRef != "exec-unknown" {
			t.Fatalf("execution_ref = %q, want %q", got.ExecutionRef, "exec-unknown")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for entry with unknown fields")
	}
}

func TestTrader_ShutsDownGracefully(t *testing.T) {
	s := startJetStreamServer(t)
	js := connectJS(t, s)
	ensureStream(t, js, "trading", "signal.>")

	store := &mockStore{}
	agent := trader.NewAgentForTest(js, store)

	go agent.Run()

	done := make(chan struct{})
	go func() {
		agent.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not complete within 2s")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func publishExec(t *testing.T, js *nats.JetStream, exec models.TradeExecution, subjects ...string) {
	t.Helper()
	subj := "signal.execute.trade"
	if len(subjects) > 0 {
		subj = subjects[0]
	}
	data, err := json.Marshal(exec)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if err := js.Publish(subj, data); err != nil {
		t.Fatalf("Publish: %v", err)
	}
}

func waitForNEntries(store *mockStore, n int) chan struct{} {
	ch := make(chan struct{}, 1)
	go func() {
		for {
			store.mu.Lock()
			count := len(store.entries)
			store.mu.Unlock()
			if count >= n {
				ch <- struct{}{}
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}()
	return ch
}

func waitForEntry(store *mockStore) chan struct{} {
	ch := make(chan struct{}, 1)
	go func() {
		for {
			store.mu.Lock()
			n := len(store.entries)
			store.mu.Unlock()
			if n > 0 {
				ch <- struct{}{}
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}()
	return ch
}
