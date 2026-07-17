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
// Mock trader
// ---------------------------------------------------------------------------

type mockTrader struct {
	mu     sync.Mutex
	execs  []*models.TradeExecution
	fail   atomic.Bool
	result *trader.OrderResult
}

func (m *mockTrader) Execute(e *models.TradeExecution) (*trader.OrderResult, error) {
	if m.fail.Load() {
		return nil, assertAnError
	}
	m.mu.Lock()
	m.execs = append(m.execs, e)
	m.mu.Unlock()

	if m.result != nil {
		return m.result, nil
	}
	return &trader.OrderResult{
		BrokerOrderID: "MOCK-" + e.ExecutionRef,
		ExecutedPrice: e.Price,
		ExecutedQty:   e.Quantity,
	}, nil
}

var assertAnError = &mockError{"trader unavailable"}

type mockError struct{ msg string }

func (e *mockError) Error() string { return e.msg }

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestNewAgent_NATSConnectFailure(t *testing.T) {
	t.Setenv("NATS_URL", "nats://localhost:1")
	_, err := trader.NewAgent()
	if err == nil {
		t.Fatal("expected error from NewAgent with invalid NATS URL")
	}
}

func TestTrader_ReceivesAndExecutes(t *testing.T) {
	s := startJetStreamServer(t)
	js := connectJS(t, s)
	ensureStream(t, js, "trading", "signal.>")

	mock := &mockTrader{}
	agent := trader.NewAgentForTest(js, mock)

	go agent.Run()
	t.Cleanup(agent.Stop)

	time.Sleep(200 * time.Millisecond)

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
	case <-waitForExec(mock):
		mock.mu.Lock()
		got = mock.execs[0]
		mock.mu.Unlock()
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for execution")
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
	if got.ExecutionRef != "exec-001" {
		t.Fatalf("execution_ref = %q, want %q", got.ExecutionRef, "exec-001")
	}
}

func TestTrader_RejectsInvalidJSON(t *testing.T) {
	s := startJetStreamServer(t)
	js := connectJS(t, s)
	ensureStream(t, js, "trading", "signal.>")

	mock := &mockTrader{}
	agent := trader.NewAgentForTest(js, mock)

	go agent.Run()
	t.Cleanup(agent.Stop)

	time.Sleep(200 * time.Millisecond)

	if err := js.Publish("signal.execute.trade", []byte("not json")); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	time.Sleep(500 * time.Millisecond)
	mock.mu.Lock()
	count := len(mock.execs)
	mock.mu.Unlock()
	if count != 0 {
		t.Fatalf("expected 0 executions for invalid JSON, got %d", count)
	}
}

func TestTrader_RejectsMissingTicker(t *testing.T) {
	s := startJetStreamServer(t)
	js := connectJS(t, s)
	ensureStream(t, js, "trading", "signal.>")

	mock := &mockTrader{}
	agent := trader.NewAgentForTest(js, mock)

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
	mock.mu.Lock()
	count := len(mock.execs)
	mock.mu.Unlock()
	if count != 0 {
		t.Fatalf("expected 0 executions for missing ticker, got %d", count)
	}
}

func TestTrader_NaksOnTraderFailure(t *testing.T) {
	s := startJetStreamServer(t)
	js := connectJS(t, s)
	ensureStream(t, js, "trading", "signal.>")

	mock := &mockTrader{}
	mock.fail.Store(true)
	agent := trader.NewAgentForTest(js, mock)

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
	mock.mu.Lock()
	count := len(mock.execs)
	mock.mu.Unlock()
	if count != 0 {
		t.Fatalf("expected 0 executions for failing trader, got %d", count)
	}
}

func TestTrader_ShortSideRoundTrip(t *testing.T) {
	s := startJetStreamServer(t)
	js := connectJS(t, s)
	ensureStream(t, js, "trading", "signal.>")

	mock := &mockTrader{}
	agent := trader.NewAgentForTest(js, mock)

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
	case <-waitForExec(mock):
		mock.mu.Lock()
		got = mock.execs[0]
		mock.mu.Unlock()
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for execution")
	}

	if got.Side != models.SideShort {
		t.Fatalf("side = %q, want %q", got.Side, models.SideShort)
	}
	if got.ExecutionRef != "exec-short-001" {
		t.Fatalf("execution_ref = %q, want %q", got.ExecutionRef, "exec-short-001")
	}
}

func TestTrader_ConcurrentMessages(t *testing.T) {
	s := startJetStreamServer(t)
	js := connectJS(t, s)
	ensureStream(t, js, "trading", "signal.>")

	mock := &mockTrader{}
	agent := trader.NewAgentForTest(js, mock)

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
	case <-waitForNExecs(mock, count):
		mock.mu.Lock()
		n := len(mock.execs)
		mock.mu.Unlock()
		if n != count {
			t.Fatalf("expected %d executions, got %d", count, n)
		}
	case <-time.After(10 * time.Second):
		mock.mu.Lock()
		n := len(mock.execs)
		mock.mu.Unlock()
		t.Fatalf("timed out waiting for %d executions, got %d", count, n)
	}
}

func TestTrader_HandlesUnknownJSONFields(t *testing.T) {
	s := startJetStreamServer(t)
	js := connectJS(t, s)
	ensureStream(t, js, "trading", "signal.>")

	mock := &mockTrader{}
	agent := trader.NewAgentForTest(js, mock)

	go agent.Run()
	t.Cleanup(agent.Stop)

	time.Sleep(200 * time.Millisecond)

	data := `{"ticker":"HDFC","side":"LONG","quantity":15,"price":1650,"execution_ref":"exec-unknown","extra_field":"ignored","nested":{"a":1}}`
	if err := js.Publish("signal.execute.trade", []byte(data)); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case <-waitForExec(mock):
		mock.mu.Lock()
		got := mock.execs[0]
		mock.mu.Unlock()
		if got.Ticker != "HDFC" {
			t.Fatalf("ticker = %q, want %q", got.Ticker, "HDFC")
		}
		if got.ExecutionRef != "exec-unknown" {
			t.Fatalf("execution_ref = %q, want %q", got.ExecutionRef, "exec-unknown")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for execution with unknown fields")
	}
}

func TestTrader_ShutsDownGracefully(t *testing.T) {
	s := startJetStreamServer(t)
	js := connectJS(t, s)
	ensureStream(t, js, "trading", "signal.>")

	mock := &mockTrader{}
	agent := trader.NewAgentForTest(js, mock)

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

func waitForExec(mock *mockTrader) chan struct{} {
	return waitForNExecs(mock, 1)
}

func waitForNExecs(mock *mockTrader, n int) chan struct{} {
	ch := make(chan struct{}, 1)
	go func() {
		for {
			mock.mu.Lock()
			count := len(mock.execs)
			mock.mu.Unlock()
			if count >= n {
				ch <- struct{}{}
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}()
	return ch
}
