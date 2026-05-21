package storage

import (
	"context"
	"errors"
	"testing"
)

func TestMemoryRepo_Save_And_Get(t *testing.T) {
	r := NewMemoryRepo()
	ctx := context.Background()

	saved, err := r.Save(ctx, "abc", "https://example.com")
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if saved.Code != "abc" || saved.URL != "https://example.com" {
		t.Fatalf("saved link mismatch: %+v", saved)
	}
	if saved.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}

	got, err := r.Get(ctx, "abc")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.URL != "https://example.com" {
		t.Errorf("Get URL = %q, want %q", got.URL, "https://example.com")
	}
}

func TestMemoryRepo_Save_Duplicate(t *testing.T) {
	r := NewMemoryRepo()
	ctx := context.Background()

	if _, err := r.Save(ctx, "dup", "https://a.com"); err != nil {
		t.Fatal(err)
	}
	_, err := r.Save(ctx, "dup", "https://b.com")
	if !errors.Is(err, ErrCodeTaken) {
		t.Fatalf("expected ErrCodeTaken, got %v", err)
	}
}

func TestMemoryRepo_Get_NotFound(t *testing.T) {
	r := NewMemoryRepo()
	_, err := r.Get(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestMemoryRepo_IncrementHits(t *testing.T) {
	r := NewMemoryRepo()
	ctx := context.Background()
	if _, err := r.Save(ctx, "k", "https://x.com"); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		if err := r.IncrementHits(ctx, "k"); err != nil {
			t.Fatalf("IncrementHits iteration %d: %v", i, err)
		}
	}
	got, _ := r.Get(ctx, "k")
	if got.HitCount != 3 {
		t.Errorf("HitCount = %d, want 3", got.HitCount)
	}
}

func TestMemoryRepo_IncrementHits_NotFound(t *testing.T) {
	r := NewMemoryRepo()
	err := r.IncrementHits(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
