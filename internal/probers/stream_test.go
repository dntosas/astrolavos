package probers_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dntosas/astrolavos/internal/handlers"
	"github.com/dntosas/astrolavos/internal/probers"

	"github.com/prometheus/client_golang/prometheus"
)

// fakeConn and fakeDialer drive the stream engine without a network.
type fakeConn struct {
	hb func(context.Context) error
}

func (f *fakeConn) Heartbeat(ctx context.Context) error { return f.hb(ctx) }
func (f *fakeConn) Close() error                        { return nil }

type fakeDialer struct {
	mu    sync.Mutex
	dials int
	dial  func(n int) (probers.StreamConn, error)
}

func (d *fakeDialer) Dial(_ context.Context) (probers.StreamConn, probers.ConnectTimings, error) {
	d.mu.Lock()
	d.dials++
	n := d.dials
	d.mu.Unlock()

	c, err := d.dial(n)

	return c, probers.ConnectTimings{}, err
}

func (d *fakeDialer) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.dials
}

func newStreamForTest(t *testing.T, d probers.StreamDialer, oneOff bool) (*probers.Stream, string) {
	t.Helper()

	domain := "fake://" + t.Name()

	cfg := probers.NewProberConfig(probers.ProberOptions{
		WG:         newTestWG(),
		PromClient: testPromC,
		Endpoint:   domain,
		Interval:   50 * time.Millisecond,
		Timeout:    50 * time.Millisecond,
		IsOneOff:   oneOff,
	})

	return probers.NewTestStream(cfg, "fake", d), domain
}

func TestStream_HeartbeatFailureReconnects(t *testing.T) {
	var hbCalls atomic.Int32

	d := &fakeDialer{dial: func(int) (probers.StreamConn, error) {
		return &fakeConn{hb: func(ctx context.Context) error {
			if hbCalls.Add(1) == 2 { // the second heartbeat is black-holed
				<-ctx.Done()

				return ctx.Err()
			}

			return nil
		}}, nil
	}}

	s, domain := newStreamForTest(t, d, false)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	s.Run(ctx)

	if d.count() < 2 {
		t.Fatalf("expected a reconnect, dials=%d", d.count())
	}

	if got := counterValue(t, "astrolavos_stream_reconnects_total", map[string]string{"domain": domain, "reason": "timeout"}); got != 1 {
		t.Errorf("reconnects{timeout} = %v, want 1", got)
	}

	if got := counterValue(t, "astrolavos_errors_total", map[string]string{"domain": domain, "conn_reused": "true", "error": "timeout"}); got != 1 {
		t.Errorf("errors{reused,timeout} = %v, want 1", got)
	}

	if got := counterValue(t, "astrolavos_requests_total", map[string]string{"domain": domain, "conn_reused": "false"}); got < 2 {
		t.Errorf("connect requests = %v, want >= 2", got)
	}

	if got := counterValue(t, "astrolavos_requests_total", map[string]string{"domain": domain, "conn_reused": "true"}); got < 3 {
		t.Errorf("heartbeats = %v, want >= 3", got)
	}
}

func TestStream_ConnectFailureBacksOffWithoutCountingReconnects(t *testing.T) {
	d := &fakeDialer{dial: func(int) (probers.StreamConn, error) {
		return nil, errors.New("dial tcp: connection refused")
	}}

	s, domain := newStreamForTest(t, d, false)

	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()

	s.Run(ctx)

	// backoff 100ms, 200ms, then capped at the 50ms interval: a handful of attempts
	if n := d.count(); n < 2 || n > 10 {
		t.Errorf("dials = %d, expected a few backed-off attempts", n)
	}

	if got := counterValue(t, "astrolavos_errors_total", map[string]string{"domain": domain, "conn_reused": "false", "error": "connection_refused"}); got < 2 {
		t.Errorf("connect errors = %v, want >= 2", got)
	}

	if got := counterValue(t, "astrolavos_stream_reconnects_total", map[string]string{"domain": domain}); got != 0 {
		t.Errorf("connect failures must not count as reconnects, got %v", got)
	}
}

func TestStream_OneOff(t *testing.T) {
	d := &fakeDialer{dial: func(int) (probers.StreamConn, error) {
		return &fakeConn{hb: func(context.Context) error { return nil }}, nil
	}}

	s, domain := newStreamForTest(t, d, true)
	s.Run(context.Background())

	if d.count() != 1 {
		t.Errorf("dials = %d, want 1", d.count())
	}

	if got := counterValue(t, "astrolavos_requests_total", map[string]string{"domain": domain, "conn_reused": "true"}); got != 1 {
		t.Errorf("heartbeats = %v, want 1", got)
	}
}

func TestStream_ShutdownIsNotAReconnect(t *testing.T) {
	d := &fakeDialer{dial: func(int) (probers.StreamConn, error) {
		return &fakeConn{hb: func(ctx context.Context) error {
			<-ctx.Done() // hangs until the run context is cancelled

			return ctx.Err()
		}}, nil
	}}

	s, domain := newStreamForTest(t, d, false)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})

	go func() {
		s.Run(ctx)
		close(done)
	}()

	time.Sleep(70 * time.Millisecond) // first heartbeat in flight
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}

	if got := counterValue(t, "astrolavos_stream_reconnects_total", map[string]string{"domain": domain}); got != 0 {
		t.Errorf("shutdown counted as reconnect: %v", got)
	}
}

// blackholeProxy forwards TCP bytes between client and upstream until hold
// is set, then keeps both sockets open and discards everything: exactly what
// a flow whose packets are silently dropped looks like to the application.
type blackholeProxy struct {
	ln   net.Listener
	hold atomic.Bool
}

func newBlackholeProxy(t *testing.T, upstream string) *blackholeProxy {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	p := &blackholeProxy{ln: ln}

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}

			u, err := net.Dial("tcp", upstream)
			if err != nil {
				_ = c.Close()

				continue
			}

			go p.pipe(c, u)
			go p.pipe(u, c)
		}
	}()

	t.Cleanup(func() { _ = ln.Close() })

	return p
}

func (p *blackholeProxy) Addr() string { return p.ln.Addr().String() }

func (p *blackholeProxy) pipe(dst, src net.Conn) {
	defer func() { _ = dst.Close() }()

	buf := make([]byte, 32*1024)

	for {
		n, err := src.Read(buf)
		if err != nil {
			return
		}

		if p.hold.Load() {
			continue // swallow silently
		}

		if _, err := dst.Write(buf[:n]); err != nil {
			return
		}
	}
}

// echoListener is a plain TCP echo server for the tcpStream tests.
func echoListener(t *testing.T) net.Listener {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}

			go func() {
				defer func() { _ = c.Close() }()

				_, _ = io.Copy(c, c)
			}()
		}
	}()

	t.Cleanup(func() { _ = ln.Close() })

	return ln
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}

		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("condition not met within %v", timeout)
}

// gaugeValue returns the value of the gauge series whose labels contain want.
func gaugeValue(t *testing.T, name string, want map[string]string) float64 {
	t.Helper()

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	for _, f := range families {
		if f.GetName() != name {
			continue
		}

	metrics:
		for _, m := range f.GetMetric() {
			labels := map[string]string{}
			for _, l := range m.GetLabel() {
				labels[l.GetName()] = l.GetValue()
			}

			for k, v := range want {
				if labels[k] != v {
					continue metrics
				}
			}

			return m.GetGauge().GetValue()
		}
	}

	return 0
}

// runBlackholeScenario connects a stream through a black-hole proxy, drops
// the flow, expects a timeout-driven reconnect, restores the flow and
// expects the connection to come back.
func runBlackholeScenario(t *testing.T, s *probers.Stream, wg *sync.WaitGroup, domain string, proxy *blackholeProxy) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())

	go s.Run(ctx)

	ageUp := func() bool {
		return gaugeValue(t, "astrolavos_stream_connection_age_seconds", map[string]string{"domain": domain}) > 0
	}

	waitFor(t, 3*time.Second, ageUp)

	proxy.hold.Store(true)

	waitFor(t, 3*time.Second, func() bool {
		return counterValue(t, "astrolavos_stream_reconnects_total", map[string]string{"domain": domain, "reason": "timeout"}) >= 1
	})

	if got := counterValue(t, "astrolavos_errors_total", map[string]string{"domain": domain, "conn_reused": "true", "error": "timeout"}); got < 1 {
		t.Errorf("expected a reused-connection timeout error, got %v", got)
	}

	proxy.hold.Store(false)

	waitFor(t, 3*time.Second, ageUp)

	cancel()
	wg.Wait()
}

func TestWebSocketStream_BlackholeIsDetected(t *testing.T) {
	srv := httptest.NewServer(handlers.NewEchoHandler())
	defer srv.Close()

	proxy := newBlackholeProxy(t, strings.TrimPrefix(srv.URL, "http://"))
	domain := "ws://" + proxy.Addr() + "/echo"

	var wg sync.WaitGroup

	wg.Add(1)

	cfg := probers.NewProberConfig(probers.ProberOptions{
		WG:         &wg,
		PromClient: testPromC,
		Endpoint:   domain,
		Interval:   50 * time.Millisecond,
		Timeout:    150 * time.Millisecond,
	})

	runBlackholeScenario(t, probers.NewWebSocketStream(cfg), &wg, domain, proxy)

	// the upgrade was traced: connect phases landed as a fresh connection
	if got := counterValue(t, "astrolavos_requests_total", map[string]string{"domain": domain, "conn_reused": "false", "prober_type": "websocket"}); got < 2 {
		t.Errorf("expected at least 2 websocket connects, got %v", got)
	}
}

func TestTCPStream_BlackholeIsDetected(t *testing.T) {
	ln := echoListener(t)
	proxy := newBlackholeProxy(t, ln.Addr().String())
	domain := proxy.Addr()

	var wg sync.WaitGroup

	wg.Add(1)

	cfg := probers.NewProberConfig(probers.ProberOptions{
		WG:         &wg,
		PromClient: testPromC,
		Endpoint:   domain,
		Interval:   50 * time.Millisecond,
		Timeout:    150 * time.Millisecond,
	})

	runBlackholeScenario(t, probers.NewTCPStream(cfg), &wg, domain, proxy)

	if got := counterValue(t, "astrolavos_requests_total", map[string]string{"domain": domain, "conn_reused": "true", "prober_type": "tcpstream"}); got < 2 {
		t.Errorf("expected tcpstream heartbeats, got %v", got)
	}
}

func TestStream_String(t *testing.T) {
	cfg := probers.NewProberConfig(probers.ProberOptions{
		WG: newTestWG(), PromClient: testPromC, Endpoint: "ws://x/echo", Interval: 5 * time.Second, Timeout: 2 * time.Second, Tag: "t",
	})

	want := fmt.Sprintf("websocket Prober Endpoint: ws://x/echo - Interval: 5s - Tag: t - Timeout: 2s")
	if got := probers.NewWebSocketStream(cfg).String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestStream_OneOffConnectFailureReturns(t *testing.T) {
	d := &fakeDialer{dial: func(int) (probers.StreamConn, error) {
		return nil, errors.New("dial tcp: connection refused")
	}}

	s, domain := newStreamForTest(t, d, true)

	done := make(chan struct{})

	go func() {
		s.Run(context.Background())
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("one-off Run did not return after a failed connect")
	}

	if d.count() != 1 {
		t.Errorf("dials = %d, want exactly 1 in one-off mode", d.count())
	}

	if got := counterValue(t, "astrolavos_errors_total", map[string]string{"domain": domain, "conn_reused": "false"}); got != 1 {
		t.Errorf("connect errors = %v, want 1", got)
	}
}
