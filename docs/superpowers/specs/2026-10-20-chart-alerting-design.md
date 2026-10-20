# Helm chart alerting: PrometheusRule / VMRule

**Date:** 2026-10-20
**Status:** approved design, pending implementation plan

## Goal

Ship a curated set of alerting rules with the astrolavos Helm chart so that an
operator who already scrapes the prober gets actionable network alerts by
flipping one value. The operator chooses the rule CRD their stack understands:
Prometheus Operator (`PrometheusRule`) or VictoriaMetrics operator (`VMRule`).

Non-goals: alert routing (Alertmanager/vmalert config), notification
templates, runbook hosting, and enabling the rules in any downstream
deployment repository. The chart ships the rules; wiring is the operator's.

## What astrolavos exposes (the signal)

From `internal/metrics/metrics.go` and `internal/probers/`:

| Metric | Labels | Semantics |
|---|---|---|
| `astrolavos_requests_total` | `domain, tag, status_code, prober_type` | Incremented on **every** probe, success or failure. `status_code` is bucketed (`2xx`…`5xx`); empty for `tcp` probes and for failed `httptrace` probes. This is the denominator for every ratio. |
| `astrolavos_errors_total` | `domain, tag, error, prober_type` | Incremented once per failed probe (after retries). `error` is one of `dns_error`, `connection_refused`, `connection_reset`, `timeout`, `tls_error`, `eof`, `canceled`, `unknown`. |
| `astrolavos_{dns,conn,tls,gotconn,firstbyte,total}_latency_seconds` | `domain, tag, prober_type` (+ `le`) | Histograms recorded **only on success** and **only by `httptrace` probes** (`tcp` records counters only, see `internal/probers/tcp.go`). Latency alerts are therefore never skewed by failed probes and never fire for tcp targets. |
| `up` | `job, pod, …` | Scrape health. The chart's ServiceMonitor gives `job` = Service name (`fullname`, `astrolavos` by default). |

`prober_type` is `httptrace` or `tcp` (lowercase, set by the probers, distinct
from the `prober: httpTrace` config spelling).

Because the chart deploys a DaemonSet by default, the scrape `pod` label maps
1:1 to a node. That lets a rule distinguish "this network path is broken" from
"this node's network is broken", which no passive exporter can do.

## Values API

New top-level `alerts` block in `deploy/kubernetes/values.yaml`, placed after
`serviceMonitor`. Every key carries a `# --` helm-docs comment.

```yaml
alerts:
  # -- Create a PrometheusRule or VMRule with the built-in alerting rules.
  enabled: false
  # -- Rule CRD to render: `PrometheusRule` (monitoring.coreos.com/v1) or `VMRule` (operator.victoriametrics.com/v1beta1). Any other value fails the render.
  kind: PrometheusRule
  # -- Namespace for the rule resource (defaults to the release namespace).
  namespace: ""
  # -- Extra labels on the rule resource, e.g. `release: kube-prometheus-stack` so the operator's ruleSelector picks it up.
  additionalLabels: {}
  # -- Extra annotations on the rule resource.
  annotations: {}
  # -- Labels merged into every alert (e.g. `team`, `env`). `severity` is set per rule and wins on conflict.
  labels: {}
  # -- Rule group evaluation interval. Omitted when empty (operator default).
  interval: ""
  # -- Value of the `job` label on astrolavos scrape targets, used by the self-health rules. Defaults to the chart fullname, which is what the chart's ServiceMonitor produces.
  job: ""
  rules:
    probeFailureRatioHigh:     {enabled: true,  threshold: 0.1,  for: 10m, severity: critical}
    probeFailingOnSingleNode:  {enabled: true,  threshold: 0.5,  for: 10m, severity: warning}
    dnsResolutionFailing:      {enabled: true,  threshold: 0.05, for: 5m,  severity: critical}
    httpNon2xxRatioHigh:       {enabled: true,  threshold: 0.1,  for: 10m, severity: warning}
    tcpConnectLatencyHigh:     {enabled: true,  quantile: 0.99, threshold: 0.1, for: 15m, severity: warning}
    dnsLatencyHigh:            {enabled: true,  quantile: 0.99, threshold: 0.1, for: 15m, severity: warning}
    tlsHandshakeLatencyHigh:   {enabled: true,  quantile: 0.99, threshold: 0.5, for: 15m, severity: warning}
    totalLatencyHigh:          {enabled: true,  quantile: 0.99, threshold: 1,   for: 15m, severity: warning}
    connectLatencyRegression:  {enabled: false, quantile: 0.99, factor: 3, baseline: 1d, floor: 0.005, for: 30m, severity: warning}
    probeStalled:              {enabled: true,  for: 10m, severity: warning}
    targetsDown:               {enabled: true,  threshold: 0.2,  for: 10m, severity: critical}
  # -- Per-target overrides for the latency rules. Each entry re-renders the latency rules with `selector` baked into every PromQL selector; the global latency rules exclude the matched series. Keys other than `name`/`selector` are rule names from `alerts.rules` whose `threshold`/`for`/`severity` override the global ones.
  latencyOverrides: []
    # - name: internal
    #   selector: 'tag="internal"'
    #   tcpConnectLatencyHigh: {threshold: 0.01}
    #   totalLatencyHigh:      {threshold: 0.05, for: 5m}
  # -- Raw Prometheus/vmalert rules appended verbatim to an `astrolavos.custom` group. Rendered through `tpl`.
  additionalRules: []
```

Thresholds are in the natural unit of the metric: ratios as fractions
(`0.1` = 10%), latencies in seconds. `for` strings are passed through as-is.

Rule names in `alerts.rules` are camelCase; the rendered `alert:` name is
`Astrolavos` + the key in PascalCase (e.g. `probeFailureRatioHigh` →
`AstrolavosProbeFailureRatioHigh`). The mapping is hardcoded per rule in the
template, not computed, so helm-unittest assertions can pin exact names.

### Latency overrides

`latencyOverrides[*].selector` is a raw PromQL label-matcher string without
braces (`tag="internal"`, `domain=~".*\.svc"`). It is inserted as
`{<selector>}` into the rule's selectors. Each
override renders its own copy of `tcpConnectLatencyHigh`, `dnsLatencyHigh`,
`tlsHandshakeLatencyHigh`, `totalLatencyHigh` and, when enabled,
`connectLatencyRegression`, with an extra `override: <name>` label. A rule
disabled globally is not rendered for any override.

Each global latency rule carries one clause per override:

```promql
unless on (domain, tag, prober_type)
  sum by (domain, tag, prober_type) (rate(astrolavos_requests_total{<selector>}[15m]))
```

so every series is evaluated by exactly one copy. The `unless` form works for
any matcher shape (no need to negate `=~`), which is why the selector is a
free string rather than a structured map.

Alert names are identical across copies. Prometheus and vmalert allow repeated
`alert:` names in one group; `domain`/`tag`/`override` labels disambiguate the
firing alerts.

## Rules

All ratio rules use a 10m rate window unless stated. All latency rules use a
15m window (an endpoint probed every 10s yields ~90 samples per pod per
window, enough for a stable p99 at the `(domain, tag)` aggregate). `R` below
is the rule's config from `alerts.rules`, `S` is the extra selector (empty
for the global copy) and `X` the `unless` exclusions (empty for override
copies).

### Group `astrolavos.reachability`

**AstrolavosProbeFailureRatioHigh** (`probeFailureRatioHigh`)
```promql
sum by (domain, tag, prober_type) (rate(astrolavos_errors_total[10m]))
  /
sum by (domain, tag, prober_type) (rate(astrolavos_requests_total[10m]))
  > R.threshold
```
The core "is the path up" signal. Description points at
`sum by (error) (rate(astrolavos_errors_total{domain="…"}[10m]))` to tell DNS
from timeouts from resets.

**AstrolavosProbeFailingOnSingleNode** (`probeFailingOnSingleNode`)
```promql
(
  sum by (pod, domain, tag, prober_type) (rate(astrolavos_errors_total[10m]))
    /
  sum by (pod, domain, tag, prober_type) (rate(astrolavos_requests_total[10m]))
  > R.threshold
)
and on (domain, tag, prober_type)
(
  sum by (domain, tag, prober_type) (rate(astrolavos_errors_total[10m]))
    /
  sum by (domain, tag, prober_type) (rate(astrolavos_requests_total[10m]))
  < rules.probeFailureRatioHigh.threshold
)
```
Fires per `pod` only while the fleet-wide ratio is below the path-wide alert's
threshold, so a real outage raises one critical alert, not one per node. Only
meaningful with `deployAsDaemonSet: true`; the description says so. Uses the
`probeFailureRatioHigh` threshold even when that rule is disabled.

**AstrolavosDNSResolutionFailing** (`dnsResolutionFailing`)
```promql
sum by (domain, tag, prober_type) (rate(astrolavos_errors_total{error="dns_error"}[5m]))
  /
sum by (domain, tag, prober_type) (rate(astrolavos_requests_total[5m]))
  > R.threshold
```
Client-side DNS failure; different responder than connectivity. 5m window
because DNS outages are sharp.

**AstrolavosHTTPNon2xxRatioHigh** (`httpNon2xxRatioHigh`)
```promql
sum by (domain, tag) (rate(astrolavos_requests_total{prober_type="httptrace", status_code!~"2xx|3xx|"}[10m]))
  /
sum by (domain, tag) (rate(astrolavos_requests_total{prober_type="httptrace"}[10m]))
  > R.threshold
```
Reachable but unhealthy (5xx from the peer, 403 from a proxy/WAF). Failed
probes (empty `status_code`) are excluded from the numerator so this does not
double-fire with `ProbeFailureRatioHigh`.

### Group `astrolavos.latency`

Shared shape, with `M` = metric (`conn`, `dns`, `tls`, `total`):
```promql
histogram_quantile(R.quantile,
  sum by (domain, tag, prober_type, le) (rate(astrolavos_M_latency_seconds_bucket{S}[15m]))
) > R.threshold
X
```

| Alert | Metric | Meaning |
|---|---|---|
| `AstrolavosTCPConnectLatencyHigh` | `conn` | RTT + SYN retransmits, independent of the application. Cleanest "network got slow or lossy" signal. |
| `AstrolavosDNSLatencyHigh` | `dns` | Client view of the resolver chain. |
| `AstrolavosTLSHandshakeLatencyHigh` | `tls` | Node CPU starvation or an egress middlebox terminating TLS. |
| `AstrolavosTotalLatencyHigh` | `total` | End-to-end catch-all. |

All four histograms are populated only by `httptrace` probes, and `tls` only
when `https: true`; the rules do not special-case this because a series that
never exists never fires.

**AstrolavosConnectLatencyRegression** (`connectLatencyRegression`, off by default)
```promql
(
  histogram_quantile(R.quantile, sum by (domain, tag, prober_type, le) (rate(astrolavos_conn_latency_seconds_bucket{S}[15m])))
  > R.factor *
  histogram_quantile(R.quantile, sum by (domain, tag, prober_type, le) (rate(astrolavos_conn_latency_seconds_bucket{S}[15m] offset R.baseline)))
)
and
histogram_quantile(R.quantile, sum by (domain, tag, prober_type, le) (rate(astrolavos_conn_latency_seconds_bucket{S}[15m])))
  > R.floor
X
```
Catches "traffic silently went cross-AZ" (e.g. `trafficDistribution` no
longer honoured): 1ms → 3ms never trips an absolute threshold. `floor` stops
sub-millisecond noise from firing. Off by default because it needs `baseline`
of history and is opinionated.

### Group `astrolavos.selfhealth`

Both use `J` = `alerts.job` or the chart fullname.

**AstrolavosProbeStalled** (`probeStalled`)
```promql
up{job="J"} == 1
unless on (pod)
sum by (pod) (rate(astrolavos_requests_total[10m])) > 0
```
Pod is scraped but has produced no probe at all for 10m: stuck goroutines or
an empty endpoints list. Per pod, not per endpoint, because a disappearing
`(pod, domain, tag)` series cannot be detected without knowing the config.

**AstrolavosTargetsDown** (`targetsDown`)
```promql
count(up{job="J"} == 0) / count(up{job="J"}) > R.threshold
or
absent(up{job="J"})
```
Watchdog for the prober itself so silence is not mistaken for health. The
`absent` arm covers the case where the scrape targets vanish entirely (the
DaemonSet or ServiceMonitor was deleted), which a ratio over zero targets
cannot express.

### Labels and annotations

Every rule gets `labels: {severity: R.severity} + alerts.labels` (rule-level
`severity` wins) and, on override copies, `override: <name>`. Annotations
are `summary` (one line, uses `{{ $labels.domain }}`/`{{ $labels.tag }}` and
`{{ $labels.pod }}` where relevant) and `description` (what it means, what
baseline to expect, one or two PromQL hints to investigate). Go template
delimiters inside annotations are emitted via `{{ \`{{ $labels.x }}\` }}` as in
the rest of the ecosystem.

## Template layout

- `deploy/kubernetes/templates/alerts.yaml`: the resource. Picks
  `apiVersion`/`kind` from `alerts.kind`; `fail`s on an unknown kind. Gated on
  `.Capabilities.APIVersions.Has` for the chosen group
  (`monitoring.coreos.com/v1` or `operator.victoriametrics.com/v1beta1`),
  mirroring `servicemonitor.yaml`. `metadata.name` is the chart fullname,
  namespace from `alerts.namespace`, labels `common.labels.standard` +
  `additionalLabels` + `commonLabels`, annotations `annotations` +
  `commonAnnotations`. Groups are emitted only when they contain at least
  one rule; `interval` is set on each group when non-empty.
- `deploy/kubernetes/templates/_alerts.tpl`: one named template per rule
  (`astrolavos.alerts.rule.<name>`) taking a dict `{"context": $, "rule":
  <merged R>, "selector": S, "exclusions": X, "override": name}` and returning
  the YAML for a single list item. Latency rules share a helper that renders
  the `histogram_quantile(...)` expression for a metric. The override loop in
  `alerts.yaml` merges the override entry over `alerts.rules.<name>` with
  `mergeOverwrite` and calls the same templates.
- `rules` entries and `latencyOverrides` are validated with `required`/`fail`
  for the fields each rule needs (`threshold`, `for`, `severity`; `quantile`
  on latency rules; `factor`/`baseline`/`floor` on the regression rule).

The spec of `PrometheusRule` and `VMRule` is identical for what we emit
(`groups[].name/interval/rules[].alert/expr/for/labels/annotations`), so one
template body serves both kinds.

## Tests

`deploy/kubernetes/tests/alerts_test.yaml`, following the conventions in
`AGENTS.md` (release `astrolavos`/`monitoring`, `capabilities.apiVersions`
declared per test, exact `equal` on sub-objects):

1. Disabled by default → `hasDocuments: 0`.
2. Enabled but CRD absent from capabilities → `hasDocuments: 0` (both kinds).
3. Default PrometheusRule: kind, apiVersion, name `astrolavos`, namespace
   `monitoring`, standard labels, three groups in order, the ten enabled
   alert names present, `AstrolavosConnectLatencyRegression` absent, no
   `interval`.
4. `kind: VMRule` → `operator.victoriametrics.com/v1beta1` / `VMRule`.
5. `kind: Foo` → `failedTemplate`.
6. Threshold/for/severity flow: set custom values on `probeFailureRatioHigh`
   and assert `expr` contains `> 0.25`, `for: 5m`, `labels.severity: page`.
7. Disabling every rule in a group drops that group. Disabling all rules with
   no `additionalRules` renders no document at all (nothing to alert on).
8. `alerts.labels` merged, `severity` not overridden by it.
9. `job` default and override appear in the self-health exprs.
10. `latencyOverrides`: one entry → the latency group has two copies of each
    latency rule; the override copy's `expr` contains `tag="internal"` and
    `labels.override: internal`; the global copy contains `unless on (domain,
    tag, prober_type)` and the selector; per-entry `threshold` and `for` win
    over the global rule; the regression rule is not duplicated while
    disabled.
11. `additionalRules` appended as an `astrolavos.custom` group, with `tpl`
    applied (`{{ .Release.Name }}` resolved).
12. `interval`, `namespace`, `additionalLabels`, `annotations` rendered.

Rendered output for the default values, for `kind: VMRule`, and for a
`latencyOverrides` example is also checked locally with `promtool check
rules` and `vmalert -rule=… -dryRun` before the PR; this is not added to CI.

## Docs

- `make helm-docs` regenerates `deploy/kubernetes/README.md` from the `# --`
  comments.
- Root `README.md`, "Deploy On Kubernetes": one sentence that the chart can
  ship alerting rules as `PrometheusRule` or `VMRule` (`alerts.enabled`,
  `alerts.kind`) with a pointer to the chart README.
- `AGENTS.md`, "Helm Chart": a bullet describing `_alerts.tpl` (one named
  template per rule, overrides re-render the latency rules) and that the
  alert name ↔ values key mapping is hardcoded.

The chart version and `image.tag` are not touched; the release workflow bumps
them from the PR label (`release:minor`, additive).

## Downstream (out of scope, for reference)

Once the chart is released, `infrastructure/k8s/common/monitoring/values.yaml`
would enable it with:

```yaml
astrolavos:
  alerts:
    enabled: true
    kind: VMRule
    labels: {team: infrastructure}
    latencyOverrides:
      - name: internal
        selector: 'tag="internal"'
        tcpConnectLatencyHigh: {threshold: 0.01}
        totalLatencyHigh: {threshold: 0.05}
```

leaving the global thresholds for the `egress` path.
