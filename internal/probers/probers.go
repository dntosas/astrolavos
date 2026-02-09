// Package probers provides the interface and common logic for network probing implementations.
package probers

import (
	"context"
	"crypto/tls"
	"net/http"
	"sync"
	"time"

	"github.com/dntosas/astrolavos/internal/metrics"

	log "github.com/sirupsen/logrus"
)

// Prober interface defines the contract each probe implementation must fulfill.
type Prober interface {
	String() string
	Run(ctx context.Context)
}

// ProberOptions contains all configuration needed to create a ProberConfig.
type ProberOptions struct {
	WG                  *sync.WaitGroup
	PromClient          *metrics.PrometheusClient
	Endpoint            string
	Tag                 string
	Retries             int
	Interval            time.Duration
	IsOneOff            bool
	ReuseConnection     bool
	SkipTLSVerification bool
}

// ProberConfig holds the shared configuration and helpers for all prober implementations.
type ProberConfig struct {
	HTTPProberConfig

	wg       *sync.WaitGroup
	promC    *metrics.PrometheusClient
	endpoint string
	retries  int
	tag      string
	interval time.Duration
	isOneOff bool
}

// HTTPProberConfig holds HTTP-specific configuration.
type HTTPProberConfig struct {
	reuseConnection bool
	skipTLS         bool
	client          *http.Client
}

// NewProberConfig creates a new ProberConfig from the given options.
func NewProberConfig(opts ProberOptions) ProberConfig {
	p := ProberConfig{
		wg:       opts.WG,
		promC:    opts.PromClient,
		endpoint: opts.Endpoint,
		retries:  opts.Retries,
		tag:      opts.Tag,
		interval: opts.Interval,
		isOneOff: opts.IsOneOff,
	}

	p.HTTPProberConfig = HTTPProberConfig{
		reuseConnection: opts.ReuseConnection,
		skipTLS:         opts.SkipTLSVerification,
		client:          getCustomClient(opts.ReuseConnection, opts.SkipTLSVerification),
	}

	return p
}

// runLoop handles the common one-off vs interval execution pattern.
// It calls probe on each tick (or once in one-off mode) and respects context cancellation.
func (p *ProberConfig) runLoop(ctx context.Context, name string, probe func(ctx context.Context)) {
	defer p.wg.Done()

	if p.isOneOff {
		log.Infof("Starting (OneOff) %s", name)
		probe(ctx)

		return
	}

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	log.Infof("Starting %s", name)

	for {
		select {
		case <-ctx.Done():
			log.Infof("%s: received shutdown signal, exiting", name)

			return
		case <-ticker.C:
			log.Debugf("%s: starting new probe", name)
			probe(ctx)
		}
	}
}

// retryWithBackoff executes fn up to p.retries times with exponential backoff.
// It respects context cancellation between retries.
func (p *ProberConfig) retryWithBackoff(ctx context.Context, fn func() error) error {
	var lastErr error

	for attempt := 0; attempt < p.retries; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		// Don't sleep on the last attempt
		if attempt < p.retries-1 {
			backoff := time.Duration(100*(1<<uint(attempt))) * time.Millisecond
			log.Debugf("Attempt %d/%d failed for %s, retrying after %v: %v",
				attempt+1, p.retries, p.endpoint, backoff, lastErr)

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}
	}

	return lastErr
}

func getCustomClient(reuseCon, skipTLS bool) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if skipTLS {
		//nolint:gosec
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	if !reuseCon {
		// with below option we force new connection every time we do a request
		transport.MaxIdleConnsPerHost = -1
	}

	return &http.Client{Transport: transport}
}
