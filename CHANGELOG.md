# Changelog

All notable changes to tinylink are tracked here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the
project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

tinylink is a learning artifact — the public "surface" is the
README's four-stage optimisation walkthrough plus the HTTP API
(`POST /links`, `GET /:code`, `GET /healthz`). The HTTP API is
considered unstable until `v1.0.0` — breaking changes between minor
versions in the `v0.x` series are allowed but will be called out
under "Changed".

## [Unreleased]

Pending milestones live in the [GitHub issues](https://github.com/goncharovart/tinylink/issues)
labelled `good first issue`.

## [v0.1.0] — 2026-05-21

First tagged release. All four `pprof`-driven optimisation stages
land together so v0.1.0 is the complete walkthrough; later releases
will be incremental — additional stages, alternative backends,
benchmark refreshes.

### Added

- **Stage 0 — baseline.** chi router + `pgxpool.New` with default
  config + a naive `Repository`. `POST /links` issues a base62
  random code with retry-on-collision; `GET /:code` looks it up
  and `http.Redirect`s. Fire-and-forget hit counter via goroutine.
- **Stage 1 — tuned pgxpool.** Explicit `MaxConns=20`, `MinConns=4`,
  `MaxConnLifetime=30m`, `MaxConnIdleTime=5m`,
  `HealthCheckPeriod=1m`. The exact numbers are conservative; the
  point is the *shape* of a production-ready pool config (see the
  block-comment in `cmd/server/main.go` for the reasoning).
- **Stage 2 — Redis cache-aside.** `CachedRepo` wraps the Postgres
  repo with a 60-second read-through cache. `Save` deliberately
  skips the cache write so a flaky Redis cannot turn into a failed
  link creation. On any cache backend error the wrapper logs and
  falls through to storage — the redirect path stays correct even
  when Redis is down.
- **Stage 3 — `sync.Pool` + escape-analysis.** Four package-level
  pools (`createRequestPool`, `createResponsePool`,
  `responseBufferPool`, `errorResponsePool`) and a `writeJSONPooled`
  helper that encodes into a pooled `*bytes.Buffer` so the response
  no longer pays a per-request `json.NewEncoder` allocation. The
  encoder itself now stack-allocates (`&json.Encoder{...} does not
  escape`) because we feed it a buffer we own.
  Walkthrough with quoted compiler output:
  [docs/stage3-escape-analysis.md](docs/stage3-escape-analysis.md).
- **k6 driver** — `benchmarks/redirect-load.js` runs a
  `constant-arrival-rate` executor against `/:code`, seeded with
  configurable link count. Thresholds: `http_req_failed < 0.01`,
  `redirect_latency p99 < 150ms`, `p95 < 60ms`.
- **Docker compose + Dockerfile** — single-command local stack
  (Postgres + Redis + the binary).

### Tests

23 unit tests, no Postgres or Redis required for the bulk of them
(`MemoryRepo` + `MemoryCache` fixtures). CI: Go 1.25 × ubuntu-latest
with a real Postgres + Redis service container so the integration
path is exercised at least once per push.

### Reproducing the benchmark

```bash
docker compose -f deploy/docker-compose.yml up -d
k6 run benchmarks/redirect-load.js
```

The optimisation table at the top of the README is meant to be
filled in *by you* on your own hardware — the project is a
walkthrough, not a hero-number flex.

### Not yet

- Stage 4 — `unsafe`-based fast-path for base62 decode (#1)
- Alternative storage backends: SQLite, SurrealDB (#2)
- Distributed rate-limiting (#3)
- Read-replica routing (#4)
