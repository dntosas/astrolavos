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

	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

// Endpoint encapsulates information needed to run probes against a destination.
type Endpoint struct {
	URI                 string
	Interval            time.Duration
	Tag                 string
	Retries             int
	ProberType          string
	ReuseConnection     bool
	SkipTLSVerification bool
}

// Astrolavos encapsulates information needed for our
// application to run.
type Astrolavos struct {
	port     int
	agent    *agent
	isOneOff bool
}

// NewAstrolavos creates a new application struct.
func NewAstrolavos(port int, endpoints []*Endpoint, promPushGateway string, isOneOff bool) *Astrolavos {
	promC := metrics.NewPrometheusClient(isOneOff, promPushGateway)
	a := newAgent(endpoints, isOneOff, promC)

	return &Astrolavos{
		port:     port,
		agent:    a,
		isOneOff: isOneOff,
	}
}

// Start inits.
func (a *Astrolavos) Start() error {
	if a.isOneOff {
		a.startOneOffMode()
	} else {
		return a.startServerMode()
	}

	log.Debug("Exiting agent now.")

	return nil
}

// startServerMode is responsible for running our agent and http components.
// After starting them it blocks and waits for shutdown signal.
func (a *Astrolavos) startServerMode() error {
	// Start the agent before the HTTP part
	log.Debug("Starting Agent")
	a.agent.start()

	// Initialize HTTP server
	server := &http.Server{
		Addr: fmt.Sprintf(":%d", a.port),
		ReadHeaderTimeout: 180*time.Second,
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
				log.Fatal(fmt.Errorf("%s: %w", "Failed to start listening server", err))
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

func (a *Astrolavos) startOneOffMode() {
	defer a.agent.promC.PrometheusPush()

	log.Debug("Starting OneOff Agent")
	a.agent.start()
	a.agent.wg.Wait()
}
