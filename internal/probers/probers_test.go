package probers

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestRetryWithBackoff_SuccessFirstAttempt(t *testing.T) {
	p := ProberConfig{retries: 3}
	calls := 0

	err := p.retryWithBackoff(context.Background(), func() error {
		calls++

		return nil
	})

	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}

	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
}

func TestRetryWithBackoff_SuccessAfterRetries(t *testing.T) {
	p := ProberConfig{retries: 3}
	calls := 0

	err := p.retryWithBackoff(context.Background(), func() error {
		calls++
		if calls < 3 {
			return errors.New("transient failure")
		}

		return nil
	})

	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}

	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

func TestRetryWithBackoff_AllAttemptsFail(t *testing.T) {
	p := ProberConfig{retries: 2}
	calls := 0
	expectedErr := errors.New("persistent failure")

	err := p.retryWithBackoff(context.Background(), func() error {
		calls++

		return expectedErr
	})

	if err == nil {
		t.Error("expected error, got nil")
	}

	if err.Error() != expectedErr.Error() {
		t.Errorf("expected error %q, got %q", expectedErr, err)
	}

	if calls != 2 {
		t.Errorf("expected 2 calls, got %d", calls)
	}
}

func TestRetryWithBackoff_RespectsContextCancellation(t *testing.T) {
	p := ProberConfig{retries: 10}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := p.retryWithBackoff(ctx, func() error {
		return errors.New("should not retry")
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestRetryWithBackoff_SingleRetry(t *testing.T) {
	p := ProberConfig{retries: 1}
	calls := 0

	err := p.retryWithBackoff(context.Background(), func() error {
		calls++

		return errors.New("fail")
	})

	if err == nil {
		t.Error("expected error, got nil")
	}

	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
}

func TestRunLoop_OneOff(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	p := ProberConfig{
		wg:       &wg,
		isOneOff: true,
	}

	probed := false
	p.runLoop(context.Background(), "test-oneoff", func(_ context.Context) {
		probed = true
	})

	if !probed {
		t.Error("expected probe to be called in one-off mode")
	}
}

func TestRunLoop_IntervalMode(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	p := ProberConfig{
		wg:       &wg,
		interval: 50 * time.Millisecond,
	}

	ctx, cancel := context.WithCancel(context.Background())
	probeCount := 0

	go p.runLoop(ctx, "test-interval", func(_ context.Context) {
		probeCount++
		if probeCount >= 3 {
			cancel()
		}
	})

	wg.Wait()

	if probeCount < 3 {
		t.Errorf("expected at least 3 probes, got %d", probeCount)
	}
}

func TestNewProberConfig(t *testing.T) {
	var wg sync.WaitGroup

	opts := ProberOptions{
		WG:                  &wg,
		Endpoint:            "https://example.com",
		Tag:                 "test",
		Retries:             5,
		Interval:            10 * time.Second,
		IsOneOff:            true,
		ReuseConnection:     true,
		SkipTLSVerification: true,
	}

	p := NewProberConfig(opts)

	if p.endpoint != "https://example.com" {
		t.Errorf("expected endpoint 'https://example.com', got %q", p.endpoint)
	}

	if p.tag != "test" {
		t.Errorf("expected tag 'test', got %q", p.tag)
	}

	if p.retries != 5 {
		t.Errorf("expected retries 5, got %d", p.retries)
	}

	if p.interval != 10*time.Second {
		t.Errorf("expected interval 10s, got %v", p.interval)
	}

	if !p.isOneOff {
		t.Error("expected isOneOff to be true")
	}

	if !p.reuseConnection {
		t.Error("expected reuseConnection to be true")
	}

	if !p.skipTLS {
		t.Error("expected skipTLS to be true")
	}

	if p.client == nil {
		t.Error("expected HTTP client to be initialized")
	}
}

func TestTCPString(t *testing.T) {
	cfg := ProberConfig{
		endpoint: "example.com:443",
		interval: 5 * time.Second,
		tag:      "prod",
		retries:  3,
	}

	tcp := NewTCP(cfg)
	s := tcp.String()

	if s != "TCP Prober Endpoint: example.com:443 - Interval: 5s - Tag: prod - Retries: 3" {
		t.Errorf("unexpected String() output: %s", s)
	}
}

func TestHTTPTraceString(t *testing.T) {
	cfg := ProberConfig{
		endpoint: "https://example.com",
		interval: 10 * time.Second,
		tag:      "staging",
		retries:  2,
	}

	h := NewHTTPTrace(cfg)
	s := h.String()

	if s != "httpTrace Prober Endpoint: https://example.com - Interval: 10s - Tag: staging - Retries: 2" {
		t.Errorf("unexpected String() output: %s", s)
	}
}
