// Package probers represent
package probers

import (
	"crypto/tls"
	"net/http"
	"sync"
	"time"

	"github.com/dntosas/astrolavos/internal/metrics"
)

// Prober interface dictates what function each Prober kind struct
// should implement
type Prober interface {
	String() string
	Run()
	Stop()
}

// ProberConfig struct holds information about configuration each
// Prober needs
type ProberConfig struct {
	wg       *sync.WaitGroup
	promC    *metrics.PrometheusClient
	exit     chan bool
	endpoint string
	retries  int
	tag      string
	interval time.Duration
	isOneOff bool
	HTTPProberConfig
}

// HTTPProberConfig holds information abou the HTTP traces
type HTTPProberConfig struct {
	reuseConnection bool
	skipTLS         bool
	client          *http.Client
}

// NewProberConfig is the constructor function for each ProberConfig struct
func NewProberConfig(w *sync.WaitGroup, endpoint string, retries int, tag string, interval time.Duration, isOneOff, reuseCon bool, skipTLS bool, promC *metrics.PrometheusClient) ProberConfig {
	p := ProberConfig{
		wg:       w,
		promC:    promC,
		exit:     make(chan bool, 1),
		endpoint: endpoint,
		retries:  retries,
		tag:      tag,
		interval: interval,
		isOneOff: isOneOff,
	}

	p.HTTPProberConfig = HTTPProberConfig{
		reuseConnection: reuseCon,
		skipTLS:         skipTLS,
		client:          getCustomClient(reuseCon, skipTLS),
	}

	return p
}

func getCustomClient(reuseCon, skipTLS bool) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if skipTLS {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	if !reuseCon {
		// with below option we force new connection every time we do a request
		transport.MaxIdleConnsPerHost = -1
	}
	return &http.Client{Transport: transport}
}
