package riskmanager

import (
	"log"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/nats"
)

type Analyst struct {
	js       *nats.JetStream
	stopChan chan struct{}
}

func NewAgent() (*Analyst, error) {
	js, err := nats.ConnectJetStream()
	if err != nil {
		return nil, err
	}
	return &Analyst{
		js:       js,
		stopChan: make(chan struct{}),
	}, nil
}

func (g *Analyst) Run() {
	log.Println("[Risk Manager] Ready...")
	<-g.stopChan
}

func (g *Analyst) Stop() {
	close(g.stopChan)
	g.js.Close()
}
