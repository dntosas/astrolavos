package machinery

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dntosas/astrolavos/internal/handlers"
	"github.com/dntosas/astrolavos/internal/metrics"
	"github.com/dntosas/astrolavos/internal/model"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

// Astrolavos is the main application struct that orchestrates the agent and HTTP server.
type Astrolavos struct {
	port     int
	agent    *agent
	isOneOff bool
}

// NewAstrolavos creates a new Astrolavos application instance.
func NewAstrolavos(port int, endpoints []*model.Endpoint, promPushGateway string, isOneOff bool) *Astrolavos {
	promC := metrics.NewPrometheusClient(isOneOff, promPushGateway)
	a := newAgent(endpoints, isOneOff, promC)

	return &Astrolavos{
		port:     port,
		agent:    a,
		isOneOff: isOneOff,
	}
}

// Start launches Astrolavos in the configured mode (server or one-off).
func (a *Astrolavos) Start() error {
	if a.isOneOff {
		a.startOneOffMode()

		return nil
	}

	return a.startServerMode()
}

// startServerMode runs the agent and HTTP server, blocking until a shutdown signal is received.
func (a *Astrolavos) startServerMode() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the agent before the HTTP server
	log.Debug("Starting Agent")
	a.agent.start(ctx)

	// Initialize HTTP server with proper timeouts
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", a.port),
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/live", handlers.OKHandler)
	http.HandleFunc("/ready", handlers.OKHandler)
	http.HandleFunc("/latency", handlers.LatencyHandler)

	// Start listening asynchronously
	go func() {
		log.WithField("port", a.port).Info("Starting HTTP server")

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.WithError(err).Fatal("Failed to start HTTP server")
		}

		log.Info("HTTP server stopped")
	}()

	// Wait for shutdown signal
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	<-shutdown

	log.Info("Shutdown signal received")

	// Cancel context to stop all probers
	cancel()
	a.agent.wait()

	// Shut down HTTP server with a grace period
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("HTTP server shutdown failed: %w", err)
	}

	return nil
}

// startOneOffMode runs all probers once and pushes metrics to the gateway.
func (a *Astrolavos) startOneOffMode() {
	defer a.agent.promC.PrometheusPush()

	ctx := context.Background()

	log.Debug("Starting OneOff Agent")
	a.agent.start(ctx)
	a.agent.wait()
}
