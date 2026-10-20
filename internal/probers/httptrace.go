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
		attemptCtx, cancel := h.attemptContext(ctx)
		defer cancel()

		var traceErr error
		t, traceErr = h.trace(attemptCtx)

		return traceErr
	})

	// Status code and connection kind of the last attempt; a failure before a
	// connection was obtained counts as a fresh connection.
	statusCode, reused := "", false
	if t != nil {
		statusCode, reused = t.statusCode, t.reused
	}

	h.promC.UpdateRequestsCounter(h.endpoint, "httptrace", h.tag, statusCode, reused)

	if err != nil {
		log.Errorf("HTTPTrace %s failed after %d attempts: %v", h, h.retries, err)
		h.promC.UpdateErrorsCounter(h.endpoint, "httptrace", h.tag, reused, err)
	} else {
		// Update all exposed Prometheus metrics histograms
		h.promC.UpdateDNSHistogram(h.endpoint, "httptrace", h.tag, reused, t.dnsDuration)
		h.promC.UpdateConnHistogram(h.endpoint, "httptrace", h.tag, reused, t.connDuration)
		h.promC.UpdateTLSHistogram(h.endpoint, "httptrace", h.tag, reused, t.tlsDuration)
		h.promC.UpdateGotConnHistogram(h.endpoint, "httptrace", h.tag, reused, t.gotConnDuration)
		h.promC.UpdateFirstByteHistogram(h.endpoint, "httptrace", h.tag, reused, t.firstByteDuration)
		h.promC.UpdateTotalHistogram(h.endpoint, "httptrace", h.tag, reused, t.totalDuration)
	}
}
