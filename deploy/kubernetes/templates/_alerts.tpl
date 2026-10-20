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
