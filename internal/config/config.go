// Package config contains logic that is related with our application's configuration.
// Configuration can come from environmental variables or yaml config file.
package config

import (
	"fmt"
	"strconv"
	"time"

	"github.com/dntosas/astrolavos/internal/machinery"

	"github.com/pkg/errors"
	"github.com/spf13/viper"

	log "github.com/sirupsen/logrus"
)

// YamlEndpoints encapsulates yaml objects that represent the
// array that holds the endpoints with the monitoring domains.
type YamlEndpoints struct {
	Endpoints []YamlEndpoint `yaml:"endpoints"`
}

// getCleanEndpoints holds the logic that gets the endpoints from the
// yaml config, and verify for each one if they are valid.
// At the end it returns a list of endpoints structures that can be used
// further.
func (r *YamlEndpoints) getCleanEndpoints() ([]*machinery.Endpoint, error) {
	if len(r.Endpoints) == 0 {
		return []*machinery.Endpoint{}, errors.Errorf("Yaml configuration seems empty or malformed, cannot proceed with no valid endpoints")
	}
	cleanEndpoints := []*machinery.Endpoint{}
	for _, req := range r.Endpoints {
		c, err := req.getCleanEndpoint()
		if err != nil {
			log.Error(err.Error())
			continue
		}
		cleanEndpoints = append(cleanEndpoints, c)

	}
	if len(cleanEndpoints) == 0 {
		return []*machinery.Endpoint{}, errors.Errorf("No valid endpoints found inside the endpoint sections coming from yaml config")
	}

	return cleanEndpoints, nil
}

// YamlEndpoint encapsulates yaml objects that represent
// endpoints that we want to monitor.
type YamlEndpoint struct {
	Domain              string         `yaml:"domain"`
	Interval            *time.Duration `yaml:"interval"`
	HTTPS               bool           `yaml:"https"`
	Tag                 string         `yaml:"tag"`
	Retries             *int           `yaml:"retries"`
	Prober              string         `yaml:"prober"`
	ReuseConnection     bool           `yaml:"reuse_connection"`
	SkipTLSVerification bool           `yaml:"skip_tls_verify"`
}

// getCleanEndpoint holds the logic of checking and creating an endpoint
// coming from the yaml config and returns an endpoint structure that
// can be used further in our code.
func (r *YamlEndpoint) getCleanEndpoint() (*machinery.Endpoint, error) {
	var defaultRetries = 3
	var defaultInterval = 5000 * time.Millisecond

	if r.Interval == nil {
		r.Interval = &defaultInterval
	}

	if r.Prober == "" {
		r.Prober = "httpTrace"
	}

	if *r.Interval < 1000*time.Millisecond {
		return nil, errors.New("Interval cannot be less that 1 seconds")
	}

	if r.Prober != "tcp" && r.Prober != "httpTrace" {
		return nil, errors.New("Prober should be one of ['tcp','httpTrace']")
	}

	uri := r.Domain
	if r.Prober == "httpTrace" {
		if r.HTTPS {
			uri = fmt.Sprintf("https://%s", r.Domain)
		} else {
			uri = fmt.Sprintf("http://%s", r.Domain)
		}
	}

	if r.Retries != nil {
		defaultRetries = *r.Retries
	}

	ep := &machinery.Endpoint{URI: uri, Interval: *r.Interval, Tag: r.Tag, Retries: defaultRetries, ProberType: r.Prober,
		ReuseConnection: r.ReuseConnection, SkipTLSVerification: r.SkipTLSVerification,
	}
	return ep, nil
}

// Config holds all our configuration coming from user that our app needs
type Config struct {
	AppPort         int
	LogLevel        string
	PromPushGateway string
	Endpoints       []*machinery.Endpoint
}

// NewConfig constructs and returns the struct that will host
// all our configuration variables
func NewConfig(path string) (*Config, error) {

	initViper(path)
	r, err := getYamlConfig()
	if err != nil {
		return nil, errors.Wrap(err, "Couldn't get a yaml config")
	}
	cleanEndpoints, err := r.getCleanEndpoints()
	if err != nil {
		return nil, errors.Wrap(err, "Couldn't get a valid yaml config")
	}

	port := viper.GetString("app_port")
	intPort, ok := strconv.Atoi(port)
	if ok != nil {
		return nil, errors.New("Couldn't get a valid integer for the ASTROLAVOS_PORT configuration variable")
	}

	return &Config{
		AppPort:         intPort,
		LogLevel:        viper.GetString("log_level"),
		PromPushGateway: viper.GetString("prom_push_gw"),
		Endpoints:       cleanEndpoints,
	}, nil
}

// initViper initializes all viper configuration that we need.
func initViper(path string) {
	// Set global options
	viper.AddConfigPath(path)
	viper.AddConfigPath(".")
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.SetEnvPrefix("astrolavos")

	// Set default for our existing env variables
	viper.SetDefault("APP_PORT", "3000")
	viper.SetDefault("LOG_LEVEL", "DEBUG")
	viper.SetDefault("PROM_PUSH_GW", "localhost")

	// Enable VIPER to read Environment Variables
	viper.AutomaticEnv()
}

// getYamlConfig reads the config yaml file that contains the user's
// requests for monitoring domains. After successfully reading the file
// the function returns an YamlEndpoints struct that contains all info from
// the file.
func getYamlConfig() (*YamlEndpoints, error) {

	if err := viper.ReadInConfig(); err != nil {
		return nil, errors.Wrap(err, "Error reading config file")
	}

	var ye YamlEndpoints

	err := viper.Unmarshal(&ye)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to decode config yaml into struct")
	}

	return &ye, nil
}
