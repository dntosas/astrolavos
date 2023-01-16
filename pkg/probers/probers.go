package probers

import (
	"net/http"
	"sync"
	"time"
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
	promC    *PrometheusClient
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
	client          *http.Client
}

// NewProberConfig is the constructor function for each ProberConfig struct
func NewProberConfig(w *sync.WaitGroup, endpoint string, retries int, tag string, interval time.Duration, isOneOff, reuseCon bool, promC *PrometheusClient) ProberConfig {
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
	if reuseCon {
		p.HTTPProberConfig = HTTPProberConfig{
			reuseConnection: reuseCon,
			client:          &http.Client{Transport: http.DefaultTransport.(*http.Transport)},
		}
	}
	return p
}
