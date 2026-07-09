package nats

import (
	"fmt"
	"os"

	"github.com/nats-io/nats.go"
)

func Connect() (*Conn, error) {
	url := os.Getenv("NATS_URL")
	if url == "" {
		url = nats.DefaultURL
	}
	return nats.Connect(url)
}

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

func (j *JetStream) Conn() *Conn {
	return j.conn
}

func (j *JetStream) EnsureStream(cfg StreamConfig) error {
	_, err := j.js.StreamInfo(cfg.Name)
	if err != nil {
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

func (j *JetStream) Close() {
	j.conn.Close()
}
