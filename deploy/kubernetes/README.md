# astrolavos

![Version: 0.3.0](https://img.shields.io/badge/Version-0.3.0-informational?style=flat-square)

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
| https://charts.bitnami.com/bitnami | common | 2.x.x |

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
| containerPorts.http | int | `3000` |  |
| containerSecurityContext.capabilities.drop[0] | string | `"ALL"` |  |
| containerSecurityContext.enabled | bool | `true` |  |
| containerSecurityContext.readOnlyRootFilesystem | bool | `true` |  |
| containerSecurityContext.runAsGroup | int | `65532` |  |
| containerSecurityContext.runAsNonRoot | bool | `true` |  |
| containerSecurityContext.runAsUser | int | `65532` |  |
| containerSecurityContext.seccompProfile.type | string | `"RuntimeDefault"` |  |
| extraArgs | object | `{}` |  |
| extraEnvVars.ASTROLAVOS_LOG_LEVEL | string | `"INFO"` |  |
| extraVolumeMounts | list | `[]` | Optionally specify extra list of additional volumeMounts for the Redis&reg; master container(s) |
| extraVolumes | list | `[]` | Optionally specify extra list of additional volumes for the Redis&reg; master pod(s) |
| fullnameOverride | string | `"astrolavos"` |  |
| global.imagePullSecrets | list | `[]` |  |
| global.imageRegistry | string | `""` |  |
| hostNetwork | bool | `false` |  |
| image.pullPolicy | string | `"Always"` |  |
| image.pullSecrets | object | `{}` |  |
| image.registry | string | `"ghcr.io"` |  |
| image.repository | string | `"dntosas/astrolavos"` |  |
| image.tag | string | `"v0.3.0"` |  |
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
| livenessProbe.enabled | bool | `true` |  |
| livenessProbe.failureThreshold | int | `3` |  |
| livenessProbe.initialDelaySeconds | int | `1` |  |
| livenessProbe.periodSeconds | int | `10` |  |
| livenessProbe.successThreshold | int | `1` |  |
| livenessProbe.timeoutSeconds | int | `5` |  |
| minReadySeconds | int | `0` |  |
| nameOverride | string | `""` |  |
| nodeAffinityPreset.key | string | `""` |  |
| nodeAffinityPreset.type | string | `""` |  |
| nodeAffinityPreset.values | list | `[]` |  |
| nodeSelector | object | `{}` |  |
| pdb.create | bool | `false` |  |
| pdb.maxUnavailable | int | `0` |  |
| pdb.minAvailable | int | `1` |  |
| podAffinityPreset | string | `""` |  |
| podAnnotations | object | `{}` |  |
| podAntiAffinityPreset | string | `"soft"` |  |
| podLabels | object | `{}` |  |
| podSecurityContext.enabled | bool | `true` |  |
| podSecurityContext.fsGroup | int | `1001` |  |
| priorityClassName | string | `""` |  |
| readinessProbe.enabled | bool | `true` |  |
| readinessProbe.failureThreshold | int | `3` |  |
| readinessProbe.initialDelaySeconds | int | `1` |  |
| readinessProbe.periodSeconds | int | `10` |  |
| readinessProbe.successThreshold | int | `1` |  |
| readinessProbe.timeoutSeconds | int | `5` |  |
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
| terminationGracePeriodSeconds | string | `""` |  |
| tolerations | list | `[]` |  |
| topologySpreadConstraints | list | `[]` |  |
| updateStrategy.rollingUpdate.maxUnavailable | int | `1` |  |
| updateStrategy.type | string | `"RollingUpdate"` |  |

----------------------------------------------
Autogenerated from chart metadata using [helm-docs v1.11.0](https://github.com/norwoodj/helm-docs/releases/v1.11.0)
