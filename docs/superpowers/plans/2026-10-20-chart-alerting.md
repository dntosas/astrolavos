# Chart Alerting (PrometheusRule / VMRule) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship 11 curated alerting rules with the Helm chart, rendered as a `PrometheusRule` or `VMRule` chosen by `alerts.kind`, with per-target latency overrides.

**Architecture:** One resource template (`templates/alerts.yaml`) picks apiVersion/kind from `alerts.kind` and loops over three rule groups; every rule body lives in its own named template in `templates/_alerts.tpl` so the `latencyOverrides` loop can re-render the latency rules with a different selector and thresholds. helm-unittest pins the rendering; promtool/vmalert validate the PromQL of the rendered output locally.

**Tech Stack:** Helm 4, Bitnami `common` 2.41 helpers, helm-unittest 1.0.3 (`make helm-test`), helm-docs 1.11 (`make helm-docs`), promtool + vmalert for PromQL validation.

Spec: `docs/superpowers/specs/2026-10-20-chart-alerting-design.md`.

## Global Constraints

- Chart conventions from `AGENTS.md`: every template change needs a test; pin default rendering, each switch, and the disabled → `hasDocuments: count: 0` case. Suites use `release: {name: astrolavos, namespace: monitoring}`. Declare `capabilities.apiVersions` per test.
- Rendering is gated on `.Capabilities.APIVersions.Has` for the chosen kind's group: `monitoring.coreos.com/v1` (PrometheusRule) or `operator.victoriametrics.com/v1beta1` (VMRule).
- Do not bump `Chart.yaml` `version`/`appVersion` or `values.yaml` `image.tag`; the release workflow does that from the PR label (`release:minor`).
- Thresholds: ratios are fractions (`0.1` = 10%), latencies in seconds. Alert names are hardcoded per rule: `Astrolavos` + PascalCase(values key).
- Go-template delimiters meant for Prometheus/vmalert are emitted with `{{ `{{ $labels.x }}` }}`.
- Every values key gets a `# --` helm-docs comment; rule objects get one comment on the object (helm-docs then documents the object, not its leaves).
- Commits follow Conventional Commits.
- Work on branch `feat/chart-alerting` (already created, contains the spec).

---

### Task 1: Resource skeleton + reachability group

**Files:**
- Modify: `deploy/kubernetes/values.yaml` (append `alerts:` block after `serviceMonitor`, before `extraVolumes`)
- Create: `deploy/kubernetes/templates/_alerts.tpl`
- Create: `deploy/kubernetes/templates/alerts.yaml`
- Create: `deploy/kubernetes/tests/alerts_test.yaml`
- Modify: `deploy/kubernetes/tests/metadata_test.yaml` (add the template + enable alerts)

**Interfaces:**
- Produces named templates used by Tasks 2–3:
  - `astrolavos.alerts.apiVersion` (ctx `.`) → `monitoring.coreos.com/v1` | `operator.victoriametrics.com/v1beta1`, `fail`s otherwise
  - `astrolavos.alerts.job` (ctx `.`) → job label value
  - `astrolavos.alerts.labels` (dict `{context, rule, override}`) → YAML map of rule labels
  - `astrolavos.alerts.selector` (string) → `{<sel>}` or empty
  - `astrolavos.alerts.rule.<key>` (dict `{context, rule, selector, exclusions, override}`) → one YAML list item
- `alerts.yaml` dispatches `include (printf "astrolavos.alerts.rule.%s" $key)` for every enabled key in `$groups`; Tasks 2–3 add rule templates for the keys already listed there.

- [ ] **Step 1: Write the failing tests**

Create `deploy/kubernetes/tests/alerts_test.yaml`:

```yaml
suite: Alerting rules (PrometheusRule / VMRule)
templates:
  - templates/alerts.yaml
release:
  name: astrolavos
  namespace: monitoring
tests:
  - it: ships no alerting rules by default
    capabilities:
      apiVersions:
        - monitoring.coreos.com/v1
    asserts:
      - hasDocuments:
          count: 0

  - it: is skipped when the PrometheusRule CRD is absent
    set:
      alerts:
        enabled: true
    asserts:
      - hasDocuments:
          count: 0

  - it: is skipped when the VMRule CRD is absent
    capabilities:
      apiVersions:
        - monitoring.coreos.com/v1
    set:
      alerts:
        enabled: true
        kind: VMRule
    asserts:
      - hasDocuments:
          count: 0

  - it: renders a PrometheusRule named after the chart in the release namespace
    capabilities:
      apiVersions:
        - monitoring.coreos.com/v1
    set:
      alerts:
        enabled: true
    asserts:
      - isKind:
          of: PrometheusRule
      - isAPIVersion:
          of: monitoring.coreos.com/v1
      - equal:
          path: metadata.name
          value: astrolavos
      - equal:
          path: metadata.namespace
          value: monitoring
      - equal:
          path: metadata.labels["app.kubernetes.io/name"]
          value: astrolavos
      - notExists:
          path: metadata.annotations
      - notExists:
          path: spec.groups[0].interval

  - it: renders a VMRule when asked to
    capabilities:
      apiVersions:
        - operator.victoriametrics.com/v1beta1
    set:
      alerts:
        enabled: true
        kind: VMRule
    asserts:
      - isKind:
          of: VMRule
      - isAPIVersion:
          of: operator.victoriametrics.com/v1beta1
      - equal:
          path: metadata.name
          value: astrolavos

  - it: rejects an unknown rule kind
    capabilities:
      apiVersions:
        - monitoring.coreos.com/v1
    set:
      alerts:
        enabled: true
        kind: AlertingRule
    asserts:
      - failedTemplate:
          errorPattern: 'alerts.kind must be PrometheusRule or VMRule, got "AlertingRule"'

  - it: renders the reachability group with its four rules by default
    capabilities:
      apiVersions:
        - monitoring.coreos.com/v1
    set:
      alerts:
        enabled: true
    asserts:
      - equal:
          path: spec.groups[0].name
          value: astrolavos.reachability
      - equal:
          path: spec.groups[0].rules[0].alert
          value: AstrolavosProbeFailureRatioHigh
      - equal:
          path: spec.groups[0].rules[1].alert
          value: AstrolavosProbeFailingOnSingleNode
      - equal:
          path: spec.groups[0].rules[2].alert
          value: AstrolavosDNSResolutionFailing
      - equal:
          path: spec.groups[0].rules[3].alert
          value: AstrolavosHTTPNon2xxRatioHigh
      - lengthEqual:
          path: spec.groups[0].rules
          count: 4
      # default thresholds, windows and severities
      - matchRegex:
          path: spec.groups[0].rules[0].expr
          pattern: 'rate\(astrolavos_errors_total\[10m\]\)'
      - matchRegex:
          path: spec.groups[0].rules[0].expr
          pattern: '> 0\.1\n'
      - equal:
          path: spec.groups[0].rules[0].for
          value: 10m
      - equal:
          path: spec.groups[0].rules[0].labels
          value:
            severity: critical
      # the single-node rule is guarded by the fleet-wide threshold
      - matchRegex:
          path: spec.groups[0].rules[1].expr
          pattern: 'sum by \(pod, domain, tag, prober_type\)'
      - matchRegex:
          path: spec.groups[0].rules[1].expr
          pattern: 'and on \(domain, tag, prober_type\)'
      - matchRegex:
          path: spec.groups[0].rules[1].expr
          pattern: '<= 0\.1\n'
      - equal:
          path: spec.groups[0].rules[1].labels.severity
          value: warning
      - matchRegex:
          path: spec.groups[0].rules[2].expr
          pattern: 'astrolavos_errors_total\{error="dns_error"\}\[5m\]'
      - equal:
          path: spec.groups[0].rules[2].for
          value: 5m
      - matchRegex:
          path: spec.groups[0].rules[3].expr
          pattern: 'status_code!~"2xx\|3xx\|"'
      - matchRegex:
          path: spec.groups[0].rules[3].expr
          pattern: 'prober_type="httptrace"'
      # alertmanager template placeholders survive helm rendering
      - matchRegex:
          path: spec.groups[0].rules[0].annotations.summary
          pattern: '\{\{ \$labels\.domain \}\}'

  - it: threads threshold, for and severity from values into a rule
    capabilities:
      apiVersions:
        - monitoring.coreos.com/v1
    set:
      alerts:
        enabled: true
        rules:
          probeFailureRatioHigh:
            threshold: 0.25
            for: 5m
            severity: page
    asserts:
      - matchRegex:
          path: spec.groups[0].rules[0].expr
          pattern: '> 0\.25\n'
      - equal:
          path: spec.groups[0].rules[0].for
          value: 5m
      - equal:
          path: spec.groups[0].rules[0].labels.severity
          value: page
      - matchRegex:
          path: spec.groups[0].rules[0].annotations.description
          pattern: 'More than 25%'

  - it: drops a disabled rule and keeps the others in order
    capabilities:
      apiVersions:
        - monitoring.coreos.com/v1
    set:
      alerts:
        enabled: true
        rules:
          probeFailingOnSingleNode:
            enabled: false
    asserts:
      - lengthEqual:
          path: spec.groups[0].rules
          count: 3
      - equal:
          path: spec.groups[0].rules[1].alert
          value: AstrolavosDNSResolutionFailing

  - it: merges alerts.labels into every rule without overriding severity
    capabilities:
      apiVersions:
        - monitoring.coreos.com/v1
    set:
      alerts:
        enabled: true
        labels:
          team: infrastructure
          severity: ignored
    asserts:
      - equal:
          path: spec.groups[0].rules[0].labels
          value:
            severity: critical
            team: infrastructure
      - equal:
          path: spec.groups[0].rules[1].labels
          value:
            severity: warning
            team: infrastructure

  - it: renders namespace, interval, extra labels and annotations
    capabilities:
      apiVersions:
        - monitoring.coreos.com/v1
    set:
      commonAnnotations:
        owner: sre
      alerts:
        enabled: true
        namespace: prometheus
        interval: 1m
        additionalLabels:
          release: kube-prometheus-stack
        annotations:
          argocd.argoproj.io/sync-options: Replace=true
    asserts:
      - equal:
          path: metadata.namespace
          value: prometheus
      - equal:
          path: metadata.labels.release
          value: kube-prometheus-stack
      - equal:
          path: metadata.annotations
          value:
            argocd.argoproj.io/sync-options: Replace=true
            owner: sre
      - equal:
          path: spec.groups[0].interval
          value: 1m
```

Add the alerts template to `deploy/kubernetes/tests/metadata_test.yaml` so the cross-resource label/namespace/name tests cover it:

```yaml
templates:
  # ...existing entries...
  - templates/dashboard-datadog.yaml
  - templates/alerts.yaml
```

and in its suite-level `set:` block:

```yaml
set:
  deployAsDaemonSet: false
  ingress:
    enabled: true
  grafanaDashboard:
    enabled: true
  datadogDashboard:
    enabled: true
  alerts:
    enabled: true
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `make helm-test`
Expected: the new suite fails with "template templates/alerts.yaml not found" (or similar) and metadata suite fails for the same reason.

- [ ] **Step 3: Add the `alerts` values block**

In `deploy/kubernetes/values.yaml`, insert after the `serviceMonitor:` block (after the `podTargetLabels: []` line) and before `# -- Optionally specify extra list of additional volumes`:

```yaml

## Alerting rules
## Rendered as a PrometheusRule (Prometheus Operator) or a VMRule (VictoriaMetrics
## operator); the spec is identical for both. Thresholds: ratios are fractions
## (0.1 = 10%), latencies are seconds. Latency histograms are only produced by
## httpTrace probes, so the latency rules never fire for tcp targets.
alerts:
  # -- Create a PrometheusRule or VMRule carrying the built-in alerting rules
  enabled: false
  # -- Rule CRD to render: `PrometheusRule` (monitoring.coreos.com/v1) or `VMRule` (operator.victoriametrics.com/v1beta1). Only rendered when the matching API group is present in the cluster capabilities.
  kind: PrometheusRule
  # -- Namespace for the rule resource (defaults to the release namespace)
  namespace: ""
  # -- Extra labels on the rule resource, e.g. `release: kube-prometheus-stack` so the operator's ruleSelector picks it up
  additionalLabels: {}
  # -- Extra annotations on the rule resource
  annotations: {}
  # -- Labels merged into every alert (e.g. `team`, `env`). Each rule's own `severity` wins on conflict.
  labels: {}
  # -- Rule group evaluation interval. Omitted when empty (operator default).
  interval: ""
  # -- Value of the `job` label on astrolavos scrape targets, used by the self-health rules. Defaults to the chart fullname, which is what the chart's ServiceMonitor produces.
  job: ""
  rules:
    # -- Probe error ratio per (domain, tag, prober_type) above `threshold` for `for`: the network path is down for the whole cluster
    probeFailureRatioHigh:
      enabled: true
      threshold: 0.1
      for: 10m
      severity: critical
    # -- One pod's (node's) error ratio above `threshold` while the fleet-wide ratio is at or below `probeFailureRatioHigh.threshold`: that node's network is broken. Only meaningful as a DaemonSet.
    probeFailingOnSingleNode:
      enabled: true
      threshold: 0.5
      for: 10m
      severity: warning
    # -- Ratio of probes failing with `error="dns_error"` above `threshold` (5m window): client-side DNS resolution is failing
    dnsResolutionFailing:
      enabled: true
      threshold: 0.05
      for: 5m
      severity: critical
    # -- Ratio of httpTrace probes answered with a non-2xx/3xx status above `threshold`: reachable but unhealthy peer (5xx, proxy 403). Failed probes are excluded, see `probeFailureRatioHigh`.
    httpNon2xxRatioHigh:
      enabled: true
      threshold: 0.1
      for: 10m
      severity: warning
    # -- p`quantile` TCP connect latency (seconds) above `threshold`: RTT or SYN loss on the path, independent of the application
    tcpConnectLatencyHigh:
      enabled: true
      quantile: 0.99
      threshold: 0.1
      for: 15m
      severity: warning
    # -- p`quantile` DNS resolution latency (seconds) above `threshold`: client view of the resolver chain
    dnsLatencyHigh:
      enabled: true
      quantile: 0.99
      threshold: 0.1
      for: 15m
      severity: warning
    # -- p`quantile` TLS handshake latency (seconds) above `threshold`: node CPU starvation or an egress middlebox terminating TLS
    tlsHandshakeLatencyHigh:
      enabled: true
      quantile: 0.99
      threshold: 0.5
      for: 15m
      severity: warning
    # -- p`quantile` end-to-end request latency (seconds) above `threshold`
    totalLatencyHigh:
      enabled: true
      quantile: 0.99
      threshold: 1
      for: 15m
      severity: warning
    # -- p`quantile` TCP connect latency above `factor` times its own value `baseline` ago and above `floor` seconds: a relative regression (e.g. traffic silently went cross-AZ) that absolute thresholds miss. Needs `baseline` of history; off by default.
    connectLatencyRegression:
      enabled: false
      quantile: 0.99
      factor: 3
      baseline: 1d
      floor: 0.005
      for: 30m
      severity: warning
    # -- A scraped astrolavos pod produced no probe at all for `for`: stuck goroutines or an empty endpoint list
    probeStalled:
      enabled: true
      for: 10m
      severity: warning
    # -- Fraction of astrolavos scrape targets with `up == 0` above `threshold`, or no targets at all: the prober itself is down
    targetsDown:
      enabled: true
      threshold: 0.2
      for: 10m
      severity: critical
  # -- Per-target overrides for the latency rules. Each entry re-renders the enabled latency rules with `selector` (raw PromQL matchers, no braces) baked in and an `override: <name>` label; the global latency rules exclude the matched series. Other keys are latency rule names whose `threshold`/`for`/`severity` override the global ones, e.g. `[{name: internal, selector: 'tag="internal"', tcpConnectLatencyHigh: {threshold: 0.01}}]`.
  latencyOverrides: []
  # -- Raw alerting rules appended verbatim in an `astrolavos.custom` group. Rendered as a template.
  additionalRules: []
```

- [ ] **Step 4: Create the helper templates and the reachability rules**

Create `deploy/kubernetes/templates/_alerts.tpl`:

```
{{/*
Alerting rule helpers. Every rule lives in its own named template
"astrolavos.alerts.rule.<values key>" and takes a dict:
  context    the root context ($)
  rule       the rule's values (alerts.rules.<key>, possibly merged with a
             latencyOverrides entry)
  selector   extra PromQL matchers baked into every metric selector ("" for
             the global copy of a rule)
  exclusions list of selector strings whose series this copy must NOT
             evaluate (latency rules only; empty for override copies)
  override   name of the latencyOverrides entry, "" for the global copy
Alert names are hardcoded per rule so tests can pin them.
Prometheus/vmalert template placeholders are emitted with {{ `{{ ... }}` }}.
*/}}

{{/*
API version for the configured rule kind. Fails on anything else.
*/}}
{{- define "astrolavos.alerts.apiVersion" -}}
{{- if eq .Values.alerts.kind "PrometheusRule" -}}
monitoring.coreos.com/v1
{{- else if eq .Values.alerts.kind "VMRule" -}}
operator.victoriametrics.com/v1beta1
{{- else -}}
{{- fail (printf "alerts.kind must be PrometheusRule or VMRule, got %q" .Values.alerts.kind) -}}
{{- end -}}
{{- end -}}

{{/*
Value of the job label on astrolavos scrape targets.
*/}}
{{- define "astrolavos.alerts.job" -}}
{{- default (include "common.names.fullname" .) .Values.alerts.job -}}
{{- end -}}

{{/*
Labels of one rule: severity (from the rule) + alerts.labels + override.
The rule's severity wins over alerts.labels.
*/}}
{{- define "astrolavos.alerts.labels" -}}
{{- $labels := merge (dict "severity" (required "alerts.rules.*.severity is required" .rule.severity)) (default (dict) .context.Values.alerts.labels) -}}
{{- if .override -}}
{{- $_ := set $labels "override" .override -}}
{{- end -}}
{{- toYaml $labels -}}
{{- end -}}

{{/*
Wrap a matcher string in braces, or render nothing when it is empty.
*/}}
{{- define "astrolavos.alerts.selector" -}}
{{- if . -}}{ {{- . -}} }{{- end -}}
{{- end -}}

{{/*
Percentage text for a ratio threshold, e.g. 0.05 -> "5%".
*/}}
{{- define "astrolavos.alerts.percent" -}}
{{- printf "%.0f%%" (mulf . 100) -}}
{{- end -}}

{{/* ----------------------------------------------------------------------
     Group astrolavos.reachability
     ---------------------------------------------------------------------- */}}

{{- define "astrolavos.alerts.rule.probeFailureRatioHigh" -}}
{{- $r := .rule -}}
{{- $threshold := required "alerts.rules.probeFailureRatioHigh.threshold is required" $r.threshold -}}
- alert: AstrolavosProbeFailureRatioHigh
  expr: |
    sum by (domain, tag, prober_type) (rate(astrolavos_errors_total[10m]))
      /
    sum by (domain, tag, prober_type) (rate(astrolavos_requests_total[10m]))
      > {{ $threshold }}
  for: {{ required "alerts.rules.probeFailureRatioHigh.for is required" $r.for }}
  labels: {{- include "astrolavos.alerts.labels" . | nindent 4 }}
  annotations:
    summary: >-
      {{ `{{ $labels.prober_type }}` }} probes to {{ `{{ $labels.domain }}` }} ({{ `{{ $labels.tag }}` }})
      failing: {{ `{{ $value | humanizePercentage }}` }} over 10m
    description: |
      More than {{ include "astrolavos.alerts.percent" $threshold }} of astrolavos probes from this
      cluster to {{ `{{ $labels.domain }}` }} fail after retries. The network path
      (DNS, TCP connect, TLS or the peer itself) is broken for the fleet, not
      for a single node.

      Investigate the error breakdown to tell DNS from timeouts from resets:
        sum by (error) (rate(astrolavos_errors_total{domain="{{ `{{ $labels.domain }}` }}"}[10m]))
      and which nodes are affected:
        sum by (pod) (rate(astrolavos_errors_total{domain="{{ `{{ $labels.domain }}` }}"}[10m]))
{{- end -}}

{{- define "astrolavos.alerts.rule.probeFailingOnSingleNode" -}}
{{- $r := .rule -}}
{{- $fleet := required "alerts.rules.probeFailureRatioHigh.threshold is required" .context.Values.alerts.rules.probeFailureRatioHigh.threshold -}}
{{- $threshold := required "alerts.rules.probeFailingOnSingleNode.threshold is required" $r.threshold -}}
- alert: AstrolavosProbeFailingOnSingleNode
  expr: |
    (
      sum by (pod, domain, tag, prober_type) (rate(astrolavos_errors_total[10m]))
        /
      sum by (pod, domain, tag, prober_type) (rate(astrolavos_requests_total[10m]))
        > {{ $threshold }}
    )
    and on (domain, tag, prober_type)
    (
      sum by (domain, tag, prober_type) (rate(astrolavos_errors_total[10m]))
        /
      sum by (domain, tag, prober_type) (rate(astrolavos_requests_total[10m]))
        <= {{ $fleet }}
    )
  for: {{ required "alerts.rules.probeFailingOnSingleNode.for is required" $r.for }}
  labels: {{- include "astrolavos.alerts.labels" . | nindent 4 }}
  annotations:
    summary: >-
      {{ `{{ $labels.pod }}` }} alone fails {{ `{{ $value | humanizePercentage }}` }} of probes
      to {{ `{{ $labels.domain }}` }} ({{ `{{ $labels.tag }}` }})
    description: |
      Probes to {{ `{{ $labels.domain }}` }} fail from {{ `{{ $labels.pod }}` }} while the
      fleet-wide failure ratio is at or below {{ include "astrolavos.alerts.percent" $fleet }}. The path
      works from other nodes, so the problem is on this node: its network
      interface, CNI agent, conntrack table or security group, not the peer.
      Only meaningful when astrolavos runs as a DaemonSet (one pod per node).

      Investigate: the node the pod runs on
        kubectl get pod -n <namespace> {{ `{{ $labels.pod }}` }} -o wide
      and the error categories it reports
        sum by (error) (rate(astrolavos_errors_total{pod="{{ `{{ $labels.pod }}` }}"}[10m]))
{{- end -}}

{{- define "astrolavos.alerts.rule.dnsResolutionFailing" -}}
{{- $r := .rule -}}
{{- $threshold := required "alerts.rules.dnsResolutionFailing.threshold is required" $r.threshold -}}
- alert: AstrolavosDNSResolutionFailing
  expr: |
    sum by (domain, tag, prober_type) (rate(astrolavos_errors_total{error="dns_error"}[5m]))
      /
    sum by (domain, tag, prober_type) (rate(astrolavos_requests_total[5m]))
      > {{ $threshold }}
  for: {{ required "alerts.rules.dnsResolutionFailing.for is required" $r.for }}
  labels: {{- include "astrolavos.alerts.labels" . | nindent 4 }}
  annotations:
    summary: >-
      DNS resolution of {{ `{{ $labels.domain }}` }} ({{ `{{ $labels.tag }}` }}) failing for
      {{ `{{ $value | humanizePercentage }}` }} of probes
    description: |
      More than {{ include "astrolavos.alerts.percent" $threshold }} of probes to {{ `{{ $labels.domain }}` }}
      fail at the DNS step (no such host, resolver timeout). This is the
      in-pod view of the resolver chain (node-local cache, cluster DNS,
      upstream), independent of whether the peer is reachable.

      Investigate: cluster DNS error/latency metrics for the same window, and
      whether the name still resolves from a pod:
        kubectl run -it --rm dns-test --image=busybox --restart=Never -- nslookup {{ `{{ $labels.domain }}` }}
{{- end -}}

{{- define "astrolavos.alerts.rule.httpNon2xxRatioHigh" -}}
{{- $r := .rule -}}
{{- $threshold := required "alerts.rules.httpNon2xxRatioHigh.threshold is required" $r.threshold -}}
- alert: AstrolavosHTTPNon2xxRatioHigh
  expr: |
    sum by (domain, tag) (rate(astrolavos_requests_total{prober_type="httptrace", status_code!~"2xx|3xx|"}[10m]))
      /
    sum by (domain, tag) (rate(astrolavos_requests_total{prober_type="httptrace"}[10m]))
      > {{ $threshold }}
  for: {{ required "alerts.rules.httpNon2xxRatioHigh.for is required" $r.for }}
  labels: {{- include "astrolavos.alerts.labels" . | nindent 4 }}
  annotations:
    summary: >-
      {{ `{{ $labels.domain }}` }} ({{ `{{ $labels.tag }}` }}) answers {{ `{{ $value | humanizePercentage }}` }}
      of HTTP probes with a non-2xx/3xx status
    description: |
      More than {{ include "astrolavos.alerts.percent" $threshold }} of HTTP probes to {{ `{{ $labels.domain }}` }}
      complete but return a 4xx/5xx status. The network path works; the peer
      (or a proxy/WAF in front of it) is rejecting or failing the request.
      Probes that did not complete are counted by AstrolavosProbeFailureRatioHigh
      instead.

      Investigate the status class breakdown:
        sum by (status_code) (rate(astrolavos_requests_total{domain="{{ `{{ $labels.domain }}` }}"}[10m]))
{{- end -}}
```

Create `deploy/kubernetes/templates/alerts.yaml`:

```
{{- if .Values.alerts.enabled }}
{{- $a := .Values.alerts }}
{{- $apiVersion := include "astrolavos.alerts.apiVersion" . }}
{{- if .Capabilities.APIVersions.Has $apiVersion }}
{{- /*
Rule keys per group, in render order. Each key has a named template
"astrolavos.alerts.rule.<key>" in _alerts.tpl. Latency rules are rendered
once globally (excluding every latencyOverrides selector) and once more per
latencyOverrides entry with that entry's selector and thresholds.
*/}}
{{- $groups := list
      (dict "name" "astrolavos.reachability" "keys" (list "probeFailureRatioHigh" "probeFailingOnSingleNode" "dnsResolutionFailing" "httpNon2xxRatioHigh"))
      (dict "name" "astrolavos.latency" "keys" (list "tcpConnectLatencyHigh" "dnsLatencyHigh" "tlsHandshakeLatencyHigh" "totalLatencyHigh" "connectLatencyRegression"))
      (dict "name" "astrolavos.selfhealth" "keys" (list "probeStalled" "targetsDown")) }}
{{- $anyRule := false }}
{{- range $groups }}
{{- range .keys }}
{{- if (index $a.rules .).enabled }}{{ $anyRule = true }}{{ end }}
{{- end }}
{{- end }}
{{- if or $anyRule $a.additionalRules }}
{{- $exclusions := list }}
{{- range $a.latencyOverrides }}
{{- $exclusions = append $exclusions (required "alerts.latencyOverrides[].selector is required" .selector) }}
{{- end }}
apiVersion: {{ $apiVersion }}
kind: {{ $a.kind }}
metadata:
  name: {{ include "common.names.fullname" . }}
  namespace: {{ default (include "common.names.namespace" .) $a.namespace | quote }}
  labels: {{- include "common.labels.standard" . | nindent 4 }}
    {{- if $a.additionalLabels }}
    {{- include "common.tplvalues.render" (dict "value" $a.additionalLabels "context" $) | nindent 4 }}
    {{- end }}
    {{- if .Values.commonLabels }}
    {{- include "common.tplvalues.render" (dict "value" .Values.commonLabels "context" $) | nindent 4 }}
    {{- end }}
  {{- if or $a.annotations .Values.commonAnnotations }}
  annotations:
    {{- if $a.annotations }}
    {{- include "common.tplvalues.render" (dict "value" $a.annotations "context" $) | nindent 4 }}
    {{- end }}
    {{- if .Values.commonAnnotations }}
    {{- include "common.tplvalues.render" (dict "value" .Values.commonAnnotations "context" $) | nindent 4 }}
    {{- end }}
  {{- end }}
spec:
  groups:
  {{- range $groups }}
  {{- $group := . }}
  {{- $isLatency := eq $group.name "astrolavos.latency" }}
  {{- $enabled := list }}
  {{- range .keys }}
  {{- if (index $a.rules .).enabled }}{{ $enabled = append $enabled . }}{{ end }}
  {{- end }}
  {{- if $enabled }}
    - name: {{ $group.name }}
      {{- with $a.interval }}
      interval: {{ . }}
      {{- end }}
      rules:
      {{- range $enabled }}
        {{- include (printf "astrolavos.alerts.rule.%s" .) (dict "context" $ "rule" (index $a.rules .) "selector" "" "exclusions" (ternary $exclusions (list) $isLatency) "override" "") | nindent 8 }}
      {{- end }}
      {{- if $isLatency }}
      {{- range $a.latencyOverrides }}
      {{- $ov := . }}
      {{- $name := required "alerts.latencyOverrides[].name is required" $ov.name }}
      {{- range $enabled }}
        {{- $rule := mergeOverwrite (deepCopy (index $a.rules .)) (default (dict) (index $ov .)) }}
        {{- include (printf "astrolavos.alerts.rule.%s" .) (dict "context" $ "rule" $rule "selector" $ov.selector "exclusions" (list) "override" $name) | nindent 8 }}
      {{- end }}
      {{- end }}
      {{- end }}
  {{- end }}
  {{- end }}
  {{- with $a.additionalRules }}
    - name: astrolavos.custom
      {{- with $a.interval }}
      interval: {{ . }}
      {{- end }}
      rules: {{- include "common.tplvalues.render" (dict "value" . "context" $) | nindent 8 }}
  {{- end }}
{{- end }}
{{- end }}
{{- end }}
```

Note: `alerts.yaml` already dispatches to the latency and self-health templates that Tasks 2 and 3 define. Until those exist, `make helm-test` fails on `template: no template "astrolavos.alerts.rule.tcpConnectLatencyHigh"`. To keep Task 1 green on its own, temporarily disable them for this task's test run only: run the suite with the other groups off (Step 5 below). Do **not** commit a values change for that.

- [ ] **Step 5: Run the tests**

Run: `make helm-test`
Expected for this task: the alerts suite fails only with "no template astrolavos.alerts.rule.tcpConnectLatencyHigh" (dispatch reaches the latency group). Confirm the reachability rendering itself by rendering with the other groups off:

```bash
helm template astrolavos deploy/kubernetes --namespace monitoring \
  --api-versions monitoring.coreos.com/v1 \
  --set alerts.enabled=true \
  --set alerts.rules.tcpConnectLatencyHigh.enabled=false \
  --set alerts.rules.dnsLatencyHigh.enabled=false \
  --set alerts.rules.tlsHandshakeLatencyHigh.enabled=false \
  --set alerts.rules.totalLatencyHigh.enabled=false \
  --set alerts.rules.probeStalled.enabled=false \
  --set alerts.rules.targetsDown.enabled=false \
  --show-only templates/alerts.yaml
```

Expected: a `PrometheusRule` with one group `astrolavos.reachability` and four rules; `{{ $labels.domain }}` appears literally in annotations; `description` says "More than 10%".

- [ ] **Step 6: Commit**

```bash
git add deploy/kubernetes/values.yaml deploy/kubernetes/templates/_alerts.tpl deploy/kubernetes/templates/alerts.yaml deploy/kubernetes/tests/alerts_test.yaml deploy/kubernetes/tests/metadata_test.yaml
git commit -m "feat(helm): add PrometheusRule/VMRule alerting with reachability rules"
```

---

### Task 2: Latency group with per-target overrides

**Files:**
- Modify: `deploy/kubernetes/templates/_alerts.tpl` (append)
- Modify: `deploy/kubernetes/tests/alerts_test.yaml` (append tests)

**Interfaces:**
- Consumes `astrolavos.alerts.labels`, `astrolavos.alerts.selector` from Task 1 and the dict contract (`context, rule, selector, exclusions, override`).
- Produces `astrolavos.alerts.quantile` (dict `{quantile, metric, selector, offset}`) and `astrolavos.alerts.exclusions` (dict with `exclusions` list) helpers, plus `astrolavos.alerts.rule.{tcpConnectLatencyHigh,dnsLatencyHigh,tlsHandshakeLatencyHigh,totalLatencyHigh,connectLatencyRegression}`.

- [ ] **Step 1: Write the failing tests**

Append to `deploy/kubernetes/tests/alerts_test.yaml`:

```yaml
  - it: renders the latency group with four rules and the regression rule off
    capabilities:
      apiVersions:
        - monitoring.coreos.com/v1
    set:
      alerts:
        enabled: true
    asserts:
      - equal:
          path: spec.groups[1].name
          value: astrolavos.latency
      - lengthEqual:
          path: spec.groups[1].rules
          count: 4
      - equal:
          path: spec.groups[1].rules[0].alert
          value: AstrolavosTCPConnectLatencyHigh
      - equal:
          path: spec.groups[1].rules[1].alert
          value: AstrolavosDNSLatencyHigh
      - equal:
          path: spec.groups[1].rules[2].alert
          value: AstrolavosTLSHandshakeLatencyHigh
      - equal:
          path: spec.groups[1].rules[3].alert
          value: AstrolavosTotalLatencyHigh
      - matchRegex:
          path: spec.groups[1].rules[0].expr
          pattern: '^histogram_quantile\(0\.99, sum by \(domain, tag, prober_type, le\) \(rate\(astrolavos_conn_latency_seconds_bucket\[15m\]\)\)\)\n  > 0\.1\n$'
      - matchRegex:
          path: spec.groups[1].rules[1].expr
          pattern: 'astrolavos_dns_latency_seconds_bucket\[15m\]'
      - matchRegex:
          path: spec.groups[1].rules[2].expr
          pattern: 'astrolavos_tls_latency_seconds_bucket\[15m\]\)\)\)\n  > 0\.5\n'
      - matchRegex:
          path: spec.groups[1].rules[3].expr
          pattern: 'astrolavos_total_latency_seconds_bucket\[15m\]\)\)\)\n  > 1\n'
      - equal:
          path: spec.groups[1].rules[0].for
          value: 15m
      - equal:
          path: spec.groups[1].rules[0].labels
          value:
            severity: warning

  - it: renders the connect latency regression rule when enabled
    capabilities:
      apiVersions:
        - monitoring.coreos.com/v1
    set:
      alerts:
        enabled: true
        rules:
          connectLatencyRegression:
            enabled: true
            factor: 2
            baseline: 7d
            floor: 0.002
    asserts:
      - lengthEqual:
          path: spec.groups[1].rules
          count: 5
      - equal:
          path: spec.groups[1].rules[4].alert
          value: AstrolavosConnectLatencyRegression
      - matchRegex:
          path: spec.groups[1].rules[4].expr
          pattern: '> 2 \*\n'
      - matchRegex:
          path: spec.groups[1].rules[4].expr
          pattern: '\[15m\] offset 7d\)'
      - matchRegex:
          path: spec.groups[1].rules[4].expr
          pattern: '\nand\n'
      - matchRegex:
          path: spec.groups[1].rules[4].expr
          pattern: '> 0\.002\n$'
      - equal:
          path: spec.groups[1].rules[4].for
          value: 30m

  - it: re-renders the latency rules per latencyOverrides entry and excludes them globally
    capabilities:
      apiVersions:
        - monitoring.coreos.com/v1
    set:
      alerts:
        enabled: true
        rules:
          dnsLatencyHigh:
            enabled: false
        latencyOverrides:
          - name: internal
            selector: 'tag="internal"'
            tcpConnectLatencyHigh:
              threshold: 0.01
              for: 5m
            totalLatencyHigh:
              threshold: 0.05
              severity: critical
    asserts:
      # 3 global copies + 3 override copies (dnsLatencyHigh disabled everywhere)
      - lengthEqual:
          path: spec.groups[1].rules
          count: 6
      # global copies exclude the override's series
      - equal:
          path: spec.groups[1].rules[0].alert
          value: AstrolavosTCPConnectLatencyHigh
      - matchRegex:
          path: spec.groups[1].rules[0].expr
          pattern: 'astrolavos_conn_latency_seconds_bucket\[15m\]'
      - matchRegex:
          path: spec.groups[1].rules[0].expr
          pattern: '> 0\.1\nunless on \(domain, tag, prober_type\)\n  sum by \(domain, tag, prober_type\) \(rate\(astrolavos_requests_total\{tag="internal"\}\[15m\]\)\)\n$'
      - notExists:
          path: spec.groups[1].rules[0].labels.override
      # override copies carry the selector, their thresholds and the override label
      - equal:
          path: spec.groups[1].rules[3].alert
          value: AstrolavosTCPConnectLatencyHigh
      - matchRegex:
          path: spec.groups[1].rules[3].expr
          pattern: '^histogram_quantile\(0\.99, sum by \(domain, tag, prober_type, le\) \(rate\(astrolavos_conn_latency_seconds_bucket\{tag="internal"\}\[15m\]\)\)\)\n  > 0\.01\n$'
      - equal:
          path: spec.groups[1].rules[3].for
          value: 5m
      - equal:
          path: spec.groups[1].rules[3].labels
          value:
            severity: warning
            override: internal
      - equal:
          path: spec.groups[1].rules[4].alert
          value: AstrolavosTLSHandshakeLatencyHigh
      - matchRegex:
          path: spec.groups[1].rules[4].expr
          pattern: 'astrolavos_tls_latency_seconds_bucket\{tag="internal"\}\[15m\]\)\)\)\n  > 0\.5\n$'
      - equal:
          path: spec.groups[1].rules[5].alert
          value: AstrolavosTotalLatencyHigh
      - matchRegex:
          path: spec.groups[1].rules[5].expr
          pattern: '> 0\.05\n$'
      - equal:
          path: spec.groups[1].rules[5].labels.severity
          value: critical

  - it: requires a selector on every latencyOverrides entry
    capabilities:
      apiVersions:
        - monitoring.coreos.com/v1
    set:
      alerts:
        enabled: true
        latencyOverrides:
          - name: internal
    asserts:
      - failedTemplate:
          errorPattern: 'alerts.latencyOverrides\[\].selector is required'
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `make helm-test`
Expected: FAIL with `no template "astrolavos.alerts.rule.tcpConnectLatencyHigh"`.

- [ ] **Step 3: Append the latency helpers and rules**

Append to `deploy/kubernetes/templates/_alerts.tpl`:

```
{{/* ----------------------------------------------------------------------
     Group astrolavos.latency
     Histograms are recorded only on successful httptrace probes, so these
     rules are never skewed by failures and never fire for tcp targets.
     ---------------------------------------------------------------------- */}}

{{/*
histogram_quantile expression over a 15m window for one latency metric.
dict: quantile, metric (dns|conn|tls|total), selector, offset (optional)
*/}}
{{- define "astrolavos.alerts.quantile" -}}
histogram_quantile({{ .quantile }}, sum by (domain, tag, prober_type, le) (rate(astrolavos_{{ .metric }}_latency_seconds_bucket{{ include "astrolavos.alerts.selector" .selector }}[15m]{{ with .offset }} offset {{ . }}{{ end }})))
{{- end -}}

{{/*
One "unless" clause per latencyOverrides selector so the global copy of a
latency rule skips the series an override copy evaluates. Empty when there
is nothing to exclude.
*/}}
{{- define "astrolavos.alerts.exclusions" -}}
{{- $lines := list -}}
{{- range .exclusions -}}
{{- $lines = append $lines (printf "unless on (domain, tag, prober_type)\n  sum by (domain, tag, prober_type) (rate(astrolavos_requests_total{%s}[15m]))" .) -}}
{{- end -}}
{{- join "\n" $lines -}}
{{- end -}}

{{- define "astrolavos.alerts.rule.tcpConnectLatencyHigh" -}}
{{- $r := .rule -}}
{{- $q := required "alerts.rules.tcpConnectLatencyHigh.quantile is required" $r.quantile -}}
{{- $threshold := required "alerts.rules.tcpConnectLatencyHigh.threshold is required" $r.threshold -}}
- alert: AstrolavosTCPConnectLatencyHigh
  expr: |
    {{ include "astrolavos.alerts.quantile" (dict "quantile" $q "metric" "conn" "selector" .selector) }}
      > {{ $threshold }}
    {{- with include "astrolavos.alerts.exclusions" . }}
    {{- . | nindent 4 }}
    {{- end }}
  for: {{ required "alerts.rules.tcpConnectLatencyHigh.for is required" $r.for }}
  labels: {{- include "astrolavos.alerts.labels" . | nindent 4 }}
  annotations:
    summary: >-
      p{{ $q }} TCP connect latency to {{ `{{ $labels.domain }}` }} ({{ `{{ $labels.tag }}` }})
      is {{ `{{ $value | humanizeDuration }}` }}
    description: |
      The p{{ $q }} time to complete the TCP handshake with {{ `{{ $labels.domain }}` }}
      has been above {{ $threshold }}s for 15m. Connect time is pure network
      round-trip plus SYN retransmits, independent of DNS, TLS and the
      application, so this is the cleanest "the path got slow or lossy"
      signal: cross-AZ or cross-region routing, packet loss, a saturated NAT
      gateway or an overloaded peer accept queue.

      Investigate: compare per node to see if it is one node or the fleet
        histogram_quantile({{ $q }}, sum by (pod, le) (rate(astrolavos_conn_latency_seconds_bucket{domain="{{ `{{ $labels.domain }}` }}"}[15m])))
      and check AstrolavosProbeFailureRatioHigh for the same target: loss
      usually shows up as timeouts shortly after.
{{- end -}}

{{- define "astrolavos.alerts.rule.dnsLatencyHigh" -}}
{{- $r := .rule -}}
{{- $q := required "alerts.rules.dnsLatencyHigh.quantile is required" $r.quantile -}}
{{- $threshold := required "alerts.rules.dnsLatencyHigh.threshold is required" $r.threshold -}}
- alert: AstrolavosDNSLatencyHigh
  expr: |
    {{ include "astrolavos.alerts.quantile" (dict "quantile" $q "metric" "dns" "selector" .selector) }}
      > {{ $threshold }}
    {{- with include "astrolavos.alerts.exclusions" . }}
    {{- . | nindent 4 }}
    {{- end }}
  for: {{ required "alerts.rules.dnsLatencyHigh.for is required" $r.for }}
  labels: {{- include "astrolavos.alerts.labels" . | nindent 4 }}
  annotations:
    summary: >-
      p{{ $q }} DNS resolution of {{ `{{ $labels.domain }}` }} ({{ `{{ $labels.tag }}` }})
      takes {{ `{{ $value | humanizeDuration }}` }}
    description: |
      The p{{ $q }} time to resolve {{ `{{ $labels.domain }}` }} from inside a pod has
      been above {{ $threshold }}s for 15m. This is the client view of the
      whole resolver chain (node-local cache, cluster DNS, upstream): lookups
      still succeed but stall, which usually precedes dns_error failures.

      Investigate: cluster DNS cache hit ratio and upstream forward latency
      for the same window, and whether the slowness is per node
        histogram_quantile({{ $q }}, sum by (pod, le) (rate(astrolavos_dns_latency_seconds_bucket{domain="{{ `{{ $labels.domain }}` }}"}[15m])))
{{- end -}}

{{- define "astrolavos.alerts.rule.tlsHandshakeLatencyHigh" -}}
{{- $r := .rule -}}
{{- $q := required "alerts.rules.tlsHandshakeLatencyHigh.quantile is required" $r.quantile -}}
{{- $threshold := required "alerts.rules.tlsHandshakeLatencyHigh.threshold is required" $r.threshold -}}
- alert: AstrolavosTLSHandshakeLatencyHigh
  expr: |
    {{ include "astrolavos.alerts.quantile" (dict "quantile" $q "metric" "tls" "selector" .selector) }}
      > {{ $threshold }}
    {{- with include "astrolavos.alerts.exclusions" . }}
    {{- . | nindent 4 }}
    {{- end }}
  for: {{ required "alerts.rules.tlsHandshakeLatencyHigh.for is required" $r.for }}
  labels: {{- include "astrolavos.alerts.labels" . | nindent 4 }}
  annotations:
    summary: >-
      p{{ $q }} TLS handshake with {{ `{{ $labels.domain }}` }} ({{ `{{ $labels.tag }}` }})
      takes {{ `{{ $value | humanizeDuration }}` }}
    description: |
      The p{{ $q }} TLS handshake time with {{ `{{ $labels.domain }}` }} has been above
      {{ $threshold }}s for 15m while TCP connect is measured separately. A
      handshake that is slow on top of a normal connect points at CPU
      starvation on the node or the peer, an egress proxy or middlebox
      terminating TLS, or certificate chain / OCSP fetches on the peer side.

      Investigate: AstrolavosTCPConnectLatencyHigh for the same target (if
      it fires too, the network is slow, not TLS), node CPU throttling, and
      whether egress for this destination goes through a proxy.
{{- end -}}

{{- define "astrolavos.alerts.rule.totalLatencyHigh" -}}
{{- $r := .rule -}}
{{- $q := required "alerts.rules.totalLatencyHigh.quantile is required" $r.quantile -}}
{{- $threshold := required "alerts.rules.totalLatencyHigh.threshold is required" $r.threshold -}}
- alert: AstrolavosTotalLatencyHigh
  expr: |
    {{ include "astrolavos.alerts.quantile" (dict "quantile" $q "metric" "total" "selector" .selector) }}
      > {{ $threshold }}
    {{- with include "astrolavos.alerts.exclusions" . }}
    {{- . | nindent 4 }}
    {{- end }}
  for: {{ required "alerts.rules.totalLatencyHigh.for is required" $r.for }}
  labels: {{- include "astrolavos.alerts.labels" . | nindent 4 }}
  annotations:
    summary: >-
      p{{ $q }} request latency to {{ `{{ $labels.domain }}` }} ({{ `{{ $labels.tag }}` }})
      is {{ `{{ $value | humanizeDuration }}` }}
    description: |
      The p{{ $q }} end-to-end time of HTTP probes to {{ `{{ $labels.domain }}` }} (DNS +
      connect + TLS + time to first byte + body) has been above {{ $threshold }}s
      for 15m.

      Investigate which phase grew, using the same quantile over
      astrolavos_dns_latency_seconds_bucket, astrolavos_conn_latency_seconds_bucket,
      astrolavos_tls_latency_seconds_bucket and astrolavos_firstbyte_latency_seconds_bucket
      for domain="{{ `{{ $labels.domain }}` }}". Only first-byte growing means the
      peer is slow; connect growing means the network is.
{{- end -}}

{{- define "astrolavos.alerts.rule.connectLatencyRegression" -}}
{{- $r := .rule -}}
{{- $q := required "alerts.rules.connectLatencyRegression.quantile is required" $r.quantile -}}
{{- $factor := required "alerts.rules.connectLatencyRegression.factor is required" $r.factor -}}
{{- $baseline := required "alerts.rules.connectLatencyRegression.baseline is required" $r.baseline -}}
{{- $floor := required "alerts.rules.connectLatencyRegression.floor is required" $r.floor -}}
{{- $now := include "astrolavos.alerts.quantile" (dict "quantile" $q "metric" "conn" "selector" .selector) -}}
{{- $then := include "astrolavos.alerts.quantile" (dict "quantile" $q "metric" "conn" "selector" .selector "offset" $baseline) -}}
- alert: AstrolavosConnectLatencyRegression
  expr: |
    (
      {{ $now }}
        > {{ $factor }} *
      {{ $then }}
    )
    and
    {{ $now }}
      > {{ $floor }}
    {{- with include "astrolavos.alerts.exclusions" . }}
    {{- . | nindent 4 }}
    {{- end }}
  for: {{ required "alerts.rules.connectLatencyRegression.for is required" $r.for }}
  labels: {{- include "astrolavos.alerts.labels" . | nindent 4 }}
  annotations:
    summary: >-
      p{{ $q }} TCP connect latency to {{ `{{ $labels.domain }}` }} ({{ `{{ $labels.tag }}` }})
      is {{ `{{ $value | humanizeDuration }}` }}, over {{ $factor }}x its value {{ $baseline }} ago
    description: |
      The p{{ $q }} TCP connect time to {{ `{{ $labels.domain }}` }} is more than {{ $factor }}
      times what it was {{ $baseline }} ago (and above the {{ $floor }}s noise floor).
      Nothing may have crossed an absolute threshold, but the path changed:
      traffic that used to stay in-zone is now crossing AZs (Service
      trafficDistribution / topology hints no longer honoured), a route or
      peering changed, or the peer moved.

      Investigate: where the probe traffic lands now vs before (endpoint
      zones, kubectl get endpointslices), recent networking or Service
      changes, and the per-node split
        histogram_quantile({{ $q }}, sum by (pod, le) (rate(astrolavos_conn_latency_seconds_bucket{domain="{{ `{{ $labels.domain }}` }}"}[15m])))
{{- end -}}
```

- [ ] **Step 4: Run the tests**

Run: `make helm-test`
Expected: the latency tests pass; the suite still fails only with `no template "astrolavos.alerts.rule.probeStalled"` (Task 3). Spot-check the override rendering:

```bash
helm template astrolavos deploy/kubernetes --namespace monitoring \
  --api-versions monitoring.coreos.com/v1 \
  --set alerts.enabled=true \
  --set alerts.rules.probeStalled.enabled=false \
  --set alerts.rules.targetsDown.enabled=false \
  --set 'alerts.latencyOverrides[0].name=internal' \
  --set 'alerts.latencyOverrides[0].selector=tag="internal"' \
  --set 'alerts.latencyOverrides[0].tcpConnectLatencyHigh.threshold=0.01' \
  --show-only templates/alerts.yaml
```

Expected: the latency group has 8 rules; the first four end with an `unless on (domain, tag, prober_type)` clause selecting `{tag="internal"}`; the last four select `{tag="internal"}` in the histogram and carry `override: internal`.

- [ ] **Step 5: Commit**

```bash
git add deploy/kubernetes/templates/_alerts.tpl deploy/kubernetes/tests/alerts_test.yaml
git commit -m "feat(helm): add latency alerting rules with per-target overrides"
```

---

### Task 3: Self-health group and additionalRules

**Files:**
- Modify: `deploy/kubernetes/templates/_alerts.tpl` (append)
- Modify: `deploy/kubernetes/tests/alerts_test.yaml` (append tests)

**Interfaces:**
- Consumes `astrolavos.alerts.job`, `astrolavos.alerts.labels` from Task 1.
- Produces `astrolavos.alerts.rule.{probeStalled,targetsDown}`.

- [ ] **Step 1: Write the failing tests**

Append to `deploy/kubernetes/tests/alerts_test.yaml`:

```yaml
  - it: renders the self-health group against the chart job label
    capabilities:
      apiVersions:
        - monitoring.coreos.com/v1
    set:
      alerts:
        enabled: true
    asserts:
      - lengthEqual:
          path: spec.groups
          count: 3
      - equal:
          path: spec.groups[2].name
          value: astrolavos.selfhealth
      - equal:
          path: spec.groups[2].rules[0].alert
          value: AstrolavosProbeStalled
      - equal:
          path: spec.groups[2].rules[0].expr
          value: |
            up{job="astrolavos"} == 1
            unless on (pod)
            sum by (pod) (rate(astrolavos_requests_total[10m])) > 0
      - equal:
          path: spec.groups[2].rules[0].for
          value: 10m
      - equal:
          path: spec.groups[2].rules[1].alert
          value: AstrolavosTargetsDown
      - equal:
          path: spec.groups[2].rules[1].expr
          value: |
            count(up{job="astrolavos"} == 0) / count(up{job="astrolavos"}) > 0.2
            or
            absent(up{job="astrolavos"})
      - equal:
          path: spec.groups[2].rules[1].labels.severity
          value: critical

  - it: uses alerts.job for the self-health rules when set
    capabilities:
      apiVersions:
        - monitoring.coreos.com/v1
    set:
      alerts:
        enabled: true
        job: netprobe
    asserts:
      - matchRegex:
          path: spec.groups[2].rules[0].expr
          pattern: '^up\{job="netprobe"\} == 1\n'
      - matchRegex:
          path: spec.groups[2].rules[1].expr
          pattern: 'count\(up\{job="netprobe"\} == 0\)'

  - it: follows the chart fullname for the job label
    capabilities:
      apiVersions:
        - monitoring.coreos.com/v1
    set:
      fullnameOverride: ""
      nameOverride: probe
      alerts:
        enabled: true
    asserts:
      - matchRegex:
          path: spec.groups[2].rules[0].expr
          pattern: '^up\{job="astrolavos-probe"\} == 1\n'

  - it: appends additionalRules as a templated custom group
    capabilities:
      apiVersions:
        - monitoring.coreos.com/v1
    set:
      alerts:
        enabled: true
        interval: 2m
        additionalRules:
          - alert: AstrolavosEgressDown
            expr: 'sum(rate(astrolavos_errors_total{tag="egress"}[5m])) > 0'
            for: 5m
            labels:
              severity: critical
              release: "{{ .Release.Name }}"
    asserts:
      - lengthEqual:
          path: spec.groups
          count: 4
      - equal:
          path: spec.groups[3].name
          value: astrolavos.custom
      - equal:
          path: spec.groups[3].interval
          value: 2m
      - equal:
          path: spec.groups[3].rules
          value:
            - alert: AstrolavosEgressDown
              expr: 'sum(rate(astrolavos_errors_total{tag="egress"}[5m])) > 0'
              for: 5m
              labels:
                severity: critical
                release: astrolavos

  - it: drops a group whose rules are all disabled
    capabilities:
      apiVersions:
        - monitoring.coreos.com/v1
    set:
      alerts:
        enabled: true
        rules:
          probeStalled:
            enabled: false
          targetsDown:
            enabled: false
    asserts:
      - lengthEqual:
          path: spec.groups
          count: 2
      - equal:
          path: spec.groups[1].name
          value: astrolavos.latency

  - it: renders nothing when every rule is disabled and no additionalRules are given
    capabilities:
      apiVersions:
        - monitoring.coreos.com/v1
    set:
      alerts:
        enabled: true
        rules:
          probeFailureRatioHigh: {enabled: false}
          probeFailingOnSingleNode: {enabled: false}
          dnsResolutionFailing: {enabled: false}
          httpNon2xxRatioHigh: {enabled: false}
          tcpConnectLatencyHigh: {enabled: false}
          dnsLatencyHigh: {enabled: false}
          tlsHandshakeLatencyHigh: {enabled: false}
          totalLatencyHigh: {enabled: false}
          connectLatencyRegression: {enabled: false}
          probeStalled: {enabled: false}
          targetsDown: {enabled: false}
    asserts:
      - hasDocuments:
          count: 0
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `make helm-test`
Expected: FAIL with `no template "astrolavos.alerts.rule.probeStalled"`.

- [ ] **Step 3: Append the self-health rules**

Append to `deploy/kubernetes/templates/_alerts.tpl`:

```
{{/* ----------------------------------------------------------------------
     Group astrolavos.selfhealth
     Watchdogs for the prober itself, so silence is not mistaken for health.
     ---------------------------------------------------------------------- */}}

{{- define "astrolavos.alerts.rule.probeStalled" -}}
{{- $r := .rule -}}
{{- $job := include "astrolavos.alerts.job" .context -}}
- alert: AstrolavosProbeStalled
  expr: |
    up{job="{{ $job }}"} == 1
    unless on (pod)
    sum by (pod) (rate(astrolavos_requests_total[10m])) > 0
  for: {{ required "alerts.rules.probeStalled.for is required" $r.for }}
  labels: {{- include "astrolavos.alerts.labels" . | nindent 4 }}
  annotations:
    summary: >-
      astrolavos pod {{ `{{ $labels.pod }}` }} is scraped but has not probed anything for 10m
    description: |
      {{ `{{ $labels.pod }}` }} answers scrapes (up == 1) but astrolavos_requests_total has
      not moved for any endpoint in 10m. Either every prober goroutine is
      stuck (a hung dial that ignores its timeout) or the pod started with an
      empty endpoint list. Latency and failure alerts are blind for this node
      until it recovers.

      Investigate:
        kubectl logs -n <namespace> {{ `{{ $labels.pod }}` }}
        kubectl get configmap -n <namespace> -l app.kubernetes.io/name=astrolavos -o yaml
      Restarting the pod clears a stuck prober.
{{- end -}}

{{- define "astrolavos.alerts.rule.targetsDown" -}}
{{- $r := .rule -}}
{{- $job := include "astrolavos.alerts.job" .context -}}
{{- $threshold := required "alerts.rules.targetsDown.threshold is required" $r.threshold -}}
- alert: AstrolavosTargetsDown
  expr: |
    count(up{job="{{ $job }}"} == 0) / count(up{job="{{ $job }}"}) > {{ $threshold }}
    or
    absent(up{job="{{ $job }}"})
  for: {{ required "alerts.rules.targetsDown.for is required" $r.for }}
  labels: {{- include "astrolavos.alerts.labels" . | nindent 4 }}
  annotations:
    summary: >-
      astrolavos scrape targets down (job {{ $job }})
    description: |
      More than {{ include "astrolavos.alerts.percent" $threshold }} of astrolavos pods are not being scraped
      (up == 0), or no target exists at all. Every other astrolavos alert is
      blind while this fires: a quiet dashboard does not mean the network is
      healthy.

      Investigate:
        kubectl get pods -n <namespace> -l app.kubernetes.io/name=astrolavos -o wide
        kubectl get servicemonitor -n <namespace> {{ $job }}
      and the scrape target page of Prometheus / vmagent for job="{{ $job }}".
{{- end -}}
```

- [ ] **Step 4: Run the whole chart test-suite**

Run: `make helm-test`
Expected: all suites PASS, including `metadata_test.yaml` now covering `templates/alerts.yaml`.

Run: `helm lint --strict deploy/kubernetes`
Expected: `1 chart(s) linted, 0 chart(s) failed`.

- [ ] **Step 5: Commit**

```bash
git add deploy/kubernetes/templates/_alerts.tpl deploy/kubernetes/tests/alerts_test.yaml
git commit -m "feat(helm): add self-health alerting rules and additionalRules group"
```

---

### Task 4: PromQL validation, docs

**Files:**
- Modify: `deploy/kubernetes/README.md` (regenerated by `make helm-docs`)
- Modify: `README.md` ("Deploy On Kubernetes" paragraph)
- Modify: `AGENTS.md` ("Helm Chart" section)

- [ ] **Step 1: Validate the rendered PromQL with promtool and vmalert**

Install the validators if missing (`brew install prometheus victoriametrics` provides `promtool` and `vmalert`; alternatively run them from their Docker images). Render the three representative variants to `/tmp` and extract the `spec` as a plain rules file:

```bash
cd deploy/kubernetes
render() {
  helm template astrolavos . --namespace monitoring --api-versions "$1" --show-only templates/alerts.yaml "${@:2}" \
    | yq '.spec' > "/tmp/astrolavos-rules-$RANDOM.yaml"
  echo "/tmp/astrolavos-rules-"*
}
# default PrometheusRule
helm template astrolavos . --namespace monitoring --api-versions monitoring.coreos.com/v1 \
  --set alerts.enabled=true --set alerts.rules.connectLatencyRegression.enabled=true \
  --show-only templates/alerts.yaml | yq '.spec' > /tmp/astrolavos-rules-default.yaml
# VMRule with an override
helm template astrolavos . --namespace monitoring --api-versions operator.victoriametrics.com/v1beta1 \
  --set alerts.enabled=true --set alerts.kind=VMRule \
  --set alerts.rules.connectLatencyRegression.enabled=true \
  --set 'alerts.latencyOverrides[0].name=internal' \
  --set 'alerts.latencyOverrides[0].selector=tag="internal"' \
  --set 'alerts.latencyOverrides[0].tcpConnectLatencyHigh.threshold=0.01' \
  --show-only templates/alerts.yaml | yq '.spec' > /tmp/astrolavos-rules-vm.yaml

promtool check rules /tmp/astrolavos-rules-default.yaml /tmp/astrolavos-rules-vm.yaml
vmalert -rule=/tmp/astrolavos-rules-default.yaml -rule=/tmp/astrolavos-rules-vm.yaml -dryRun
```

Expected: promtool reports `SUCCESS: N rules found` for each file (no "duplicate rule" complaints: Prometheus allows repeated alert names); vmalert exits 0. Fix any parse error in `_alerts.tpl` and re-run `make helm-test`. Delete the `/tmp` files afterwards.

- [ ] **Step 2: Regenerate the chart README**

Run: `make helm-docs`
Expected: `deploy/kubernetes/README.md` gains `alerts.*` rows, one per `# --` comment (rule objects appear as single rows with their object default).

- [ ] **Step 3: Mention alerting in the root README**

In `README.md`, section "Deploy On Kubernetes", replace the last paragraph

```markdown
By default the chart deploys a DaemonSet (one prober per node), a Service, a PodDisruptionBudget and a Prometheus Operator `ServiceMonitor`. Set `deployAsDaemonSet=false` for a Deployment with HPA instead. Optional Grafana and Datadog dashboards ship with the chart. All values are documented in the [chart README](./deploy/kubernetes/README.md).
```

with

```markdown
By default the chart deploys a DaemonSet (one prober per node), a Service, a PodDisruptionBudget and a Prometheus Operator `ServiceMonitor`. Set `deployAsDaemonSet=false` for a Deployment with HPA instead. Optional Grafana and Datadog dashboards ship with the chart, as does a curated set of alerting rules (probe failure ratio, single-node failures, DNS failures, connect/DNS/TLS/total latency, prober self-health) rendered as a `PrometheusRule` or a `VMRule`: set `alerts.enabled=true` and pick `alerts.kind`. Per-target latency thresholds go in `alerts.latencyOverrides`. All values are documented in the [chart README](./deploy/kubernetes/README.md).
```

- [ ] **Step 4: Document the template pattern in AGENTS.md**

In `AGENTS.md`, section "Helm Chart", after the bullet `- **ServiceMonitor** enabled by default for Prometheus Operator scraping.` add:

```markdown
- **Alerting rules** (`alerts.*`) render as a `PrometheusRule` or `VMRule` from `templates/alerts.yaml`; both CRDs share the same `groups[].rules[]` spec so one template body serves both. Each rule is a named template `astrolavos.alerts.rule.<values key>` in `templates/_alerts.tpl` taking `{context, rule, selector, exclusions, override}`; `alerts.yaml` dispatches by key and re-renders the latency rules once per `latencyOverrides` entry. Alert names (`Astrolavos` + PascalCase key) are hardcoded in each template, not derived, so tests can pin them. Keep the key lists in `alerts.yaml` in sync when adding a rule, and give every new values key a `# --` helm-docs comment.
```

Also add `alerts` to the test-suite list in the "Chart unit tests" paragraph: change `(`workload`, `config`, `service`, `serviceaccount`, `servicemonitor`, `scaling` for PDB+HPA, `ingress`, `dashboards`, `metadata` for cross-resource naming/labels)` to `(`workload`, `config`, `service`, `serviceaccount`, `servicemonitor`, `alerts`, `scaling` for PDB+HPA, `ingress`, `dashboards`, `metadata` for cross-resource naming/labels)`.

- [ ] **Step 5: Final checks and commit**

Run: `make helm-test && helm lint --strict deploy/kubernetes && git status --short`
Expected: all green; only the four doc files changed in this task (plus nothing left in `/tmp` that matters).

```bash
git add deploy/kubernetes/README.md README.md AGENTS.md
git commit -m "docs(helm): document the alerting rules and the _alerts.tpl pattern"
```

---

## Self-review

- **Spec coverage:** values API (T1), apiVersion switch + fail + capabilities gate (T1), resource metadata/labels/annotations/namespace/interval (T1), reachability rules incl. the `<=` fleet guard (T1), latency rules + quantile helper + `unless` exclusions + overrides with merged thresholds and `override` label (T2), regression rule off by default with factor/baseline/floor (T2), self-health with `job` default/override and `absent` watchdog (T3), `additionalRules` with `tpl` (T3), empty-group / no-document behaviour (T3), 12 spec test cases (T1–T3), promtool/vmalert validation and the three doc updates (T4). Spec deviation: `AstrolavosTargetsDown` gains `or absent(up{job=…})` so a fully missing DaemonSet also fires; the spec is updated to match in the same PR.
- **Placeholder scan:** none.
- **Type consistency:** dict keys `context/rule/selector/exclusions/override` are identical in `alerts.yaml` and every `_alerts.tpl` template; `astrolavos.alerts.quantile` takes `quantile/metric/selector/offset` in T2 and both call sites; `astrolavos.alerts.percent` defined in T1 and used in T1 and T3.
