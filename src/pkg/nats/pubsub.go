package nats

import (
	"errors"
	"fmt"
	"strings"

	"github.com/nats-io/nats.go"
)

// ErrEmptySubject is returned by publish and subscribe methods when the
// subject argument is empty.
var ErrEmptySubject = errors.New("subject cannot be empty")

// Publish sends data on the given subject with at-least-once semantics
// (the underlying JetStream publish waits for a confirmation from the
// server).  Returns an error if the subject is empty or the publish fails.
func (j *JetStream) Publish(subj string, data []byte) error {
	if subj == "" {
		return ErrEmptySubject
	}
	_, err := j.js.Publish(subj, data)
	if err != nil {
		return fmt.Errorf("publish %q: %w", subj, err)
	}
	return nil
}

// PublishMsg is identical to Publish but accepts a pre-constructed *Msg
// value, allowing the caller to set headers, reply subjects, etc.
func (j *JetStream) PublishMsg(msg *Msg) error {
	if msg.Subject == "" {
		return ErrEmptySubject
	}
	_, err := j.js.PublishMsg(msg)
	if err != nil {
		return fmt.Errorf("publish msg %q: %w", msg.Subject, err)
	}
	return nil
}

// Subscribe creates a JetStream push consumer on the given subject.
// Messages are delivered asynchronously via the supplied callback and
// must be explicitly Acked (AckExplicit).  Only new messages are
// delivered (DeliverNew).  When durablePrefix is set on the JetStream,
// a durable consumer is created so messages survive restarts.
func (j *JetStream) Subscribe(subj string, cb MsgHandler) (*Subscription, error) {
	if subj == "" {
		return nil, ErrEmptySubject
	}
	opts := []nats.SubOpt{nats.DeliverNew(), nats.AckExplicit()}
	if j.durablePrefix != "" {
		opts = append(opts, nats.Durable(durableName(j.durablePrefix, subj)))
	}
	sub, err := j.js.Subscribe(subj, cb, opts...)
	if err != nil {
		return nil, fmt.Errorf("subscribe %q: %w", subj, err)
	}
	return sub, nil
}

// QueueSubscribe creates a load-balanced JetStream push consumer.
// Messages on the given subject are distributed among all subscribers
// sharing the same queue name.  Each message must be explicitly Acked.
func (j *JetStream) QueueSubscribe(subj, queue string, cb MsgHandler) (*Subscription, error) {
	if subj == "" {
		return nil, ErrEmptySubject
	}
	opts := []nats.SubOpt{nats.DeliverNew(), nats.AckExplicit()}
	if j.durablePrefix != "" {
		opts = append(opts, nats.Durable(durableName(j.durablePrefix, subj)))
	}
	sub, err := j.js.QueueSubscribe(subj, queue, cb, opts...)
	if err != nil {
		return nil, fmt.Errorf("queue subscribe %q/%q: %w", subj, queue, err)
	}
	return sub, nil
}

// PullSubscribe creates a JetStream pull consumer identified by the given
// durable name.  The caller must explicitly call Fetch or FetchBatch on the
// returned subscription to receive messages.  Each message requires an
// explicit Ack.
func (j *JetStream) PullSubscribe(subj, durable string) (*Subscription, error) {
	if subj == "" {
		return nil, ErrEmptySubject
	}
	sub, err := j.js.PullSubscribe(subj, durable, nats.AckExplicit())
	if err != nil {
		return nil, fmt.Errorf("pull subscribe %q: %w", subj, err)
	}
	return sub, nil
}

// durableName derives a JetStream durable-consumer name from a prefix and
// the subject.  Wildcards and dots are replaced with dashes so the name
// is a valid NATS durable identifier.
func durableName(prefix, subj string) string {
	s := strings.ReplaceAll(subj, ".", "-")
	s = strings.ReplaceAll(s, "*", "star")
	s = strings.ReplaceAll(s, ">", "gt")
	return prefix + "-" + s
}
