package nats

import (
	"errors"
	"fmt"
	"os"

	"github.com/nats-io/nats.go"
)

// Connect opens a plain NATS connection using the NATS_URL environment
// variable (default nats://localhost:4222).  Most callers should use
// ConnectJetStream instead, which also wraps the connection with a
// JetStream context.
func Connect() (*Conn, error) {
	url := os.Getenv("NATS_URL")
	if url == "" {
		url = nats.DefaultURL
	}
	return nats.Connect(url)
}

// ConnectJetStream creates a NATS connection and wraps it with a JetStream
// context.  The returned JetStream value provides stream management,
// publishing, and subscribing in a single struct.  The caller must call
// Close() to release the underlying NATS connection.
func ConnectJetStream() (*JetStream, error) {
	nc, err := Connect()
	if err != nil {
		return nil, fmt.Errorf("nats: %w", err)
	}
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("jetstream: %w", err)
	}
	return &JetStream{conn: nc, js: js}, nil
}

// SetDurablePrefix configures the JetStream to create durable consumers
// for all future Subscribe/QueueSubscribe calls.  This ensures messages
// survive agent restarts (OBS-04).  An empty prefix disables durability
// (ephemeral consumers, the pre-fix default).
func (j *JetStream) SetDurablePrefix(prefix string) {
	j.durablePrefix = prefix
}

// Conn returns the underlying NATS connection, primarily used for
// inspecting connection state or closing it directly.
func (j *JetStream) Conn() *Conn {
	return j.conn
}

// ErrEmptyStreamName is returned by EnsureStream when the provided config
// has an empty Name field.
var ErrEmptyStreamName = errors.New("stream name cannot be empty")

// EnsureStream checks whether a stream with the given name exists and
// creates it (with the supplied subjects, max-age, and storage type) if it
// does not.  This operation is idempotent — calling it multiple times with
// the same config is safe.
func (j *JetStream) EnsureStream(cfg StreamConfig) error {
	if cfg.Name == "" {
		return ErrEmptyStreamName
	}
	_, err := j.js.StreamInfo(cfg.Name)
	if err != nil {
		if !errors.Is(err, nats.ErrStreamNotFound) {
			return fmt.Errorf("ensure stream %q: %w", cfg.Name, err)
		}
		_, err = j.js.AddStream(&nats.StreamConfig{
			Name:     cfg.Name,
			Subjects: cfg.Subjects,
			MaxAge:   cfg.MaxAge,
			Storage:  cfg.Storage,
		})
		if err != nil {
			return fmt.Errorf("ensure stream %q: %w", cfg.Name, err)
		}
	}
	return nil
}

// Close shuts down the underlying NATS connection.  After this call the
// JetStream value must not be used.
func (j *JetStream) Close() {
	j.conn.Close()
}
