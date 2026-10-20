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
