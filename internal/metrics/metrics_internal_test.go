package metrics

import (
	"context"
	"errors"
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
	p.UpdateTotalHistogram("example.com", "httpTrace", "test", false, 0.05)

	got := gatherBucketBounds(t, reg, "astrolavos_total_latency_seconds")
	if !slices.Equal(got, custom) {
		t.Errorf("bucket bounds = %v, want %v", got, custom)
	}
}

func TestNewPrometheusClient_EmptyBucketsUseDefault(t *testing.T) {
	reg := prometheus.NewRegistry()

	p := newPrometheusClient(reg, "localhost", nil)
	p.UpdateDNSHistogram("example.com", "httpTrace", "test", false, 0.002)

	got := gatherBucketBounds(t, reg, "astrolavos_dns_latency_seconds")
	if !slices.Equal(got, DefaultTimeBuckets) {
		t.Errorf("bucket bounds = %v, want default %v", got, DefaultTimeBuckets)
	}
}

func TestNewPrometheusClient_AllHistogramsShareBuckets(t *testing.T) {
	custom := []float64{0.5, 2}
	reg := prometheus.NewRegistry()

	p := newPrometheusClient(reg, "localhost", custom)
	p.UpdateDNSHistogram("d", "httpTrace", "t", false, 1)
	p.UpdateConnHistogram("d", "httpTrace", "t", false, 1)
	p.UpdateTLSHistogram("d", "httpTrace", "t", false, 1)
	p.UpdateGotConnHistogram("d", "httpTrace", "t", false, 1)
	p.UpdateFirstByteHistogram("d", "httpTrace", "t", false, 1)
	p.UpdateTotalHistogram("d", "httpTrace", "t", false, 1)

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

func TestNewPrometheusClient_AllMetricsCarryConnReused(t *testing.T) {
	reg := prometheus.NewRegistry()
	p := newPrometheusClient(reg, "localhost", nil)

	p.UpdateDNSHistogram("d", "httptrace", "t", true, 0.1)
	p.UpdateConnHistogram("d", "httptrace", "t", true, 0.1)
	p.UpdateTLSHistogram("d", "httptrace", "t", true, 0.1)
	p.UpdateGotConnHistogram("d", "httptrace", "t", true, 0.1)
	p.UpdateFirstByteHistogram("d", "httptrace", "t", true, 0.1)
	p.UpdateTotalHistogram("d", "httptrace", "t", true, 0.1)
	p.UpdateRequestsCounter("d", "httptrace", "t", "200", true)
	p.UpdateErrorsCounter("d", "httptrace", "t", true, errors.New("timeout"))

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	if len(families) != 8 {
		t.Fatalf("expected 8 probe metric families, got %d", len(families))
	}

	for _, f := range families {
		for _, m := range f.GetMetric() {
			found := false

			for _, l := range m.GetLabel() {
				if l.GetName() == "conn_reused" && l.GetValue() == "true" {
					found = true
				}
			}

			if !found {
				t.Errorf("%s lacks conn_reused=\"true\"", f.GetName())
			}
		}
	}
}

func TestStreamMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	p := newPrometheusClient(reg, "localhost", nil)

	p.SetStreamConnectionAge("d", "websocket", "t", 42)
	p.IncStreamReconnects("d", "websocket", "t", context.DeadlineExceeded)

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	byName := map[string]*dto.MetricFamily{}
	for _, f := range families {
		byName[f.GetName()] = f
	}

	age, ok := byName["astrolavos_stream_connection_age_seconds"]
	if !ok || age.GetMetric()[0].GetGauge().GetValue() != 42 {
		t.Fatalf("age gauge missing or wrong: %v", age)
	}

	rec, ok := byName["astrolavos_stream_reconnects_total"]
	if !ok {
		t.Fatal("reconnects counter missing")
	}

	var reason string

	for _, l := range rec.GetMetric()[0].GetLabel() {
		if l.GetName() == "reason" {
			reason = l.GetValue()
		}
	}

	if reason != "timeout" {
		t.Errorf("reason = %q, want timeout", reason)
	}
}
