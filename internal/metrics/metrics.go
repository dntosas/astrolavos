// Package metrics provides functionality for building Prometheus-compatible structs.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
	log "github.com/sirupsen/logrus"
)

var (
	// TimeBuckets is based on Prometheus client_golang prometheus.DefBuckets.
	timeBuckets = prometheus.ExponentialBuckets(0.00025, 2, 16) // from 0.25ms to 8 seconds

	dnsLatencyHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "astrolavos_dns_latency_seconds",
			Help:    "Histogram of response times of Astrolavos for DNS part",
			Buckets: timeBuckets,
		},
		[]string{"endpoint", "tag", "type"},
	)

	connLatencyHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "astrolavos_conn_latency_seconds",
			Help:    "Histogram of response times of Astrolavos for Connection part",
			Buckets: timeBuckets,
		},
		[]string{"endpoint", "tag", "type"},
	)

	tlsLatencyHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "astrolavos_tls_latency_seconds",
			Help:    "Histogram of response times of Astrolavos for TLS part",
			Buckets: timeBuckets,
		},
		[]string{"endpoint", "tag", "type"},
	)

	gotConnLatencyHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "astrolavos_gotconn_latency_seconds",
			Help:    "Histogram of response times of Astrolavos for GotConnection part",
			Buckets: timeBuckets,
		},
		[]string{"endpoint", "tag", "type"},
	)

	firstByteLatencyHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "astrolavos_firstbyte_latency_seconds",
			Help:    "Histogram of response times of Astrolavos for First Byte part",
			Buckets: timeBuckets,
		},
		[]string{"endpoint", "tag", "type"},
	)

	totalLatencyHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "astrolavos_total_latency_seconds",
			Help:    "Histogram of response times of Astrolavos for Total part",
			Buckets: timeBuckets,
		},
		[]string{"endpoint", "tag", "type"},
	)
	totalRequestsCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "astrolavos_requests_total",
			Help: "Statistics of requests made from Astrolavos",
		},
		[]string{"endpoint", "tag", "status_code", "type"},
	)
	totalErrorsCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "astrolavos_errors_total",
			Help: "Statistics of errors made from Astrolavos",
		},
		[]string{"endpoint", "tag", "error", "type"},
	)
)

// PrometheusClient struct holds information that will be needed
// during the program's lifecycle regarding prometheus communication.
type PrometheusClient struct {
	pusher *push.Pusher
}

// NewPrometheusClient initializes a new prometheus clinets
// that we can use to deal with our metrics.
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

// UpdateDNSHistogram appends metrics values into corresponding Histogram.
func (p *PrometheusClient) UpdateDNSHistogram(endpoint, proberType string, tag string, duration float64) {
	dnsLatencyHistogram.WithLabelValues(endpoint, tag, proberType).Observe(duration)
	log.Debug("Update metric for DNS part")
}

// UpdateConnHistogram appends metrics values into corresponding Histogram.
func (p *PrometheusClient) UpdateConnHistogram(endpoint, proberType string, tag string, duration float64) {
	connLatencyHistogram.WithLabelValues(endpoint, tag, proberType).Observe(duration)
	log.Debug("Update metric for Connection part")
}

// UpdateTLSHistogram appends metrics values into corresponding Histogram.
func (p *PrometheusClient) UpdateTLSHistogram(endpoint, proberType string, tag string, duration float64) {
	tlsLatencyHistogram.WithLabelValues(endpoint, tag, proberType).Observe(duration)
	log.Debug("Update metric for TLS part")
}

// UpdateGotConnHistogram appends metrics values into corresponding Histogram.
func (p *PrometheusClient) UpdateGotConnHistogram(endpoint, proberType string, tag string, duration float64) {
	gotConnLatencyHistogram.WithLabelValues(endpoint, tag, proberType).Observe(duration)
	log.Debug("Update metric for GotConnection part")
}

// UpdateFirstByteHistogram appends metrics values into corresponding Histogram.
func (p *PrometheusClient) UpdateFirstByteHistogram(endpoint, proberType string, tag string, duration float64) {
	firstByteLatencyHistogram.WithLabelValues(endpoint, tag, proberType).Observe(duration)
	log.Debug("Update metric for FirstByte part")
}

// UpdateTotalHistogram appends metrics values into corresponding Histogram.
func (p *PrometheusClient) UpdateTotalHistogram(endpoint, proberType string, tag string, duration float64) {
	totalLatencyHistogram.WithLabelValues(endpoint, tag, proberType).Observe(duration)
	log.Debug("Update metric for Total part")
}

// UpdateRequestsCounter appends metrics values into corresponding Histogram.
func (p *PrometheusClient) UpdateRequestsCounter(endpoint, proberType string, tag, statusCode string) {
	totalRequestsCounter.WithLabelValues(endpoint, tag, statusCode, proberType).Inc()
	log.Debug("Update metric for Total requests counter")
}

// UpdateErrorsCounter appends metrics values into corresponding Histogram.
func (p *PrometheusClient) UpdateErrorsCounter(endpoint, proberType string, tag, errorMsg string) {
	totalErrorsCounter.WithLabelValues(endpoint, tag, errorMsg, proberType).Inc()
	log.Debug("Update metric for Total errors counter")
}

// PrometheusPush sends the collected prometheus stats to
// the prometheus push gateway.
func (p *PrometheusClient) PrometheusPush() {
	log.Debugf("Pushing metrics to pushgateway")

	err := p.pusher.Push()
	if err != nil {
		log.Error(err)
	}
}
