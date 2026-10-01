// Package config handles loading and validating application configuration
// from YAML files and environment variables.
package config

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/dntosas/astrolavos/internal/metrics"
	"github.com/dntosas/astrolavos/internal/model"

	"github.com/spf13/viper"

	log "github.com/sirupsen/logrus"
)

const (
	// histogramBucketsKey is the config.yaml path of the latency histogram buckets.
	histogramBucketsKey = "metrics.histogramBuckets"
	// histogramBucketsEnv is the env var that overrides histogramBucketsKey.
	// It is bound explicitly so the YAML key stays camelCase like the rest of
	// config.yaml while the env var follows the ASTROLAVOS_SNAKE_CASE convention.
	histogramBucketsEnv = "ASTROLAVOS_HISTOGRAM_BUCKETS"
)

// YamlEndpoints encapsulates the top-level YAML configuration containing
// the list of endpoints to monitor.
type YamlEndpoints struct {
	Endpoints []YamlEndpoint `yaml:"endpoints"`
}

// getCleanEndpoints validates and converts YAML endpoint configurations
// into application-ready Endpoint structs.
func (r *YamlEndpoints) getCleanEndpoints() ([]*model.Endpoint, error) {
	if len(r.Endpoints) == 0 {
		return []*model.Endpoint{}, errors.New("YAML configuration is empty or malformed: no endpoints defined")
	}

	cleanEndpoints := []*model.Endpoint{}

	for _, req := range r.Endpoints {
		c, err := req.getCleanEndpoint()
		if err != nil {
			log.Error(err.Error())

			continue
		}

		cleanEndpoints = append(cleanEndpoints, c)
	}

	if len(cleanEndpoints) == 0 {
		return []*model.Endpoint{}, errors.New("no valid endpoints found in configuration")
	}

	return cleanEndpoints, nil
}

// YamlEndpoint represents a single endpoint configuration from the YAML file.
type YamlEndpoint struct {
	Domain              string         `yaml:"domain"`
	Interval            *time.Duration `yaml:"interval"`
	HTTPS               bool           `yaml:"https"`
	Tag                 string         `yaml:"tag"`
	Retries             *int           `yaml:"retries"`
	Prober              string         `yaml:"prober"`
	ReuseConnection     bool           `yaml:"reuseConnection"`
	SkipTLSVerification bool           `yaml:"skipTLSVerification"`
	TCPTimeout          *time.Duration `yaml:"tcpTimeout"`
}

// getCleanEndpoint validates and converts a YAML endpoint into an application Endpoint.
func (r *YamlEndpoint) getCleanEndpoint() (*model.Endpoint, error) {
	var defaultRetries = 1

	var defaultInterval = 5000 * time.Millisecond

	if r.Interval == nil {
		r.Interval = &defaultInterval
	}

	if r.Prober == "" {
		r.Prober = "httpTrace"
	}

	if *r.Interval < 1000*time.Millisecond {
		return nil, errors.New("interval cannot be less than 1 second")
	}

	if r.Prober != "tcp" && r.Prober != "httpTrace" {
		return nil, fmt.Errorf("invalid prober type '%s': must be one of ['tcp', 'httpTrace']", r.Prober)
	}

	uri := r.Domain

	if r.Prober == "httpTrace" {
		if r.HTTPS {
			uri = "https://" + r.Domain
		} else {
			uri = "http://" + r.Domain
		}
	}

	if r.Retries != nil {
		defaultRetries = *r.Retries
	}

	var defaultTCPTimeout = 10 * time.Second

	if r.TCPTimeout != nil {
		defaultTCPTimeout = *r.TCPTimeout
	}

	ep := &model.Endpoint{
		URI:                 uri,
		Interval:            *r.Interval,
		Tag:                 r.Tag,
		Retries:             defaultRetries,
		ProberType:          r.Prober,
		ReuseConnection:     r.ReuseConnection,
		SkipTLSVerification: r.SkipTLSVerification,
		TCPTimeout:          defaultTCPTimeout,
	}

	return ep, nil
}

// Config holds all application configuration.
type Config struct {
	AppPort         int
	MaxPayloadSize  int
	LogLevel        string
	PromPushGateway string
	// HistogramBuckets are the upper bounds (seconds) shared by all latency histograms.
	HistogramBuckets []float64
	Endpoints        []*model.Endpoint
}

// NewConfig loads and validates configuration from the given path.
func NewConfig(path string) (*Config, error) {
	initViper(path)

	r, err := getYamlConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load YAML config: %w", err)
	}

	cleanEndpoints, err := r.getCleanEndpoints()
	if err != nil {
		return nil, fmt.Errorf("failed to validate endpoints: %w", err)
	}

	port := viper.GetString("app_port")

	intPort, err := strconv.Atoi(port)
	if err != nil {
		return nil, fmt.Errorf("invalid ASTROLAVOS_PORT value %q: %w", port, err)
	}

	buckets, err := parseHistogramBuckets(viper.Get(histogramBucketsKey))
	if err != nil {
		return nil, fmt.Errorf("invalid histogram buckets (%s / %s): %w", histogramBucketsKey, histogramBucketsEnv, err)
	}

	return &Config{
		AppPort:          intPort,
		MaxPayloadSize:   viper.GetInt("max_payload_size"),
		LogLevel:         viper.GetString("log_level"),
		PromPushGateway:  viper.GetString("prom_push_gw"),
		HistogramBuckets: buckets,
		Endpoints:        cleanEndpoints,
	}, nil
}

// parseHistogramBuckets converts the raw Viper value of the histogram buckets
// setting into a validated []float64. Depending on the source, Viper hands us
// a comma-separated string (env var), a []any of numbers (YAML) or the
// []float64 default. An empty value resolves to metrics.DefaultTimeBuckets.
func parseHistogramBuckets(raw any) ([]float64, error) {
	buckets, err := rawToFloat64s(raw)
	if err != nil {
		return nil, err
	}

	if len(buckets) == 0 {
		return metrics.DefaultTimeBuckets, nil
	}

	if err := validateHistogramBuckets(buckets); err != nil {
		return nil, err
	}

	return buckets, nil
}

// rawToFloat64s normalizes the shapes Viper may return for a list setting.
func rawToFloat64s(raw any) ([]float64, error) {
	switch v := raw.(type) {
	case nil:
		return nil, nil
	case []float64:
		return v, nil
	case string:
		return parseCommaSeparatedFloats(v)
	case []any:
		return parseFloatList(v)
	default:
		return nil, fmt.Errorf("unsupported value type %T", raw)
	}
}

// parseCommaSeparatedFloats parses "0.1, 0.5,1" style env var values,
// skipping empty fields.
func parseCommaSeparatedFloats(s string) ([]float64, error) {
	var out []float64

	for _, field := range strings.Split(s, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}

		f, err := strconv.ParseFloat(field, 64)
		if err != nil {
			return nil, fmt.Errorf("bucket %q is not a number", field)
		}

		out = append(out, f)
	}

	return out, nil
}

// parseFloatList converts a decoded YAML sequence into floats.
func parseFloatList(items []any) ([]float64, error) {
	out := make([]float64, 0, len(items))

	for _, item := range items {
		f, err := toFloat64(item)
		if err != nil {
			return nil, err
		}

		out = append(out, f)
	}

	return out, nil
}

// toFloat64 converts the scalar types a YAML decoder may produce for a number.
func toFloat64(v any) (float64, error) {
	switch n := v.(type) {
	case float64:
		return n, nil
	case float32:
		return float64(n), nil
	case int:
		return float64(n), nil
	case int64:
		return float64(n), nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		if err != nil {
			return 0, fmt.Errorf("bucket %q is not a number", n)
		}

		return f, nil
	default:
		return 0, fmt.Errorf("bucket %v has unsupported type %T", v, v)
	}
}

// validateHistogramBuckets enforces the invariants prometheus.NewHistogram
// would otherwise panic on: finite values in strictly increasing order.
func validateHistogramBuckets(buckets []float64) error {
	for i, b := range buckets {
		if math.IsNaN(b) || math.IsInf(b, 0) {
			return fmt.Errorf("bucket at index %d is not a finite number", i)
		}

		if i > 0 && b <= buckets[i-1] {
			return fmt.Errorf("buckets must be strictly increasing: %v <= %v at index %d", b, buckets[i-1], i)
		}
	}

	return nil
}

// initViper initializes Viper configuration with defaults and env variable support.
func initViper(path string) {
	// Set global options
	viper.AddConfigPath(path)
	viper.AddConfigPath(".")
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.SetEnvPrefix("astrolavos")

	// Set defaults for environment variables
	viper.SetDefault("APP_PORT", "3000")
	viper.SetDefault("LOG_LEVEL", "DEBUG")
	viper.SetDefault("PROM_PUSH_GW", "localhost")
	viper.SetDefault("MAX_PAYLOAD_SIZE", 0) // 0 means use handler's default (10MB)
	viper.SetDefault(histogramBucketsKey, metrics.DefaultTimeBuckets)

	// Precedence follows Viper: env var > config.yaml > default above.
	if err := viper.BindEnv(histogramBucketsKey, histogramBucketsEnv); err != nil {
		log.WithError(err).WithField("env", histogramBucketsEnv).Warn("Failed to bind histogram buckets env var")
	}

	// Enable VIPER to read Environment Variables
	viper.AutomaticEnv()
}

// getYamlConfig reads and parses the YAML configuration file.
func getYamlConfig() (*YamlEndpoints, error) {
	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	var ye YamlEndpoints

	if err := viper.Unmarshal(&ye); err != nil {
		return nil, fmt.Errorf("unable to decode config YAML into struct: %w", err)
	}

	return &ye, nil
}
