// Package metrics provides functionality for building Prometheus-compatible metric collectors.
package metrics

import (
	"context"
	"errors"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
	log "github.com/sirupsen/logrus"
)

var (
	// timeBuckets is based on Prometheus client_golang prometheus.DefBuckets.
	timeBuckets = prometheus.ExponentialBuckets(0.00025, 2, 16) // from 0.25ms to 8 seconds

	dnsLatencyHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "astrolavos_dns_latency_seconds",
			Help:    "Histogram of DNS resolution latency in seconds",
			Buckets: timeBuckets,
		},
		[]string{"domain", "tag", "prober_type"},
	)

	connLatencyHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "astrolavos_conn_latency_seconds",
			Help:    "Histogram of TCP connection latency in seconds",
			Buckets: timeBuckets,
		},
		[]string{"domain", "tag", "prober_type"},
	)

	tlsLatencyHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "astrolavos_tls_latency_seconds",
			Help:    "Histogram of TLS handshake latency in seconds",
			Buckets: timeBuckets,
		},
		[]string{"domain", "tag", "prober_type"},
	)

	gotConnLatencyHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "astrolavos_gotconn_latency_seconds",
			Help:    "Histogram of time to obtain a connection in seconds",
			Buckets: timeBuckets,
		},
		[]string{"domain", "tag", "prober_type"},
	)

	firstByteLatencyHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "astrolavos_firstbyte_latency_seconds",
			Help:    "Histogram of time to first byte in seconds",
			Buckets: timeBuckets,
		},
		[]string{"domain", "tag", "prober_type"},
	)

	totalLatencyHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "astrolavos_total_latency_seconds",
			Help:    "Histogram of total request latency in seconds",
			Buckets: timeBuckets,
		},
		[]string{"domain", "tag", "prober_type"},
	)

	totalRequestsCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "astrolavos_requests_total",
			Help: "Total number of probe requests made by Astrolavos",
		},
		[]string{"domain", "tag", "status_code", "prober_type"},
	)

	totalErrorsCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "astrolavos_errors_total",
			Help: "Total number of probe errors encountered by Astrolavos",
		},
		[]string{"domain", "tag", "error", "prober_type"},
	)
)

// PrometheusClient holds state needed for Prometheus metric collection and pushing.
type PrometheusClient struct {
	pusher *push.Pusher
}

// NewPrometheusClient initializes a new Prometheus client and registers all metrics.
func NewPrometheusClient(_ bool, promPushGateway string) *PrometheusClient {
	prometheus.MustRegister(dnsLatencyHistogram)
	prometheus.MustRegister(connLatencyHistogram)
	prometheus.MustRegister(tlsLatencyHistogram)
	prometheus.MustRegister(gotConnLatencyHistogram)
	prometheus.MustRegister(firstByteLatencyHistogram)
	prometheus.MustRegister(totalLatencyHistogram)
	prometheus.MustRegister(totalRequestsCounter)
	prometheus.MustRegister(totalErrorsCounter)

	pusher := push.New(promPushGateway, "astrolavos").
		Collector(dnsLatencyHistogram).
		Collector(connLatencyHistogram).
		Collector(tlsLatencyHistogram).
		Collector(gotConnLatencyHistogram).
		Collector(firstByteLatencyHistogram).
		Collector(totalLatencyHistogram).
		Collector(totalRequestsCounter).
		Collector(totalErrorsCounter)

	log.Info("Metrics setup - scrape /metrics")

	return &PrometheusClient{pusher: pusher}
}

// UpdateDNSHistogram records a DNS resolution duration observation.
func (p *PrometheusClient) UpdateDNSHistogram(domain, proberType, tag string, duration float64) {
	dnsLatencyHistogram.WithLabelValues(domain, tag, proberType).Observe(duration)
	log.Debug("Updated metric for DNS latency")
}

// UpdateConnHistogram records a TCP connection duration observation.
func (p *PrometheusClient) UpdateConnHistogram(domain, proberType, tag string, duration float64) {
	connLatencyHistogram.WithLabelValues(domain, tag, proberType).Observe(duration)
	log.Debug("Updated metric for connection latency")
}

// UpdateTLSHistogram records a TLS handshake duration observation.
func (p *PrometheusClient) UpdateTLSHistogram(domain, proberType, tag string, duration float64) {
	tlsLatencyHistogram.WithLabelValues(domain, tag, proberType).Observe(duration)
	log.Debug("Updated metric for TLS latency")
}

// UpdateGotConnHistogram records the time to obtain a connection.
func (p *PrometheusClient) UpdateGotConnHistogram(domain, proberType, tag string, duration float64) {
	gotConnLatencyHistogram.WithLabelValues(domain, tag, proberType).Observe(duration)
	log.Debug("Updated metric for GotConnection latency")
}

// UpdateFirstByteHistogram records the time to first byte.
func (p *PrometheusClient) UpdateFirstByteHistogram(domain, proberType, tag string, duration float64) {
	firstByteLatencyHistogram.WithLabelValues(domain, tag, proberType).Observe(duration)
	log.Debug("Updated metric for FirstByte latency")
}

// UpdateTotalHistogram records the total request duration.
func (p *PrometheusClient) UpdateTotalHistogram(domain, proberType, tag string, duration float64) {
	totalLatencyHistogram.WithLabelValues(domain, tag, proberType).Observe(duration)
	log.Debug("Updated metric for total latency")
}

// UpdateRequestsCounter increments the total requests counter.
func (p *PrometheusClient) UpdateRequestsCounter(domain, proberType, tag, statusCode string) {
	totalRequestsCounter.WithLabelValues(domain, tag, statusCode, proberType).Inc()
	log.Debug("Updated metric for total requests counter")
}

// UpdateErrorsCounter increments the total errors counter with a categorized error type.
// Error messages are categorized into a fixed set of labels to prevent cardinality explosion.
func (p *PrometheusClient) UpdateErrorsCounter(domain, proberType, tag string, err error) {
	category := CategorizeError(err)
	totalErrorsCounter.WithLabelValues(domain, tag, category, proberType).Inc()
	log.Debug("Updated metric for total errors counter")
}

// errorPattern maps an error message substring to a known error category.
type errorPattern struct {
	substr   string
	category string
}

// errorPatterns defines the mapping from lowercase error substrings to categories.
// Order matters: first match wins.
var errorPatterns = []errorPattern{
	{"no such host", "dns_error"},
	{"dns", "dns_error"},
	{"connection refused", "connection_refused"},
	{"connection reset", "connection_reset"},
	{"timeout", "timeout"},
	{"tls", "tls_error"},
	{"x509", "tls_error"},
	{"certificate", "tls_error"},
	{"eof", "eof"},
}

// CategorizeError maps an error to a known category string for use as a Prometheus label.
// This prevents high cardinality from raw error messages.
func CategorizeError(err error) string {
	if err == nil {
		return "unknown"
	}

	// Check sentinel errors first via errors.Is for proper unwrapping
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}

	if errors.Is(err, context.Canceled) {
		return "canceled"
	}

	// Fall back to substring matching on the lowercased error message
	errStr := strings.ToLower(err.Error())

	for _, p := range errorPatterns {
		if strings.Contains(errStr, p.substr) {
			return p.category
		}
	}

	return "unknown"
}

// PrometheusPush sends the collected Prometheus metrics to the push gateway.
func (p *PrometheusClient) PrometheusPush() {
	log.Debug("Pushing metrics to push gateway")

	if err := p.pusher.Push(); err != nil {
		log.WithError(err).Error("Failed to push metrics to push gateway")
	}
}
