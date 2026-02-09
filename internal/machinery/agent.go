// Package machinery holds the core application logic for the Astrolavos agent.
package machinery

import (
	"context"
	"sync"

	"github.com/dntosas/astrolavos/internal/metrics"
	"github.com/dntosas/astrolavos/internal/model"
	"github.com/dntosas/astrolavos/internal/probers"

	log "github.com/sirupsen/logrus"
)

// agent manages a collection of probers and coordinates their lifecycle.
type agent struct {
	probers []probers.Prober
	wg      *sync.WaitGroup
	promC   *metrics.PrometheusClient
}

// newAgent creates a new agent with probers for each configured endpoint.
func newAgent(endpoints []*model.Endpoint, isOneOff bool, promC *metrics.PrometheusClient) *agent {
	var wg sync.WaitGroup

	probersList := []probers.Prober{}

	for _, e := range endpoints {
		p := probers.NewProberConfig(probers.ProberOptions{
			WG:                  &wg,
			PromClient:          promC,
			Endpoint:            e.URI,
			Tag:                 e.Tag,
			Retries:             e.Retries,
			Interval:            e.Interval,
			TCPTimeout:          e.TCPTimeout,
			IsOneOff:            isOneOff,
			ReuseConnection:     e.ReuseConnection,
			SkipTLSVerification: e.SkipTLSVerification,
		})

		var o probers.Prober

		switch e.ProberType {
		case "tcp":
			o = probers.NewTCP(p)
		case "httpTrace":
			o = probers.NewHTTPTrace(p)
		default:
			log.Errorf("Unknown prober type: %s", e.ProberType)

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

// start launches all probers as goroutines with the given context.
func (a *agent) start(ctx context.Context) {
	for _, prober := range a.probers {
		log.Debugf("Starting goroutine for prober: %s", prober)
		go prober.Run(ctx)
	}
}

// wait blocks until all prober goroutines have finished.
func (a *agent) wait() {
	log.Debug("Waiting for all agent probers to exit")
	a.wg.Wait()
	log.Info("All agent probers have stopped")
}
