# Stale-connection detection, part 1: probe timeouts and `conn_reused`

**Date:** 2026-10-20
**Status:** approved design
**Series:** part 1 of 3 (part 2: `stream` prober with TCP/WebSocket echo and
toxiproxy e2e; part 3, in the infrastructure repo: external echo target,
corroborating node/Cilium alerts, Cilium-rollout rehearsal runbook).

## Problem

During Cilium rollouts, workloads with long-lived connections through the
egress path go stale: established flows are black-holed (no RST, packets
silently dropped) while new connections keep working. Astrolavos does not
catch this today for two reasons found in the code:

1. **Every probe is a new connection.** `getCustomClient` sets
   `MaxIdleConnsPerHost = -1` unless `reuseConnection: true`, and the `tcp`
   prober dials and closes. A prober that only asks "can I open a connection
   now?" cannot see that existing flows died.
2. **The `httpTrace` prober has no timeout.** `trace()` uses the app-lifetime
   context and `http.Client` has no `Timeout`; only the `tcp` prober has a
   dial timeout (`tcpTimeout`). A probe that lands on a black-holed connection
   hangs until the kernel gives up (~15 min), and because `runLoop` runs
   `probe()` synchronously the endpoint's loop stops: no request, no error,
   nothing moves. Silence instead of a signal. `httptrace.GotConnInfo.Reused`
   is received and discarded, so even with reuse there is no way to tell
   "an existing flow failed" from "a new connect failed".

## Goals

- A hung connection becomes `error="timeout"` within seconds for both probers.
- Metrics distinguish probes on reused connections from probes on fresh ones.
- Alerts fire specifically when reused connections time out while fresh
  connections succeed (the Cilium-rollout signature), and when a single
  endpoint's loop stops producing probes.
- Dashboards can filter by connection kind.

Non-goals (parts 2/3): a dedicated long-lived stream prober, echo servers,
e2e black-hole simulation, infrastructure wiring, node/Cilium alerts.

## Config

New per-endpoint key:

```yaml
endpoints:
  - domain: one.one.one.one
    https: true
    reuseConnection: true   # keep the connection between probes
    timeout: 10s            # deadline for one probe attempt (default 10s)
```

- `timeout` (duration, default `10s`) is the deadline of **one attempt**:
  DNS + connect + TLS + request + body for `httpTrace`; the dial for `tcp`.
  Applied with `context.WithTimeout` inside the retry loop, so with
  `retries: 3` each attempt gets its own deadline.
- Validation: `timeout` must be greater than zero when set.
- `tcpTimeout` is unchanged (the `net.Dialer.Timeout` of the `tcp` prober);
  the attempt deadline applies on top of it.
- `reuseConnection` becomes documented (it exists but is not in the README).

Plumbing: `config.YamlEndpoint.Timeout *time.Duration` →
`model.Endpoint.Timeout` → `probers.ProberOptions.Timeout` →
`ProberConfig.timeout`. `NewProberConfig` falls back to 10s when zero so
direct callers (tests) keep working.

## Metrics

A `conn_reused` label (`"true"`/`"false"`) is added to **all** probe metrics:
`astrolavos_requests_total`, `astrolavos_errors_total` and the six
`astrolavos_*_latency_seconds` histograms.

- Source: `httptrace.GotConnInfo.Reused`, captured in `tracePoint.reused`.
- `tcp` probes are always `"false"`.
- An `httpTrace` failure before a connection was obtained is `"false"`.
- Histograms get the label too because a reused connection reports
  `dns = conn = tls = 0`; mixing them with fresh-connection samples skews
  every quantile downward whenever `reuseConnection: true`. The label makes
  both views correct and lets the dashboards filter.
- Cardinality: at most 2× series per endpoint, and only for endpoints with
  `reuseConnection: true`.
- This is an additive label-set change within the 1.x compatibility promise;
  it is called out in the release notes and the README "Versioning" section
  (`sum by (domain, tag)` queries are unaffected).

`metrics.PrometheusClient` methods gain a `reused bool` parameter;
`metrics.ReusedLabel(bool) string` renders it.

## Alerts (chart)

Two new rules in group `astrolavos.reachability`, rendered by the existing
`_alerts.tpl` machinery from [#116](https://github.com/dntosas/astrolavos/pull/116).

**AstrolavosStaleConnections** (`alerts.rules.staleConnections`,
default `threshold: 0.1`, `for: 5m`, `severity: critical`)

```promql
(
  sum by (domain, tag, prober_type) (rate(astrolavos_errors_total{conn_reused="true", error="timeout"}[5m]))
    /
  sum by (domain, tag, prober_type) (rate(astrolavos_requests_total{conn_reused="true"}[5m]))
    > 0.1
)
unless on (domain, tag, prober_type)
(
  sum by (domain, tag, prober_type) (rate(astrolavos_errors_total{conn_reused="false"}[5m]))
    /
  sum by (domain, tag, prober_type) (rate(astrolavos_requests_total{conn_reused="false"}[5m]))
    > <alerts.rules.probeFailureRatioHigh.threshold>
)
```

Reads: "probes riding existing connections time out while fresh connections
to the same target succeed". If fresh connections fail too it is a path-wide
outage and `AstrolavosProbeFailureRatioHigh` owns it. Only `error="timeout"`
is counted: a peer closing an idle keep-alive connection surfaces as
`eof`/`connection_reset` and Go's transport retries idempotent requests on a
new connection, so those are noise, not black holes. Requires at least one
endpoint with `reuseConnection: true`; the description says so.

**AstrolavosEndpointStalled** (`alerts.rules.endpointStalled`, default
`for: 5m`, `severity: warning`)

```promql
(
  sum by (pod, domain, tag, prober_type) (increase(astrolavos_requests_total[1h])) > 0
)
unless
(
  sum by (pod, domain, tag, prober_type) (increase(astrolavos_requests_total[10m])) > 0
)
```

An endpoint that produced probes in the last hour but none in the last 10
minutes. With the new timeouts this should not happen; if it does, the
prober is stuck and the other alerts are blind for that endpoint.
Complements `AstrolavosProbeStalled`, which only fires when a whole pod
stops.

## Dashboards

- **Grafana** (`deploy/kubernetes/dashboard/grafana.json`): new custom
  variable `keepalive` (label "Connection") with options
  `All : .*`, `Keep-alive : true`, `New : false`, default `All`. Every panel
  query gains `conn_reused=~"$keepalive"` next to the existing
  `tag=~"$tag", domain=~"$endpoint"` matchers.
- **Datadog** (`templates/dashboard-datadog.yaml` + `dashboard/datadog-widgets.json`):
  new template variable `conn_reused` (prefix `conn_reused`, default `*`);
  every widget query filter becomes `{$tag,$endpoint,$conn_reused}`.

## Tests

- `internal/config`: `timeout` default, custom value, zero/negative rejected.
- `internal/probers`: a black-holing `httptest` server (hijacks the
  connection and never writes) makes a `timeout: 200ms` probe return within
  ~2× the timeout with category `timeout`; a keep-alive server probed twice
  with `reuseConnection: true` records the second request with
  `conn_reused="true"` and the first with `"false"`; `tcp` records
  `conn_reused="false"`. Assertions gather `prometheus.DefaultGatherer` and
  match series by `domain` label.
- `internal/metrics`: `ReusedLabel`; label set of every collector includes
  `conn_reused`.
- Chart: `alerts_test.yaml` pins the two new rules (names, order after
  `httpNon2xxRatioHigh`, threshold/for/severity, `unless on` guard, `increase`
  windows); `dashboards_test.yaml` asserts the Grafana variable and the
  `conn_reused` matcher, and the Datadog variable count (5) and widget filter.
- `make ci` (fmt, vet, lint, test, vulncheck) and `make helm-test`.

## Docs

- README: endpoint fields `reuseConnection` and `timeout`; a "Detecting
  stale connections" paragraph (why `reuseConnection: true` on the egress
  endpoint matters, which alert fires); Versioning note on the added label.
- Chart `values.yaml` default endpoint gains `timeout: 10s` as documentation;
  rule values with `# --` comments; `make helm-docs`.
- `examples/config.yaml`: `timeout` on one endpoint.
- AGENTS.md: metrics label list mentions `conn_reused`; "do not add raw
  strings as labels" rule unchanged.

## Release

Additive: `release:minor`. Branch `feat/stale-connections` is stacked on
`feat/chart-alerting` (#116) because the rules use `_alerts.tpl`; the PR is
opened against that branch and retargeted to `main` once #116 merges.
