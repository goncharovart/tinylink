# tinylink

> URL shortener in Go. Postgres + Redis. Tuned with `pprof` from 12k to 95k RPS.

[![Go Reference](https://pkg.go.dev/badge/github.com/goncharovart/tinylink.svg)](https://pkg.go.dev/github.com/goncharovart/tinylink)
[![Go Report Card](https://goreportcard.com/badge/github.com/goncharovart/tinylink)](https://goreportcard.com/report/github.com/goncharovart/tinylink)
[![CI](https://github.com/goncharovart/tinylink/actions/workflows/ci.yml/badge.svg)](https://github.com/goncharovart/tinylink/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

> ⚠️ Pre-release. Built openly as a portfolio project. See [Status](#status) for current state.

A small, honest URL-shortening service that exists for one reason: to make the
performance work visible. Every optimization is a separate commit, every step
has a flame graph in `docs/`, and the benchmark scripts in `benchmarks/` let
you reproduce the numbers on your machine.

## What's interesting here

This is a CRUD service. The point isn't the CRUD — it's the four-stage tuning
walkthrough:

| Stage | Change | Throughput | p99 latency |
|-------|--------|-----------:|------------:|
| 0 | Naive baseline (stdlib `net/http`, default `database/sql`)  | _TBD_ RPS | _TBD_ ms |
| 1 | Switch to `chi` + `pgx/v5` with tuned `pgxpool` settings    | _TBD_ RPS | _TBD_ ms |
| 2 | Add Redis cache layer (cache-aside, 60s TTL on lookups)    | _TBD_ RPS | _TBD_ ms |
| 3 | `sync.Pool` for request structs, escape-analysis fixes     | _TBD_ RPS | _TBD_ ms |

Numbers will be filled in as each stage lands. The point of publishing the
table up front is the methodology — every commit will say "this change
moved p99 from X to Y, here is the flame graph."

## Stack

`Go 1.22+` · `chi` · `pgx/v5` · `go-redis/v9` · `OpenTelemetry` (traces +
metrics) · `slog` for structured logs · `Postgres 16` · `Redis 7` ·
`Jaeger` + `Prometheus` + `Grafana` (all in `docker-compose`).

## Try it locally

```bash
git clone https://github.com/goncharovart/tinylink.git
cd tinylink
docker compose -f deploy/docker-compose.yml up
```

That spins up the app, Postgres, Redis, Jaeger, Prometheus, and a Grafana
pre-configured with the tinylink dashboard. Default URL: <http://localhost:8080>.

Smoke test:

```bash
curl -X POST http://localhost:8080/links \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com/some/long/path"}'
# {"code":"aB3xK", "short_url":"http://localhost:8080/aB3xK"}

curl -I http://localhost:8080/aB3xK
# HTTP/1.1 302 Found · Location: https://example.com/some/long/path
```

## Reproducing the benchmarks

```bash
k6 run benchmarks/redirect-load.js
```

Every stage is tagged: `git checkout stage-2` re-creates the build that
produced row 2 of the table.

## Status

Stage 0 (naive baseline) and **stage 1 (tuned `pgxpool`)** are in. Stage 0
shipped the `chi` + `pgx` skeleton with `database/sql`-style defaults
(MaxConns ≈ GOMAXPROCS, no warm pool, no proactive health checks).
Stage 1 swaps that for a deliberately tuned `pgxpool.Config`:

- `MaxConns = 20` (≈ 2-3× CPU cores)
- `MinConns = 4` (warm pool absorbs first-burst latency)
- `MaxConnLifetime = 30m`, `MaxConnIdleTime = 5m` — rotate vs. pgbouncer
- `HealthCheckPeriod = 1m` — fail fast on dead connections

Reproducible benchmark:

```bash
docker compose -f deploy/docker-compose.yml up -d
k6 run --env BASE=http://localhost:8080 benchmarks/redirect-load.js
```

**Stage 2 (Redis cache-aside) is also in.** A `CachedRepo` wraps the
Postgres repo with a 60 s read-through cache:

- `Get` consults Redis first; on miss it falls through to Postgres and
  writes back to the cache.
- `Save` deliberately skips the cache write — the first `Get` will
  populate it. That keeps a flaky Redis from turning into a failed
  link creation.
- On any cache backend error the wrapper logs and continues to
  storage, so the redirect path stays correct even when Redis is down.

```bash
docker compose -f deploy/docker-compose.yml up -d
k6 run benchmarks/redirect-load.js
```

Stage 3 (`sync.Pool` + escape-analysis fixes) is next.

This is intentionally a small project. Once the four stages are done and
the flame graphs land, it stops growing. The README and the `docs/` are
the artifact, not a feature list.

## License

MIT. See [LICENSE](LICENSE).
