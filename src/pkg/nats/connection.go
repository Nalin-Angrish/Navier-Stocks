package nats

import (
	"os"

	"github.com/nats-io/nats.go"
)

type Conn = nats.Conn

func Connect() (*Conn, error) {
	url := os.Getenv("NATS_URL")
	if url == "" {
		url = nats.DefaultURL
	}
	return nats.Connect(url)
}
