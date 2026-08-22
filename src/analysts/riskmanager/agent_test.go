package riskmanager_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	natsserver "github.com/nats-io/nats-server/v2/test"

	"github.com/Nalin-Angrish/Navier-Stocks/src/analysts/riskmanager"
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

func publishIntent(t *testing.T, js *nats.JetStream, intent models.TradeIntent, subject string) {
	t.Helper()
	data, err := json.Marshal(intent)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if err := js.Publish(subject, data); err != nil {
		t.Fatalf("Publish(%q): %v", subject, err)
	}
}

// waitForAccept polls the agent's accepted counter until it reaches n.
func waitForAccept(a *riskmanager.Analyst, n int64) chan struct{} {
	ch := make(chan struct{}, 1)
	go func() {
		for {
			if a.Accepted() >= n {
				ch <- struct{}{}
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}()
	return ch
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestNewAgent_NATSConnectFailure(t *testing.T) {
	t.Setenv("NATS_URL", "nats://localhost:1")
	_, err := riskmanager.NewAgent()
	if err == nil {
		t.Fatal("expected error from NewAgent with invalid NATS URL")
	}
}

func TestRiskManager_SubscribesToWildcard(t *testing.T) {
	s := startJetStreamServer(t)
	js := connectJS(t, s)

	agent := riskmanager.NewAgentForTest(js)
	go agent.Run()
	t.Cleanup(agent.Stop)

	time.Sleep(200 * time.Millisecond)

	intent := models.TradeIntent{
		Ticker:       "RELIANCE",
		Side:         models.SideLong,
		CurrentPrice: 2500.50,
		SignalReason: "VWAP_breakout_plus_volume_plus_BB",
		Timestamp:    time.Now(),
	}
	publishIntent(t, js, intent, "signal.intent.RELIANCE")

	select {
	case <-waitForAccept(agent, 1):
		if agent.Accepted() != 1 {
			t.Fatalf("Accepted() = %d, want 1", agent.Accepted())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for intent to be accepted")
	}
}

func TestRiskManager_RejectsInvalidJSON(t *testing.T) {
	s := startJetStreamServer(t)
	js := connectJS(t, s)

	agent := riskmanager.NewAgentForTest(js)
	go agent.Run()
	t.Cleanup(agent.Stop)

	time.Sleep(200 * time.Millisecond)

	if err := js.Publish("signal.intent.RELIANCE", []byte("not json")); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	time.Sleep(500 * time.Millisecond)
	if agent.Accepted() != 0 {
		t.Fatalf("Accepted() = %d, want 0 for invalid JSON", agent.Accepted())
	}
}

func TestRiskManager_RejectsInvalidIntent(t *testing.T) {
	s := startJetStreamServer(t)
	js := connectJS(t, s)

	agent := riskmanager.NewAgentForTest(js)
	go agent.Run()
	t.Cleanup(agent.Stop)

	time.Sleep(200 * time.Millisecond)

	intent := models.TradeIntent{
		Ticker:       "",
		Side:         models.SideLong,
		CurrentPrice: 0,
	}
	publishIntent(t, js, intent, "signal.intent.XYZ")

	time.Sleep(500 * time.Millisecond)
	if agent.Accepted() != 0 {
		t.Fatalf("Accepted() = %d, want 0 for invalid intent", agent.Accepted())
	}
}

func TestRiskManager_HandlesUnknownJSONFields(t *testing.T) {
	s := startJetStreamServer(t)
	js := connectJS(t, s)

	agent := riskmanager.NewAgentForTest(js)
	go agent.Run()
	t.Cleanup(agent.Stop)

	time.Sleep(200 * time.Millisecond)

	data := []byte(`{"ticker":"HDFC","side":"LONG","current_price":1650,"vwap":1640,"signal_reason":"test","extra_field":"ignored","nested":{"a":1}}`)
	if err := js.Publish("signal.intent.HDFC", data); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case <-waitForAccept(agent, 1):
		if agent.Accepted() != 1 {
			t.Fatalf("Accepted() = %d, want 1", agent.Accepted())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for intent with unknown fields")
	}
}

func TestRiskManager_ShutsDownGracefully(t *testing.T) {
	s := startJetStreamServer(t)
	js := connectJS(t, s)

	agent := riskmanager.NewAgentForTest(js)
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
