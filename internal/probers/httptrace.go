package probers

import (
	"context"
	"fmt"

	log "github.com/sirupsen/logrus"
)

// HTTPTrace implements the Prober interface for HTTP trace probes,
// measuring detailed connection timing (DNS, TLS, TTFB, etc.).
type HTTPTrace struct {
	ProberConfig
}

// NewHTTPTrace creates a new HTTPTrace prober with the given configuration.
func NewHTTPTrace(c ProberConfig) *HTTPTrace {
	return &HTTPTrace{c}
}

// String returns a human-readable description of the prober configuration.
func (h *HTTPTrace) String() string {
	return fmt.Sprintf("httpTrace Prober Endpoint: %s - Interval: %v - Tag: %s - Retries: %d", h.endpoint, h.interval, h.tag, h.retries)
}

// Run starts the HTTP trace prober, executing probes according to the configured mode.
func (h *HTTPTrace) Run(ctx context.Context) {
	h.runLoop(ctx, h.String(), h.probe)
}

// probe performs a single HTTP trace measurement with retry logic and records metrics.
func (h *HTTPTrace) probe(ctx context.Context) {
	var t *tracePoint

	err := h.retryWithBackoff(ctx, func() error {
		var traceErr error
		t, traceErr = h.trace(ctx)

		return traceErr
	})

	// Determine status code for the request counter
	statusCode := ""
	if t != nil {
		statusCode = t.statusCode
	}

	h.promC.UpdateRequestsCounter(h.endpoint, "httptrace", h.tag, statusCode)

	if err != nil {
		log.Errorf("HTTPTrace %s failed after %d attempts: %v", h, h.retries, err)
		h.promC.UpdateErrorsCounter(h.endpoint, "httptrace", h.tag, err)
	} else {
		// Update all exposed Prometheus metrics histograms
		h.promC.UpdateDNSHistogram(h.endpoint, "httptrace", h.tag, t.dnsDuration)
		h.promC.UpdateConnHistogram(h.endpoint, "httptrace", h.tag, t.connDuration)
		h.promC.UpdateTLSHistogram(h.endpoint, "httptrace", h.tag, t.tlsDuration)
		h.promC.UpdateGotConnHistogram(h.endpoint, "httptrace", h.tag, t.gotConnDuration)
		h.promC.UpdateFirstByteHistogram(h.endpoint, "httptrace", h.tag, t.firstByteDuration)
		h.promC.UpdateTotalHistogram(h.endpoint, "httptrace", h.tag, t.totalDuration)
	}
}
