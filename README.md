<h1 align="center" style="margin-top: 0px;">Astrolavos</h1>

<p align="center" style="margin-bottom: 0px !important;">
  <img width="250" src="https://user-images.githubusercontent.com/15010919/216572877-c5f5dd29-a0e6-40ca-8bf8-e28be7efcfa6.png" alt="Astrolavos logo" align="center">
</p>

<p align="center" >Measure Latencies and Network Behaviors between different endpoints and protocols</p>

<div align="center" >

[![CI](https://github.com/dntosas/astrolavos/actions/workflows/go-ci.yml/badge.svg?branch=main)](https://github.com/dntosas/astrolavos/actions/workflows/go-ci.yml) | [![Go Report](https://goreportcard.com/badge/github.com/dntosas/astrolavos)](https://goreportcard.com/badge/github.com/dntosas/astrolavos) | [![Go Release](https://github.com/dntosas/astrolavos/actions/workflows/go-release.yml/badge.svg)](https://github.com/dntosas/astrolavos/actions/workflows/go-release.yml) | [![e2e Tests](https://github.com/dntosas/astrolavos/actions/workflows/e2e.yml/badge.svg)](https://github.com/dntosas/astrolavos/actions/workflows/e2e.yml)

</div>

Astrolavos (αστρολάβος) is a tool built to measure latencies and network behaviours between different endpoints.

Given an endpoint astrolavos can run different kind of measurements towards it and expose the metrics in a premetheus format or send them to a Prometheus push gateway.

Astrolavos come from the Greek work [αστρολάβος](https://el.wikipedia.org/wiki/%CE%91%CF%83%CF%84%CF%81%CE%BF%CE%BB%CE%AC%CE%B2%CE%BF%CF%82) which was a tool for the sailors and astronomers to perform various measurements.

## Why Yet Another Measuring Tool
Some might ask why do we need another measuring tool? Aren't there enough out there? The honest answer is yes there are enough out there, probably more than is needed.
We couldn't find though what we needed, and initially we needed something that would break the latencies in a HTTP request in similar fashion like [httpstat](https://github.com/reorx/httpstat). We started with httptrace measuremnts and we thought this might be used for any measurements really. So here we are with yet another measurement tool that we thing might be useful for the community.

## How Does It Work
Astrolavos is a basically a loop that spawns go routines to execute the different measurements to the different endpoints. You can specify an endpoint that you want to measure using the config file that astrolavos reads on boot time. The config file is in a yaml format, an example can be found [here](./examples/config.yaml).
Each endpoint entry has the following structure:
```
  - domain: "www.httpbin.org"
    interval: 5s
    https: true
    prober: httpTrace
    tag: mytag
    retries: 3
```
- `domain`: the IP or domain name that will be used
- `interval`: the time period in seconds that will be used between the different probe attempts. Default is 5 seconds.
- `prober`: the type of the measurement: `httpTrace`, `tcp`, `websocket` or `tcpStream` (case-sensitive). The default is `httpTrace`.
- `https`: for `httpTrace` (http/https) and `websocket` (ws/wss) measurements, whether to use TLS.
    - `httpTrace`, are measurements that track all phases of HTTP calls and they are based on [httptrace](https://golang.google.cn/pkg/net/http/httptrace/) golang library. This was inspired by [httpstat](https://github.com/reorx/httpstat) cli tool. One request per `interval`.
    - `tcp`, are measurements that try to open a simple TCP connection. One dial per `interval`.
    - `websocket`, holds **one** WebSocket open to `domain` (e.g. `astrolavos/echo`) for as long as it lives and sends a ping every `interval`, expecting the pong within `timeout`. Any RFC 6455 server answers pings, so any WebSocket endpoint is a valid target. A failed heartbeat closes the connection and reconnects with backoff.
    - `tcpStream`, holds **one** plain TCP connection to `domain` (`host:port`) and sends a newline-framed `ping <seq>` every `interval`, expecting the same line echoed back within `timeout`. Needs an echo peer such as the astrolavos TCP echo listener (`ASTROLAVOS_ECHO_PORT`).
- `tag`: the tags that you might want to attach to Prometheus metrics that astrolavos is exposing.
- `retries`: how many times to attempt the probe. Default is 1 (single attempt, no retries). For production environments experiencing cluster scaling events, consider increasing to 5+ to handle transient failures gracefully with exponential backoff.
- `reuseConnection`: keep the HTTP connection open between probes (`httpTrace` only). Default `false`, i.e. every probe opens a new connection. Set it to `true` on at least one external endpoint to detect stale connections (see below).
- `timeout`: deadline for a single probe attempt (DNS, connect, TLS, request and body for `httpTrace`; the dial for `tcp`). Default `10s`. Each retry gets its own deadline. A probe that hits a black-holed connection fails with `error="timeout"` instead of hanging.

### Histogram Buckets
All `astrolavos_*_latency_seconds` histograms share one set of bucket upper bounds (in seconds). The default is `[0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5]`; trimming the list reduces the number of time series per endpoint, extending it improves quantile resolution. The list must be strictly increasing. Configure it either in the config file:
```
metrics:
  histogramBuckets: [0.01, 0.1, 1]
```
or via the `ASTROLAVOS_HISTOGRAM_BUCKETS` env var as a comma-separated list (`ASTROLAVOS_HISTOGRAM_BUCKETS="0.01,0.1,1"`). The env var takes precedence over the config file, which takes precedence over the built-in default. In the Helm chart this is `config.metrics.histogramBuckets`.

### Intelligent Retry Logic (Optional)
Astrolavos implements **exponential backoff retry logic** when `retries` is set to 2 or higher. When a probe fails, it automatically retries with increasing delays (100ms, 200ms, 400ms, etc.) before reporting an error. This can eliminate false positives during cluster scaling events or temporary network disruptions.

**Note:** The default is `retries: 1` (no retry) for immediate failure detection. Increase retries if you need resilience during operational events such as node rollouts or cluster scaling.

### Detecting Stale Connections
A CNI or egress-gateway rollout can silently break *established* connections while new ones keep working: packets of existing flows are dropped without a reset and the application hangs until its own timeout. A prober that opens a fresh connection every time never sees this. Set `reuseConnection: true` on an endpoint that crosses the path you care about (for example your egress target) and astrolavos will keep the connection between probes; every metric carries a `conn_reused="true|false"` label, so a probe that times out on a reused connection while fresh connections succeed is exactly the signature of that failure. The Helm chart ships the matching `AstrolavosStaleConnections` alert, and the Grafana dashboard has a "Connection" selector to look at keep-alive and new connections separately.

HTTP keep-alive is opportunistic (peers close idle connections after seconds), so for a connection that is *guaranteed* to be long-lived use the stream probers. A `websocket` or `tcpStream` endpoint keeps a single connection open for its whole life and heartbeats it: every heartbeat is a `conn_reused="true"` request, every (re)connect a `conn_reused="false"` one, so the same alert applies, and two extra metrics describe the connection itself: `astrolavos_stream_connection_age_seconds` (0 while disconnected) and `astrolavos_stream_reconnects_total{reason}`. The chart alerts on a stream that keeps reconnecting (`AstrolavosStreamReconnecting`) and, more importantly, on more than half of the pods losing their stream within the same 5 minutes (`AstrolavosStreamReconnectBurst`): established flows do not die fleet-wide on their own, a dataplane event did it.

Every astrolavos instance can be the peer: `GET /echo` on the HTTP port upgrades to a WebSocket that echoes messages and answers pings, and `ASTROLAVOS_ECHO_PORT` (default `3001`, `0` disables) serves a plain TCP echo for `tcpStream`. The Helm chart's default config probes the release's own `/echo` through its Service; point a second cluster's astrolavos at it through your egress path to watch the path your workloads use.

### Running Modes
Astrolavos can run either as a server mode, where we expose `latency` endpoint that another astrolavos deployment can target from different cluster and `metrics` endpoint that we expose our metrics in prometheus format.

Besides server mode astrolavos can also run in oneoff mode, where it will run given measurements once, send the metrics to a push gateway and exit. This can be useful for a cronjob setup.

## How To Run
After you have built the binary(you can use `make build-local` for local use) you can run it with just specifying the path of the config file you have `./astrolavos -config-path ./examples`.
Astrolavos support also an oneoff mode which you can use by specifying `-oneoff` flag.
For more info on flags you can use `-h` flag.
```
$> ./astrolavos -h
Usage of ./bin/astrolavos:
  -config-path string
        Specify the path of the config file. (default "/etc/astrolavos")
  -oneoff
        Run the probe measurements one time and exit.
```

Container images are published to `ghcr.io/dntosas/astrolavos` for `linux/amd64` and `linux/arm64` with every tagged release.

## Deploy On Kubernetes
A Helm chart lives in [`deploy/kubernetes`](./deploy/kubernetes) and is published from this repository:

```
helm repo add astrolavos https://dntosas.github.io/astrolavos
helm repo update
helm install astrolavos astrolavos/astrolavos \
  --namespace astrolavos --create-namespace \
  --set 'config.endpoints[0].domain=www.httpbin.org' \
  --set 'config.endpoints[0].https=true' \
  --set 'config.endpoints[0].tag=example'
```

By default the chart deploys a DaemonSet (one prober per node), a Service, a PodDisruptionBudget and a Prometheus Operator `ServiceMonitor`. Set `deployAsDaemonSet=false` for a Deployment with HPA instead. Optional Grafana and Datadog dashboards ship with the chart, as does a curated set of alerting rules (probe failure ratio, single-node failures, DNS failures, connect/DNS/TLS/total latency, prober self-health) rendered as a `PrometheusRule` or a `VMRule`: set `alerts.enabled=true` and pick `alerts.kind`. Per-target latency thresholds go in `alerts.latencyOverrides`. All values are documented in the [chart README](./deploy/kubernetes/README.md).

## Versioning And Compatibility
Astrolavos follows [Semantic Versioning](https://semver.org). The application and the Helm chart are released in lockstep (chart `1.x.y` ships `appVersion` `1.x.y`). Within a major version the following are stable and only change in a backwards-compatible way:

- the `config.yaml` schema and its defaults
- the `ASTROLAVOS_*` environment variables
- Prometheus metric names and label sets (adding a label is considered backwards compatible; `conn_reused` was added to every probe metric in 1.5)
- the HTTP endpoints `/metrics`, `/live`, `/ready`, `/prestop`, `/latency`, `/status` and `/echo`
- Helm chart values (removals are announced one minor release ahead)

Breaking changes are marked with a `!` in the commit subject and listed under "Breaking changes" in the release notes.

Releases are cut automatically: a pull request merged into `main` with a `release:patch`, `release:minor` or `release:major` label bumps the chart and image version on `main`, tags it, and publishes the binaries, images and chart.

## Verifying Releases
Releases after `v1.0.0` are signed keylessly with [Sigstore cosign](https://docs.sigstore.dev) from the tagged release workflow and carry [GitHub build provenance attestations](https://docs.github.com/en/actions/security-for-github-actions/using-artifact-attestations). No long-lived signing key exists. To verify an image:

```
cosign verify ghcr.io/dntosas/astrolavos:vX.Y.Z \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp '^https://github.com/dntosas/astrolavos/\.github/workflows/go-release\.yml@refs/tags/v'
```

or, with the GitHub CLI: `gh attestation verify oci://ghcr.io/dntosas/astrolavos:vX.Y.Z --owner dntosas`. See [SECURITY.md](./SECURITY.md) for archive verification, SBOMs and how to report a vulnerability.
