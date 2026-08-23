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

type mockInternalTrader struct {
	fail     bool
	executed *models.TradeExecution
}

func (t *mockInternalTrader) Execute(e *models.TradeExecution) (*OrderResult, error) {
	t.executed = e
	if t.fail {
		return nil, assertInternalError
	}
	return &OrderResult{
		BrokerOrderID: "INT-" + e.ExecutionRef,
		ExecutedPrice: e.Price,
		ExecutedQty:   e.Quantity,
	}, nil
}

var assertInternalError = &mockInternalError{"trader unavailable"}

type mockInternalError struct{ msg string }

func (e *mockInternalError) Error() string { return e.msg }

func rawMsg(subj string, data []byte) *natscore.Msg {
	return &natscore.Msg{Subject: subj, Data: data}
}

func newTestAnalyst(trader TraderInterface) *Analyst {
	return &Analyst{
		trader:   trader,
		stopChan: make(chan struct{}),
		done:     make(chan struct{}),
	}
}

func TestAckOrLog_AckError(t *testing.T) {
	m := rawMsg("signal.execute.trade", nil)
	ackOrLog(m)
}

func TestHandleExecution_InvalidJSON_AckError(t *testing.T) {
	agent := newTestAnalyst(&mockInternalTrader{})
	m := rawMsg("signal.execute.trade", []byte("{invalid}"))
	agent.handleExecution(m)
}

func TestHandleExecution_StoreFailure_NakError(t *testing.T) {
	agent := newTestAnalyst(&mockInternalTrader{fail: true})
	data := []byte(`{"ticker":"TCS","side":"LONG","quantity":10,"price":100,"execution_ref":"exec-nak"}`)
	m := rawMsg("signal.execute.trade", data)
	agent.handleExecution(m)
}

func TestHandleExecution_Success_AckError(t *testing.T) {
	agent := newTestAnalyst(&mockInternalTrader{})
	data := []byte(`{"ticker":"INFY","side":"SHORT","quantity":5,"price":1500,"execution_ref":"exec-ack-err"}`)
	m := rawMsg("signal.execute.trade", data)
	agent.handleExecution(m)
}

func TestHandleExecution_EmptyData(t *testing.T) {
	mock := &mockInternalTrader{}
	agent := newTestAnalyst(mock)
	m := rawMsg("signal.execute.trade", nil)
	agent.handleExecution(m)
	if mock.executed != nil {
		t.Fatal("expected no execution for empty data")
	}
}

func TestHandleExecution_EmptyObject(t *testing.T) {
	mock := &mockInternalTrader{}
	agent := newTestAnalyst(mock)
	m := rawMsg("signal.execute.trade", []byte("{}"))
	agent.handleExecution(m)
	if mock.executed != nil {
		t.Fatal("expected no execution for empty object")
	}
}

func TestHandleExecution_ExtraFieldsIgnored(t *testing.T) {
	mock := &mockInternalTrader{}
	agent := newTestAnalyst(mock)
	data := []byte(`{"ticker":"WIPRO","side":"LONG","quantity":20,"price":500,"execution_ref":"exec-extra","unknown_field":"should_be_ignored","another_unknown":42}`)
	m := rawMsg("signal.execute.trade", data)
	agent.handleExecution(m)
	if mock.executed == nil {
		t.Fatal("expected execution to be processed")
	}
	if mock.executed.Ticker != "WIPRO" {
		t.Fatalf("ticker = %q, want %q", mock.executed.Ticker, "WIPRO")
	}
}

func TestRun_EnsureStreamFailure(t *testing.T) {
	srvOpts := &server.Options{
		Port:      -1,
		JetStream: false,
	}
	s := natsserver.RunServer(srvOpts)
	t.Cleanup(func() { s.Shutdown() })

	t.Setenv("NATS_URL", s.ClientURL())
	js, err := nats.ConnectJetStream()
	if err != nil {
		t.Skipf("ConnectJetStream: %v", err)
	}
	t.Cleanup(func() { js.Close() })

	agent := &Analyst{
		js:       js,
		trader:   &mockInternalTrader{},
		stopChan: make(chan struct{}),
		done:     make(chan struct{}),
	}
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

	t.Setenv("NATS_URL", s.ClientURL())
	js, err := nats.ConnectJetStream()
	if err != nil {
		t.Skipf("ConnectJetStream: %v", err)
	}
	t.Cleanup(func() { js.Close() })

	if err := js.EnsureStream(nats.StreamTrading); err != nil {
		t.Skipf("EnsureStream: %v", err)
	}
	js.Conn().Close()

	agent := &Analyst{
		js:       js,
		trader:   &mockInternalTrader{},
		stopChan: make(chan struct{}),
		done:     make(chan struct{}),
	}
	agent.Run()
}

func TestShutdownWithActiveAgent(t *testing.T) {
	srvOpts := &server.Options{
		Port:      -1,
		JetStream: true,
		StoreDir:  t.TempDir(),
	}
	s := natsserver.RunServer(srvOpts)
	t.Cleanup(func() { s.Shutdown() })

	t.Setenv("NATS_URL", s.ClientURL())
	js, err := nats.ConnectJetStream()
	if err != nil {
		t.Skipf("ConnectJetStream: %v", err)
	}
	t.Cleanup(func() { js.Close() })

	if err := js.EnsureStream(nats.StreamTrading); err != nil {
		t.Skipf("EnsureStream: %v", err)
	}

	agent := &Analyst{
		js:       js,
		trader:   &mockInternalTrader{},
		stopChan: make(chan struct{}),
		done:     make(chan struct{}),
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
