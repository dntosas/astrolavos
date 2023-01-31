# Astrolavos

[![CI](https://github.com/dntosas/astrolavos/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/dntosas/astrolavos/actions/workflows/ci.yml) | [![Go Report](https://goreportcard.com/badge/github.com/dntosas/astrolavos)](https://goreportcard.com/badge/github.com/dntosas/astrolavos) | [![Go Release](https://github.com/dntosas/astrolavos/actions/workflows/go-release.yml/badge.svg)](https://github.com/dntosas/astrolavos/actions/workflows/go-release.yml) | [![Helm Chart Release](https://github.com/dntosas/astrolavos/actions/workflows/helm-release.yml/badge.svg)](https://github.com/dntosas/astrolavos/actions/workflows/helm-release.yml)

![astrolavos](https://user-images.githubusercontent.com/15010919/215696467-82ef5d9b-8340-4f05-bc44-6e2fe0460e66.png)

Astrolavos (αστρολάβος) is a tool used to measure latencies and network behaviours between our different Kubernetes clusters.

Given an endpoint astrolavos can run different kind of measurements towards it and expose the metrics or send them to a push gateway.

Astrolavos come from the Greek work [αστρολάβος](https://el.wikipedia.org/wiki/%CE%91%CF%83%CF%84%CF%81%CE%BF%CE%BB%CE%AC%CE%B2%CE%BF%CF%82) which was a tool for the sailors and astronomers to perform various measurements.

The different kind of measurements that one can define in the config file are for now:
* httpTrace measurements, which are measurements that track all phases of HTTP calls and they are based on [httptrace](https://golang.google.cn/pkg/net/http/httptrace/) golang library. This was inspired by [httpstat](https://github.com/davecheney/httpstat) cli tool.
* tcp measurement, which are measurement that try to open a simple tcp connection.

Future work includes DNS and grpc measurements.

The tool can run either as a server mode, where we expose `latency` endpoint that another astrolavos deployment can target from different cluster and `metrics` endpoint that we expose our metrics in prometheus format.

Besides server mode astrolavos can also run in oneoff mode, where it will run given measurements once, send the metrics to a push gateway and exit. This can be useful for cronjobs for example.

You can specify the endpoint and measurements by having a config yaml file similar to the [one](./config.yaml.example) in root directory.
