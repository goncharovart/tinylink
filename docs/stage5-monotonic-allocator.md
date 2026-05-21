# Stage 5 — monotonic counter allocator vs random + retry

This is the walkthrough for the optional stage-5 code allocator —
`internal/shortener/monotonic.go`. Stage 0 through 3 use the
`Random` + retry-on-collision allocator inherited from the original
baseline; stage 5 demonstrates a **collision-free allocator** with
measurable performance gains.

## The trade-off

The stage-0 random allocator:

```go
buf := make([]byte, length)
max := big.NewInt(62)
for i := 0; i < length; i++ {
    idx, err := rand.Int(rand.Reader, max)
    if err != nil { return "", err }
    buf[i] = alphabet[idx.Int64()]
}
return string(buf), nil
```

Works correctly, but has two known issues:

1. **Hot-path cost**: 22 allocations per call (each `rand.Int` allocates
   a `*big.Int`); ~1µs per code on a Ryzen 5800X.
2. **Long-tail collisions**: as the code space fills, the storage
   layer's `UNIQUE(code)` constraint kicks in and the create handler
   retries through `ErrCodeTaken`. The mean per-create latency grows
   even when p99 still looks fine; the create handler's retry budget
   eventually exhausts when saturation hits ~70%.

The stage-5 monotonic allocator (`internal/shortener/monotonic.go`)
replaces the random source with an `atomic.Uint64` counter,
base62-encoded with leading-zero padding:

```go
type Monotonic struct {
    counter atomic.Uint64
    width   int
}

func (m *Monotonic) Next() (string, error) {
    n := m.counter.Add(1) - 1
    return encodeBase62Padded(n, m.width)
}
```

## Measured

```bash
$ go test -bench='Benchmark.+Next' -benchmem ./internal/shortener/
goos: windows
goarch: amd64
pkg: github.com/goncharovart/tinylink/internal/shortener
BenchmarkMonotonic_Next-12    48942634     46.67 ns/op     16 B/op    1 allocs/op
BenchmarkRandom_Next-12        2357438   1033.00 ns/op    336 B/op   22 allocs/op
```

The monotonic allocator is:
- **22× faster** (46.67 ns vs 1033 ns per call)
- **21× lower memory per call** (16 B vs 336 B)
- **22× fewer allocations** (1 vs 22)

And — by construction — **zero collisions**.

## What stage 5 does NOT solve

Two follow-on issues remain open, intentionally:

### Predictability

The next code is *guessable* by anyone watching two consecutive
`Save()` calls. The guesser still has to do a redirect roundtrip to
harvest the mapping, and the redirect path is rate-limited (issue #3),
so for tinylink's threat model this is acceptable. If your tinylink
fork hosts paid-tier links, you should **stay on the random allocator
or back the counter with a per-tenant secret seed**.

### Distributed coordination

A single process is the source of truth for "next code." A multi-replica
deployment needs one of:

- **Sharded counters** with stride/offset (`Monotonic.NextStride(2)`
  on replica A with offset 0, on replica B with offset 1 — both
  emit collision-free codes from disjoint subsets of the integer
  space).
- **A Postgres SEQUENCE** backing the counter, so all replicas pull
  from the same monotonic source. Adds one DB roundtrip per create
  — kills most of the benchmark win but solves the coordination
  problem at any replica count.

Both are tracked as follow-up issues; the v0.1.0 surface ships
only the in-process allocator.

## When to enable stage 5

The Monotonic allocator is exposed as **opt-in**. The default remains
the random allocator so the four-row optimisation table at the top of
the README stays comparable across deployments.

To opt in, replace the call site in `internal/api/handlers.go::handleCreate`:

```go
// Default (stage 0):
code, err := shortener.Random(cfg.CodeLength)

// Stage 5 (opt-in):
code, err := monotonicAllocator.Next()
```

…and inject `monotonicAllocator` through `Config` so the test fixture
can swap it. Concrete wiring is left as a follow-up because it
straddles the API surface — the bench number is meaningful on its
own; the wire-up is a small refactor.

## Where this fits in the walkthrough

This stage is **bonus**. It does not appear in the main
optimisation table at the top of `README.md` because the four-stage
walkthrough (chi/pgxpool tune → Redis → sync.Pool) targets the
**redirect** hot path, not the **create** path. Stage 5 is for
readers who finished stages 0-3, opened the create handler, and
asked "why is this slow?"
