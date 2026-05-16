# astrolavos

![Version: 0.18.0](https://img.shields.io/badge/Version-0.18.0-informational?style=flat-square)

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
| https://charts.bitnami.com/bitnami | common | 2.24.0 |

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| affinity | object | `{}` |  |
| autoscaling.enabled | bool | `true` |  |
| autoscaling.maxReplicas | string | `"5"` |  |
| autoscaling.minReplicas | string | `"2"` |  |
| autoscaling.targetCPU | int | `80` |  |
| autoscaling.targetMemory | int | `80` |  |
| commonAnnotations | object | `{}` |  |
| commonLabels | object | `{}` |  |
| config.application.logLevel | string | `"INFO"` |  |
| config.enabled | bool | `true` |  |
| config.endpoints[0].domain | string | `"www.httpbin.org"` |  |
| config.endpoints[0].https | bool | `true` |  |
| config.endpoints[0].interval | string | `"10s"` |  |
| config.endpoints[0].prober | string | `"httpTrace"` |  |
| config.endpoints[0].retries | int | `1` |  |
| config.endpoints[0].tag | string | `"example"` |  |
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
| extraArgs | object | `{}` |  |
| extraEnvVars.ASTROLAVOS_LOG_LEVEL | string | `"INFO"` |  |
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
| image.pullSecrets | object | `{}` |  |
| image.registry | string | `"ghcr.io"` |  |
| image.repository | string | `"dntosas/astrolavos"` |  |
| image.tag | string | `"v0.18.0"` |  |
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
