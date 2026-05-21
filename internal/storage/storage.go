// Package storage is the persistence layer for tinylink. It exposes a small
// Repository interface so that handlers can be unit-tested against in-memory
// implementations and the real Postgres-backed implementation lives in one
// place.
package storage

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a lookup misses.
var ErrNotFound = errors.New("storage: link not found")

// ErrCodeTaken is returned when an insert collides with an existing code.
var ErrCodeTaken = errors.New("storage: code already exists")

// Link is the row that backs a short URL.
type Link struct {
	Code      string
	URL       string
	CreatedAt time.Time
	HitCount  int64
}

// Repository is the abstraction handlers use. The real Postgres-backed
// implementation lives below; tests use a small in-memory fake.
type Repository interface {
	Save(ctx context.Context, code, url string) (Link, error)
	Get(ctx context.Context, code string) (Link, error)
	IncrementHits(ctx context.Context, code string) error
}

// PostgresRepo is the production Repository implementation using pgxpool.
type PostgresRepo struct {
	pool *pgxpool.Pool
}

// NewPostgresRepo wraps a pre-configured pgxpool.Pool.
func NewPostgresRepo(pool *pgxpool.Pool) *PostgresRepo {
	return &PostgresRepo{pool: pool}
}

const insertLinkSQL = `
INSERT INTO links (code, url) VALUES ($1, $2)
RETURNING code, url, created_at, hit_count
`

func (r *PostgresRepo) Save(ctx context.Context, code, url string) (Link, error) {
	row := r.pool.QueryRow(ctx, insertLinkSQL, code, url)
	var l Link
	if err := row.Scan(&l.Code, &l.URL, &l.CreatedAt, &l.HitCount); err != nil {
		// Postgres unique_violation = SQLSTATE 23505.
		var pgErr interface{ SQLState() string }
		if errors.As(err, &pgErr) && pgErr.SQLState() == "23505" {
			return Link{}, ErrCodeTaken
		}
		return Link{}, err
	}
	return l, nil
}

const selectLinkSQL = `
SELECT code, url, created_at, hit_count
FROM links
WHERE code = $1
`

func (r *PostgresRepo) Get(ctx context.Context, code string) (Link, error) {
	row := r.pool.QueryRow(ctx, selectLinkSQL, code)
	var l Link
	if err := row.Scan(&l.Code, &l.URL, &l.CreatedAt, &l.HitCount); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Link{}, ErrNotFound
		}
		return Link{}, err
	}
	return l, nil
}

const incrementHitsSQL = `
UPDATE links
SET hit_count = hit_count + 1
WHERE code = $1
`

func (r *PostgresRepo) IncrementHits(ctx context.Context, code string) error {
	tag, err := r.pool.Exec(ctx, incrementHitsSQL, code)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// MemoryRepo is an in-memory Repository for unit tests. Safe for concurrent
// use via the embedded mutex.
type MemoryRepo struct {
	mu    sync.RWMutex
	links map[string]*Link
}

// NewMemoryRepo returns an empty in-memory repository.
func NewMemoryRepo() *MemoryRepo {
	return &MemoryRepo{links: make(map[string]*Link)}
}

func (r *MemoryRepo) Save(_ context.Context, code, url string) (Link, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.links[code]; exists {
		return Link{}, ErrCodeTaken
	}
	l := &Link{Code: code, URL: url, CreatedAt: time.Now().UTC()}
	r.links[code] = l
	return *l, nil
}

func (r *MemoryRepo) Get(_ context.Context, code string) (Link, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	l, ok := r.links[code]
	if !ok {
		return Link{}, ErrNotFound
	}
	return *l, nil
}

func (r *MemoryRepo) IncrementHits(_ context.Context, code string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	l, ok := r.links[code]
	if !ok {
		return ErrNotFound
	}
	l.HitCount++
	return nil
}
