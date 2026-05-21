# tinylink

> URL shortener на Go. Postgres + Redis. Tuned `pprof`-driven через 4 стадии.

[![Go Reference](https://pkg.go.dev/badge/github.com/goncharovart/tinylink.svg)](https://pkg.go.dev/github.com/goncharovart/tinylink)
[![Go Report Card](https://goreportcard.com/badge/github.com/goncharovart/tinylink)](https://goreportcard.com/report/github.com/goncharovart/tinylink)
[![CI](https://github.com/goncharovart/tinylink/actions/workflows/ci.yml/badge.svg)](https://github.com/goncharovart/tinylink/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/goncharovart/tinylink?sort=semver&display_name=tag&color=blue)](https://github.com/goncharovart/tinylink/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![README (English)](https://img.shields.io/badge/README-English-blue.svg)](README.md)

> 🚧 `v0.1.0` зафиксирован с полным 4-стадийным walkthrough. Открытые issues отслеживают работу stage 4+ и альтернативные backend-ы.

Маленький честный URL-сокращающий сервис, существующий по одной причине: сделать
performance-работу видимой. Каждая оптимизация — отдельный коммит, каждый шаг
имеет flame graph в `docs/`, и benchmark-скрипты в `benchmarks/` позволяют
воспроизвести числа на твоей машине.

## Что здесь интересно

Это CRUD-сервис. Точка не в CRUD — точка в **четырёхстадийном tuning walkthrough**:

| Стадия | Изменение | Throughput | p99 latency |
|--------|-----------|-----------:|------------:|
| 0 | Naive baseline (stdlib `net/http`, default `database/sql`)  | _TBD_ RPS | _TBD_ ms |
| 1 | Switch на `chi` + `pgx/v5` с tuned `pgxpool` settings        | _TBD_ RPS | _TBD_ ms |
| 2 | Redis cache-aside layer для горячих lookups                  | _TBD_ RPS | _TBD_ ms |
| 3 | `sync.Pool` для request/response, escape-analysis fixes      | _TBD_ RPS | _TBD_ ms |

Числа заполняются твоими k6 запусками на твоём железе — это walkthrough,
не hero-number flex.

## Архитектура

- **Stage 0** — baseline. chi router + `pgxpool.New` с дефолтным config'ом
  + naive `Repository`. `POST /links` выдаёт base62 random код с
  retry-on-collision; `GET /:code` ищет его и `http.Redirect`-ит.
  Fire-and-forget hit counter через горутину.
- **Stage 1** — tuned pgxpool. Explicit `MaxConns=20`, `MinConns=4`,
  `MaxConnLifetime=30m`, `MaxConnIdleTime=5m`, `HealthCheckPeriod=1m`.
- **Stage 2** — Redis cache-aside. `CachedRepo` оборачивает Postgres-репо
  с 60-секундным read-through cache. `Save` намеренно skip-ает write
  в cache — flaky Redis не должен превращаться в failed link creation.
- **Stage 3** — `sync.Pool` + escape-analysis. Четыре package-level пула
  (`createRequestPool`, `createResponsePool`, `responseBufferPool`,
  `errorResponsePool`) + `writeJSONPooled` helper. Encoder теперь
  stack-allocates.

Полный walkthrough stage 3 с цитированным compiler output:
[docs/stage3-escape-analysis.md](docs/stage3-escape-analysis.md).

## Запуск

```bash
git clone https://github.com/goncharovart/tinylink
cd tinylink
docker compose -f deploy/docker-compose.yml up -d
k6 run benchmarks/redirect-load.js
```

Tweak `RATE` и `DURATION` в k6 скрипте под твоё железо.

## Тесты

23 unit-теста, в основном работают через `MemoryRepo` + `MemoryCache`
fixtures так что bulk suite не требует Postgres или Redis. CI: Go 1.25 ×
ubuntu-latest с реальным Postgres + Redis service container.

## Статус

`v0.1.0` shipped — полный 4-стадийный walkthrough. Дальнейшая работа
инкрементальна: дополнительные стадии, альтернативные backends, обновление
benchmarks.

Открытые issues:
- [#1](https://github.com/goncharovart/tinylink/issues/1) Stage 4: `unsafe`-based fast-path для base62 decode
- [#2](https://github.com/goncharovart/tinylink/issues/2) Альтернативные storage: SQLite, SurrealDB
- [#3](https://github.com/goncharovart/tinylink/issues/3) Distributed rate-limiting
- [#4](https://github.com/goncharovart/tinylink/issues/4) Read-replica routing
- [#5](https://github.com/goncharovart/tinylink/issues/5) Monotonic-counter код allocator

## Лицензия

MIT.
