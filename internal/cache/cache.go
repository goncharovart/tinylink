// Package cache is the read-through cache layer that sits between
// handlers and storage. Two implementations live here:
//
//   - RedisCache — production, backed by go-redis/v9.
//   - MemoryCache — in-process map + RWMutex for unit tests and the
//     stage-1-versus-stage-2 benchmark when Redis is not available.
//
// Both satisfy the same Cache interface, so handlers do not care
// which backend they got.
package cache

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrMiss is returned by Get when the key is not present. Distinct
// from a real Redis error so callers can use errors.Is to disambiguate.
var ErrMiss = errors.New("cache: miss")

// Cache is the abstraction handlers depend on. Get returns ErrMiss
// on a clean miss; any other error is a backend failure that callers
// should log but treat as a miss (fall through to storage).
type Cache interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
}

// --- RedisCache -----------------------------------------------------------

// RedisCache uses go-redis/v9. The cache is intentionally tiny: we
// store opaque strings keyed by short code, no JSON, no compression.
// Stage-2 storage layer is the only writer of these keys.
type RedisCache struct {
	client *redis.Client
}

// NewRedis builds a RedisCache from a redis URL
// (e.g. "redis://localhost:6379/0").
func NewRedis(url string) (*RedisCache, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	return &RedisCache{client: redis.NewClient(opt)}, nil
}

// Close releases the underlying connection pool. Safe to call multiple
// times.
func (r *RedisCache) Close() error {
	if r.client == nil {
		return nil
	}
	return r.client.Close()
}

func (r *RedisCache) Get(ctx context.Context, key string) (string, error) {
	val, err := r.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrMiss
	}
	return val, err
}

func (r *RedisCache) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return r.client.Set(ctx, key, value, ttl).Err()
}

// --- MemoryCache ----------------------------------------------------------

// MemoryCache is an in-process TTL cache for tests and benchmarks. It
// is safe for concurrent use. TTL is honoured lazily (on Get), which
// is plenty for unit tests and the local benchmark stub.
type MemoryCache struct {
	mu  sync.RWMutex
	now func() time.Time
	m   map[string]memEntry
}

type memEntry struct {
	value     string
	expiresAt time.Time // zero means "never"
}

// NewMemory returns an empty in-process cache.
func NewMemory() *MemoryCache {
	return &MemoryCache{
		m:   make(map[string]memEntry),
		now: time.Now,
	}
}

func (m *MemoryCache) Get(_ context.Context, key string) (string, error) {
	m.mu.RLock()
	e, ok := m.m[key]
	m.mu.RUnlock()
	if !ok {
		return "", ErrMiss
	}
	if !e.expiresAt.IsZero() && !m.now().Before(e.expiresAt) {
		m.mu.Lock()
		delete(m.m, key)
		m.mu.Unlock()
		return "", ErrMiss
	}
	return e.value, nil
}

func (m *MemoryCache) Set(_ context.Context, key, value string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := memEntry{value: value}
	if ttl > 0 {
		e.expiresAt = m.now().Add(ttl)
	}
	m.m[key] = e
	return nil
}

// Compile-time sanity that both backends satisfy Cache.
var (
	_ Cache = (*RedisCache)(nil)
	_ Cache = (*MemoryCache)(nil)
)
