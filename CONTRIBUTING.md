# Contributing to tinylink

tinylink is a learning artifact — a URL shortener documented as a
four-stage `pprof`-driven optimisation walkthrough. Contributions
that **preserve that shape** are welcome: a new stage (with a real
benchmark), a tighter test, a fix to a sloppy claim in the README.

What it is **not**: a competitor to commercial URL-shorteners. PRs
that add JWT-auth, multi-tenancy, custom domains, or a billing
service are out of scope here — fork it and rename if that's what
you need.

## Quick orientation

```
cmd/server/main.go            # entry point — wires chi + pgxpool + optional Redis
internal/api/handlers.go      # HTTP handlers + sync.Pool stage-3 pools
internal/storage/             # Repository interface, Postgres + memory + cached impls
internal/cache/               # Redis cache-aside wrapper (stage 2)
internal/shortener/           # base62 random-code generator
benchmarks/redirect-load.js   # k6 constant-arrival-rate driver
deploy/                       # docker-compose, Dockerfile
docs/                         # stage walkthroughs (currently stage 3 escape analysis)
migrations/                   # Postgres schema
```

Every stage of the walkthrough is **one commit** (`feat: stage 0`,
`feat(stage1): tuned pgxpool`, ...) plus its own row in the README's
optimisation table. Adding a stage 4 is the most-valuable PR shape:
one commit, one row, one new section in `docs/`.

## Before you open a PR

1. **Open an issue first** for anything beyond a one-line fix.
2. **Tests** for behaviour changes. Storage and cache layers both
   have `MemoryRepo`/`MemoryCache` fixtures so the full suite runs
   without Postgres or Redis.
3. **`go fmt`, `go vet`, `golangci-lint run`.** CI runs all three.
4. **Stage commits stay separate from refactors.** If you're
   refactoring across stages, do it in one PR; the stage-feature
   PR goes in a second PR on top.

## Reproducing the benchmarks

```bash
docker compose -f deploy/docker-compose.yml up -d
k6 run benchmarks/redirect-load.js
```

Tweak `RATE` and `DURATION` in `benchmarks/redirect-load.js` for
your hardware. The README's optimisation table is meant to be
*reproducible by you* — that's the whole point of the artifact.

## Adding a stage

If you'd like to land a new stage (e.g. read-replica sharding,
`unsafe`-based code-decoding, custom HTTP parser), please:

1. Open an issue describing the change you want to measure.
2. Land the code as one commit, with the stage's diff staying
   focused on the optimisation under study.
3. Update README's table with reproducible RPS / p95 / p99 numbers
   (note your test box: CPU, RAM, OS).
4. Add a one-page write-up under `docs/stageN-name.md` with quoted
   compiler output / pprof snippets where relevant.

The `docs/stage3-escape-analysis.md` walkthrough is the model.

## Reporting bugs

A minimal reproducing curl + expected output is the most valuable
thing you can include. Failing that: Go version, OS, Postgres
version, the exact commands you ran, what you expected vs what
happened.

## License

By contributing you agree your work is released under the project's
[MIT license](LICENSE).
