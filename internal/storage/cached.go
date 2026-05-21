package storage

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/goncharovart/tinylink/internal/cache"
)

// CachedRepo wraps a Repository with a read-through cache.
//
// Reads (Get) go to the cache first; on miss they fall through to the
// inner repository and write back. Writes (Save) bypass the cache —
// the next Get of the same code will populate it. IncrementHits is a
// write-through-to-storage operation that ignores the cache entirely;
// hit_count drift in the cached Link value is acceptable for the
// redirect path (we don't surface hit_count there).
//
// On any cache backend error the wrapper logs and falls through to
// storage. The redirect path stays correct even if Redis is down.
type CachedRepo struct {
	inner  Repository
	cache  cache.Cache
	ttl    time.Duration
	logger *slog.Logger
}

// NewCachedRepo wraps `inner` with a read-through cache. A nil cache
// disables caching and returns inner directly via the Repository
// interface — but callers should not pass nil; use storage.NewMemoryRepo
// for tests instead.
func NewCachedRepo(inner Repository, c cache.Cache, ttl time.Duration, logger *slog.Logger) *CachedRepo {
	if logger == nil {
		logger = slog.Default()
	}
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	return &CachedRepo{inner: inner, cache: c, ttl: ttl, logger: logger}
}

func (r *CachedRepo) Save(ctx context.Context, code, url string) (Link, error) {
	// We intentionally don't write to the cache here. The first Get
	// for this code will populate it. Skipping the write keeps Save
	// out of the cache-failure mode (a flaky Redis must not surface
	// as a failed link creation).
	return r.inner.Save(ctx, code, url)
}

func (r *CachedRepo) Get(ctx context.Context, code string) (Link, error) {
	if cached, err := r.cache.Get(ctx, cacheKey(code)); err == nil {
		// The cached value is the destination URL — that is all
		// the redirect path needs.
		return Link{Code: code, URL: cached}, nil
	} else if !errors.Is(err, cache.ErrMiss) {
		r.logger.Warn("cache get failed; falling through to storage",
			"code", code, "error", err,
		)
	}

	link, err := r.inner.Get(ctx, code)
	if err != nil {
		return Link{}, err
	}

	if setErr := r.cache.Set(ctx, cacheKey(code), link.URL, r.ttl); setErr != nil {
		r.logger.Warn("cache set failed",
			"code", code, "error", setErr,
		)
	}
	return link, nil
}

func (r *CachedRepo) IncrementHits(ctx context.Context, code string) error {
	// Always go to storage. hit_count is not surfaced on the redirect
	// path, so a stale cached Link does not need invalidation here.
	return r.inner.IncrementHits(ctx, code)
}

func cacheKey(code string) string { return "tinylink:link:" + code }

// Compile-time check.
var _ Repository = (*CachedRepo)(nil)
