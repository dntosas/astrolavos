package main

import (
	"astrolavos/pkg/probers"
	"sync"

	log "github.com/sirupsen/logrus"
)

// agent struct is holding info for our tracing agent
type agent struct {
	probers []probers.Prober
	wg      *sync.WaitGroup
	promC   *probers.PrometheusClient
}

// newAgent is the constructor of Agent struct
func newAgent(endpoints []*endpoint, isOneOff bool, promC *probers.PrometheusClient) *agent {
	var wg sync.WaitGroup
	var o probers.Prober
	probersList := []probers.Prober{}

	for _, e := range endpoints {
		p := probers.NewProberConfig(&wg, e.uri, e.retries, e.tag, e.interval, isOneOff, e.reuseConnection, promC)
		switch e.proberType {
		case "tcp":
			o = probers.NewTCP(p)
		case "httpTrace":
			o = probers.NewHTTPTrace(p)
		default:
			log.Errorf("Couldn't find a legit prober type: %s", e.proberType)
			continue
		}

		wg.Add(1)
		probersList = append(probersList, o)
	}

	return &agent{
		probers: probersList,
		wg:      &wg,
		promC:   promC,
	}
}

// start is starting all workers of the agent
func (a *agent) start() {
	log.Debug(a.probers)
	for _, prober := range a.probers {
		log.Debugf("Starting go routing for prober: %s", prober)
		go prober.Run()
	}
}

// stop  is responsible to send exit signal to all workers
func (a *agent) stop() {
	log.Info("Stopping all individual probers of the agent")
	for _, prober := range a.probers {
		prober.Stop()
	}

	log.Debug("Waiting for all agent probers to exit")
	a.wg.Wait()
	log.Info("Exiting agent now.")
}
