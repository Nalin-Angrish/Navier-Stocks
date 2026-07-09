package nats_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	natsserver "github.com/nats-io/nats-server/v2/test"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/nats"
)

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

func connect(t *testing.T, s *server.Server) *nats.JetStream {
	t.Helper()
	t.Setenv("NATS_URL", s.ClientURL())
	js, err := nats.ConnectJetStream()
	if err != nil {
		t.Fatalf("ConnectJetStream() failed: %v", err)
	}
	t.Cleanup(func() { js.Close() })
	return js
}

func ensureStream(t *testing.T, js *nats.JetStream, name string, subjects ...string) {
	t.Helper()
	if err := js.EnsureStream(nats.StreamConfig{Name: name, Subjects: subjects}); err != nil {
		t.Fatalf("EnsureStream(%q) failed: %v", name, err)
	}
}

func TestConnect(t *testing.T) {
	s := startJetStreamServer(t)
	t.Setenv("NATS_URL", s.ClientURL())

	nc, err := nats.Connect()
	if err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}
	defer nc.Close()

	if !nc.IsConnected() {
		t.Fatal("expected connected state")
	}
}

func TestConnectJetStream(t *testing.T) {
	s := startJetStreamServer(t)
	js := connect(t, s)

	if js.Conn() == nil {
		t.Fatal("Conn() returned nil")
	}
	if !js.Conn().IsConnected() {
		t.Fatal("expected connected state")
	}
}

func TestEnsureStream_Creates(t *testing.T) {
	s := startJetStreamServer(t)
	js := connect(t, s)

	ensureStream(t, js, "test-create", "test.create.>")
}

func TestEnsureStream_Idempotent(t *testing.T) {
	s := startJetStreamServer(t)
	js := connect(t, s)

	cfg := nats.StreamConfig{
		Name:     "test-idempotent",
		Subjects: []string{"test.idempotent.>"},
	}
	if err := js.EnsureStream(cfg); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := js.EnsureStream(cfg); err != nil {
		t.Fatalf("second call (should be no-op): %v", err)
	}
}

func TestPublishSubscribe_RoundTrip(t *testing.T) {
	s := startJetStreamServer(t)
	js := connect(t, s)
	ensureStream(t, js, "test-ps", "test.ps.>")

	received := make(chan *nats.Msg, 1)
	sub, err := js.Subscribe("test.ps.roundtrip", func(m *nats.Msg) {
		received <- m
		if err := m.Ack(); err != nil {
			t.Errorf("Ack: %v", err)
		}
	})
	if err != nil {
		t.Fatalf("Subscribe() failed: %v", err)
	}
	t.Cleanup(func() {
		if err := sub.Unsubscribe(); err != nil {
			t.Errorf("Unsubscribe: %v", err)
		}
	})

	payload := []byte("hello world")
	if err := js.Publish("test.ps.roundtrip", payload); err != nil {
		t.Fatalf("Publish() failed: %v", err)
	}

	select {
	case m := <-received:
		if string(m.Data) != string(payload) {
			t.Fatalf("got %q, want %q", m.Data, payload)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for message")
	}
}

func TestPublishMsg_RoundTrip(t *testing.T) {
	s := startJetStreamServer(t)
	js := connect(t, s)
	ensureStream(t, js, "test-msg", "test.msg.>")

	received := make(chan *nats.Msg, 1)
	sub, err := js.Subscribe("test.msg.roundtrip", func(m *nats.Msg) {
		received <- m
		if err := m.Ack(); err != nil {
			t.Errorf("Ack: %v", err)
		}
	})
	if err != nil {
		t.Fatalf("Subscribe() failed: %v", err)
	}
	t.Cleanup(func() {
		if err := sub.Unsubscribe(); err != nil {
			t.Errorf("Unsubscribe: %v", err)
		}
	})

	if err := js.PublishMsg(&nats.Msg{Subject: "test.msg.roundtrip", Data: []byte("via msg")}); err != nil {
		t.Fatalf("PublishMsg() failed: %v", err)
	}

	select {
	case m := <-received:
		if string(m.Data) != "via msg" {
			t.Fatalf("got %q, want %q", m.Data, "via msg")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for message")
	}
}

func TestSubscribe_MultipleMessages(t *testing.T) {
	s := startJetStreamServer(t)
	js := connect(t, s)
	ensureStream(t, js, "test-multi", "test.multi.>")

	count := 0
	done := make(chan struct{})
	sub, err := js.Subscribe("test.multi.batch", func(m *nats.Msg) {
		count++
		if err := m.Ack(); err != nil {
			t.Errorf("Ack: %v", err)
		}
		if count == 3 {
			close(done)
		}
	})
	if err != nil {
		t.Fatalf("Subscribe() failed: %v", err)
	}
	t.Cleanup(func() {
		if err := sub.Unsubscribe(); err != nil {
			t.Errorf("Unsubscribe: %v", err)
		}
	})

	for _, msg := range []string{"one", "two", "three"} {
		if err := js.Publish("test.multi.batch", []byte(msg)); err != nil {
			t.Fatalf("Publish(%q) failed: %v", msg, err)
		}
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("got %d messages, want 3", count)
	}
}

func TestQueueSubscribe_Distribution(t *testing.T) {
	s := startJetStreamServer(t)
	js := connect(t, s)
	ensureStream(t, js, "test-queue", "test.queue.>")

	const msgCount = 10

	var (
		aCount, bCount int64
		done           = make(chan struct{})
		closeOnce      sync.Once
	)

	subA, err := js.QueueSubscribe("test.queue.work", "workers", func(m *nats.Msg) {
		atomic.AddInt64(&aCount, 1)
		if err := m.Ack(); err != nil {
			t.Errorf("Ack: %v", err)
		}
		if atomic.LoadInt64(&aCount)+atomic.LoadInt64(&bCount) >= msgCount {
			closeOnce.Do(func() { close(done) })
		}
	})
	if err != nil {
		t.Fatalf("QueueSubscribe(A) failed: %v", err)
	}
	t.Cleanup(func() {
		if err := subA.Unsubscribe(); err != nil {
			t.Errorf("Unsubscribe: %v", err)
		}
	})

	subB, err := js.QueueSubscribe("test.queue.work", "workers", func(m *nats.Msg) {
		atomic.AddInt64(&bCount, 1)
		if err := m.Ack(); err != nil {
			t.Errorf("Ack: %v", err)
		}
		if atomic.LoadInt64(&aCount)+atomic.LoadInt64(&bCount) >= msgCount {
			closeOnce.Do(func() { close(done) })
		}
	})
	if err != nil {
		t.Fatalf("QueueSubscribe(B) failed: %v", err)
	}
	t.Cleanup(func() {
		if err := subB.Unsubscribe(); err != nil {
			t.Errorf("Unsubscribe: %v", err)
		}
	})

	for i := 0; i < msgCount; i++ {
		if err := js.Publish("test.queue.work", []byte("job")); err != nil {
			t.Fatalf("Publish() failed: %v", err)
		}
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out: A=%d B=%d", atomic.LoadInt64(&aCount), atomic.LoadInt64(&bCount))
	}

	ta, tb := atomic.LoadInt64(&aCount), atomic.LoadInt64(&bCount)
	if ta+tb != msgCount {
		t.Fatalf("expected %d total, got %d (A=%d B=%d)", msgCount, ta+tb, ta, tb)
	}
}

func TestPullSubscribe(t *testing.T) {
	s := startJetStreamServer(t)
	js := connect(t, s)
	ensureStream(t, js, "test-pull", "test.pull.>")

	sub, err := js.PullSubscribe("test.pull.fetch", "pull-worker")
	if err != nil {
		t.Fatalf("PullSubscribe() failed: %v", err)
	}
	t.Cleanup(func() {
		if err := sub.Unsubscribe(); err != nil {
			t.Errorf("Unsubscribe: %v", err)
		}
	})

	for i := 0; i < 3; i++ {
		if err := js.Publish("test.pull.fetch", []byte("msg")); err != nil {
			t.Fatalf("Publish() failed: %v", err)
		}
	}

	msgs, err := sub.Fetch(3)
	if err != nil {
		t.Fatalf("Fetch() failed: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	for _, m := range msgs {
		if err := m.Ack(); err != nil {
			t.Errorf("Ack: %v", err)
		}
	}
}

func TestPullSubscribe_EmptyFetch(t *testing.T) {
	s := startJetStreamServer(t)
	js := connect(t, s)
	ensureStream(t, js, "test-pull-empty", "test.pull.empty.>")

	sub, err := js.PullSubscribe("test.pull.empty.nomsg", "pull-empty")
	if err != nil {
		t.Fatalf("PullSubscribe() failed: %v", err)
	}
	t.Cleanup(func() {
		if err := sub.Unsubscribe(); err != nil {
			t.Errorf("Unsubscribe: %v", err)
		}
	})

	_, err = sub.Fetch(1)
	if err == nil {
		t.Fatal("expected timeout error on Fetch with no messages")
	}
}

func TestConnectJetStream_InvalidURL(t *testing.T) {
	t.Setenv("NATS_URL", "nats://localhost:1")

	_, err := nats.ConnectJetStream()
	if err == nil {
		t.Fatal("expected error connecting to invalid URL")
	}
}

func TestStreamConfig_Defaults(t *testing.T) {
	if nats.StreamTrading.Name != "trading" {
		t.Fatalf("StreamTrading.Name = %q, want %q", nats.StreamTrading.Name, "trading")
	}
	if nats.StreamSentiment.Name != "sentiment" {
		t.Fatalf("StreamSentiment.Name = %q, want %q", nats.StreamSentiment.Name, "sentiment")
	}
}

func TestSubjectConstants(t *testing.T) {
	tests := []struct {
		got, want string
	}{
		{nats.SubjectIntentNew, "signal.intent.new"},
		{nats.SubjectExecuteTrade, "signal.execute.trade"},
		{nats.SubjectSentimentUpd, "sentiment.update"},
	}
	for _, tc := range tests {
		if tc.got != tc.want {
			t.Fatalf("got %q, want %q", tc.got, tc.want)
		}
	}
}

func TestConnect_DefaultURLFallback(t *testing.T) {
	t.Setenv("NATS_URL", "")
	nc, err := nats.Connect()
	if err == nil {
		nc.Close()
	}
}

func TestEnsureStream_AddStreamError(t *testing.T) {
	s := startJetStreamServer(t)
	js := connect(t, s)

	err := js.EnsureStream(nats.StreamConfig{Name: ""})
	if err == nil {
		t.Fatal("expected error creating stream with empty name")
	}
}

func TestPublish_NoMatchingStream(t *testing.T) {
	s := startJetStreamServer(t)
	js := connect(t, s)

	err := js.Publish("no.such.stream", []byte("data"))
	if err == nil {
		t.Fatal("expected error publishing outside any stream")
	}
}

func TestPublishMsg_NoMatchingStream(t *testing.T) {
	s := startJetStreamServer(t)
	js := connect(t, s)

	err := js.PublishMsg(&nats.Msg{Subject: "no.such.stream", Data: []byte("data")})
	if err == nil {
		t.Fatal("expected error publishing outside any stream")
	}
}

func TestSubscribe_NoMatchingStream(t *testing.T) {
	s := startJetStreamServer(t)
	js := connect(t, s)

	_, err := js.Subscribe("no.such.stream", func(m *nats.Msg) {
		if err := m.Ack(); err != nil {
			t.Errorf("Ack: %v", err)
		}
	})
	if err == nil {
		t.Fatal("expected error subscribing outside any stream")
	}
}

func TestQueueSubscribe_NoMatchingStream(t *testing.T) {
	s := startJetStreamServer(t)
	js := connect(t, s)

	_, err := js.QueueSubscribe("no.such.stream", "workers", func(m *nats.Msg) {
		if err := m.Ack(); err != nil {
			t.Errorf("Ack: %v", err)
		}
	})
	if err == nil {
		t.Fatal("expected error subscribing outside any stream")
	}
}

func TestPullSubscribe_NoMatchingStream(t *testing.T) {
	s := startJetStreamServer(t)
	js := connect(t, s)

	_, err := js.PullSubscribe("no.such.stream", "pull-worker")
	if err == nil {
		t.Fatal("expected error subscribing outside any stream")
	}
}
