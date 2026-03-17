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
	"github.com/dntosas/astrolavos/internal/health"
	"github.com/dntosas/astrolavos/internal/metrics"
	"github.com/dntosas/astrolavos/internal/model"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

const (
	// preStopDrainDuration is passed to the PreStopHandler and controls
	// how long the httpGet preStop hook blocks before returning to the
	// kubelet. During this time kube-proxy propagates the endpoint
	// removal.
	//
	// Constraints:
	//   preStopDrainDuration < httpWriteTimeout  (or the server kills the response early)
	//   preStopDrainDuration + shutdownDrainDelay + httpServerShutdownTimeout < terminationGracePeriodSeconds
	preStopDrainDuration = 15 * time.Second

	// shutdownDrainDelay is a short safety-net pause in the SIGTERM
	// handler. In Kubernetes the preStop hook already handled the long
	// drain, so this only covers edge cases (bare-metal, docker-compose,
	// or preStop misconfiguration).
	shutdownDrainDelay = 3 * time.Second

	httpServerShutdownTimeout = 10 * time.Second

	// httpWriteTimeout must be greater than preStopDrainDuration,
	// otherwise the server closes the preStop response before the
	// drain finishes and the kubelet proceeds to SIGTERM prematurely.
	httpWriteTimeout = preStopDrainDuration + 15*time.Second
)

// Astrolavos is the main application struct that orchestrates the agent and HTTP server.
type Astrolavos struct {
	port           int
	agent          *agent
	endpoints      []*model.Endpoint
	version        string
	maxPayloadSize int
	isOneOff       bool
	health         *health.State
}

// NewAstrolavos creates a new Astrolavos application instance.
func NewAstrolavos(port int, endpoints []*model.Endpoint, promPushGateway string, version string, maxPayloadSize int, isOneOff bool) *Astrolavos {
	promC := metrics.NewPrometheusClient(isOneOff, promPushGateway)
	a := newAgent(endpoints, isOneOff, promC)

	return &Astrolavos{
		port:           port,
		agent:          a,
		endpoints:      endpoints,
		version:        version,
		maxPayloadSize: maxPayloadSize,
		isOneOff:       isOneOff,
		health:         health.NewState(),
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
//
// Kubernetes termination lifecycle:
//  1. Pod enters Terminating
//  2. preStop httpGet /prestop → marks not-ready, blocks for preStopDrainDuration
//     (kube-proxy removes pod from endpoints during this window)
//  3. preStop returns → kubelet sends SIGTERM
//  4. SIGTERM handler: short safety drain → cancel probers → shutdown HTTP
func (a *Astrolavos) startServerMode() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log.Debug("Starting Agent")
	a.agent.start(ctx)

	server := a.newHTTPServer()

	go func() {
		log.WithField("port", a.port).Info("Starting HTTP server")

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.WithError(err).Fatal("Failed to start HTTP server")
		}

		log.Info("HTTP server stopped")
	}()

	a.health.SetAlive()
	a.health.SetReady()
	log.Info("Application is alive and ready")

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	<-sig

	log.Info("Shutdown signal received")

	return a.gracefulShutdown(cancel, server)
}

// newHTTPServer wires up routes and returns a configured *http.Server.
func (a *Astrolavos) newHTTPServer() *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/live", health.LiveHandler(a.health))
	mux.HandleFunc("/ready", health.ReadyHandler(a.health))
	mux.HandleFunc("/prestop", health.PreStopHandler(a.health, preStopDrainDuration))
	mux.HandleFunc("/latency", handlers.NewLatencyHandler(a.maxPayloadSize))
	mux.HandleFunc("/status", handlers.NewStatusHandler(a.version, a.endpoints))

	return &http.Server{
		Addr:              fmt.Sprintf(":%d", a.port),
		Handler:           mux,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      httpWriteTimeout,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

// gracefulShutdown executes the ordered teardown after SIGTERM.
// In Kubernetes the preStop hook already marked the pod not-ready and
// waited for endpoint propagation; the short drain here is a safety net
// for non-K8s environments.
func (a *Astrolavos) gracefulShutdown(cancel context.CancelFunc, server *http.Server) error {
	a.health.SetNotReady()
	log.WithField("delay", shutdownDrainDelay).Info("Marked as not-ready, safety drain")

	time.Sleep(shutdownDrainDelay)
	log.Info("Safety drain complete")

	cancel()
	a.agent.wait()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), httpServerShutdownTimeout)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("HTTP server shutdown failed: %w", err)
	}

	a.health.SetNotAlive()
	log.Info("Shutdown complete")

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
