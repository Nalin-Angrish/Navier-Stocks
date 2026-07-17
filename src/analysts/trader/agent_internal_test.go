package trader

import (
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	natsserver "github.com/nats-io/nats-server/v2/test"
	natscore "github.com/nats-io/nats.go"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/nats"
)

type mockInternalStore struct {
	fail     bool
	inserted *models.TradeExecution
}

func (s *mockInternalStore) InsertTradeLogEntry(e *models.TradeExecution) error {
	s.inserted = e
	if s.fail {
		return assertInternalError
	}
	return nil
}

var assertInternalError = &mockInternalError{"store unavailable"}

type mockInternalError struct{ msg string }

func (e *mockInternalError) Error() string { return e.msg }

func rawMsg(subj string, data []byte) *natscore.Msg {
	return &natscore.Msg{Subject: subj, Data: data}
}

func newJetStreamForURL(t *testing.T, url string) *nats.JetStream {
	t.Helper()
	t.Setenv("NATS_URL", url)
	js, err := nats.ConnectJetStream()
	if err != nil {
		t.Skipf("ConnectJetStream: %v", err)
	}
	t.Cleanup(func() { js.Close() })
	return js
}

func TestAckOrLog_AckError(t *testing.T) {
	m := rawMsg("signal.execute.trade", nil)
	ackOrLog(m)
}

func TestHandleExecution_InvalidJSON_AckError(t *testing.T) {
	agent := &Analyst{
		store:    &mockInternalStore{},
		stopChan: make(chan struct{}),
	}
	m := rawMsg("signal.execute.trade", []byte("{invalid}"))
	agent.handleExecution(m)
}

func TestHandleExecution_StoreFailure_NakError(t *testing.T) {
	agent := &Analyst{
		store:    &mockInternalStore{fail: true},
		stopChan: make(chan struct{}),
	}
	data := []byte(`{"ticker":"TCS","side":"LONG","quantity":10,"price":100,"execution_ref":"exec-nak"}`)
	m := rawMsg("signal.execute.trade", data)
	agent.handleExecution(m)
}

func TestHandleExecution_Success_AckError(t *testing.T) {
	agent := &Analyst{
		store:    &mockInternalStore{},
		stopChan: make(chan struct{}),
	}
	data := []byte(`{"ticker":"INFY","side":"SHORT","quantity":5,"price":1500,"execution_ref":"exec-ack-err"}`)
	m := rawMsg("signal.execute.trade", data)
	agent.handleExecution(m)
}

func TestHandleExecution_EmptyData(t *testing.T) {
	store := &mockInternalStore{}
	agent := &Analyst{store: store, stopChan: make(chan struct{})}
	m := rawMsg("signal.execute.trade", nil)
	agent.handleExecution(m)
	if store.inserted != nil {
		t.Fatal("expected no insertion for empty data")
	}
}

func TestHandleExecution_EmptyObject(t *testing.T) {
	store := &mockInternalStore{}
	agent := &Analyst{store: store, stopChan: make(chan struct{})}
	m := rawMsg("signal.execute.trade", []byte("{}"))
	agent.handleExecution(m)
	if store.inserted != nil {
		t.Fatal("expected no insertion for empty object")
	}
}

func TestHandleExecution_ExtraFieldsIgnored(t *testing.T) {
	store := &mockInternalStore{}
	agent := &Analyst{store: store, stopChan: make(chan struct{})}
	data := []byte(`{"ticker":"WIPRO","side":"LONG","quantity":20,"price":500,"execution_ref":"exec-extra","unknown_field":"should_be_ignored","another_unknown":42}`)
	m := rawMsg("signal.execute.trade", data)
	agent.handleExecution(m)
	if store.inserted == nil {
		t.Fatal("expected execution to be processed")
	}
	if store.inserted.Ticker != "WIPRO" {
		t.Fatalf("ticker = %q, want %q", store.inserted.Ticker, "WIPRO")
	}
}

func TestRun_EnsureStreamFailure(t *testing.T) {
	srvOpts := &server.Options{
		Port:      -1,
		JetStream: false,
	}
	s := natsserver.RunServer(srvOpts)
	t.Cleanup(func() { s.Shutdown() })

	js := newJetStreamForURL(t, s.ClientURL())
	agent := &Analyst{
		js:       js,
		store:    &mockInternalStore{},
		stopChan: make(chan struct{}),
	}
	// EnsureStream should fail since the server has JetStream disabled
	agent.Run()
}

func TestRun_SubscribeFailure(t *testing.T) {
	srvOpts := &server.Options{
		Port:      -1,
		JetStream: true,
		StoreDir:  t.TempDir(),
	}
	s := natsserver.RunServer(srvOpts)
	t.Cleanup(func() { s.Shutdown() })

	js := newJetStreamForURL(t, s.ClientURL())

	if err := js.EnsureStream(nats.StreamTrading); err != nil {
		t.Skipf("EnsureStream: %v", err)
	}
	js.Conn().Close()

	agent := &Analyst{
		js:       js,
		store:    &mockInternalStore{},
		stopChan: make(chan struct{}),
	}
	agent.Run()
}

func TestShutdownWithActiveAgent(t *testing.T) {
	s := startJetStreamServer(t)
	js := connectJS(t, s)
	ensureStream(t, js, "trading", "signal.>")

	store := &mockInternalStore{}
	agent := &Analyst{
		js:       js,
		store:    store,
		stopChan: make(chan struct{}),
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		agent.Run()
	}()

	time.Sleep(100 * time.Millisecond)

	agent.Stop()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit within 2s of Stop")
	}
}

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
		t.Fatalf("ConnectJetStream: %v", err)
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
