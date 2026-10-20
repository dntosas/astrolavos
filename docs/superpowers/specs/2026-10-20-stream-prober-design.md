# Stale-connection detection, part 2: `websocket` and `tcpStream` probers

**Date:** 2026-10-20
**Status:** approved design
**Series:** part 2 of 3; ships in the same PR as part 1
(`2026-10-20-stale-connection-detection-design.md`). Part 3 (infrastructure
repo): echo target beyond the egress gateway, corroborating node/Cilium
alerts, Cilium-rollout rehearsal runbook.

## Problem

Part 1 lets a keep-alive HTTP probe notice that its pooled connection died,
but HTTP keep-alive is opportunistic: peers close idle connections after
seconds, so the pooled connection is often gone before the next probe. The
workloads that go stale during Cilium rollouts hold connections for minutes
or hours. Astrolavos needs a prober that models that: one connection, kept
open, exercised continuously, with a deterministic reaction when it stops
carrying bytes.

## Goals

- A long-lived connection per endpoint with a periodic heartbeat bounded by
  `timeout`; a heartbeat that does not return is a failure, not a hang.
- Detect "existing flow black-holed while new connections work" and "fleet
  reconnected at once" as distinct alerts.
- Any astrolavos instance can be the peer: WebSocket echo on the HTTP port,
  TCP echo on its own port.
- Reproduce the black hole deterministically in `go test` and in the Kind
  e2e (toxiproxy).

Non-goals: HTTP/2 or gRPC streams, payload integrity checks beyond the echo,
TLS client certificates.

## Config

```yaml
endpoints:
  - domain: astrolavos/echo       # host[:port]/path; scheme added from https
    prober: websocket             # or tcpStream (domain is host:port)
    interval: 5s                  # heartbeat period
    timeout: 5s                   # connect and heartbeat deadline
    https: false                  # websocket only: true -> wss://
    skipTLSVerification: false    # websocket only
    tag: internal-stream
```

- New `prober` values `websocket` and `tcpStream` (case-sensitive, like
  `httpTrace`). `websocket` builds `ws://`/`wss://` + domain; `tcpStream`
  dials `domain` as `host:port`.
- `interval` is the heartbeat period, `timeout` the deadline of one connect
  or one heartbeat (defaults unchanged: 5s / 10s).
- `retries` is ignored by stream probers (a failed heartbeat *is* the
  signal); a value above 1 logs a warning at startup.
- `tcpTimeout` is not used by stream probers.

## Behaviour (`internal/probers/stream.go`)

One engine, two dialers:

```go
type streamConn interface {
	Heartbeat(ctx context.Context) error // one round-trip
	Close() error
}

type streamDialer interface {
	Dial(ctx context.Context) (streamConn, connectTimings, error)
}
```

Loop per endpoint (goroutine, `wg.Done()` on exit):

1. **Connect** under `attemptContext`. Record `requests_total{conn_reused="false"}`;
   on failure `errors_total{conn_reused="false", error=<category>}`, set the
   age gauge to 0, sleep the backoff (100ms·2ⁿ, capped at `interval`), retry.
   On success record the connect phases into the latency histograms with
   `conn_reused="false"` (`websocket`: DNS/conn/TLS/gotconn/firstbyte/total
   from `httptrace` on the dial; `tcpStream`: conn and total = dial time),
   reset the backoff.
2. **Heartbeat** every `interval`, bounded by `attemptContext`:
   `requests_total{conn_reused="true"}`; RTT into
   `total_latency_seconds{conn_reused="true"}`; age gauge = now − connectedAt.
   On failure: `errors_total{conn_reused="true", error=<category>}`,
   `stream_reconnects_total{reason=<category>}`, age gauge 0, close, back to 1.
3. **One-off mode**: connect, one heartbeat, close, exit.
4. **Shutdown**: context cancellation closes the connection and exits; a
   `canceled` category is never counted as a reconnect.

`websocket`: `coder/websocket` `Dial` with an `http.Client` whose transport
honours `skipTLSVerification`, under `httptrace.WithClientTrace` reusing the
existing `tracePoint` handlers. After the dial, `Conn.CloseRead` runs the
reader that delivers pongs. Heartbeat = `Conn.Ping(ctx)`, which every RFC
6455 server must answer, so any WebSocket endpoint is a valid target. Close =
`StatusNormalClosure`.

`tcpStream`: `net.Dialer.DialContext`; heartbeat writes `ping <seq>\n` and
reads one line under the context deadline (`SetDeadline`), expecting the same
bytes back.

`prober_type` label values: `websocket`, `tcpstream`.

## Echo servers

- `GET /echo` on the HTTP port (`internal/handlers/echo.go`): accepts any
  origin, reads messages in a loop and writes them back with the same type;
  pings are answered by the library during `Read`. Closes on client close or
  read error. Always registered.
- TCP echo (`internal/machinery/echo.go`): listener on
  `ASTROLAVOS_ECHO_PORT` (default `3001`, `0` disables), `io.Copy(c, c)` per
  connection, each connection closed when the server context is cancelled;
  listener closed during graceful shutdown before the HTTP server.
- `config.Config.EchoPort`; `machinery.NewAstrolavos` gains the port.

## Metrics

Stream probes feed the existing metrics as described above, so
`AstrolavosStaleConnections` (part 1) covers streams without a new rule.
Two new collectors:

| Metric | Labels | Meaning |
|---|---|---|
| `astrolavos_stream_connection_age_seconds` (gauge) | `domain, tag, prober_type` | Age of the current connection at the last heartbeat; `0` while disconnected. |
| `astrolavos_stream_reconnects_total` (counter) | `domain, tag, prober_type, reason` | Connections dropped after a failed heartbeat; `reason` is the `CategorizeError` bucket. |

## Alerts (chart, new group `astrolavos.stream`)

**AstrolavosStreamReconnecting** (`alerts.rules.streamReconnecting`,
`threshold: 3`, `for: 5m`, `severity: warning`)
```promql
sum by (domain, tag, prober_type) (increase(astrolavos_stream_reconnects_total[10m])) > 3
```
The stream cannot stay up: flapping path, idle-killing middlebox, or a peer
that drops long-lived connections.

**AstrolavosStreamReconnectBurst** (`alerts.rules.streamReconnectBurst`,
`threshold: 0.5`, `for: 1m`, `severity: critical`)
```promql
count by (domain, tag, prober_type) (
  sum by (pod, domain, tag, prober_type) (increase(astrolavos_stream_reconnects_total[5m])) > 0
)
  /
count by (domain, tag, prober_type) (astrolavos_stream_connection_age_seconds)
  > 0.5
```
More than half of the pods had their stream severed within 5m: a dataplane
event (CNI agent restart, egress gateway reselection, NAT rebuild) just
killed established flows fleet-wide, even if heartbeats recovered right
after. This is the alert that names the Cilium rollout.

## Chart

- `echo.enabled` (default `true`): TCP echo listener, `containerPorts.echo`
  (3001), Service port `echo` (`service.ports.echo`, 3001 → `echo`), env
  `ASTROLAVOS_ECHO_PORT` (= container port, or `0` when disabled). The
  `/echo` WebSocket handler is always served on `http`.
- Default `config.endpoints` gains a second entry probing this release's own
  WebSocket echo through the Service:
  ```yaml
  - domain: '{{ include "common.names.fullname" . }}/echo'
    prober: websocket
    interval: 5s
    timeout: 5s
    https: false
    tag: internal-stream
  ```
- Grafana: a "Streams" row with "Stream connection age" (per pod) and
  "Stream reconnects by reason"; Datadog: matching widgets.
- helm-unittest: workload (env/port, disabled case), service (echo port),
  config (default endpoints), alerts (group + 2 rules), dashboards.

## Tests

- `internal/probers/stream_test.go`: engine with a fake dialer (connect
  failure → backoff and `conn_reused="false"` error; heartbeat failure →
  reconnect counter, age reset; cancellation exits without a reconnect
  count); `websocket` dialer against an `httptest` server running the echo
  handler; `tcpStream` dialer against an in-test echo listener.
- **Black hole**: `blackholeProxy` in the test package, a TCP proxy between
  prober and echo server that forwards until a flag flips and then keeps the
  sockets open while discarding bytes. For both transports: heartbeat fails
  with `timeout`, `stream_reconnects_total{reason="timeout"}` increments, age
  gauge returns to 0, and after the flag flips back the prober reconnects
  through the proxy and the age grows again.
- `internal/handlers/echo_test.go`: text echo round-trip and ping/pong via
  a `coder/websocket` client.
- `internal/machinery`: TCP echo listener echoes and closes on cancel.
- `internal/config`: new prober names, scheme, `retries` warning.
- E2E (`tests/e2e_test.go`, `TestE2EStreamBlackhole`): deploy toxiproxy
  (`ghcr.io/shopify/toxiproxy`) from `tests/testdata/toxiproxy.yaml`; create
  a proxy `echo` on `:8000` upstream `<release>:80`; deploy astrolavos with a
  `websocket` endpoint `toxiproxy:8000/echo`; wait until `/metrics` shows
  `astrolavos_stream_connection_age_seconds{...} > 0`; add a `timeout` toxic
  (`timeout: 0`, both directions, the black hole); wait until
  `astrolavos_stream_reconnects_total{...,reason="timeout"} >= 1`; remove the
  toxic and wait until the age gauge is positive again. Runs in the existing
  `e2e.yml` after a release, like `TestE2EAstrolavos`.

## Docs

README: prober list (`websocket`, `tcpStream`), "Detecting Stale
Connections" extended with the stream probers and the echo endpoints,
`ASTROLAVOS_ECHO_PORT` in the env table; `examples/config.yaml` entries;
AGENTS.md (prober list, echo servers, `stream.go` engine, new metrics, env
var); chart README via helm-docs.

## Release

Lands in PR #117 with part 1; `release:minor`. New dependency
`github.com/coder/websocket`, vendored.
