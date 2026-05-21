package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/goncharovart/tinylink/internal/cache"
)

func TestCachedRepo_GetMissThenHit(t *testing.T) {
	ctx := context.Background()
	inner := NewMemoryRepo()
	if _, err := inner.Save(ctx, "abc", "https://example.com/long"); err != nil {
		t.Fatal(err)
	}
	c := cache.NewMemory()
	wrapped := NewCachedRepo(inner, c, time.Minute, nil)

	// First Get: miss → falls through to inner, populates cache.
	link, err := wrapped.Get(ctx, "abc")
	if err != nil {
		t.Fatalf("first Get failed: %v", err)
	}
	if link.URL != "https://example.com/long" {
		t.Errorf("URL mismatch: %q", link.URL)
	}

	// Second Get: cache should now hold the value. Prove by mutating
	// the inner store and showing the cached value still comes back.
	inner.links["abc"].URL = "https://example.com/MUTATED"
	link, err = wrapped.Get(ctx, "abc")
	if err != nil {
		t.Fatalf("second Get failed: %v", err)
	}
	if link.URL != "https://example.com/long" {
		t.Errorf("expected cached value 'long', got %q (cache bypassed)", link.URL)
	}
}

func TestCachedRepo_GetUnknown(t *testing.T) {
	inner := NewMemoryRepo()
	c := cache.NewMemory()
	wrapped := NewCachedRepo(inner, c, time.Minute, nil)

	_, err := wrapped.Get(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestCachedRepo_SaveDoesNotPopulateCache(t *testing.T) {
	ctx := context.Background()
	inner := NewMemoryRepo()
	c := cache.NewMemory()
	wrapped := NewCachedRepo(inner, c, time.Minute, nil)

	if _, err := wrapped.Save(ctx, "abc", "https://example.com"); err != nil {
		t.Fatal(err)
	}

	// Cache should still be empty.
	if _, err := c.Get(ctx, "tinylink:link:abc"); !errors.Is(err, cache.ErrMiss) {
		t.Fatalf("Save unexpectedly populated cache; got %v", err)
	}
}

// flakyCache simulates a backend that returns transient errors so we
// can prove the wrapper degrades gracefully.
type flakyCache struct {
	getErr error
	setErr error
}

func (f *flakyCache) Get(_ context.Context, _ string) (string, error) {
	return "", f.getErr
}
func (f *flakyCache) Set(_ context.Context, _ string, _ string, _ time.Duration) error {
	return f.setErr
}

func TestCachedRepo_FallsThroughOnCacheError(t *testing.T) {
	ctx := context.Background()
	inner := NewMemoryRepo()
	if _, err := inner.Save(ctx, "abc", "https://example.com/long"); err != nil {
		t.Fatal(err)
	}
	wrapped := NewCachedRepo(inner, &flakyCache{getErr: errors.New("redis down")}, time.Minute, nil)

	link, err := wrapped.Get(ctx, "abc")
	if err != nil {
		t.Fatalf("Get should succeed via fallthrough even when cache is down: %v", err)
	}
	if link.URL != "https://example.com/long" {
		t.Errorf("URL mismatch: %q", link.URL)
	}
}

func TestCachedRepo_IncrementHitsAlwaysHitsStorage(t *testing.T) {
	ctx := context.Background()
	inner := NewMemoryRepo()
	if _, err := inner.Save(ctx, "abc", "https://example.com"); err != nil {
		t.Fatal(err)
	}
	c := cache.NewMemory()
	wrapped := NewCachedRepo(inner, c, time.Minute, nil)

	for i := 0; i < 3; i++ {
		if err := wrapped.IncrementHits(ctx, "abc"); err != nil {
			t.Fatalf("iter %d: %v", i, err)
		}
	}
	got, _ := inner.Get(ctx, "abc")
	if got.HitCount != 3 {
		t.Errorf("HitCount = %d, want 3", got.HitCount)
	}
}
