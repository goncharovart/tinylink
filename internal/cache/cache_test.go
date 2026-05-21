package cache

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMemoryCache_GetMiss(t *testing.T) {
	c := NewMemory()
	if _, err := c.Get(context.Background(), "missing"); !errors.Is(err, ErrMiss) {
		t.Fatalf("expected ErrMiss, got %v", err)
	}
}

func TestMemoryCache_SetGetRoundtrip(t *testing.T) {
	c := NewMemory()
	ctx := context.Background()
	if err := c.Set(ctx, "k", "v", 0); err != nil {
		t.Fatal(err)
	}
	got, err := c.Get(ctx, "k")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "v" {
		t.Errorf("Get returned %q, want %q", got, "v")
	}
}

func TestMemoryCache_TTLExpiry(t *testing.T) {
	c := NewMemory()
	frozen := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return frozen }

	if err := c.Set(context.Background(), "k", "v", time.Minute); err != nil {
		t.Fatal(err)
	}

	// Before expiry — present.
	got, err := c.Get(context.Background(), "k")
	if err != nil || got != "v" {
		t.Fatalf("pre-expiry Get = %q, %v", got, err)
	}

	// Move time forward past TTL.
	c.now = func() time.Time { return frozen.Add(2 * time.Minute) }
	if _, err := c.Get(context.Background(), "k"); !errors.Is(err, ErrMiss) {
		t.Fatalf("post-expiry expected ErrMiss, got %v", err)
	}
}

func TestMemoryCache_ZeroTTLNeverExpires(t *testing.T) {
	c := NewMemory()
	frozen := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return frozen }
	if err := c.Set(context.Background(), "k", "v", 0); err != nil {
		t.Fatal(err)
	}
	// Far in the future, still present.
	c.now = func() time.Time { return frozen.Add(100 * 365 * 24 * time.Hour) }
	got, err := c.Get(context.Background(), "k")
	if err != nil || got != "v" {
		t.Fatalf("zero-TTL value disappeared: %q, %v", got, err)
	}
}
