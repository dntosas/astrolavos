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

// DefaultTimeBuckets covers the practical latency range (1ms – 5s) with fewer
// buckets to limit the number of time series exposed to scrapers. It is used
// whenever no histogram buckets are configured.
var DefaultTimeBuckets = []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5}

// latencyLabels are the labels shared by every latency histogram.
var latencyLabels = []string{"domain", "tag", "prober_type"}

// PrometheusClient holds state needed for Prometheus metric collection and pushing.
type PrometheusClient struct {
	pusher *push.Pusher

	dnsLatencyHistogram       *prometheus.HistogramVec
	connLatencyHistogram      *prometheus.HistogramVec
	tlsLatencyHistogram       *prometheus.HistogramVec
	gotConnLatencyHistogram   *prometheus.HistogramVec
	firstByteLatencyHistogram *prometheus.HistogramVec
	totalLatencyHistogram     *prometheus.HistogramVec
	totalRequestsCounter      *prometheus.CounterVec
	totalErrorsCounter        *prometheus.CounterVec
}

// NewPrometheusClient initializes a new Prometheus client and registers all metrics
// with the default registry. The latency histograms use the given bucket upper
// bounds (in seconds); an empty slice falls back to DefaultTimeBuckets.
func NewPrometheusClient(_ bool, promPushGateway string, buckets []float64) *PrometheusClient {
	return newPrometheusClient(prometheus.DefaultRegisterer, promPushGateway, buckets)
}

// newPrometheusClient builds all collectors and registers them with reg.
// Taking the registerer as a parameter keeps the constructor testable without
// polluting the process-wide default registry.
func newPrometheusClient(reg prometheus.Registerer, promPushGateway string, buckets []float64) *PrometheusClient {
	if len(buckets) == 0 {
		buckets = DefaultTimeBuckets
	}

	newLatencyHistogram := func(name, help string) *prometheus.HistogramVec {
		return prometheus.NewHistogramVec(
			prometheus.HistogramOpts{Name: name, Help: help, Buckets: buckets},
			latencyLabels,
		)
	}

	p := &PrometheusClient{
		dnsLatencyHistogram:       newLatencyHistogram("astrolavos_dns_latency_seconds", "Histogram of DNS resolution latency in seconds"),
		connLatencyHistogram:      newLatencyHistogram("astrolavos_conn_latency_seconds", "Histogram of TCP connection latency in seconds"),
		tlsLatencyHistogram:       newLatencyHistogram("astrolavos_tls_latency_seconds", "Histogram of TLS handshake latency in seconds"),
		gotConnLatencyHistogram:   newLatencyHistogram("astrolavos_gotconn_latency_seconds", "Histogram of time to obtain a connection in seconds"),
		firstByteLatencyHistogram: newLatencyHistogram("astrolavos_firstbyte_latency_seconds", "Histogram of time to first byte in seconds"),
		totalLatencyHistogram:     newLatencyHistogram("astrolavos_total_latency_seconds", "Histogram of total request latency in seconds"),
		totalRequestsCounter: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "astrolavos_requests_total",
				Help: "Total number of probe requests made by Astrolavos",
			},
			[]string{"domain", "tag", "status_code", "prober_type"},
		),
		totalErrorsCounter: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "astrolavos_errors_total",
				Help: "Total number of probe errors encountered by Astrolavos",
			},
			[]string{"domain", "tag", "error", "prober_type"},
		),
	}

	collectors := []prometheus.Collector{
		p.dnsLatencyHistogram,
		p.connLatencyHistogram,
		p.tlsLatencyHistogram,
		p.gotConnLatencyHistogram,
		p.firstByteLatencyHistogram,
		p.totalLatencyHistogram,
		p.totalRequestsCounter,
		p.totalErrorsCounter,
	}

	reg.MustRegister(collectors...)

	pusher := push.New(promPushGateway, "astrolavos")
	for _, c := range collectors {
		pusher = pusher.Collector(c)
	}

	p.pusher = pusher

	log.WithField("histogram_buckets", buckets).Info("Metrics setup - scrape /metrics")

	return p
}

// UpdateDNSHistogram records a DNS resolution duration observation.
func (p *PrometheusClient) UpdateDNSHistogram(domain, proberType, tag string, duration float64) {
	p.dnsLatencyHistogram.WithLabelValues(domain, tag, proberType).Observe(duration)
	log.Debug("Updated metric for DNS latency")
}

// UpdateConnHistogram records a TCP connection duration observation.
func (p *PrometheusClient) UpdateConnHistogram(domain, proberType, tag string, duration float64) {
	p.connLatencyHistogram.WithLabelValues(domain, tag, proberType).Observe(duration)
	log.Debug("Updated metric for connection latency")
}

// UpdateTLSHistogram records a TLS handshake duration observation.
func (p *PrometheusClient) UpdateTLSHistogram(domain, proberType, tag string, duration float64) {
	p.tlsLatencyHistogram.WithLabelValues(domain, tag, proberType).Observe(duration)
	log.Debug("Updated metric for TLS latency")
}

// UpdateGotConnHistogram records the time to obtain a connection.
func (p *PrometheusClient) UpdateGotConnHistogram(domain, proberType, tag string, duration float64) {
	p.gotConnLatencyHistogram.WithLabelValues(domain, tag, proberType).Observe(duration)
	log.Debug("Updated metric for GotConnection latency")
}

// UpdateFirstByteHistogram records the time to first byte.
func (p *PrometheusClient) UpdateFirstByteHistogram(domain, proberType, tag string, duration float64) {
	p.firstByteLatencyHistogram.WithLabelValues(domain, tag, proberType).Observe(duration)
	log.Debug("Updated metric for FirstByte latency")
}

// UpdateTotalHistogram records the total request duration.
func (p *PrometheusClient) UpdateTotalHistogram(domain, proberType, tag string, duration float64) {
	p.totalLatencyHistogram.WithLabelValues(domain, tag, proberType).Observe(duration)
	log.Debug("Updated metric for total latency")
}

// UpdateRequestsCounter increments the total requests counter.
// The status code is bucketed (e.g. "2xx") to limit label cardinality.
func (p *PrometheusClient) UpdateRequestsCounter(domain, proberType, tag, statusCode string) {
	p.totalRequestsCounter.WithLabelValues(domain, tag, BucketStatusCode(statusCode), proberType).Inc()
	log.Debug("Updated metric for total requests counter")
}

// BucketStatusCode maps an HTTP status code string to its class bucket
// (e.g. "200" -> "2xx"). Unknown or empty codes are returned as-is.
func BucketStatusCode(code string) string {
	if len(code) == 3 && code[0] >= '1' && code[0] <= '5' {
		return string(code[0]) + "xx"
	}

	return code
}

// UpdateErrorsCounter increments the total errors counter with a categorized error type.
// Error messages are categorized into a fixed set of labels to prevent cardinality explosion.
func (p *PrometheusClient) UpdateErrorsCounter(domain, proberType, tag string, err error) {
	category := CategorizeError(err)
	p.totalErrorsCounter.WithLabelValues(domain, tag, category, proberType).Inc()
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
