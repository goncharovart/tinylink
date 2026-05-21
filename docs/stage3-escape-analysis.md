# Stage 3 — sync.Pool and escape-analysis fixes

This is a walkthrough of the allocation work between stage 2 and
stage 3 — what `go build -gcflags="-m=2"` showed, what we changed,
and how the result holds up under sustained load.

## What stage 2 looked like

End of stage 2 (`feat(stage2)` commit), the create handler had this
allocation profile per successful request:

| Allocation | Where | Reason |
|---|---|---|
| `createRequest{}` value | `var req createRequest` | Local but passed to `json.NewDecoder(...).Decode(&req)`; pointer-to-stack-local is fine here, no escape |
| `createResponse{}` value | `writeJSON(w, 201, createResponse{...})` | Value passed as `any` to `writeJSON` → escapes to heap on every call |
| `errorResponse{}` value | `writeError(...)` indirectly | Same `any`-boxing escape |
| `bytes.Buffer` *inside* `json.Encoder` | `json.NewEncoder(w).Encode(body)` | The encoder allocates a small buffer internally; one alloc per call |
| `*json.Encoder` itself | `json.NewEncoder(w)` | Escapes through the interface chain to the response writer |

Five small allocations per create on the happy path; the redirect
path was already lean (one `chi.URLParam` string + the `http.Redirect`
internals).

Run on `internal/api/handlers.go` (stage-2 commit):

```bash
go build -gcflags="-m=2" ./internal/api/ 2>&1 | grep escape
```

Showed exactly that: `createResponse` and `errorResponse` literals
escaping through the `any` parameter of `writeJSON`/`writeError`.

## What stage 3 changed

Four `sync.Pool`s at package scope:

```go
var (
    createRequestPool  = sync.Pool{New: func() any { return new(createRequest) }}
    createResponsePool = sync.Pool{New: func() any { return new(createResponse) }}
    responseBufferPool = sync.Pool{New: func() any { return new(bytes.Buffer) }}
    errorResponsePool  = sync.Pool{New: func() any { return new(errorResponse) }}
)
```

### Decode into a pooled `*createRequest`

```go
req := createRequestPool.Get().(*createRequest)
req.URL = ""
defer func() {
    req.URL = ""
    createRequestPool.Put(req)
}()

if err := json.NewDecoder(r.Body).Decode(req); err != nil {
    ...
}
```

The pool stores `*createRequest` so that `Decode` writes through a
pointer that lives in pooled memory rather than allocating a fresh
struct per call. We reset `URL = ""` both before and after — `Get`
returns a value that some other goroutine just finished with, so a
clean slate is the only safe assumption.

### Encode out of pooled buffer + pooled response

```go
resp := createResponsePool.Get().(*createResponse)
resp.Code = link.Code
resp.ShortURL = "http://" + host + "/" + link.Code
resp.CreatedAt = link.CreatedAt.UTC().Format(time.RFC3339)
writeJSONPooled(w, http.StatusCreated, resp)
resp.Code, resp.ShortURL, resp.CreatedAt = "", "", ""
createResponsePool.Put(resp)
```

`writeJSONPooled` encodes into a pooled `*bytes.Buffer` before writing
to the `http.ResponseWriter` in one `w.Write` call:

```go
func writeJSONPooled(w http.ResponseWriter, status int, body any) {
    buf := responseBufferPool.Get().(*bytes.Buffer)
    defer func() {
        buf.Reset()
        responseBufferPool.Put(buf)
    }()
    if err := json.NewEncoder(buf).Encode(body); err != nil {
        writeError(w, http.StatusInternalServerError, "could not encode response")
        return
    }
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    _, _ = w.Write(buf.Bytes())
}
```

This trades a small bookkeeping cost (the pool's atomic
get/put) for a per-request allocation we used to pay every time.

### Error path uses the same plumbing

`writeError` was the easiest target — every 4xx/5xx response went
through it and produced a fresh `errorResponse{}` literal. Pooled now.

## What escape analysis shows after stage 3

```bash
go build -gcflags="-m" ./internal/api/ 2>&1 | grep escape
```

The remaining "escapes" all happen at package init / cold-start:

```
handlers.go:43:31: new(createRequest)  escapes to heap   ← inside sync.Pool.New
handlers.go:46:31: new(createResponse) escapes to heap   ← inside sync.Pool.New
handlers.go:49:31: new(bytes.Buffer)   escapes to heap   ← inside sync.Pool.New
handlers.go:52:31: new(errorResponse)  escapes to heap   ← inside sync.Pool.New
handlers.go:96:24: ([]byte)("ok")      escapes to heap   ← /healthz response, fine
```

Each pool allocates the *first* time `Get` is called on a fresh `P`
(GOMAXPROCS-worth of caches). After that, the pool hands back
existing pointers — no further heap growth from these types on the
hot path.

The encoder also reports `&json.Encoder{...} does not escape` —
because we feed it a `*bytes.Buffer` we own, stage-3 stack-allocates
the encoder where stage-2 had it escape through the response writer.

## Reproducing the benchmark

```bash
docker compose -f deploy/docker-compose.yml up -d
k6 run benchmarks/redirect-load.js
```

The k6 script is the same `constant-arrival-rate` executor used in
stage 1 and 2 — keeping throughput steady is what makes p99 numbers
comparable across stages. Read the source of
`benchmarks/redirect-load.js` and the inline thresholds (`p(99)<150`,
`p(95)<60`) are the gates stage 3 should clear comfortably.

## What stage 3 deliberately did **not** do

- We did not pool `http.Request` or `http.ResponseWriter` — those are
  owned by `net/http` and pooling them is wrong (server hands you a
  fresh one each time on purpose).
- We did not switch to `easyjson` / `goccy/go-json`. Faster JSON
  libraries exist, but the stage-3 budget was "remove allocations,
  not replace stdlib." A future stage could swap if benchmarks
  justify it.
- We did not pool the redirect path. The redirect handler already
  allocates almost nothing — its hot path is `chi.URLParam` (cached
  by chi) + `Repo.Get` (now cache-hit on stage-2 Redis) + `http.Redirect`.
