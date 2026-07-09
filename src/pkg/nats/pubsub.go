package nats

import (
	"fmt"

	"github.com/nats-io/nats.go"
)

func (j *JetStream) Publish(subj string, data []byte) error {
	_, err := j.js.Publish(subj, data)
	if err != nil {
		return fmt.Errorf("publish %q: %w", subj, err)
	}
	return nil
}

func (j *JetStream) PublishMsg(msg *Msg) error {
	_, err := j.js.PublishMsg(msg)
	if err != nil {
		return fmt.Errorf("publish msg %q: %w", msg.Subject, err)
	}
	return nil
}

func (j *JetStream) Subscribe(subj string, cb MsgHandler) (*Subscription, error) {
	sub, err := j.js.Subscribe(subj, cb, nats.DeliverNew(), nats.AckExplicit())
	if err != nil {
		return nil, fmt.Errorf("subscribe %q: %w", subj, err)
	}
	return sub, nil
}

func (j *JetStream) QueueSubscribe(subj, queue string, cb MsgHandler) (*Subscription, error) {
	sub, err := j.js.QueueSubscribe(subj, queue, cb, nats.DeliverNew(), nats.AckExplicit())
	if err != nil {
		return nil, fmt.Errorf("queue subscribe %q/%q: %w", subj, queue, err)
	}
	return sub, nil
}

func (j *JetStream) PullSubscribe(subj, durable string) (*Subscription, error) {
	sub, err := j.js.PullSubscribe(subj, durable, nats.AckExplicit())
	if err != nil {
		return nil, fmt.Errorf("pull subscribe %q: %w", subj, err)
	}
	return sub, nil
}
