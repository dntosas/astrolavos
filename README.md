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
    prober: httptrace
    tag: mytag
    retries: 3
```
- `domain`: the IP or domain name that will be used
- `interval`: the time period in seconds that will be used between the different probe attempts. Default is 5 seconds.
- `prober`: the type of the measurement. For now we support `httptrace` and `tcp`. The default is `httptrace`.
- `https`: in case of `httptrace` measurement if we will use TLS or not.
    - `httpTrace`, are measurements that track all phases of HTTP calls and they are based on [httptrace](https://golang.google.cn/pkg/net/http/httptrace/) golang library. This was inspired by [httpstat](https://github.com/reorx/httpstat) cli tool.
    - `tcp`, are measurements that try to open a simple TCP connection.
- `tag`: the tags that you might want to attach to Prometheus metrics that astrolavos is exposing.
- `retries`: how many times to attempt the probe. Default is 1 (single attempt, no retries). For production environments experiencing cluster scaling events, consider increasing to 5+ to handle transient failures gracefully with exponential backoff.

### Intelligent Retry Logic (Optional)
Astrolavos implements **exponential backoff retry logic** when `retries` is set to 2 or higher. When a probe fails, it automatically retries with increasing delays (100ms, 200ms, 400ms, etc.) before reporting an error. This can eliminate false positives during cluster scaling events or temporary network disruptions.

**Note:** The default is `retries: 1` (no retry) for immediate failure detection. Increase retries if you need resilience during operational events.

For details on configuring retries to handle cluster changes smoothly, see [Smooth Cluster Scaling Guide](./docs/SMOOTH_CLUSTER_SCALING.md).

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
