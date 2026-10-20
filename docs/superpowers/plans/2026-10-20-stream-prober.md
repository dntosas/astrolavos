# Stream prober (part 2) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `websocket` and `tcpStream` probers that hold one connection open and heartbeat it, echo servers so astrolavos can be its own peer, stream metrics/alerts, a deterministic black-hole test, and a toxiproxy Kind e2e.

**Architecture:** `internal/probers/stream.go` is one engine over a `streamDialer`/`streamConn` pair; `stream_websocket.go` and `stream_tcp.go` implement the pair. Heartbeats and connects feed the existing `conn_reused`-labelled metrics (so part 1's `AstrolavosStaleConnections` covers streams) plus two new collectors. Echo servers live in `internal/handlers/echo.go` (WebSocket, HTTP mux) and `internal/machinery/echo.go` (TCP listener).

**Tech Stack:** Go 1.27 vendored, `github.com/coder/websocket`, prometheus/client_golang, Helm + helm-unittest, Terratest + Kind + toxiproxy.

Spec: `docs/superpowers/specs/2026-10-20-stream-prober-design.md`. Branch: `feat/stale-connections` (PR #117).

## Global Constraints

- `make ci` (fmt/vet/lint/test) and `make helm-test` green before pushing; `make modsync` after `go.mod` changes and commit `vendor/`.
- No raw error strings as labels: `reason` and `error` come from `metrics.CategorizeError`.
- Every network call in a prober goes through `attemptContext`.
- Conventional Commits; no chart version bump.

---

### Task 1: dependency + stream metrics

**Files:** `go.mod`, `go.sum`, `vendor/` (modsync); `internal/metrics/metrics.go`; test `internal/metrics/metrics_internal_test.go`.

**Produces:** `PrometheusClient.SetStreamConnectionAge(domain, proberType, tag string, seconds float64)` and `PrometheusClient.IncStreamReconnects(domain, proberType, tag string, err error)`.

- [ ] Add dep: `go get github.com/coder/websocket@latest && make modsync` (vendors it; verify `vendor/modules.txt` lists it).
- [ ] Failing test (`metrics_internal_test.go`):

```go
func TestStreamMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	p := newPrometheusClient(reg, "localhost", nil)

	p.SetStreamConnectionAge("d", "websocket", "t", 42)
	p.IncStreamReconnects("d", "websocket", "t", context.DeadlineExceeded)

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	names := map[string]*dto.MetricFamily{}
	for _, f := range families {
		names[f.GetName()] = f
	}

	age, ok := names["astrolavos_stream_connection_age_seconds"]
	if !ok || age.GetMetric()[0].GetGauge().GetValue() != 42 {
		t.Fatalf("age gauge missing or wrong: %v", age)
	}

	rec, ok := names["astrolavos_stream_reconnects_total"]
	if !ok {
		t.Fatal("reconnects counter missing")
	}

	var reason string
	for _, l := range rec.GetMetric()[0].GetLabel() {
		if l.GetName() == "reason" {
			reason = l.GetValue()
		}
	}

	if reason != "timeout" {
		t.Errorf("reason = %q, want timeout", reason)
	}
}
```

- [ ] Implement in `metrics.go`: fields `streamConnectionAge *prometheus.GaugeVec` (labels `domain, tag, prober_type`) and `streamReconnects *prometheus.CounterVec` (labels `domain, tag, prober_type, reason`), registered with the others and added to the pusher; methods:

```go
// SetStreamConnectionAge records how long the current stream connection has been up (0 while disconnected).
func (p *PrometheusClient) SetStreamConnectionAge(domain, proberType, tag string, seconds float64) {
	p.streamConnectionAge.WithLabelValues(domain, tag, proberType).Set(seconds)
}

// IncStreamReconnects counts a stream connection dropped after a failed heartbeat.
func (p *PrometheusClient) IncStreamReconnects(domain, proberType, tag string, err error) {
	p.streamReconnects.WithLabelValues(domain, tag, proberType, CategorizeError(err)).Inc()
}
```

Update `TestNewPrometheusClient_AllMetricsCarryConnReused` to expect 8 families *with* `conn_reused` and skip the two `astrolavos_stream_*` families (they do not carry it).

- [ ] `go test -mod=vendor ./internal/metrics/` → PASS; lint. Commit `feat(metrics): add stream connection age and reconnect counters`.

---

### Task 2: stream engine + dialers + black-hole test

**Files:** create `internal/probers/stream.go`, `internal/probers/stream_websocket.go`, `internal/probers/stream_tcp.go`; test `internal/probers/stream_test.go`.

**Produces:** `probers.NewWebSocketStream(c ProberConfig) *Stream`, `probers.NewTCPStream(c ProberConfig) *Stream`, both satisfying `Prober`.

- [ ] Failing tests (`stream_test.go`, package `probers` internal test so the fake dialer can be injected):

```go
package probers

// fakeConn / fakeDialer let the engine be driven without a network.
type fakeConn struct{ hb func(context.Context) error }
func (f *fakeConn) Heartbeat(ctx context.Context) error { return f.hb(ctx) }
func (f *fakeConn) Close() error                        { return nil }

type fakeDialer struct {
	mu    sync.Mutex
	dials int
	dial  func(n int) (streamConn, error)
}
func (d *fakeDialer) Dial(_ context.Context) (streamConn, connectTimings, error) {
	d.mu.Lock(); d.dials++; n := d.dials; d.mu.Unlock()
	c, err := d.dial(n)
	return c, connectTimings{conn: 0.001, total: 0.002}, err
}

func newStreamForTest(t *testing.T, d streamDialer, oneOff bool) (*Stream, string) {
	t.Helper()
	domain := fmt.Sprintf("fake-%s", t.Name())
	var wg sync.WaitGroup
	wg.Add(1)
	cfg := NewProberConfig(ProberOptions{WG: &wg, PromClient: testPromC, Endpoint: domain,
		Interval: 50 * time.Millisecond, Timeout: 50 * time.Millisecond, IsOneOff: oneOff})
	return &Stream{ProberConfig: cfg, proberType: "fake", dialer: d}, domain
}

func TestStream_HeartbeatFailureReconnects(t *testing.T) {
	hbCalls := 0
	d := &fakeDialer{dial: func(int) (streamConn, error) {
		return &fakeConn{hb: func(ctx context.Context) error {
			hbCalls++
			if hbCalls == 2 { <-ctx.Done(); return ctx.Err() } // black hole on 2nd heartbeat
			return nil
		}}, nil
	}}
	s, domain := newStreamForTest(t, d, false)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	s.Run(ctx)

	if d.dials < 2 { t.Fatalf("expected a reconnect, dials=%d", d.dials) }
	if got := counterValue(t, "astrolavos_stream_reconnects_total", map[string]string{"domain": domain, "reason": "timeout"}); got != 1 {
		t.Errorf("reconnects{timeout} = %v, want 1", got)
	}
	if got := counterValue(t, "astrolavos_errors_total", map[string]string{"domain": domain, "conn_reused": "true", "error": "timeout"}); got != 1 {
		t.Errorf("errors{reused,timeout} = %v, want 1", got)
	}
	if got := counterValue(t, "astrolavos_requests_total", map[string]string{"domain": domain, "conn_reused": "false"}); got < 2 {
		t.Errorf("connect requests = %v, want >= 2", got)
	}
}

func TestStream_ConnectFailureBacksOff(t *testing.T) {
	d := &fakeDialer{dial: func(int) (streamConn, error) { return nil, errors.New("connection refused") }}
	s, domain := newStreamForTest(t, d, false)
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	s.Run(ctx)
	if d.dials < 2 || d.dials > 10 { t.Errorf("dials = %d, expected a few backed-off attempts", d.dials) }
	if got := counterValue(t, "astrolavos_errors_total", map[string]string{"domain": domain, "conn_reused": "false", "error": "connection_refused"}); got < 2 {
		t.Errorf("connect errors = %v", got)
	}
	if got := counterValue(t, "astrolavos_stream_reconnects_total", map[string]string{"domain": domain}); got != 0 {
		t.Errorf("connect failures must not count as reconnects, got %v", got)
	}
}

func TestStream_OneOff(t *testing.T) {
	d := &fakeDialer{dial: func(int) (streamConn, error) { return &fakeConn{hb: func(context.Context) error { return nil }}, nil }}
	s, domain := newStreamForTest(t, d, true)
	s.Run(context.Background())
	if d.dials != 1 { t.Errorf("dials = %d", d.dials) }
	if got := counterValue(t, "astrolavos_requests_total", map[string]string{"domain": domain, "conn_reused": "true"}); got != 1 {
		t.Errorf("heartbeats = %v, want 1", got)
	}
}
```

`counterValue` and `testPromC` exist in the external test package; move/duplicate them into an internal `export_test.go`-style helper (`internal/probers/helpers_internal_test.go`) so both packages can use them.

Black-hole proxy and real-transport tests (same file):

```go
// blackholeProxy forwards TCP bytes between client and upstream until Hold is
// set, then keeps both sockets open and discards everything: a black hole.
type blackholeProxy struct {
	ln   net.Listener
	hold atomic.Bool
}

func newBlackholeProxy(t *testing.T, upstream string) *blackholeProxy {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil { t.Fatal(err) }
	p := &blackholeProxy{ln: ln}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil { return }
			u, err := net.Dial("tcp", upstream)
			if err != nil { _ = c.Close(); continue }
			go p.pipe(c, u)
			go p.pipe(u, c)
		}
	}()
	t.Cleanup(func() { _ = ln.Close() })
	return p
}

func (p *blackholeProxy) pipe(dst, src net.Conn) {
	defer dst.Close()
	buf := make([]byte, 32*1024)
	for {
		n, err := src.Read(buf)
		if err != nil { return }
		if p.hold.Load() { continue } // swallow
		if _, err := dst.Write(buf[:n]); err != nil { return }
	}
}

func (p *blackholeProxy) Addr() string { return p.ln.Addr().String() }

func TestWebSocketStream_BlackholeIsDetected(t *testing.T) {
	srv := httptest.NewServer(handlers.NewEchoHandler())
	defer srv.Close()
	proxy := newBlackholeProxy(t, strings.TrimPrefix(srv.URL, "http://"))
	var wg sync.WaitGroup
	wg.Add(1)
	domain := "ws://" + proxy.Addr() + "/echo"
	cfg := NewProberConfig(ProberOptions{WG: &wg, PromClient: testPromC, Endpoint: domain,
		Interval: 50 * time.Millisecond, Timeout: 100 * time.Millisecond})
	s := NewWebSocketStream(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	go s.Run(ctx)

	waitFor(t, 2*time.Second, func() bool {
		return gaugeValue(t, "astrolavos_stream_connection_age_seconds", map[string]string{"domain": domain}) > 0
	})
	proxy.hold.Store(true)
	waitFor(t, 2*time.Second, func() bool {
		return counterValue(t, "astrolavos_stream_reconnects_total", map[string]string{"domain": domain, "reason": "timeout"}) >= 1
	})
	proxy.hold.Store(false)
	waitFor(t, 2*time.Second, func() bool {
		return gaugeValue(t, "astrolavos_stream_connection_age_seconds", map[string]string{"domain": domain}) > 0
	})
	cancel()
	wg.Wait()
}
```

plus the same shape for `tcpStream` against an in-test echo listener (`echoListener(t)` doing `io.Copy(c, c)`), and `waitFor`/`gaugeValue` helpers.

- [ ] `go test` → FAIL (types undefined).
- [ ] Implement `stream.go`:

```go
package probers

// connectTimings are the phases of one successful stream connect, in seconds.
// Zero values are not observed (tcpStream has no DNS/TLS/first-byte phases).
type connectTimings struct{ dns, conn, tls, gotConn, firstByte, total float64 }

type streamConn interface {
	Heartbeat(ctx context.Context) error
	Close() error
}

type streamDialer interface {
	Dial(ctx context.Context) (streamConn, connectTimings, error)
}

// Stream keeps one connection open and heartbeats it, reconnecting on failure.
type Stream struct {
	ProberConfig
	proberType string
	dialer     streamDialer
}

func (s *Stream) String() string {
	return fmt.Sprintf("%s Prober Endpoint: %s - Interval: %v - Tag: %s - Timeout: %v", s.proberType, s.endpoint, s.interval, s.tag, s.timeout)
}

func (s *Stream) Run(ctx context.Context) {
	defer s.wg.Done()
	log.Infof("Starting %s", s)
	attempt := 0
	for ctx.Err() == nil {
		conn, ok := s.connect(ctx)
		if !ok {
			if !s.sleepBackoff(ctx, attempt) { return }
			attempt++
			continue
		}
		attempt = 0
		if s.isOneOff {
			s.heartbeat(ctx, conn, time.Now())
			_ = conn.Close()
			return
		}
		s.serve(ctx, conn)
	}
	log.Infof("%s: received shutdown signal, exiting", s)
}

// connect dials once and records the connect as a fresh-connection request.
func (s *Stream) connect(ctx context.Context) (streamConn, bool) {
	actx, cancel := s.attemptContext(ctx)
	defer cancel()
	conn, tm, err := s.dialer.Dial(actx)
	s.promC.UpdateRequestsCounter(s.endpoint, s.proberType, s.tag, "", false)
	if err != nil {
		if ctx.Err() == nil {
			log.WithError(err).Warnf("%s: connect failed", s)
			s.promC.UpdateErrorsCounter(s.endpoint, s.proberType, s.tag, false, err)
		}
		s.promC.SetStreamConnectionAge(s.endpoint, s.proberType, s.tag, 0)
		return nil, false
	}
	s.observeConnect(tm)
	return conn, true
}

func (s *Stream) observeConnect(tm connectTimings) {
	obs := func(f func(string, string, string, bool, float64), v float64) {
		if v > 0 { f(s.endpoint, s.proberType, s.tag, false, v) }
	}
	obs(s.promC.UpdateDNSHistogram, tm.dns)
	obs(s.promC.UpdateConnHistogram, tm.conn)
	obs(s.promC.UpdateTLSHistogram, tm.tls)
	obs(s.promC.UpdateGotConnHistogram, tm.gotConn)
	obs(s.promC.UpdateFirstByteHistogram, tm.firstByte)
	obs(s.promC.UpdateTotalHistogram, tm.total)
}

// serve heartbeats until one fails or ctx is done; the connection is closed on return.
func (s *Stream) serve(ctx context.Context, conn streamConn) {
	defer conn.Close()
	connectedAt := time.Now()
	s.promC.SetStreamConnectionAge(s.endpoint, s.proberType, s.tag, 0)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.heartbeat(ctx, conn, connectedAt); err != nil {
				if ctx.Err() == nil {
					s.promC.IncStreamReconnects(s.endpoint, s.proberType, s.tag, err)
				}
				s.promC.SetStreamConnectionAge(s.endpoint, s.proberType, s.tag, 0)
				return
			}
		}
	}
}

// heartbeat performs one round-trip and records it as a reused-connection request.
func (s *Stream) heartbeat(ctx context.Context, conn streamConn, connectedAt time.Time) error {
	actx, cancel := s.attemptContext(ctx)
	defer cancel()
	start := time.Now()
	err := conn.Heartbeat(actx)
	s.promC.UpdateRequestsCounter(s.endpoint, s.proberType, s.tag, "", true)
	if err != nil {
		if ctx.Err() == nil {
			log.WithError(err).Warnf("%s: heartbeat failed, reconnecting", s)
			s.promC.UpdateErrorsCounter(s.endpoint, s.proberType, s.tag, true, err)
		}
		return err
	}
	s.promC.UpdateTotalHistogram(s.endpoint, s.proberType, s.tag, true, time.Since(start).Seconds())
	s.promC.SetStreamConnectionAge(s.endpoint, s.proberType, s.tag, time.Since(connectedAt).Seconds())
	return nil
}

// sleepBackoff waits 100ms·2^attempt (capped at interval); false when ctx ended.
func (s *Stream) sleepBackoff(ctx context.Context, attempt int) bool {
	shift := min(attempt, 30)
	d := min(time.Duration(100*(1<<uint(shift)))*time.Millisecond, s.interval)
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}
```

`stream_websocket.go`:

```go
type wsDialer struct{ url string; client *http.Client }

func NewWebSocketStream(c ProberConfig) *Stream {
	return &Stream{ProberConfig: c, proberType: "websocket", dialer: &wsDialer{url: c.endpoint, client: c.client}}
}

func (d *wsDialer) Dial(ctx context.Context) (streamConn, connectTimings, error) {
	t := newTracePoint()
	trace := &httptrace.ClientTrace{DNSStart: t.dnsStartHandler, DNSDone: t.dnsDoneHandler,
		ConnectStart: t.connStartHandler, ConnectDone: t.connDoneHandler,
		TLSHandshakeStart: t.tlsStartHandler, TLSHandshakeDone: t.tlsDoneHandler,
		GetConn: t.getConnTimeHandler, GotConn: t.gotConnTimeHandler, GotFirstResponseByte: t.firstByteTimeHandler}
	c, _, err := websocket.Dial(httptrace.WithClientTrace(ctx, trace), d.url, &websocket.DialOptions{HTTPClient: d.client})
	if err != nil {
		return nil, connectTimings{}, fmt.Errorf("websocket dial failed: %w", err)
	}
	t.totalDoneHandler()
	t.setDNSDuration(); t.setConnDuration(); t.setTLSDuration(); t.setGotConnDuration(); t.setFirstByteDuration(); t.setTotalDuration()
	// CloseRead runs the reader goroutine that delivers pongs to Ping.
	readCtx := c.CloseRead(context.Background())
	return &wsConn{c: c, readCtx: readCtx}, connectTimings{dns: t.dnsDuration, conn: t.connDuration, tls: t.tlsDuration,
		gotConn: t.gotConnDuration, firstByte: t.firstByteDuration, total: t.totalDuration}, nil
}

type wsConn struct{ c *websocket.Conn; readCtx context.Context }

func (w *wsConn) Heartbeat(ctx context.Context) error {
	if err := w.readCtx.Err(); err != nil { return fmt.Errorf("websocket closed: %w", err) }
	if err := w.c.Ping(ctx); err != nil { return fmt.Errorf("websocket ping failed: %w", err) }
	return nil
}

func (w *wsConn) Close() error { return w.c.Close(websocket.StatusNormalClosure, "") }
```

Note: when `ctx` of `Ping` expires, coder/websocket closes the connection; that is what we want (the engine reconnects). `tracePoint.setXDuration` on unset times yields negative values; guard in `observeConnect` by `v > 0` (already) and make the setters tolerate zero times (compute only when both timestamps are set).

`stream_tcp.go`:

```go
type tcpDialer struct{ addr string }

func NewTCPStream(c ProberConfig) *Stream {
	return &Stream{ProberConfig: c, proberType: "tcpstream", dialer: &tcpDialer{addr: c.endpoint}}
}

func (d *tcpDialer) Dial(ctx context.Context) (streamConn, connectTimings, error) {
	start := time.Now()
	var dialer net.Dialer
	c, err := dialer.DialContext(ctx, "tcp", d.addr)
	if err != nil { return nil, connectTimings{}, fmt.Errorf("tcp dial failed: %w", err) }
	el := time.Since(start).Seconds()
	return &tcpConn{c: c, r: bufio.NewReader(c)}, connectTimings{conn: el, total: el}, nil
}

type tcpConn struct{ c net.Conn; r *bufio.Reader; seq uint64 }

func (t *tcpConn) Heartbeat(ctx context.Context) error {
	deadline, ok := ctx.Deadline()
	if !ok { deadline = time.Now().Add(DefaultTimeout) }
	if err := t.c.SetDeadline(deadline); err != nil { return err }
	t.seq++
	msg := fmt.Sprintf("ping %d\n", t.seq)
	if _, err := io.WriteString(t.c, msg); err != nil { return fmt.Errorf("tcp heartbeat write failed: %w", err) }
	line, err := t.r.ReadString('\n')
	if err != nil {
		if ne, ok := err.(net.Error); ok && ne.Timeout() { return fmt.Errorf("tcp heartbeat timeout: %w", context.DeadlineExceeded) }
		return fmt.Errorf("tcp heartbeat read failed: %w", err)
	}
	if line != msg { return fmt.Errorf("tcp heartbeat mismatch: sent %q got %q", msg, line) }
	return nil
}

func (t *tcpConn) Close() error { return t.c.Close() }
```

- [ ] `go test -mod=vendor -race ./internal/probers/` → PASS (black-hole tests need Task 3's `handlers.NewEchoHandler`; implement Task 3 handler first if the import blocks compilation, then return). Lint. Commit `feat(probers): add websocket and tcpStream long-lived probers`.

---

### Task 3: echo servers

**Files:** create `internal/handlers/echo.go`, `internal/handlers/echo_test.go`, `internal/machinery/echo.go`, `internal/machinery/echo_test.go`; modify `internal/machinery/app.go`, `internal/config/config.go` (`EchoPort`), `main.go`.

- [ ] Failing tests: `echo_test.go` dials the handler via `httptest` with `websocket.Dial`, writes `"hello"`, reads `"hello"`, then `Ping(ctx)` succeeds. `machinery/echo_test.go` starts `newEchoListener(ctx, port 0)`, dials, writes `"x\n"`, reads `"x\n"`, cancels ctx, expects the listener to stop accepting.
- [ ] Implement:

```go
// handlers/echo.go
// NewEchoHandler upgrades to WebSocket and echoes every message back; pings
// are answered during Read. Any origin is accepted: this is a probe target.
func NewEchoHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"*"}})
		if err != nil { log.WithError(err).Debug("echo: websocket accept failed"); return }
		defer c.CloseNow()
		for {
			typ, data, err := c.Read(r.Context())
			if err != nil { return }
			if err := c.Write(r.Context(), typ, data); err != nil { return }
		}
	}
}
```

```go
// machinery/echo.go
// startEchoListener serves a plain TCP echo on port until ctx is done.
// Returns the listener (nil when port == 0) so shutdown can close it.
func startEchoListener(ctx context.Context, port int) (net.Listener, error) {
	if port == 0 { return nil, nil }
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil { return nil, fmt.Errorf("echo listener: %w", err) }
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil { return }
			go func() {
				defer c.Close()
				go func() { <-ctx.Done(); _ = c.Close() }()
				_, _ = io.Copy(c, c)
			}()
		}
	}()
	log.WithField("port", port).Info("Starting TCP echo listener")
	return ln, nil
}
```

Wire: `Astrolavos.echoPort int`; `NewAstrolavos(..., echoPort int)`; in `startServerMode` after `a.agent.start(ctx)`: `echoLn, err := startEchoListener(ctx, a.echoPort)`; in `gracefulShutdown` close `echoLn` right after `cancel()`. Register `mux.HandleFunc("/echo", handlers.NewEchoHandler())` — but `http.Server.WriteTimeout` (30s) would kill long WebSocket connections: WebSocket hijacks the connection, so the server timeouts no longer apply after the upgrade (document in a comment). `config`: `viper.SetDefault("ECHO_PORT", "3001")`, `EchoPort: viper.GetInt("echo_port")`. `main.go` passes `cfg.EchoPort`. Update `agent_test.go` call.

- [ ] Tests + lint → PASS. Commit `feat(server): serve WebSocket and TCP echo endpoints for stream probers`.

---

### Task 4: config + agent wiring

**Files:** `internal/config/config.go`, `internal/machinery/agent.go`, tests.

- [ ] Failing config tests: `prober: websocket` → `URI ws://d`, with `HTTPS` → `wss://d`; `prober: tcpStream` → `URI d`; `prober: bogus` error lists all four.
- [ ] Implement: `validProbers = []string{"tcp", "httpTrace", "websocket", "tcpStream"}`; URI scheme switch (`httpTrace` http/https, `websocket` ws/wss, else bare); warn when `Retries > 1` for stream probers. `agent.go` switch: `case "websocket": o = probers.NewWebSocketStream(p)`, `case "tcpStream": o = probers.NewTCPStream(p)`.
- [ ] `make ci` → PASS. Commit `feat(config): accept websocket and tcpStream probers`.

---

### Task 5: chart (values, templates, alerts, tests)

**Files:** `deploy/kubernetes/values.yaml`, `templates/deployment.yaml`, `templates/service.yaml`, `templates/alerts.yaml`, `templates/_alerts.tpl`, tests `workload_test.yaml`, `service_test.yaml`, `config_test.yaml`, `alerts_test.yaml`.

- [ ] Failing tests: workload renders env `ASTROLAVOS_ECHO_PORT='3001'` and container port `echo: 3001`; with `echo.enabled: false` env is `'0'` and no `echo` port; service has `{name: echo, port: 3001, targetPort: echo}` only when enabled; config default endpoints render two entries, the second `domain: astrolavos/echo`, `prober: websocket`; alerts: group `astrolavos.stream` (index 2, before selfhealth) with `AstrolavosStreamReconnecting` (`> 3`, `[10m]`) and `AstrolavosStreamReconnectBurst` (`> 0.5`, `[5m]`, `count by`).
- [ ] values:

```yaml
## Echo endpoints: lets this release be the peer of websocket / tcpStream probes.
echo:
  # -- Serve a plain TCP echo on `containerPorts.echo` (exposed as Service port `echo`). The WebSocket echo at `/echo` is always served on the http port.
  enabled: true
containerPorts:
  http: 3000
  # -- TCP echo listener port (only used when `echo.enabled`)
  echo: 3001
service:
  ports:
    http: 3000
    # -- Service port for the TCP echo (targets container port `echo`)
    echo: 3001
```

Default endpoints: add the websocket entry from the spec. Rules:

```yaml
    # -- A stream endpoint reconnected more than `threshold` times in 10m: the long-lived connection cannot stay up
    streamReconnecting: {enabled: true, threshold: 3, for: 5m, severity: warning}
    # -- Fraction of pods whose stream reconnected within 5m above `threshold`: a dataplane event severed established flows fleet-wide
    streamReconnectBurst: {enabled: true, threshold: 0.5, for: 1m, severity: critical}
```

deployment.yaml env (before the `extraEnv` range): `- name: ASTROLAVOS_ECHO_PORT` / `value: {{ ternary (.Values.containerPorts.echo | toString) "0" .Values.echo.enabled | squote }}`; ports loop skips `echo` when `echo.enabled` is false. service.yaml: `{{- if .Values.echo.enabled }}` block adding the `echo` port with `targetPort: echo`. alerts.yaml: new group dict `(dict "name" "astrolavos.stream" "keys" (list "streamReconnecting" "streamReconnectBurst"))` inserted before selfhealth. `_alerts.tpl`: the two rule templates per the spec PromQL, with annotations explaining the Cilium-rollout meaning and `kubectl get pods -l app.kubernetes.io/name=astrolavos -o wide` / `kubectl rollout history ds/cilium -n kube-system` hints.

- [ ] `make helm-test`, `helm lint --strict`, promtool/vmalert check via Docker. Commit `feat(helm): echo ports, default websocket stream endpoint and stream alerts`.

---

### Task 6: dashboards

- [ ] Failing `dashboards_test.yaml` asserts: Grafana JSON contains `"title": "Stream connection age"` and `astrolavos_stream_reconnects_total`; Datadog widgets contain `astrolavos_stream_connection_age_seconds`.
- [ ] Grafana: clone the "Errors" panel twice with jq into a new row at the bottom: "Stream connection age" `min by (pod, domain) (astrolavos_stream_connection_age_seconds{tag=~"$tag", domain=~"$endpoint"})` (unit `s`) and "Stream reconnects (5m)" `sum by (domain, reason) (increase(astrolavos_stream_reconnects_total{tag=~"$tag", domain=~"$endpoint"}[5m]))`. Datadog: two timeseries widgets `min:astrolavos_stream_connection_age_seconds{$tag,$endpoint} by {pod_name,domain}` and `sum:astrolavos_stream_reconnects_total{$tag,$endpoint} by {domain,reason}.as_count()` appended after the existing ones (layout y below the last widget).
- [ ] `make helm-test` → PASS. Commit `feat(dashboards): stream connection age and reconnect panels`.

---

### Task 7: toxiproxy e2e

**Files:** `tests/e2e_test.go`, create `tests/testdata/toxiproxy.yaml`.

- [ ] `toxiproxy.yaml`: Deployment `toxiproxy` (image `ghcr.io/shopify/toxiproxy:2.11.0`, ports 8474 api, 8000 proxy) + Service `toxiproxy` exposing both.
- [ ] `TestE2EStreamBlackhole`:
  1. `k8s.KubectlApply(t, opts, "testdata/toxiproxy.yaml")`, `k8s.WaitUntilServiceAvailable`.
  2. Deploy astrolavos (release A) as the echo peer with `config.endpoints` = `kubernetes` https (any), `echo.enabled=true`.
  3. Tunnel to toxiproxy 8474; `POST /proxies {"name":"echo","listen":"0.0.0.0:8000","upstream":"<releaseA>:80","enabled":true}`.
  4. Deploy astrolavos release B with `config.endpoints` = `- domain: toxiproxy:8000/echo\n  prober: websocket\n  interval: 1s\n  timeout: 2s\n  tag: e2e`.
  5. Tunnel to B's pod 3000; poll `/metrics` until `astrolavos_stream_connection_age_seconds{domain="ws://toxiproxy:8000/echo"` has value `> 0`.
  6. `POST /proxies/echo/toxics {"type":"timeout","stream":"downstream","attributes":{"timeout":0}}` and the same for `upstream`.
  7. Poll `/metrics` until `astrolavos_stream_reconnects_total{...reason="timeout"}` ≥ 1.
  8. `DELETE /proxies/echo/toxics/timeout_downstream` and `_upstream`; poll until age `> 0` again.
  Helpers: `metricValue(body, name, labelSubstring) (float64, bool)` parsing the exposition text; `toxiproxyDo(method, path, body)` over the tunnel.
- [ ] Verify locally if `kind` is available: build the image (`docker build -t astrolavos:e2e .`), `kind create cluster`, `kind load docker-image`, `IMAGE_TAG=e2e go test ./... -run TestE2EStreamBlackhole -v` in `tests/` with `image.repository` pointing at the local image and `image.pullPolicy=IfNotPresent`; otherwise rely on the post-release `e2e.yml` run and say so in the PR. Commit `test(e2e): black-hole a websocket stream through toxiproxy and expect a reconnect`.

---

### Task 8: docs and PR update

- [ ] README: prober list gains `websocket` and `tcpStream` with the heartbeat semantics; "Detecting Stale Connections" gets a second paragraph on streams + echo endpoints; env table `ASTROLAVOS_ECHO_PORT | 3001 | TCP echo listener port (0 disables)`; HTTP endpoints list adds `/echo`. `examples/config.yaml`: websocket and tcpStream examples. AGENTS.md: layout (`stream*.go`, `echo.go`), prober list in "Adding a New Prober" step 7, env table, metrics note. `make helm-docs`.
- [ ] `make ci && make helm-test`. Commit `docs: document the stream probers and echo endpoints`.
- [ ] Push; edit PR #117 title to `feat: detect stale connections (timeouts, conn_reused, websocket/tcpStream probers, alerts)` and prepend a "Part 2" section to the body.

---

## Self-review

- Spec coverage: config (T4), engine/dialers/metrics semantics (T1–T2), echo servers + env (T3), chart ports/default endpoint/alerts (T5), dashboards (T6), black-hole unit test (T2) and toxiproxy e2e (T7), docs (T8).
- Placeholders: none. Type consistency: `streamConn`/`streamDialer`/`connectTimings` names identical across T2 code and tests; `NewEchoHandler` used by T2 tests and T3; `SetStreamConnectionAge`/`IncStreamReconnects` signatures identical in T1 and T2.
