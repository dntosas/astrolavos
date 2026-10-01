package metrics

import (
	"math"
	"slices"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// gatherBucketBounds returns the configured upper bounds of the named
// histogram after a single observation, excluding the implicit +Inf bucket.
func gatherBucketBounds(t *testing.T, reg *prometheus.Registry, name string) []float64 {
	t.Helper()

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	idx := slices.IndexFunc(families, func(f *dto.MetricFamily) bool { return f.GetName() == name })
	if idx < 0 {
		t.Fatalf("metric %q not found in registry", name)
	}

	metrics := families[idx].GetMetric()
	if len(metrics) != 1 {
		t.Fatalf("expected 1 series for %q, got %d", name, len(metrics))
	}

	var bounds []float64

	for _, b := range metrics[0].GetHistogram().GetBucket() {
		if !math.IsInf(b.GetUpperBound(), 1) {
			bounds = append(bounds, b.GetUpperBound())
		}
	}

	return bounds
}

func TestNewPrometheusClient_CustomBuckets(t *testing.T) {
	custom := []float64{0.01, 0.1, 1}
	reg := prometheus.NewRegistry()

	p := newPrometheusClient(reg, "localhost", custom)
	p.UpdateTotalHistogram("example.com", "httpTrace", "test", 0.05)

	got := gatherBucketBounds(t, reg, "astrolavos_total_latency_seconds")
	if !slices.Equal(got, custom) {
		t.Errorf("bucket bounds = %v, want %v", got, custom)
	}
}

func TestNewPrometheusClient_EmptyBucketsUseDefault(t *testing.T) {
	reg := prometheus.NewRegistry()

	p := newPrometheusClient(reg, "localhost", nil)
	p.UpdateDNSHistogram("example.com", "httpTrace", "test", 0.002)

	got := gatherBucketBounds(t, reg, "astrolavos_dns_latency_seconds")
	if !slices.Equal(got, DefaultTimeBuckets) {
		t.Errorf("bucket bounds = %v, want default %v", got, DefaultTimeBuckets)
	}
}

func TestNewPrometheusClient_AllHistogramsShareBuckets(t *testing.T) {
	custom := []float64{0.5, 2}
	reg := prometheus.NewRegistry()

	p := newPrometheusClient(reg, "localhost", custom)
	p.UpdateDNSHistogram("d", "httpTrace", "t", 1)
	p.UpdateConnHistogram("d", "httpTrace", "t", 1)
	p.UpdateTLSHistogram("d", "httpTrace", "t", 1)
	p.UpdateGotConnHistogram("d", "httpTrace", "t", 1)
	p.UpdateFirstByteHistogram("d", "httpTrace", "t", 1)
	p.UpdateTotalHistogram("d", "httpTrace", "t", 1)

	for _, name := range []string{
		"astrolavos_dns_latency_seconds",
		"astrolavos_conn_latency_seconds",
		"astrolavos_tls_latency_seconds",
		"astrolavos_gotconn_latency_seconds",
		"astrolavos_firstbyte_latency_seconds",
		"astrolavos_total_latency_seconds",
	} {
		if got := gatherBucketBounds(t, reg, name); !slices.Equal(got, custom) {
			t.Errorf("%s bucket bounds = %v, want %v", name, got, custom)
		}
	}
}
