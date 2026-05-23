# CLAUDE.md — tinylink

> Project context for AI coding agents. Keep in sync with reality.

## What this is

`tinylink` — **URL shortener в 5 этапов с pprof-driven walkthrough**. Учебно-демо проект, где каждый stage показывает конкретный performance win: chi+pgxpool tune (S1) → Redis cache-aside (S2) → sync.Pool + escape analysis (S3) → benchmark harness (S4) → **monotonic ID allocator** (S5, 22× faster чем random base62).

Цель — не конкурировать с TinyURL, а **показать profiling-driven engineering** на маленькой surface. Каждый коммит — измеряемый шаг (`benchstat before.txt after.txt` есть в `benchmarks/`).

## Stack

- **Go 1.25** (modern atomic types, generics, `slices`/`cmp`)
- **chi/v5** — HTTP router (zero-dep, идиоматичный)
- **pgx/v5** + pgxpool — Postgres driver и connection pool
- **PostgreSQL 14+** — primary storage
- **Redis 7+** — cache-aside layer (stage 2+)
- **OpenTelemetry** + Prometheus — observability (deploy/)
- Стандартная библиотека для всего остального

## Project layout

```
tinylink/
├── cmd/server/main.go        # Single binary entry point (HTTP server)
├── internal/api/             # chi router, handlers, middleware
├── internal/cache/           # Redis cache-aside abstraction
├── internal/shortener/       # base62 encoder + monotonic allocator (stage 5)
├── internal/storage/         # Postgres + cached-storage wrapper
├── benchmarks/               # benchstat results per stage (before.txt / after.txt)
├── deploy/                   # docker-compose, Prometheus, Grafana dashboards
├── docs/                     # design notes per stage, profiling write-ups
└── go.mod                    # module github.com/goncharovart/tinylink
```

`cmd/` + `internal/` layout (community-de-facto golang-standards/project-layout). Никаких `pkg/` — это binary, не библиотека.

## Build & test

```bash
# Tests with race + cover
go test -race -count=1 -cover ./...

# Single package
go test -race ./internal/shortener/...

# Build binary
go build -o tinylink ./cmd/server

# Benchmarks (per stage)
go test -bench=. -benchmem -count=10 ./internal/shortener/ > new.txt
benchstat benchmarks/stage5_monotonic.txt new.txt   # compare with baseline

# Vet + format
go vet ./...
gofmt -s -w .

# Stack via docker-compose (Postgres + Redis + Prometheus + Grafana)
cd deploy && docker compose up -d
```

## Coding conventions

### Performance-first patterns

Этот проект существует чтобы **демонстрировать** performance work. Когда добавляешь функциональность — **измеряй**:

1. Baseline benchmark в `benchmarks/`
2. Implement change
3. Re-run benchmark с тем же seed
4. `benchstat before.txt after.txt` — must show statistical significance
5. Commit message: `feat(stageN): <change> — Xx <metric> (<benchstat numbers>)`

Pattern example (stage 5 — monotonic allocator):
- Before: base62 random encode, ~280 ns/op, 1 alloc/op
- After: monotonic counter + atomic increment, ~13 ns/op, 0 allocs/op
- Win: **22× faster, zero-alloc**

### Idiomatic Go (stage-relevant)

- **`atomic.Uint64` over `atomic.AddUint64`** — modern atomic types preferred (Go 1.22+)
- **`sync.Pool` for short-lived buffers только** — never для long-held state
- **Escape analysis aware** — small structs returned by value (`go build -gcflags='-m'` checks)
- **`errors.Is`/`errors.As`** для проверки sentinel/wrapped errors, не string match
- **`chi.URLParam(r, "code")`** один раз в начале handler'a, потом local var

### Postgres patterns

- `pgxpool.Pool` shared в `internal/storage/` (один pool на процесс)
- `FOR UPDATE SKIP LOCKED` для конкурентного allocation в monotonic mode
- Prepared statements для hot path (`/r/{code}` redirect)
- Connection lifecycle через `ctx.Context` — все queries cancellable

### Cache patterns

- Cache-aside (NOT write-through) — application explicit GET-or-FALLBACK
- TTL: 24h для shortened URL → original mapping (immutable data)
- Negative cache (404 results): 60s TTL чтобы избежать DB hammering при 404 storms
- Redis as **secondary index** только, Postgres = source of truth

### Testing

- **Table-driven с subtests**, имена сценариями: `t.Run("monotonic allocator returns increasing IDs under concurrent load", ...)`
- **`-race` clean** — все тесты pass `-race -count=10`
- **Benchmark suite в каждом stage'е** — `Benchmark*` функции рядом с `Test*`
- **testcontainers-go** для integration tests (Postgres + Redis)
- **Coverage target: 80%+** на `internal/`

## Pre-commit hook (recommended)

```bash
#!/usr/bin/env bash
# .githooks/pre-commit
set -e
gofmt -s -w .
go vet ./...
go test -race -short -count=1 ./...
golangci-lint run --new-from-rev=HEAD~1 || true
```

Enable: `git config core.hooksPath .githooks`

## Stage roadmap

| Stage | Topic | Status |
|---|---|---|
| 0 | chi + Postgres baseline (single connection per request) | ✅ |
| 1 | pgxpool tuning + bounded connections | ✅ |
| 2 | Redis cache-aside layer + negative cache | ✅ |
| 3 | sync.Pool + escape analysis — handler/encoder hot path | ✅ |
| 4 | Benchmark harness в CI (benchstat regression detection) | ✅ |
| 5 | Monotonic ID allocator (22× faster than random base62) | ✅ |
| 6 | Distributed allocation across N nodes (Sundial-style advisory locks) | planned |

## Stability

v0.1.x — public HTTP API stable; internal/ packages могут shift между minor versions для демонстрации patterns. Каждое breaking change документировано в CHANGELOG + benchstat numbers.

## Related docs

- `README.md` / `README.ru.md` — user-facing intro + curl examples
- `CHANGELOG.md` — Keep-a-Changelog format
- `benchmarks/` — per-stage benchstat before/after files
- `docs/` — design notes per stage (profiling, escape analysis screenshots)
- `deploy/` — full Prometheus + Grafana observability stack via docker-compose
