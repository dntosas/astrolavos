# astrolavos

![Version: 1.5.1](https://img.shields.io/badge/Version-1.5.1-informational?style=flat-square) ![AppVersion: 1.5.1](https://img.shields.io/badge/AppVersion-1.5.1-informational?style=flat-square)

A Helm Chart for deploying Astrolavos Latency Measuring Tool

**Homepage:** <https://github.com/dntosas/astrolavos>

## Maintainers

| Name | Email | Url |
| ---- | ------ | --- |
| Jim Ntosas |  |  |
| Andreas Strikos |  |  |

## Source Code

* <https://github.com/dntosas/astrolavos>

## Requirements

| Repository | Name | Version |
|------------|------|---------|
| https://charts.bitnami.com/bitnami | common | 2.41.0 |

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| affinity | object | `{}` |  |
| alerts.additionalLabels | object | `{}` | Extra labels on the rule resource, e.g. `release: kube-prometheus-stack` so the operator's ruleSelector picks it up |
| alerts.additionalRules | list | `[]` | Raw alerting rules appended verbatim in an `astrolavos.custom` group. Rendered as a template. |
| alerts.annotations | object | `{}` | Extra annotations on the rule resource |
| alerts.enabled | bool | `false` | Create a PrometheusRule or VMRule carrying the built-in alerting rules |
| alerts.interval | string | `""` | Rule group evaluation interval. Omitted when empty (operator default). |
| alerts.job | string | `""` | Value of the `job` label on astrolavos scrape targets, used by the self-health rules. Defaults to the chart fullname, which is what the chart's ServiceMonitor produces. |
| alerts.kind | string | `"PrometheusRule"` | Rule CRD to render: `PrometheusRule` (monitoring.coreos.com/v1) or `VMRule` (operator.victoriametrics.com/v1beta1). Only rendered when the matching API group is present in the cluster capabilities. |
| alerts.labels | object | `{}` | Labels merged into every alert (e.g. `team`, `env`). Each rule's own `severity` wins on conflict. |
| alerts.latencyOverrides | list | `[]` | Per-target overrides for the latency rules. Each entry re-renders the enabled latency rules with `selector` (raw PromQL matchers, no braces) baked in and an `override: <name>` label; the global latency rules exclude the matched series. Other keys are latency rule names whose `threshold`/`for`/`severity` override the global ones, e.g. `[{name: internal, selector: 'tag="internal"', tcpConnectLatencyHigh: {threshold: 0.01}}]`. |
| alerts.namespace | string | `""` | Namespace for the rule resource (defaults to the release namespace) |
| alerts.rules.connectLatencyRegression | object | `{"baseline":"1d","enabled":false,"factor":3,"floor":0.005,"for":"30m","quantile":0.99,"severity":"warning"}` | p`quantile` TCP connect latency above `factor` times its own value `baseline` ago and above `floor` seconds: a relative regression (e.g. traffic silently went cross-AZ) that absolute thresholds miss. Needs `baseline` of history; off by default. |
| alerts.rules.dnsLatencyHigh | object | `{"enabled":true,"for":"15m","quantile":0.99,"severity":"warning","threshold":0.1}` | p`quantile` DNS resolution latency (seconds) above `threshold`: client view of the resolver chain |
| alerts.rules.dnsResolutionFailing | object | `{"enabled":true,"for":"5m","severity":"critical","threshold":0.05}` | Ratio of probes failing with `error="dns_error"` above `threshold` (5m window): client-side DNS resolution is failing |
| alerts.rules.endpointStalled | object | `{"enabled":true,"for":"5m","severity":"warning"}` | An endpoint on a still-scraped pod that produced probes in the last hour but none in the last 10m: that prober loop is stuck and the other alerts are blind for it (terminated pods are excluded) |
| alerts.rules.httpNon2xxRatioHigh | object | `{"enabled":true,"for":"10m","severity":"warning","threshold":0.1}` | Ratio of httpTrace probes answered with a non-2xx/3xx status above `threshold`: reachable but unhealthy peer (5xx, proxy 403). Failed probes are excluded, see `probeFailureRatioHigh`. |
| alerts.rules.probeFailingOnSingleNode | object | `{"enabled":true,"for":"10m","severity":"warning","threshold":0.5}` | One pod's (node's) error ratio above `threshold` while the fleet-wide ratio is at or below `probeFailureRatioHigh.threshold`: that node's network is broken. Only meaningful as a DaemonSet. |
| alerts.rules.probeFailureRatioHigh | object | `{"enabled":true,"for":"10m","severity":"critical","threshold":0.1}` | Probe error ratio per (domain, tag, prober_type) above `threshold` for `for`: the network path is down for the whole cluster |
| alerts.rules.probeStalled | object | `{"enabled":true,"for":"10m","severity":"warning"}` | A scraped astrolavos pod produced no probe at all for `for`: stuck goroutines or an empty endpoint list |
| alerts.rules.staleConnections | object | `{"enabled":true,"for":"5m","severity":"critical","threshold":0.1}` | Timeouts on reused (keep-alive) connections above `threshold` while fresh connections to the same target succeed: established flows are black-holed, e.g. by a CNI/egress-gateway rollout. Needs at least one endpoint with `reuseConnection: true`. |
| alerts.rules.streamReconnectBurst | object | `{"enabled":true,"for":"1m","severity":"critical","threshold":0.5}` | Fraction of pods whose stream connection was severed within 5m above `threshold`: a dataplane event (CNI agent restart, egress gateway reselection) killed established flows fleet-wide |
| alerts.rules.streamReconnecting | object | `{"enabled":true,"for":"5m","severity":"warning","threshold":3}` | One pod's websocket/tcpStream connection to an endpoint reconnected more than `threshold` times in 10m: the long-lived connection cannot stay up on that path |
| alerts.rules.targetsDown | object | `{"enabled":true,"for":"10m","severity":"critical","threshold":0.2}` | Fraction of astrolavos scrape targets with `up == 0` above `threshold`, or no targets at all: the prober itself is down |
| alerts.rules.tcpConnectLatencyHigh | object | `{"enabled":true,"for":"15m","quantile":0.99,"severity":"warning","threshold":0.1}` | p`quantile` TCP connect latency (seconds) above `threshold`: RTT or SYN loss on the path, independent of the application |
| alerts.rules.tlsHandshakeLatencyHigh | object | `{"enabled":true,"for":"15m","quantile":0.99,"severity":"warning","threshold":0.5}` | p`quantile` TLS handshake latency (seconds) above `threshold`: node CPU starvation or an egress middlebox terminating TLS |
| alerts.rules.totalLatencyHigh | object | `{"enabled":true,"for":"15m","quantile":0.99,"severity":"warning","threshold":1}` | p`quantile` end-to-end request latency (seconds) above `threshold` |
| autoscaling.enabled | bool | `true` |  |
| autoscaling.maxReplicas | string | `"5"` |  |
| autoscaling.minReplicas | string | `"2"` |  |
| autoscaling.targetCPU | int | `80` |  |
| autoscaling.targetMemory | int | `80` |  |
| commonAnnotations | object | `{}` |  |
| commonLabels | object | `{}` |  |
| config.application.logLevel | string | `"INFO"` | Log level of the astrolavos process (DEBUG, INFO, WARN, ERROR). Rendered as the `ASTROLAVOS_LOG_LEVEL` env var unless `extraEnvVars` already defines it. |
| config.enabled | bool | `true` |  |
| config.endpoints[0].domain | string | `"www.httpbin.org"` |  |
| config.endpoints[0].https | bool | `true` |  |
| config.endpoints[0].interval | string | `"10s"` |  |
| config.endpoints[0].prober | string | `"httpTrace"` |  |
| config.endpoints[0].retries | int | `1` |  |
| config.endpoints[0].reuseConnection | bool | `false` |  |
| config.endpoints[0].tag | string | `"example"` |  |
| config.endpoints[0].timeout | string | `"10s"` |  |
| config.endpoints[1].domain | string | `"{{ include \"common.names.fullname\" . }}/echo"` |  |
| config.endpoints[1].https | bool | `false` |  |
| config.endpoints[1].interval | string | `"5s"` |  |
| config.endpoints[1].prober | string | `"websocket"` |  |
| config.endpoints[1].tag | string | `"internal-stream"` |  |
| config.endpoints[1].timeout | string | `"5s"` |  |
| config.metrics.histogramBuckets | list | `[0.001,0.005,0.01,0.025,0.05,0.1,0.25,0.5,1,2.5,5]` | Latency histogram bucket upper bounds in seconds, shared by all `astrolavos_*_latency_seconds` histograms. Must be strictly increasing. Overridable at runtime via the `ASTROLAVOS_HISTOGRAM_BUCKETS` env var (comma-separated). |
| containerPorts.echo | int | `3001` | TCP echo listener port, rendered only when `echo.enabled` |
| containerPorts.http | int | `3000` |  |
| containerSecurityContext.capabilities.drop[0] | string | `"ALL"` |  |
| containerSecurityContext.enabled | bool | `true` |  |
| containerSecurityContext.readOnlyRootFilesystem | bool | `true` |  |
| containerSecurityContext.runAsGroup | int | `65532` |  |
| containerSecurityContext.runAsNonRoot | bool | `true` |  |
| containerSecurityContext.runAsUser | int | `65532` |  |
| containerSecurityContext.seccompProfile.type | string | `"RuntimeDefault"` |  |
| datadogDashboard.annotations | object | `{}` | Additional annotations for the DatadogDashboard resource |
| datadogDashboard.enabled | bool | `false` | Deploy Datadog dashboard as a DatadogDashboard CRD (requires Datadog Operator v0.6+) |
| datadogDashboard.extraTags | list | `[]` | Extra tags to add to the dashboard (in addition to app:astrolavos) e.g. ["team:platform", "env:production"] |
| datadogDashboard.extraTemplateVariables | list | `[{"defaults":["*"],"name":"cluster","prefix":"kube_cluster_name"},{"defaults":["*"],"name":"region","prefix":"region"}]` | Additional template variables for the dashboard e.g. extraTemplateVariables:   - name: cluster     prefix: kube_cluster_name     defaults:       - "*" |
| datadogDashboard.namespace | string | `""` | Override the namespace for the DatadogDashboard resource (defaults to release namespace) |
| datadogDashboard.title | string | `"Astrolavos Network Metrics"` | Dashboard title (supports tpl rendering, e.g. "{{ include \"common.names.fullname\" . }} Network Metrics") |
| deployAsDaemonSet | bool | `true` |  |
| echo.enabled | bool | `true` | Run the TCP echo listener on `containerPorts.echo` and expose it as Service port `echo`. When false the listener is disabled (`ASTROLAVOS_ECHO_PORT=0`) and the ports are not rendered. |
| extraArgs | object | `{}` |  |
| extraEnvVars | object | `{}` | Extra environment variables as a name/value map. Takes precedence over `config.application.logLevel` for `ASTROLAVOS_LOG_LEVEL`. |
| extraVolumeMounts | list | `[]` | Optionally specify extra list of additional volumeMounts for the Redis&reg; master container(s) |
| extraVolumes | list | `[]` | Optionally specify extra list of additional volumes for the Redis&reg; master pod(s) |
| fullnameOverride | string | `"astrolavos"` |  |
| global.imagePullSecrets | list | `[]` |  |
| global.imageRegistry | string | `""` |  |
| grafanaDashboard.annotations | object | `{}` | Additional annotations for the dashboard ConfigMap |
| grafanaDashboard.enabled | bool | `false` | Deploy Grafana dashboard as a ConfigMap for sidecar autodiscovery |
| grafanaDashboard.folder | string | `""` | Grafana folder to place the dashboard in (annotation: grafana_folder) |
| grafanaDashboard.namespace | string | `""` | Override the namespace for the Grafana dashboard ConfigMap (defaults to release namespace) |
| grafanaDashboard.sidecarLabel | string | `"grafana_dashboard"` | Label used by Grafana sidecar to discover dashboard ConfigMaps |
| grafanaDashboard.sidecarLabelValue | string | `"1"` | Value for the sidecar discovery label |
| hostNetwork | bool | `false` |  |
| image.pullPolicy | string | `"Always"` |  |
| image.pullSecrets | list | `[]` | Image pull secrets for this image (names or `{name: ...}` entries). Merged with `global.imagePullSecrets`. |
| image.registry | string | `"ghcr.io"` |  |
| image.repository | string | `"dntosas/astrolavos"` |  |
| image.tag | string | `"v1.5.1"` |  |
| ingress.annotations | object | `{}` |  |
| ingress.apiVersion | string | `""` |  |
| ingress.enabled | bool | `false` |  |
| ingress.extraHosts | list | `[]` |  |
| ingress.extraPaths | list | `[]` |  |
| ingress.extraRules | list | `[]` |  |
| ingress.extraTls | list | `[]` |  |
| ingress.hostname | string | `"Astrolavos.local"` |  |
| ingress.ingressClassName | string | `""` |  |
| ingress.path | string | `"/"` |  |
| ingress.pathType | string | `"ImplementationSpecific"` |  |
| ingress.secrets | list | `[]` |  |
| ingress.selfSigned | bool | `false` |  |
| ingress.tls | bool | `false` |  |
| initContainers | list | `[]` |  |
| lifecycleHooks.preStop.httpGet.path | string | `"/prestop"` |  |
| lifecycleHooks.preStop.httpGet.port | string | `"http"` |  |
| livenessProbe.enabled | bool | `true` |  |
| livenessProbe.failureThreshold | int | `3` |  |
| livenessProbe.initialDelaySeconds | int | `0` |  |
| livenessProbe.periodSeconds | int | `10` |  |
| livenessProbe.successThreshold | int | `1` |  |
| livenessProbe.timeoutSeconds | int | `5` |  |
| minReadySeconds | int | `10` |  |
| nameOverride | string | `""` |  |
| nodeAffinityPreset.key | string | `""` |  |
| nodeAffinityPreset.type | string | `""` |  |
| nodeAffinityPreset.values | list | `[]` |  |
| nodeSelector | object | `{}` |  |
| pdb.create | bool | `true` |  |
| pdb.maxUnavailable | int | `0` |  |
| pdb.minAvailable | string | `"50%"` |  |
| podAffinityPreset | string | `""` |  |
| podAnnotations | object | `{}` |  |
| podAntiAffinityPreset | string | `"soft"` |  |
| podLabels | object | `{}` |  |
| podSecurityContext.enabled | bool | `true` |  |
| podSecurityContext.fsGroup | int | `1001` |  |
| priorityClassName | string | `""` |  |
| readinessProbe.enabled | bool | `true` |  |
| readinessProbe.failureThreshold | int | `1` |  |
| readinessProbe.initialDelaySeconds | int | `0` |  |
| readinessProbe.periodSeconds | int | `5` |  |
| readinessProbe.successThreshold | int | `1` |  |
| readinessProbe.timeoutSeconds | int | `3` |  |
| replicaCount | int | `1` |  |
| resources.limits.cpu | string | `"100m"` |  |
| resources.limits.memory | string | `"256Mi"` |  |
| resources.requests.cpu | string | `"50m"` |  |
| resources.requests.memory | string | `"64Mi"` |  |
| revisionHistoryLimit | int | `3` |  |
| schedulerName | string | `""` |  |
| service.annotations | object | `{}` |  |
| service.enabled | bool | `true` |  |
| service.externalTrafficPolicy | string | `"Cluster"` |  |
| service.extraPorts | list | `[]` |  |
| service.internalTrafficPolicy | string | `"Cluster"` |  |
| service.loadBalancerSourceRanges | list | `[]` |  |
| service.nodePorts.http | string | `""` |  |
| service.ports.echo | int | `3001` | Service port for the TCP echo listener (targets container port `echo`); rendered only when `echo.enabled` |
| service.ports.http | int | `3000` |  |
| service.sessionAffinity | string | `"None"` |  |
| service.sessionAffinityConfig | object | `{}` |  |
| service.trafficDistribution | string | `"PreferSameZone"` | Service spec `trafficDistribution`. Leave empty to omit the field. |
| service.type | string | `"ClusterIP"` |  |
| serviceAccount.annotations | object | `{}` |  |
| serviceAccount.automountServiceAccountToken | bool | `false` |  |
| serviceAccount.create | bool | `true` |  |
| serviceAccount.name | string | `""` |  |
| serviceMonitor.additionalLabels | object | `{}` | Additional labels that can be used so ServiceMonitor resource(s) can be discovered by Prometheus |
| serviceMonitor.enabled | bool | `true` | Create ServiceMonitor resource(s) for scraping metrics using PrometheusOperator |
| serviceMonitor.honorLabels | bool | `false` | Specify honorLabels parameter to add the scrape endpoint |
| serviceMonitor.interval | string | `"30s"` | The interval at which metrics should be scraped |
| serviceMonitor.metricRelabelings | list | `[]` | Metrics RelabelConfigs to apply to samples before ingestion. |
| serviceMonitor.namespace | string | `""` | The namespace in which the ServiceMonitor will be created |
| serviceMonitor.podTargetLabels | list | `[]` | Labels from the Kubernetes pod to be transferred to the created metrics |
| serviceMonitor.relabellings | list | `[]` | Metrics RelabelConfigs to apply to samples before scraping. |
| serviceMonitor.scrapeTimeout | string | `""` | The timeout after which the scrape is ended |
| startupProbe.enabled | bool | `true` |  |
| startupProbe.failureThreshold | int | `15` |  |
| startupProbe.initialDelaySeconds | int | `5` |  |
| startupProbe.periodSeconds | int | `2` |  |
| startupProbe.successThreshold | int | `1` |  |
| startupProbe.timeoutSeconds | int | `3` |  |
| terminationGracePeriodSeconds | int | `40` |  |
| tolerations | list | `[]` |  |
| topologySpreadConstraints | list | `[]` |  |
| updateStrategy.rollingUpdate.maxUnavailable | string | `"25%"` |  |
| updateStrategy.type | string | `"RollingUpdate"` |  |

----------------------------------------------
Autogenerated from chart metadata using [helm-docs v1.11.0](https://github.com/norwoodj/helm-docs/releases/v1.11.0)
