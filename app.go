package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"astrolavos/pkg/handlers"
	"astrolavos/pkg/probers"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

// app encapsulates information needed for our
// application to run.
type app struct {
	port     int
	agent    *agent
	isOneOff bool
}

// newApp creates a new application struct.
func newApp(port int, endpoints []*endpoint, promPushGateway string, isOneOff bool) *app {
	promC := probers.NewPrometheusClient(isOneOff, promPushGateway)
	a := newAgent(endpoints, isOneOff, promC)
	return &app{
		port:     port,
		agent:    a,
		isOneOff: isOneOff,
	}
}

func (a *app) runOneOffMode() {
	defer a.agent.promC.PrometheusPush()

	log.Debug("Starting OneOff Agent")
	a.agent.start()
	a.agent.wg.Wait()
}

func (a *app) run() error {
	if a.isOneOff {
		a.runOneOffMode()
	} else {
		return a.runServerMode()
	}
	log.Debug("Exiting agent now.")
	return nil
}

// runServerMode is responsible for running our agent and http components.
// After starting them it blocks and waits for shutdown signal
func (a *app) runServerMode() error {
	// Start the agent before the HTTP part
	log.Debug("Starting Agent")
	a.agent.start()

	// Initialize HTTP server
	server := &http.Server{
		Addr: fmt.Sprintf(":%d", a.port),
	}
	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/live", handlers.OKHandler)
	http.HandleFunc("/ready", handlers.OKHandler)
	http.HandleFunc("/latency", handlers.LatencyHandler)

	// Start listening asynchronously
	go func() {
		log.Info("Starting server on port:", a.port)
		if err := server.ListenAndServe(); err != nil {
			if err.Error() != "http: Server closed" {
				log.Fatal(fmt.Errorf("%s: %s", "Failed to start listening server", err))
			}
			log.Info("Server shutdown")
		}
	}()

	// Setting up signal capturing
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	// Waiting for SIGINT/SIGTERM
	<-shutdown

	// Kill trace agent go routine workers
	a.agent.stop()

	// Shut down server, waiting 5secs for all requests before kill them.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatal("Failed to stop listening server")
	}

	return nil
}
