package nats

import (
	"time"

	"github.com/nats-io/nats.go"
)

type Conn = nats.Conn
type Msg = nats.Msg
type Subscription = nats.Subscription
type MsgHandler = nats.MsgHandler

const (
	SubjectIntentNew    = "signal.intent.new"
	SubjectExecuteTrade = "signal.execute.trade"
	SubjectSentimentUpd = "sentiment.update"
)

var (
	StreamTrading = StreamConfig{
		Name:     "trading",
		Subjects: []string{"signal.>"},
		MaxAge:   24 * time.Hour,
		Storage:  nats.FileStorage,
	}

	StreamSentiment = StreamConfig{
		Name:     "sentiment",
		Subjects: []string{"sentiment.>"},
		MaxAge:   7 * 24 * time.Hour,
		Storage:  nats.FileStorage,
	}
)

type StreamConfig struct {
	Name     string
	Subjects []string
	MaxAge   time.Duration
	Storage  nats.StorageType
}

type JetStream struct {
	conn *Conn
	js   nats.JetStreamContext
}
