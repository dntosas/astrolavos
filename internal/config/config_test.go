package config //nolint:testpackage // tests access unexported methods for thorough validation

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/dntosas/astrolavos/internal/metrics"

	"github.com/spf13/viper"
)

func TestGetCleanEndpoint_Defaults(t *testing.T) {
	ye := &YamlEndpoint{
		Domain: "example.com",
	}

	ep, err := ye.getCleanEndpoint()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ep.ProberType != "httpTrace" {
		t.Errorf("expected prober type 'httpTrace', got %q", ep.ProberType)
	}

	if ep.Retries != 1 {
		t.Errorf("expected retries 1, got %d", ep.Retries)
	}

	if ep.URI != "http://example.com" {
		t.Errorf("expected URI 'http://example.com', got %q", ep.URI)
	}

	expectedInterval := 5000 * time.Millisecond
	if ep.Interval != expectedInterval {
		t.Errorf("expected interval %v, got %v", expectedInterval, ep.Interval)
	}
}

func TestGetCleanEndpoint_HTTPS(t *testing.T) {
	ye := &YamlEndpoint{
		Domain: "example.com",
		HTTPS:  true,
	}

	ep, err := ye.getCleanEndpoint()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ep.URI != "https://example.com" {
		t.Errorf("expected URI 'https://example.com', got %q", ep.URI)
	}
}

func TestGetCleanEndpoint_TCP(t *testing.T) {
	ye := &YamlEndpoint{
		Domain: "example.com:443",
		Prober: "tcp",
	}

	ep, err := ye.getCleanEndpoint()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ep.URI != "example.com:443" {
		t.Errorf("expected URI 'example.com:443', got %q", ep.URI)
	}

	if ep.ProberType != "tcp" {
		t.Errorf("expected prober type 'tcp', got %q", ep.ProberType)
	}
}

func TestGetCleanEndpoint_CustomRetries(t *testing.T) {
	retries := 5
	ye := &YamlEndpoint{
		Domain:  "example.com",
		Retries: &retries,
	}

	ep, err := ye.getCleanEndpoint()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ep.Retries != 5 {
		t.Errorf("expected retries 5, got %d", ep.Retries)
	}
}

func TestGetCleanEndpoint_IntervalTooSmall(t *testing.T) {
	interval := 500 * time.Millisecond
	ye := &YamlEndpoint{
		Domain:   "example.com",
		Interval: &interval,
	}

	_, err := ye.getCleanEndpoint()
	if err == nil {
		t.Fatal("expected error for interval too small")
	}
}

func TestGetCleanEndpoint_InvalidProber(t *testing.T) {
	ye := &YamlEndpoint{
		Domain: "example.com",
		Prober: "invalid",
	}

	_, err := ye.getCleanEndpoint()
	if err == nil {
		t.Fatal("expected error for invalid prober type")
	}
}

func TestGetCleanEndpoints_Empty(t *testing.T) {
	ye := &YamlEndpoints{}

	_, err := ye.getCleanEndpoints()
	if err == nil {
		t.Fatal("expected error for empty endpoints")
	}
}

func TestGetCleanEndpoints_AllInvalid(t *testing.T) {
	ye := &YamlEndpoints{
		Endpoints: []YamlEndpoint{
			{Domain: "example.com", Prober: "invalid"},
		},
	}

	_, err := ye.getCleanEndpoints()
	if err == nil {
		t.Fatal("expected error when all endpoints are invalid")
	}
}

func TestGetCleanEndpoints_MixedValid(t *testing.T) {
	ye := &YamlEndpoints{
		Endpoints: []YamlEndpoint{
			{Domain: "example.com"},
			{Domain: "invalid.com", Prober: "invalid"},
		},
	}

	endpoints, err := ye.getCleanEndpoints()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(endpoints) != 1 {
		t.Errorf("expected 1 valid endpoint, got %d", len(endpoints))
	}
}

func TestGetCleanEndpoint_ReuseConnection(t *testing.T) {
	ye := &YamlEndpoint{
		Domain:          "example.com",
		ReuseConnection: true,
	}

	ep, err := ye.getCleanEndpoint()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !ep.ReuseConnection {
		t.Error("expected ReuseConnection to be true")
	}
}

func TestGetCleanEndpoint_SkipTLSVerification(t *testing.T) {
	ye := &YamlEndpoint{
		Domain:              "example.com",
		SkipTLSVerification: true,
	}

	ep, err := ye.getCleanEndpoint()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !ep.SkipTLSVerification {
		t.Error("expected SkipTLSVerification to be true")
	}
}

func TestParseHistogramBuckets(t *testing.T) {
	tests := []struct {
		name    string
		raw     any
		want    []float64
		wantErr bool
	}{
		{name: "nil uses default", raw: nil, want: metrics.DefaultTimeBuckets},
		{name: "empty string uses default", raw: "", want: metrics.DefaultTimeBuckets},
		{name: "whitespace-only string uses default", raw: " , ", want: metrics.DefaultTimeBuckets},
		{name: "empty yaml list uses default", raw: []any{}, want: metrics.DefaultTimeBuckets},
		{name: "float64 slice passthrough", raw: []float64{0.1, 1}, want: []float64{0.1, 1}},
		{name: "env comma separated", raw: "0.001,0.01, 0.1 ,1", want: []float64{0.001, 0.01, 0.1, 1}},
		{name: "yaml mixed int and float", raw: []any{0.5, 1, 2.5, int64(5)}, want: []float64{0.5, 1, 2.5, 5}},
		{name: "yaml numeric strings", raw: []any{"0.5", "1"}, want: []float64{0.5, 1}},
		{name: "env not a number", raw: "0.1,abc", wantErr: true},
		{name: "yaml not a number", raw: []any{0.1, "abc"}, wantErr: true},
		{name: "yaml unsupported element type", raw: []any{0.1, true}, wantErr: true},
		{name: "unsupported top-level type", raw: 42, wantErr: true},
		{name: "not increasing", raw: "0.1,0.1,1", wantErr: true},
		{name: "decreasing", raw: []any{1.0, 0.5}, wantErr: true},
		{name: "nan rejected", raw: "0.1,NaN,1", wantErr: true},
		{name: "inf rejected", raw: "0.1,Inf", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseHistogramBuckets(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %v", got)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !slices.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// writeConfigFile writes a config.yaml with one valid endpoint plus the given
// extra YAML and returns its directory.
func writeConfigFile(t *testing.T, extra string) string {
	t.Helper()

	dir := t.TempDir()
	body := "endpoints:\n  - domain: example.com\n" + extra

	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	return dir
}

// TestNewConfig_HistogramBucketsPrecedence verifies the env > YAML > default
// hierarchy end-to-end through Viper, not just the parser.
func TestNewConfig_HistogramBucketsPrecedence(t *testing.T) {
	yamlBuckets := "metrics:\n  histogramBuckets: [0.01, 0.1, 1]\n"

	tests := []struct {
		name string
		yaml string
		env  string
		want []float64
	}{
		{name: "default when unset", want: metrics.DefaultTimeBuckets},
		{name: "yaml overrides default", yaml: yamlBuckets, want: []float64{0.01, 0.1, 1}},
		{name: "env overrides yaml", yaml: yamlBuckets, env: "0.5,5", want: []float64{0.5, 5}},
		{name: "empty env falls through to yaml", yaml: yamlBuckets, env: "", want: []float64{0.01, 0.1, 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)
			t.Setenv(histogramBucketsEnv, tt.env)

			cfg, err := NewConfig(writeConfigFile(t, tt.yaml))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !slices.Equal(cfg.HistogramBuckets, tt.want) {
				t.Errorf("HistogramBuckets = %v, want %v", cfg.HistogramBuckets, tt.want)
			}
		})
	}
}

func TestNewConfig_InvalidHistogramBucketsFailsFast(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv(histogramBucketsEnv, "1,0.5")

	if _, err := NewConfig(writeConfigFile(t, "")); err == nil {
		t.Fatal("expected error for non-increasing buckets")
	}
}
