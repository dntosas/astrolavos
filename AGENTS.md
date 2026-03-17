# AGENTS.md

## Project Overview

Astrolavos is a Go-based network latency probe that measures HTTP and TCP endpoint behavior, exposing Prometheus metrics. It runs as a long-lived DaemonSet/Deployment or in one-off (cronjob) mode. The codebase is intentionally small (~1.5K LoC) and dependency-light.

## Repository Layout

```
main.go                         # Entry point: flags, config, logging, starts machinery.Astrolavos
internal/
  config/                       # YAML + Viper config loading and validation
  machinery/                    # Core app orchestration: agent lifecycle, HTTP server, graceful shutdown
  handlers/                     # HTTP handlers: /live, /ready, /metrics, /latency, /status
  probers/                      # Prober interface + implementations (httpTrace, tcp)
  metrics/                      # Prometheus histogram/counter registration, push gateway
  model/                        # Shared domain types (Endpoint struct)
deploy/kubernetes/              # Helm chart (DaemonSet by default, Deployment optional)
tests/                          # E2E tests (separate Go module, Terratest + Kind)
examples/                       # Example config.yaml
.github/workflows/              # CI (go-ci), Release (GoReleaser), Helm release, E2E
.github/config/goreleaser.yaml  # GoReleaser multi-arch build config
```

## Tech Stack

| Component     | Technology                                                    |
|---------------|---------------------------------------------------------------|
| Language      | Go 1.25, vendored dependencies (`go mod vendor`)             |
| Config        | YAML + Viper (env prefix `ASTROLAVOS_`)                      |
| Logging       | logrus (JSON structured)                                      |
| Metrics       | prometheus/client_golang (scrape + push gateway)              |
| Linting       | golangci-lint v2.4.0 (gosec, gocritic, misspell, revive)     |
| Container     | Distroless static (`gcr.io/distroless/static:nonroot`)        |
| Release       | GoReleaser (linux/amd64 + arm64, GHCR multi-arch manifests)  |
| Helm          | Chart v0.12.0, Bitnami common dependency                      |
| E2E           | Terratest + Kind cluster                                      |

## Setup and Build

```bash
# Build for Linux amd64
make build

# Build for local OS/arch
make build-local

# Run locally with example config
make run
# equivalent to: go run -mod=vendor *.go -config-path ./examples/

# Sync modules after dependency changes
make modsync
```

## Validation Commands

Run these before every commit. CI enforces the same checks.

```bash
# Full CI pipeline (fmt → vet → lint → test)
make ci

# Individual steps
make fmt                    # go fmt ./...
make vet                    # go vet ./...
make lint                   # golangci-lint run --timeout 5m --modules-download-mode=vendor --build-tags integration
make test                   # go test with -race and coverage
```

### E2E Tests

E2E tests deploy the Helm chart into a Kind cluster via Terratest. They require a published image.

```bash
make e2e ASTROLAVOS_VERSION=v0.12.0
```

## Code Style and Conventions

- **Go fmt** is the only formatter. No additional style tools.
- **golangci-lint v2** config lives in `.golangci.yml`. Enabled: `gosec`, `gocritic`, `misspell`, `revive`, `unconvert`. Disabled: `godox`, `lll`, `depguard`, `mnd`.
- Use `//nolint:<linter>` with a justification comment when suppressing a lint.
- All packages live under `internal/` — nothing is exported outside the module.
- Prober implementations must satisfy the `probers.Prober` interface (`String()` + `Run(ctx)`).
- Use `ProberConfig.runLoop()` for the one-off vs interval execution pattern — do not reimplement ticker logic.
- Use `ProberConfig.retryWithBackoff()` for retry logic — do not write custom retry loops.
- Error categories in metrics use `metrics.CategorizeError()` to prevent label cardinality explosion. Never pass raw error strings as Prometheus labels.
- Structured logging: always use `log.WithField`/`log.WithFields` for context, not string interpolation.
- Config defaults are set in `initViper()`. Environment variables override YAML via `ASTROLAVOS_` prefix.

## Architecture Constraints

- **No external HTTP frameworks.** The server uses `net/http` directly. Do not introduce gorilla/mux, chi, gin, or similar.
- **No ORM or database.** Astrolavos is stateless — config is read-only from YAML.
- **Vendored dependencies.** Always use `-mod=vendor`. Run `make modsync` after any `go.mod` change.
- **Distroless container.** The Docker image has no shell. Do not add debug tools or alpine base.
- **Graceful shutdown.** The server handles SIGINT/SIGTERM, cancels probe contexts, waits for goroutines, then shuts down HTTP with a 5s grace period. Preserve this pattern.

## Adding a New Prober

1. Create `internal/probers/<name>.go` implementing the `Prober` interface.
2. Use `NewProberConfig()` and embed `ProberConfig` for shared behavior.
3. Call `p.runLoop(ctx, "<name>", probeFn)` from `Run()`.
4. Use `p.retryWithBackoff(ctx, fn)` for resilient probing.
5. Register the new type in `internal/machinery/agent.go` switch statement.
6. Add unit tests in `internal/probers/<name>_test.go`.
7. Update `internal/config/config.go` validation to accept the new prober name.

## Helm Chart

The chart lives in `deploy/kubernetes/`. Key design decisions:

- **DaemonSet by default** (`deployAsDaemonSet: true`) for cluster-wide coverage.
- **PDB enabled** with `minAvailable: 50%` to survive node drains.
- **SecurityContext** runs as non-root (UID 65532), read-only rootfs, all capabilities dropped.
- **preStop hook** sleeps 15s for graceful connection draining.
- **ServiceMonitor** enabled by default for Prometheus Operator scraping.

```bash
# Regenerate Helm docs after values.yaml changes
make helm-docs
```

## CI/CD Pipeline

| Workflow         | Trigger                          | What it does                                    |
|------------------|----------------------------------|-------------------------------------------------|
| `go-ci.yml`      | Push/PR to `main` (Go files)    | fmt → vet → golangci-lint → test                |
| `go-release.yml` | Tag `v*.*.*`                     | GoReleaser build + GHCR push, then triggers E2E |
| `helm-release.yml`| Push to `main`                  | Publishes Helm chart via chart-releaser          |
| `e2e.yml`        | `workflow_call` / `dispatch`     | Kind cluster → Helm deploy → Terratest          |

## Release Process

1. Ensure `main` is green.
2. Tag: `git tag v0.X.0 && git push origin v0.X.0`.
3. GoReleaser builds binaries + multi-arch Docker images → pushes to `ghcr.io/dntosas/astrolavos`.
4. E2E runs automatically post-release.
5. Helm chart version in `Chart.yaml` must be bumped manually before merge.

## Environment Variables

| Variable                     | Default     | Description                              |
|------------------------------|-------------|------------------------------------------|
| `ASTROLAVOS_APP_PORT`        | `3000`      | HTTP server port                         |
| `ASTROLAVOS_LOG_LEVEL`       | `DEBUG`     | Log level (DEBUG, INFO, WARN, ERROR)     |
| `ASTROLAVOS_PROM_PUSH_GW`   | `localhost` | Prometheus push gateway address          |
| `ASTROLAVOS_MAX_PAYLOAD_SIZE`| `0`         | Max latency endpoint payload (0 = 10MB)  |

## Do Not

- Add dependencies without running `make modsync` and committing `vendor/`.
- Use raw error strings as Prometheus metric labels.
- Bypass the `Prober` interface or `ProberConfig` shared logic.
- Introduce init() functions — explicit initialization only.
- Modify the Dockerfile base image away from distroless.
- Skip `make ci` before opening a PR.

## Commit Convention

Follow [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/):

```
feat: add UDP prober implementation
fix: handle nil interval in config validation
chore: bump golangci-lint to v2.5.0
docs: update Helm chart README with new values
test: add E2E test for TCP prober timeout
```
