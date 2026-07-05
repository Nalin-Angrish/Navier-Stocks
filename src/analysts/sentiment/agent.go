package sentiment

import (
	"log"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/nats"
)

type Analyst struct {
	nc       *nats.Conn
	stopChan chan struct{}
}

func NewAgent() (*Analyst, error) {
	nc, err := nats.Connect()
	if err != nil {
		return nil, err
	}
	return &Analyst{
		nc:       nc,
		stopChan: make(chan struct{}),
	}, nil
}

func (g *Analyst) Run() {
	log.Println("[Sentiment Analyst] Ready...")
	<-g.stopChan
}

func (g *Analyst) Stop() {
	close(g.stopChan)
	g.nc.Close()
}
