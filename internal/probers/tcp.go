package probers

import (
	"context"
	"fmt"
	"net"

	log "github.com/sirupsen/logrus"
)

// TCP implements the Prober interface for TCP connection probes.
type TCP struct {
	ProberConfig
}

// NewTCP creates a new TCP prober with the given configuration.
func NewTCP(c ProberConfig) *TCP {
	return &TCP{c}
}

// String returns a human-readable description of the TCP prober configuration.
func (t *TCP) String() string {
	return fmt.Sprintf("TCP Prober Endpoint: %s - Interval: %v - Tag: %s - Retries: %d", t.endpoint, t.interval, t.tag, t.retries)
}

// Run starts the TCP prober, executing probes according to the configured mode.
func (t *TCP) Run(ctx context.Context) {
	t.runLoop(ctx, t.String(), t.probe)
}

// probe performs a single TCP dial with retry logic and records metrics.
func (t *TCP) probe(ctx context.Context) {
	err := t.retryWithBackoff(ctx, func() error {
		return t.dial(ctx)
	})

	t.promC.UpdateRequestsCounter(t.endpoint, "tcp", t.tag, "")

	if err != nil {
		log.Errorf("TCP prober %s failed after %d attempts: %v", t, t.retries, err)
		t.promC.UpdateErrorsCounter(t.endpoint, "tcp", t.tag, err)
	}
}

func (t *TCP) dial(ctx context.Context) error {
	dialer := net.Dialer{Timeout: t.tcpTimeout}

	conn, err := dialer.DialContext(ctx, "tcp", t.endpoint)
	if err != nil {
		return err
	}

	return conn.Close()
}
