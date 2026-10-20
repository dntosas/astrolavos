# Stale-connection detection (part 1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make hung probes fail fast with a per-attempt `timeout`, label every probe metric with `conn_reused`, and alert when reused connections time out while fresh ones succeed.

**Architecture:** `timeout` flows config → model → `ProberConfig` and wraps each retry attempt in `context.WithTimeout`. `tracePoint` captures `GotConnInfo.Reused`; every `PrometheusClient.Update*` method takes a `reused bool` that becomes the `conn_reused` label. Two new rule templates plug into the `_alerts.tpl` dispatch from #116. Dashboards get a connection-kind variable.

**Tech Stack:** Go 1.27 (vendored, `-mod=vendor`), prometheus/client_golang, Helm 4 + helm-unittest 1.0.3, helm-docs 1.11, jq for the dashboard JSON edits.

Spec: `docs/superpowers/specs/2026-10-20-stale-connection-detection-design.md`.

## Global Constraints

- Branch `feat/stale-connections`, created from `feat/chart-alerting` (stacked on #116).
- `make ci` must pass before the PR (fmt → vet → lint → test → vulncheck); `make helm-test` and `helm lint --strict deploy/kubernetes` for the chart.
- Never pass raw error strings as labels; `conn_reused` values are exactly `"true"`/`"false"` via `metrics.ReusedLabel`.
- Do not bump `Chart.yaml`/`image.tag`; the PR gets the `release:minor` label.
- Conventional Commits.

---

### Task 1: per-attempt `timeout`

**Files:**
- Modify: `internal/model/endpoint.go`
- Modify: `internal/config/config.go` (`YamlEndpoint`, `getCleanEndpoint`)
- Modify: `internal/probers/probers.go` (`ProberOptions`, `ProberConfig`, `NewProberConfig`)
- Modify: `internal/probers/httptrace.go`, `internal/probers/tcp.go`
- Modify: `internal/machinery/agent.go`
- Test: `internal/config/config_test.go`, `internal/probers/probers_test.go`

**Interfaces:**
- Produces `model.Endpoint.Timeout time.Duration`, `probers.ProberOptions.Timeout time.Duration`, `probers.ProberConfig.timeout` (default `10 * time.Second` when zero), `probers.DefaultTimeout` exported const.

- [ ] **Step 1: Failing config tests** — append to `internal/config/config_test.go`:

```go
func TestGetCleanEndpoint_TimeoutDefault(t *testing.T) {
	ep, err := (&YamlEndpoint{Domain: "example.com"}).getCleanEndpoint()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ep.Timeout != 10*time.Second {
		t.Errorf("expected default timeout 10s, got %v", ep.Timeout)
	}
}

func TestGetCleanEndpoint_TimeoutCustom(t *testing.T) {
	timeout := 2 * time.Second

	ep, err := (&YamlEndpoint{Domain: "example.com", Timeout: &timeout}).getCleanEndpoint()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ep.Timeout != timeout {
		t.Errorf("expected timeout %v, got %v", timeout, ep.Timeout)
	}
}

func TestGetCleanEndpoint_TimeoutMustBePositive(t *testing.T) {
	for _, bad := range []time.Duration{0, -time.Second} {
		_, err := (&YamlEndpoint{Domain: "example.com", Timeout: &bad}).getCleanEndpoint()
		if err == nil {
			t.Errorf("expected error for timeout %v", bad)
		}
	}
}
```

- [ ] **Step 2: Failing prober test** — append to `internal/probers/probers_test.go` a black-hole server test:

```go
// blackholeServer accepts connections and never answers, like a flow whose
// packets are silently dropped by the datapath.
func blackholeServer(t *testing.T) *httptest.Server {
	t.Helper()

	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		<-release
	}))
	t.Cleanup(func() { close(release); srv.Close() })

	return srv
}

func TestHTTPTrace_TimeoutUnblocksHungProbe(t *testing.T) {
	srv := blackholeServer(t)

	cfg := probers.NewProberConfig(probers.ProberOptions{
		WG: newTestWG(), PromClient: testPromC, Endpoint: srv.URL,
		Interval: time.Second, Retries: 1, IsOneOff: true, Timeout: 200 * time.Millisecond,
	})

	done := make(chan struct{})
	go func() { probers.NewHTTPTrace(cfg).Run(context.Background()); close(done) }()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("probe did not return within 2s; timeout not applied")
	}

	if got := counterValue(t, "astrolavos_errors_total", map[string]string{"domain": srv.URL, "error": "timeout"}); got != 1 {
		t.Errorf("expected 1 timeout error for %s, got %v", srv.URL, got)
	}
}
```

with the gather helper (same file):

```go
// counterValue sums the samples of a counter family whose labels contain want.
func counterValue(t *testing.T, name string, want map[string]string) float64 {
	t.Helper()

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	var total float64

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
			total += m.GetCounter().GetValue()
		}
	}

	return total
}
```

(imports: `github.com/prometheus/client_golang/prometheus`.)

- [ ] **Step 3: Run** `go test -mod=vendor ./internal/config/ ./internal/probers/` → FAIL (unknown field `Timeout`).

- [ ] **Step 4: Implement**

`internal/model/endpoint.go`: add `Timeout time.Duration` after `TCPTimeout`.

`internal/config/config.go`: add `Timeout *time.Duration \`yaml:"timeout"\`` to `YamlEndpoint`; in `getCleanEndpoint` after the tcpTimeout block:

```go
	timeout := defaultTimeout
	if r.Timeout != nil {
		if *r.Timeout <= 0 {
			return nil, fmt.Errorf("timeout must be greater than zero, got %v", *r.Timeout)
		}

		timeout = *r.Timeout
	}
```

with `const defaultTimeout = 10 * time.Second` at package level and `Timeout: timeout` in the `model.Endpoint` literal.

`internal/probers/probers.go`: `const DefaultTimeout = 10 * time.Second`; `Timeout time.Duration` in `ProberOptions`; `timeout time.Duration` in `ProberConfig`; in `NewProberConfig`:

```go
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
```

and `timeout: timeout`. Add helper:

```go
// attemptContext bounds a single probe attempt by the configured timeout.
func (p *ProberConfig) attemptContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, p.timeout)
}
```

`httptrace.go` `probe`:

```go
	err := h.retryWithBackoff(ctx, func() error {
		attemptCtx, cancel := h.attemptContext(ctx)
		defer cancel()

		var traceErr error
		t, traceErr = h.trace(attemptCtx)

		return traceErr
	})
```

`tcp.go` `probe`: same shape around `t.dial(attemptCtx)`.

`agent.go`: `Timeout: e.Timeout,` in the `ProberOptions` literal.

- [ ] **Step 5: Run** `go test -mod=vendor ./internal/...` → PASS. `make fmt vet lint`.

- [ ] **Step 6: Commit** `fix(probers): bound every probe attempt by a per-endpoint timeout`

---

### Task 2: `conn_reused` label

**Files:**
- Modify: `internal/metrics/metrics.go`
- Modify: `internal/probers/httptrace_point.go`, `internal/probers/httptrace.go`, `internal/probers/tcp.go`
- Test: `internal/metrics/metrics_test.go`, `internal/metrics/metrics_internal_test.go`, `internal/probers/probers_test.go`

**Interfaces:**
- `metrics.ReusedLabel(reused bool) string` → `"true"`/`"false"`.
- Every `PrometheusClient.Update*Histogram(domain, proberType, tag string, reused bool, duration float64)`; `UpdateRequestsCounter(domain, proberType, tag, statusCode string, reused bool)`; `UpdateErrorsCounter(domain, proberType, tag string, reused bool, err error)`.
- `tracePoint.reused bool`.

- [ ] **Step 1: Failing tests**

`internal/metrics/metrics_test.go`:

```go
func TestReusedLabel(t *testing.T) {
	if got := metrics.ReusedLabel(true); got != "true" {
		t.Errorf("ReusedLabel(true) = %q", got)
	}
	if got := metrics.ReusedLabel(false); got != "false" {
		t.Errorf("ReusedLabel(false) = %q", got)
	}
}
```

`internal/metrics/metrics_internal_test.go`:

```go
func TestNewPrometheusClient_AllMetricsCarryConnReused(t *testing.T) {
	reg := prometheus.NewRegistry()
	p := newPrometheusClient(reg, "localhost", nil)

	p.UpdateTotalHistogram("d", "httptrace", "t", true, 0.1)
	p.UpdateRequestsCounter("d", "httptrace", "t", "200", true)
	p.UpdateErrorsCounter("d", "httptrace", "t", true, errors.New("timeout"))

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	for _, f := range families {
		for _, m := range f.GetMetric() {
			found := false
			for _, l := range m.GetLabel() {
				if l.GetName() == "conn_reused" && l.GetValue() == "true" {
					found = true
				}
			}
			if !found {
				t.Errorf("%s lacks conn_reused=\"true\"", f.GetName())
			}
		}
	}
}
```

`internal/probers/probers_test.go`:

```go
func TestHTTPTrace_ReusedConnectionIsLabelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := probers.NewProberConfig(probers.ProberOptions{
		WG: newTestWG(), PromClient: testPromC, Endpoint: srv.URL,
		Interval: time.Second, Retries: 1, IsOneOff: true, ReuseConnection: true,
	})
	h := probers.NewHTTPTrace(cfg)
	h.Run(context.Background())

	cfg.WG().Add(1) // second one-off run on the same prober reuses the pooled connection
	h.Run(context.Background())

	fresh := counterValue(t, "astrolavos_requests_total", map[string]string{"domain": srv.URL, "conn_reused": "false"})
	reused := counterValue(t, "astrolavos_requests_total", map[string]string{"domain": srv.URL, "conn_reused": "true"})

	if fresh != 1 || reused != 1 {
		t.Errorf("expected 1 fresh + 1 reused request, got fresh=%v reused=%v", fresh, reused)
	}
}

func TestTCP_IsNeverReused(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()

	cfg := probers.NewProberConfig(probers.ProberOptions{
		WG: newTestWG(), PromClient: testPromC, Endpoint: ln.Addr().String(),
		Interval: time.Second, Retries: 1, IsOneOff: true, TCPTimeout: time.Second,
	})
	probers.NewTCP(cfg).Run(context.Background())

	if got := counterValue(t, "astrolavos_requests_total", map[string]string{"domain": ln.Addr().String(), "conn_reused": "false", "prober_type": "tcp"}); got != 1 {
		t.Errorf("expected 1 tcp request with conn_reused=false, got %v", got)
	}
}
```

`WG()` does not exist: add to `probers.go` the accessor `func (p *ProberConfig) WG() *sync.WaitGroup { return p.wg }` (the test needs to re-arm the WaitGroup between runs). Use `import "net"` in the test.

- [ ] **Step 2: Run** `go test -mod=vendor ./internal/...` → FAIL (signature mismatches).

- [ ] **Step 3: Implement**

`metrics.go`: `latencyLabels = []string{"domain", "tag", "prober_type", "conn_reused"}`; requests counter labels `{"domain", "tag", "status_code", "prober_type", "conn_reused"}`; errors counter labels `{"domain", "tag", "error", "prober_type", "conn_reused"}`.

```go
// ReusedLabel renders the conn_reused label value for a probe.
func ReusedLabel(reused bool) string {
	return strconv.FormatBool(reused)
}
```

Each `Update*Histogram(domain, proberType, tag string, reused bool, duration float64)` → `WithLabelValues(domain, tag, proberType, ReusedLabel(reused))`; `UpdateRequestsCounter(domain, proberType, tag, statusCode string, reused bool)` → `WithLabelValues(domain, tag, BucketStatusCode(statusCode), proberType, ReusedLabel(reused))`; `UpdateErrorsCounter(domain, proberType, tag string, reused bool, err error)` → `WithLabelValues(domain, tag, category, proberType, ReusedLabel(reused))`.

`httptrace_point.go`: add `reused bool` to `tracePoint`; `gotConnTimeHandler(info httptrace.GotConnInfo)` sets `t.gotConnTime = time.Now(); t.reused = info.Reused`.

`httptrace.go` `probe`: after the retry loop:

```go
	statusCode, reused := "", false
	if t != nil {
		statusCode, reused = t.statusCode, t.reused
	}

	h.promC.UpdateRequestsCounter(h.endpoint, "httptrace", h.tag, statusCode, reused)
	if err != nil {
		h.promC.UpdateErrorsCounter(h.endpoint, "httptrace", h.tag, reused, err)
	} else {
		h.promC.UpdateDNSHistogram(h.endpoint, "httptrace", h.tag, reused, t.dnsDuration)
		// ...same for Conn, TLS, GotConn, FirstByte, Total
	}
```

`tcp.go`: `UpdateRequestsCounter(t.endpoint, "tcp", t.tag, "", false)` and `UpdateErrorsCounter(t.endpoint, "tcp", t.tag, false, err)`.

- [ ] **Step 4: Run** `go test -mod=vendor -race ./internal/...` → PASS; `make fmt vet lint`.

- [ ] **Step 5: Commit** `feat(metrics): label probe metrics with conn_reused`

---

### Task 3: alert rules

**Files:**
- Modify: `deploy/kubernetes/values.yaml` (two rule entries after `httpNon2xxRatioHigh`; `timeout: 10s` and `reuseConnection: false` on the default endpoint)
- Modify: `deploy/kubernetes/templates/alerts.yaml` (reachability key list)
- Modify: `deploy/kubernetes/templates/_alerts.tpl` (two rule templates)
- Test: `deploy/kubernetes/tests/alerts_test.yaml`

- [ ] **Step 1: Failing tests** — append to `alerts_test.yaml`:

```yaml
  - it: renders the stale-connection and endpoint-stall rules after the HTTP rule
    capabilities:
      apiVersions:
        - monitoring.coreos.com/v1
    set:
      alerts:
        enabled: true
    asserts:
      - lengthEqual:
          path: spec.groups[0].rules
          count: 6
      - equal:
          path: spec.groups[0].rules[4].alert
          value: AstrolavosStaleConnections
      - matchRegex:
          path: spec.groups[0].rules[4].expr
          pattern: 'astrolavos_errors_total\{conn_reused="true", error="timeout"\}\[5m\]'
      - matchRegex:
          path: spec.groups[0].rules[4].expr
          pattern: '> 0\.1\n\)\nunless on \(domain, tag, prober_type\)\n\('
      - matchRegex:
          path: spec.groups[0].rules[4].expr
          pattern: 'astrolavos_errors_total\{conn_reused="false"\}\[5m\]'
      - matchRegex:
          path: spec.groups[0].rules[4].expr
          pattern: '> 0\.1\n\)\n$'
      - equal:
          path: spec.groups[0].rules[4].for
          value: 5m
      - equal:
          path: spec.groups[0].rules[4].labels.severity
          value: critical
      - equal:
          path: spec.groups[0].rules[5].alert
          value: AstrolavosEndpointStalled
      - matchRegex:
          path: spec.groups[0].rules[5].expr
          pattern: 'increase\(astrolavos_requests_total\[1h\]\)\) > 0\n\)\nunless\n\(\n  sum by \(pod, domain, tag, prober_type\) \(increase\(astrolavos_requests_total\[10m\]\)\) > 0'
      - equal:
          path: spec.groups[0].rules[5].labels.severity
          value: warning

  - it: guards the stale-connection rule with the path-wide threshold
    capabilities:
      apiVersions:
        - monitoring.coreos.com/v1
    set:
      alerts:
        enabled: true
        rules:
          probeFailureRatioHigh:
            threshold: 0.3
          staleConnections:
            threshold: 0.05
    asserts:
      - matchRegex:
          path: spec.groups[0].rules[4].expr
          pattern: '> 0\.05\n\)\nunless'
      - matchRegex:
          path: spec.groups[0].rules[4].expr
          pattern: '> 0\.3\n\)\n$'
```

Also update the existing assertion `lengthEqual: spec.groups[0].rules count: 4` → `6` and the "drops a disabled rule" test `count: 3` → `5`.

- [ ] **Step 2: Run** `make helm-test` → FAIL.

- [ ] **Step 3: Implement**

`values.yaml` after `httpNon2xxRatioHigh`:

```yaml
    # -- Timeouts on reused (keep-alive) connections above `threshold` while fresh connections to the same target succeed: established flows are black-holed, e.g. by a CNI/egress-gateway rollout. Needs at least one endpoint with `reuseConnection: true`.
    staleConnections:
      enabled: true
      threshold: 0.1
      for: 5m
      severity: critical
    # -- An endpoint that produced probes in the last hour but none in the last 10m: that prober loop is stuck and the other alerts are blind for it
    endpointStalled:
      enabled: true
      for: 5m
      severity: warning
```

Default endpoint in `values.yaml`: add `reuseConnection: false` and `timeout: 10s` lines (documentation of the knobs).

`alerts.yaml` reachability keys: `(list "probeFailureRatioHigh" "probeFailingOnSingleNode" "dnsResolutionFailing" "httpNon2xxRatioHigh" "staleConnections" "endpointStalled")`.

`_alerts.tpl`, after `httpNon2xxRatioHigh`:

```
{{- define "astrolavos.alerts.rule.staleConnections" -}}
{{- $r := .rule -}}
{{- $fleet := required "alerts.rules.probeFailureRatioHigh.threshold is required" .context.Values.alerts.rules.probeFailureRatioHigh.threshold -}}
{{- $threshold := required "alerts.rules.staleConnections.threshold is required" $r.threshold -}}
- alert: AstrolavosStaleConnections
  expr: |
    (
      sum by (domain, tag, prober_type) (rate(astrolavos_errors_total{conn_reused="true", error="timeout"}[5m]))
        /
      sum by (domain, tag, prober_type) (rate(astrolavos_requests_total{conn_reused="true"}[5m]))
        > {{ $threshold }}
    )
    unless on (domain, tag, prober_type)
    (
      sum by (domain, tag, prober_type) (rate(astrolavos_errors_total{conn_reused="false"}[5m]))
        /
      sum by (domain, tag, prober_type) (rate(astrolavos_requests_total{conn_reused="false"}[5m]))
        > {{ $fleet }}
    )
  for: {{ required "alerts.rules.staleConnections.for is required" $r.for }}
  labels: {{- include "astrolavos.alerts.labels" . | nindent 4 }}
  annotations:
    summary: >-
      established connections to {{ `{{ $labels.domain }}` }} ({{ `{{ $labels.tag }}` }}) are stale:
      {{ `{{ $value | humanizePercentage }}` }} of keep-alive probes time out
    description: |
      More than {{ include "astrolavos.alerts.percent" $threshold }} of probes that reuse an existing
      connection to {{ `{{ $labels.domain }}` }} time out, while fresh connections to the same
      target succeed. Existing flows are being black-holed (packets dropped
      without a reset) and only new connections get a working path. This is
      the signature of a CNI or egress-gateway rollout (conntrack/NAT state
      lost, gateway reselected mid-flow): workloads holding long-lived
      connections hang until their own timeouts fire.

      Investigate: what changed on the egress path in the last 15m (CNI agent
      rollout, egress gateway node change, NAT gateway), per node
        sum by (pod) (rate(astrolavos_errors_total{domain="{{ `{{ $labels.domain }}` }}", conn_reused="true", error="timeout"}[5m]))
      and TCP retransmits on the same nodes (node_netstat_Tcp_RetransSegs).
      Restarting affected workloads re-establishes their connections.
{{- end -}}

{{- define "astrolavos.alerts.rule.endpointStalled" -}}
{{- $r := .rule -}}
- alert: AstrolavosEndpointStalled
  expr: |
    (
      sum by (pod, domain, tag, prober_type) (increase(astrolavos_requests_total[1h])) > 0
    )
    unless
    (
      sum by (pod, domain, tag, prober_type) (increase(astrolavos_requests_total[10m])) > 0
    )
  for: {{ required "alerts.rules.endpointStalled.for is required" $r.for }}
  labels: {{- include "astrolavos.alerts.labels" . | nindent 4 }}
  annotations:
    summary: >-
      {{ `{{ $labels.pod }}` }} stopped probing {{ `{{ $labels.domain }}` }} ({{ `{{ $labels.tag }}` }})
    description: |
      {{ `{{ $labels.pod }}` }} recorded probes to {{ `{{ $labels.domain }}` }} within the last hour
      but none in the last 10 minutes, while the pod is still scraped. The
      prober loop for this endpoint is stuck, so failure and latency alerts
      are blind for it. Every attempt is bounded by the endpoint `timeout`,
      so this should not happen; treat it as a bug and capture the logs
      before restarting:
        kubectl logs -n <namespace> {{ `{{ $labels.pod }}` }}
{{- end -}}
```

- [ ] **Step 4: Run** `make helm-test`, `helm lint --strict deploy/kubernetes` → PASS. Validate PromQL: render with `--set alerts.enabled=true`, `yq .spec` to `/tmp`, `docker run --rm -v /tmp/x:/r:ro --entrypoint promtool prom/prometheus:latest check rules /r/rules.yaml`, then remove `/tmp/x`.

- [ ] **Step 5: Commit** `feat(helm): alert on stale keep-alive connections and stalled endpoints`

---

### Task 4: dashboards

**Files:**
- Modify: `deploy/kubernetes/dashboard/grafana.json`, `deploy/kubernetes/dashboard/datadog-widgets.json`
- Modify: `deploy/kubernetes/templates/dashboard-datadog.yaml`
- Test: `deploy/kubernetes/tests/dashboards_test.yaml`

- [ ] **Step 1: Failing tests** — in `dashboards_test.yaml`, Grafana test adds:

```yaml
      - matchRegex:
          path: data["astrolavos-dashboard.json"]
          pattern: '"name": "keepalive"'
      - matchRegex:
          path: data["astrolavos-dashboard.json"]
          pattern: 'conn_reused=~\\"\$keepalive\\"'
```

Datadog test: `lengthEqual spec.templateVariables count: 4` → `5`, add

```yaml
      - contains:
          path: spec.templateVariables
          content:
            name: conn_reused
            prefix: conn_reused
            defaults: ["*"]
      - matchRegex:
          path: spec.widgets
          pattern: '\{\$tag,\$endpoint,\$conn_reused\}'
```

and the "templates the Datadog title" test `count: 3` → `4`, `spec.templateVariables[2]` → `[3]`.

- [ ] **Step 2: Run** `make helm-test` → FAIL.

- [ ] **Step 3: Implement**

Grafana, with jq (then verify the diff is only the intended lines):

```bash
cd deploy/kubernetes/dashboard
jq '
  .templating.list += [{
    "name": "keepalive", "label": "Connection", "type": "custom",
    "query": "All : .*,Keep-alive : true,New : false",
    "current": {"selected": true, "text": "All", "value": ".*"},
    "options": [
      {"selected": true,  "text": "All",        "value": ".*"},
      {"selected": false, "text": "Keep-alive", "value": "true"},
      {"selected": false, "text": "New",        "value": "false"}
    ],
    "includeAll": false, "multi": false, "hide": 0
  }]
  | (.. | objects | select(has("expr")) | .expr)
      |= gsub("domain=~\"\\$endpoint\"}"; "domain=~\"$endpoint\", conn_reused=~\"$keepalive\"}")
' grafana.json > grafana.json.new && mv grafana.json.new grafana.json
jq '(.. | objects | select(has("query")) | .query) |= gsub("\\{\\$tag,\\$endpoint\\}"; "{$tag,$endpoint,$conn_reused}")' datadog-widgets.json > d.new && mv d.new datadog-widgets.json
```

Check that `git diff --stat` shows only expr/query lines plus the new variable; if jq reformatted the whole file (indentation), re-run with `--indent 2` to match the existing style.

`dashboard-datadog.yaml` after the `endpoint` variable:

```yaml
    - name: conn_reused
      prefix: conn_reused
      defaults:
        - "*"
```

- [ ] **Step 4: Run** `make helm-test` → PASS; `jq . deploy/kubernetes/dashboard/*.json > /dev/null` (valid JSON).

- [ ] **Step 5: Commit** `feat(dashboards): filter by connection kind (conn_reused)`

---

### Task 5: docs, CI, PR

**Files:**
- Modify: `README.md`, `AGENTS.md`, `examples/config.yaml`, `deploy/kubernetes/README.md` (helm-docs)

- [ ] **Step 1: README** — in the endpoint field list after `retries`:

```markdown
- `reuseConnection`: keep the HTTP connection open between probes (`httpTrace` only). Default `false`, i.e. every probe opens a new connection. Set it to `true` on at least one external endpoint to detect stale connections (see below).
- `timeout`: deadline for a single probe attempt (DNS, connect, TLS, request and body for `httpTrace`; the dial for `tcp`). Default `10s`. Each retry gets its own deadline. A probe that hits a black-holed connection fails with `error="timeout"` instead of hanging.
```

New section after "Intelligent Retry Logic":

```markdown
### Detecting Stale Connections
A CNI or egress-gateway rollout can silently break *established* connections while new ones keep working: packets of existing flows are dropped without a reset and the application hangs until its own timeout. A prober that opens a fresh connection every time never sees this. Set `reuseConnection: true` on an endpoint that crosses the path you care about (for example your egress target) and astrolavos will keep the connection between probes; every metric carries a `conn_reused="true|false"` label, so a probe that times out on a reused connection while fresh connections succeed is exactly the signature of that failure. The Helm chart ships the matching `AstrolavosStaleConnections` alert.
```

Versioning section: add a bullet "label additions are backwards compatible; `conn_reused` was added in 1.5".

- [ ] **Step 2: `examples/config.yaml`** — on the first endpoint add `timeout: 5s` with a comment `# deadline per attempt (default 10s)`.

- [ ] **Step 3: AGENTS.md** — in Code Style, extend the CategorizeError bullet: "Every probe metric carries `conn_reused` (`metrics.ReusedLabel`); pass the value through, never recompute it."

- [ ] **Step 4: `make helm-docs`**, then `make ci` and `make helm-test`. Expected: all green (vulncheck needs network).

- [ ] **Step 5: Commit** `docs: document timeout, reuseConnection and stale-connection detection`

- [ ] **Step 6: Push and open the PR** against `feat/chart-alerting` with label `release:minor`, as `dntosas` (see the previous PR for the `GH_TOKEN` approach); note in the body that it must be retargeted to `main` once #116 merges.

---

## Self-review

- **Spec coverage:** config/plumbing (T1), timeout bound per attempt for both probers (T1), `conn_reused` on all metrics incl. histograms with `ReusedLabel` (T2), both rules with thresholds/guards (T3), Grafana variable + Datadog variable/widgets (T4), docs incl. versioning note and examples (T5), release label and stacking (T5).
- **Placeholders:** none.
- **Type consistency:** `Timeout` field name identical across `YamlEndpoint`, `model.Endpoint`, `ProberOptions`; `reused bool` parameter position (before `duration`/`err`, after `statusCode`) identical in T2 implementation and tests; `counterValue` helper defined in T1 and reused in T2.
