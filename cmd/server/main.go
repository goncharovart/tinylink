// Command server runs tinylink as a single HTTP service.
//
// Configuration is read from environment variables. The naive baseline
// (stage 0) uses the same package layout as the optimized stages so that
// every later optimization is a focused diff.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/goncharovart/tinylink/internal/api"
	"github.com/goncharovart/tinylink/internal/storage"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		logger.Error("DATABASE_URL is required")
		os.Exit(1)
	}
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	// Honour standard process signals so docker/k8s graceful shutdown works.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := newTunedPool(ctx, databaseURL)
	if err != nil {
		logger.Error("connect to Postgres", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		logger.Error("ping Postgres", "error", err)
		os.Exit(1)
	}

	router := api.NewRouter(api.Config{
		Repo:   storage.NewPostgresRepo(pool),
		Logger: logger,
	})

	srv := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		logger.Info("tinylink listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("listen", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	logger.Info("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown", "error", err)
		os.Exit(1)
	}
	logger.Info("tinylink stopped cleanly")
}

// newTunedPool creates a pgxpool.Pool with explicit settings tuned for
// hot-path lookups. Defaults from pgxpool.New are fine for a getting-
// started script but quickly become the bottleneck under load:
//
//   - MaxConns: pgx defaults to GOMAXPROCS (often 4-16). Under load
//     the pool drains and requests serialise on acquisition. The
//     stage-1 setting matches the rule "≈ 2-3 × CPU cores" that pgx
//     docs recommend and that we validated with k6 in benchmarks/.
//   - MinConns: keep a small warm pool so the first burst after a
//     quiet minute does not pay TCP+TLS startup latency for every
//     request.
//   - MaxConnLifetime: rotate connections occasionally so a pgbouncer
//     restart or PG failover does not strand stale handles.
//   - MaxConnIdleTime: trim down to MinConns after a quiet period.
//   - HealthCheckPeriod: catch dead connections proactively, before
//     they surface as a request error.
//
// The exact numbers are deliberately conservative; the point of this
// file is the *shape* of a production-ready pool config, not the
// peak-RPS hero number.
func newTunedPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 20
	cfg.MinConns = 4
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.HealthCheckPeriod = 1 * time.Minute
	return pgxpool.NewWithConfig(ctx, cfg)
}
