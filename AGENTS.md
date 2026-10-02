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
scripts/                        # release.py (version bump + tag), dispatch-and-wait.sh (used by release.yml)
```

## Tech Stack

| Component     | Technology                                                    |
|---------------|---------------------------------------------------------------|
| Language      | Go 1.27, vendored dependencies (`go mod vendor`)             |
| Config        | YAML + Viper (env prefix `ASTROLAVOS_`)                      |
| Logging       | logrus (JSON structured)                                      |
| Metrics       | prometheus/client_golang (scrape + push gateway)              |
| Linting       | golangci-lint v2.14.0 (gosec, gocritic, misspell, revive)    |
| Container     | Distroless static (`gcr.io/distroless/static:nonroot`)        |
| Release       | GoReleaser (linux/amd64 + arm64, GHCR multi-arch manifests)  |
| Helm          | Chart released in lockstep with the app, Bitnami common dependency |
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
# Full CI pipeline (fmt → vet → lint → test → vulncheck)
make ci

# Individual steps
make fmt                    # go fmt ./...
make vet                    # go vet ./...
make lint                   # golangci-lint run --timeout 5m --modules-download-mode=vendor --build-tags integration
make test                   # go test with -race and coverage
make vulncheck              # govulncheck ./... (reachable known vulnerabilities; needs network)
make helm-test              # helm unittest --strict deploy/kubernetes (chart template tests)
```

### E2E Tests

E2E tests deploy the Helm chart into a Kind cluster via Terratest. They require a published image.

```bash
make e2e ASTROLAVOS_VERSION=v1.0.0
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
- Latency histogram buckets are shared by all `astrolavos_*_latency_seconds` metrics and resolved as env (`ASTROLAVOS_HISTOGRAM_BUCKETS`) > `config.yaml` (`metrics.histogramBuckets`) > `metrics.DefaultTimeBuckets`. Validation (finite, strictly increasing) lives in `internal/config`; `metrics.NewPrometheusClient` only falls back to the default on an empty slice.

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

# Run the chart unit tests (helm-unittest plugin, pinned to v1.0.3 in CI)
helm plugin install https://github.com/helm-unittest/helm-unittest.git --version v1.0.3
make helm-test
```

### Chart unit tests (`deploy/kubernetes/tests/`)

[helm-unittest](https://github.com/helm-unittest/helm-unittest) suites, one `*_test.yaml` per template group (`workload`, `config`, `service`, `serviceaccount`, `servicemonitor`, `scaling` for PDB+HPA, `ingress`, `dashboards`, `metadata` for cross-resource naming/labels). `tests/` is in `.helmignore`, so it is not shipped in the packaged chart.

- Every template change needs a test: pin the default rendering, each `values.yaml` switch, and the "disabled → no document" case (`hasDocuments: count: 0`).
- Prefer exact `equal` on whole sub-objects (`securityContext`, `ports`, `httpGet`) over one assertion per leaf; use `matchRegex` only for free text such as `data["config.yaml"]`.
- Suites use `release: {name: astrolavos, namespace: monitoring}` so names are stable; the chart's `fullnameOverride: astrolavos` default means most resources are named `astrolavos`.
- Templates that branch on `Capabilities` need them declared: `capabilities.majorVersion/minorVersion` (Service `internalTrafficPolicy`, HPA/Ingress API versions) and `capabilities.apiVersions` for the ServiceMonitor. Declare `apiVersions` per test, not per suite — a test-level empty list does not clear a suite-level one.
- Values rendered through `common.tplvalues.render` come back single-quoted when they contain `/` or `{{ }}`; regex for them accordingly.

## CI/CD Pipeline

| Workflow         | Trigger                          | What it does                                    |
|------------------|----------------------------------|-------------------------------------------------|
| `go-ci.yml`      | Push/PR to `main` (Go files)    | fmt → vet → golangci-lint → test → govulncheck  |
| `release.yml`    | PR merged to `main` with `release:*` label, or dispatch | Bump chart+image, commit on main, tag, dispatch go-release then helm-release |
| `go-release.yml` | Tag `v*.*.*` push, or dispatch at a tag | GoReleaser build + GHCR push, keyless cosign signatures, SBOMs, provenance attestations, then triggers E2E |
| `helm-ci.yml`    | Push (chart files)               | helm dep update → lint --strict → unittest → template → package |
| `helm-release.yml`| Dispatch only (by `release.yml`) | Publishes Helm chart via chart-releaser (`skip_existing`) |
| `e2e.yml`        | `workflow_call` / `dispatch`     | Kind cluster → Helm deploy → Terratest          |

Workflow conventions:

- Every workflow declares `permissions: contents: read` at the top and widens it per job only where needed (`go-release.yml` needs `contents`/`packages`/`id-token`/`attestations: write`; `helm-release.yml` needs `contents: write`; `release.yml` needs `contents: write` to push and `actions: write` to dispatch).
- Third-party actions are pinned to a full commit SHA with the version in a trailing comment (`uses: owner/action@<sha> # vN`). Dependabot keeps the SHAs current. Do not pin to a tag.
- Release signing is keyless: cosign gets a short-lived certificate from the workflow's OIDC token. There is no signing key or secret to rotate. Verification commands live in `SECURITY.md`; keep them in sync with the identity (`go-release.yml@refs/tags/vX.Y.Z`) if the workflow file is renamed.
- `cosign-installer` tracks cosign 3.x. Blob signatures must be written with `--bundle` (one `.sigstore.json` per artifact); image signatures are pinned to the legacy `sha256-<digest>.sig` layout with `--new-bundle-format=false` until every consumer can verify OCI 1.1 referrers. Both are explained inline in `.github/config/goreleaser.yaml`.

## Release Process

Versions follow SemVer and the chart is released in lockstep with the app
(chart `X.Y.Z` has `appVersion: X.Y.Z` and `image.tag: vX.Y.Z`). Releases are
cut by [`release.yml`](.github/workflows/release.yml); **do not bump
`Chart.yaml` or `image.tag` by hand in a PR.**

1. Open the PR as normal and add **exactly one** label before merging:
   `release:patch` (fix, no new behaviour), `release:minor` (additive,
   backwards-compatible) or `release:major` (breaking, `!` in the subject).
   PRs without a label do not release; more than one fails the job.
2. On merge, `release.yml` checks out `main`, computes the next tag from the
   latest `vX.Y.Z`, runs `scripts/release.py release <bump>` — which bumps
   `version`/`appVersion` in `deploy/kubernetes/Chart.yaml`, `image.tag` in
   `values.yaml`, regenerates the chart README with helm-docs, commits
   `chore(release): vX.Y.Z` on `main` and pushes an annotated tag — then
   dispatches `go-release.yml` at the tag and waits for it.
3. `go-release.yml` builds binaries + multi-arch images, signs everything,
   attaches SBOMs/provenance, publishes the GitHub release and runs E2E.
4. Only after that succeeds does `release.yml` dispatch `helm-release.yml`,
   so the published chart never references an image that does not exist yet.

A release can also be cut by hand from the Actions UI ("Cut Release" →
Run workflow → bump), or locally from a clean, up-to-date `main` with
`make release-patch|minor|major` (then dispatch the two workflows:
`gh workflow run go-release.yml --ref vX.Y.Z`, `gh workflow run
helm-release.yml --ref main`).

Why the dispatches: pushes made with `GITHUB_TOKEN` never trigger `on: push`
workflows, so the bot's tag would otherwise sit there unreleased. API-triggered
`workflow_dispatch` is GitHub's documented exception. Running `go-release.yml`
at the tag ref keeps `github.ref` (and therefore the cosign signing identity)
at `refs/tags/vX.Y.Z`, identical to a human `git push --tags`.

`helm-release.yml` has **no push trigger** on purpose. Between the release
commit and a successful image build, `main` carries a chart whose `image.tag`
does not exist yet; a push trigger would publish it on any unrelated merge in
that window (this is how 1.1.0 shipped a chart with no image).

**FOOT-GUN: `GITHUB_TOKEN` cannot bypass a branch ruleset that requires pull
requests.** `main` currently has no such rule, which is what lets the bot
commit directly. If one is added, `release.yml` needs a GitHub App token (or a
fine-grained PAT) with `contents: write` listed in the ruleset bypass.

If `go-release.yml` fails, the chart has not been published (step 4 never
ran), but GoReleaser pushes images **before** signing them, so unsigned
`vX.Y.Z`/`X.Y.Z`/`latest` manifests may already be in GHCR; a rerun overwrites
them. Fix forward on `main` (do not label that PR), then move the tag onto the
fix (`git push --delete origin vX.Y.Z && git tag -f vX.Y.Z origin/main && git
push origin vX.Y.Z`). The human tag push fires `go-release.yml`; when it is
green, `gh workflow run helm-release.yml --ref main`. Only once a GitHub
release exists for the tag is it consumed and immutable; cut a patch instead.

## Environment Variables

| Variable                     | Default     | Description                              |
|------------------------------|-------------|------------------------------------------|
| `ASTROLAVOS_APP_PORT`        | `3000`      | HTTP server port                         |
| `ASTROLAVOS_LOG_LEVEL`       | `DEBUG`     | Log level (DEBUG, INFO, WARN, ERROR)     |
| `ASTROLAVOS_PROM_PUSH_GW`   | `localhost` | Prometheus push gateway address          |
| `ASTROLAVOS_MAX_PAYLOAD_SIZE`| `0`         | Max latency endpoint payload (0 = 10MB)  |
| `ASTROLAVOS_HISTOGRAM_BUCKETS`| `metrics.DefaultTimeBuckets` | Comma-separated latency histogram bucket bounds in seconds; overrides `metrics.histogramBuckets` in `config.yaml` |

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
