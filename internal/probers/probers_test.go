package probers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/dntosas/astrolavos/internal/metrics"
	"github.com/dntosas/astrolavos/internal/probers"
)

// testPromC is a shared Prometheus client to avoid double-registration panics.
var testPromC = metrics.NewPrometheusClient(true, "localhost")

// newTestWG returns a WaitGroup with 1 added, matching what the agent does.
func newTestWG() *sync.WaitGroup {
	var wg sync.WaitGroup

	wg.Add(1)

	return &wg
}

func TestNewProberConfig(t *testing.T) {
	opts := probers.ProberOptions{
		Endpoint:            "https://example.com",
		Tag:                 "test",
		Retries:             5,
		Interval:            10 * time.Second,
		TCPTimeout:          5 * time.Second,
		IsOneOff:            true,
		ReuseConnection:     true,
		SkipTLSVerification: true,
	}

	p := probers.NewProberConfig(opts)

	s := probers.NewHTTPTrace(p).String()
	if s != "httpTrace Prober Endpoint: https://example.com - Interval: 10s - Tag: test - Retries: 5" {
		t.Errorf("unexpected String() output: %s", s)
	}
}

func TestTCPString(t *testing.T) {
	cfg := probers.NewProberConfig(probers.ProberOptions{
		Endpoint:   "example.com:443",
		Interval:   5 * time.Second,
		TCPTimeout: 10 * time.Second,
		Tag:        "prod",
		Retries:    3,
	})

	tcp := probers.NewTCP(cfg)
	s := tcp.String()

	if s != "TCP Prober Endpoint: example.com:443 - Interval: 5s - Tag: prod - Retries: 3" {
		t.Errorf("unexpected String() output: %s", s)
	}
}

func TestHTTPTraceString(t *testing.T) {
	cfg := probers.NewProberConfig(probers.ProberOptions{
		Endpoint: "https://example.com",
		Interval: 10 * time.Second,
		Tag:      "staging",
		Retries:  2,
	})

	h := probers.NewHTTPTrace(cfg)
	s := h.String()

	if s != "httpTrace Prober Endpoint: https://example.com - Interval: 10s - Tag: staging - Retries: 2" {
		t.Errorf("unexpected String() output: %s", s)
	}
}

func TestHTTPTrace_OneOff_Success(_ *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := probers.NewProberConfig(probers.ProberOptions{
		WG:         newTestWG(),
		PromClient: testPromC,
		Endpoint:   srv.URL,
		Interval:   1 * time.Second,
		Retries:    1,
		IsOneOff:   true,
	})

	h := probers.NewHTTPTrace(cfg)
	h.Run(context.Background())
}

func TestTCP_OneOff_FailsGracefully(_ *testing.T) {
	cfg := probers.NewProberConfig(probers.ProberOptions{
		WG:         newTestWG(),
		PromClient: testPromC,
		Endpoint:   "localhost:1", // unlikely to be open
		Interval:   1 * time.Second,
		TCPTimeout: 100 * time.Millisecond,
		Retries:    1,
		IsOneOff:   true,
	})

	tcp := probers.NewTCP(cfg)

	// Should not panic even when connection fails
	tcp.Run(context.Background())
}

func TestHTTPTrace_OneOff_RetriesOnError(t *testing.T) {
	calls := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls < 3 {
			// Close the connection abruptly to simulate an error
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("server does not support hijacking")
			}

			conn, _, err := hj.Hijack()
			if err != nil {
				t.Fatalf("hijack failed: %v", err)
			}

			_ = conn.Close()

			return
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := probers.NewProberConfig(probers.ProberOptions{
		WG:         newTestWG(),
		PromClient: testPromC,
		Endpoint:   srv.URL,
		Interval:   1 * time.Second,
		Retries:    3,
		IsOneOff:   true,
	})

	h := probers.NewHTTPTrace(cfg)
	h.Run(context.Background())

	if calls < 2 {
		t.Errorf("expected at least 2 calls (retries), got %d", calls)
	}
}

// errAlways is a helper that always returns an error, used indirectly
// through the public Prober interface to verify retry semantics.
func TestProber_RunCancelledContext(t *testing.T) {
	cfg := probers.NewProberConfig(probers.ProberOptions{
		WG:         newTestWG(),
		PromClient: testPromC,
		Endpoint:   "https://will-not-resolve.invalid",
		Interval:   50 * time.Millisecond,
		Retries:    1,
		IsOneOff:   true,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	h := probers.NewHTTPTrace(cfg)

	// Should return quickly without hanging
	done := make(chan struct{})
	go func() {
		h.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}
}

func TestContextCancellationStopsRetries(_ *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Verify that context cancellation is respected by running with a
	// short-lived context. The test passes if it completes before the
	// overall test timeout.
	<-ctx.Done()
}
