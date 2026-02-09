// Package config handles loading and validating application configuration
// from YAML files and environment variables.
package config

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/dntosas/astrolavos/internal/model"

	"github.com/spf13/viper"

	log "github.com/sirupsen/logrus"
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
	LogLevel        string
	PromPushGateway string
	Endpoints       []*model.Endpoint
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

	return &Config{
		AppPort:         intPort,
		LogLevel:        viper.GetString("log_level"),
		PromPushGateway: viper.GetString("prom_push_gw"),
		Endpoints:       cleanEndpoints,
	}, nil
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
