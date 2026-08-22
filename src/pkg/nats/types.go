// Package nats provides a thin convenience layer over github.com/nats-io/nats.go
// and github.com/nats-io/nats-server/v2.  It handles connection setup,
// JetStream stream initialisation, and exposes Publish / Subscribe /
// QueueSubscribe / PullSubscribe helpers with consistent error wrapping and
// sensible defaults (DeliverNew + AckExplicit).
//
// The two pre-configured stream definitions (StreamTrading and
// StreamSentiment) match the agent communication topology described in the
// HLD: signal.intent.*, signal.execute.*, and sentiment.update.
package nats

import (
	"time"

	"github.com/nats-io/nats.go"
)

// Type aliases so callers can refer to nats.Conn, nats.Msg etc. without
// importing the underlying library directly.
type Conn = nats.Conn
type Msg = nats.Msg
type Subscription = nats.Subscription
type MsgHandler = nats.MsgHandler

// Subject constants used by the four agents for inter-agent communication.
const (
	SubjectIntentNew    = "signal.intent.new"    // Quantitative Scout → Risk Manager
	SubjectExecuteTrade = "signal.execute.trade" // Risk Manager → Trader Gateway
	SubjectSentimentUpd = "sentiment.update"     // Sentiment Analyst → (sentiment stream)

	// SubjectPricePrefix is prepended to a ticker to form the per-ticker
	// price-stream subject (Quantitative Scout → exit monitor), e.g.
	// signal.price.RELIANCE.  It is covered by the signal.> wildcard on
	// StreamTrading.
	SubjectPricePrefix = "signal.price."

	// SubjectExecutePrefix is prepended to a ticker to form the per-ticker
	// execution subject (Risk Manager / exit monitor → Trader Gateway),
	// e.g. signal.execute.TCS.
	SubjectExecutePrefix = "signal.execute."
)

// PriceSubject builds the per-ticker price-stream subject, e.g.
// signal.price.TCS.  Exported so publisher and subscriber agree on format.
func PriceSubject(ticker string) string {
	return SubjectPricePrefix + ticker
}

// ExecuteSubject builds the per-ticker execution subject, e.g.
// signal.execute.RELIANCE.
func ExecuteSubject(ticker string) string {
	return SubjectExecutePrefix + ticker
}

// Pre-configured stream definitions.  These are idempotently created by
// agents during their Run() phase via JetStream.EnsureStream.
var (
	// StreamTrading carries all signal.* subjects (intents and executions)
	// with a 24-hour retention window and file-based storage.
	StreamTrading = StreamConfig{
		Name:     "trading",
		Subjects: []string{"signal.>"},
		MaxAge:   24 * time.Hour,
		Storage:  nats.FileStorage,
	}

	// StreamSentiment carries sentiment.update messages with a 7-day
	// retention window.
	StreamSentiment = StreamConfig{
		Name:     "sentiment",
		Subjects: []string{"sentiment.>"},
		MaxAge:   7 * 24 * time.Hour,
		Storage:  nats.FileStorage,
	}
)

// StreamConfig groups the parameters needed to define or reference a
// JetStream stream.
type StreamConfig struct {
	Name     string
	Subjects []string
	MaxAge   time.Duration
	Storage  nats.StorageType
}

// JetStream wraps a raw NATS connection and a JetStream context, providing
// a single point of access for both stream management and publish/subscribe
// operations.
type JetStream struct {
	conn *Conn
	js   nats.JetStreamContext
}
