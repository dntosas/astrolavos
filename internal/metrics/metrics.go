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
		[]string{"domain", "tag", "proberType"},
	)

	connLatencyHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "astrolavos_conn_latency_seconds",
			Help:    "Histogram of TCP connection latency in seconds",
			Buckets: timeBuckets,
		},
		[]string{"domain", "tag", "proberType"},
	)

	tlsLatencyHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "astrolavos_tls_latency_seconds",
			Help:    "Histogram of TLS handshake latency in seconds",
			Buckets: timeBuckets,
		},
		[]string{"domain", "tag", "proberType"},
	)

	gotConnLatencyHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "astrolavos_gotconn_latency_seconds",
			Help:    "Histogram of time to obtain a connection in seconds",
			Buckets: timeBuckets,
		},
		[]string{"domain", "tag", "proberType"},
	)

	firstByteLatencyHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "astrolavos_firstbyte_latency_seconds",
			Help:    "Histogram of time to first byte in seconds",
			Buckets: timeBuckets,
		},
		[]string{"domain", "tag", "proberType"},
	)

	totalLatencyHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "astrolavos_total_latency_seconds",
			Help:    "Histogram of total request latency in seconds",
			Buckets: timeBuckets,
		},
		[]string{"domain", "tag", "proberType"},
	)

	totalRequestsCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "astrolavos_requests_total",
			Help: "Total number of probe requests made by Astrolavos",
		},
		[]string{"domain", "tag", "status_code", "proberType"},
	)

	totalErrorsCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "astrolavos_errors_total",
			Help: "Total number of probe errors encountered by Astrolavos",
		},
		[]string{"domain", "tag", "error", "proberType"},
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

// CategorizeError maps an error to a known category string for use as a Prometheus label.
// This prevents high cardinality from raw error messages.
func CategorizeError(err error) string {
	if err == nil {
		return "unknown"
	}

	errStr := strings.ToLower(err.Error())

	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case strings.Contains(errStr, "no such host") || strings.Contains(errStr, "dns"):
		return "dns_error"
	case strings.Contains(errStr, "connection refused"):
		return "connection_refused"
	case strings.Contains(errStr, "connection reset"):
		return "connection_reset"
	case strings.Contains(errStr, "timeout"):
		return "timeout"
	case strings.Contains(errStr, "tls") || strings.Contains(errStr, "x509") || strings.Contains(errStr, "certificate"):
		return "tls_error"
	case strings.Contains(errStr, "eof"):
		return "eof"
	default:
		return "unknown"
	}
}

// PrometheusPush sends the collected Prometheus metrics to the push gateway.
func (p *PrometheusClient) PrometheusPush() {
	log.Debug("Pushing metrics to push gateway")

	if err := p.pusher.Push(); err != nil {
		log.WithError(err).Error("Failed to push metrics to push gateway")
	}
}
