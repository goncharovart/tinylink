// Package api wires the HTTP handlers for tinylink. The package depends only
// on storage.Repository, which keeps the surface mock-able and the
// optimization stages comparable (every stage replaces lower-level pieces
// without changing handlers).
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/goncharovart/tinylink/internal/shortener"
	"github.com/goncharovart/tinylink/internal/storage"
)

// Config is the small bag of dependencies the API needs to run.
type Config struct {
	Repo         storage.Repository
	Logger       *slog.Logger
	CodeLength   int           // length of generated short codes (default 7)
	MaxURLLength int           // hard cap on accepted URL size (default 2048)
	MaxAttempts  int           // retries for code-collision (default 5)
	WriteTimeout time.Duration // not enforced here; surfaced for symmetry
}

// NewRouter assembles the chi.Mux. Handlers depend only on Config; the caller
// owns the *http.Server lifecycle.
func NewRouter(cfg Config) http.Handler {
	if cfg.CodeLength == 0 {
		cfg.CodeLength = 7
	}
	if cfg.MaxURLLength == 0 {
		cfg.MaxURLLength = 2048
	}
	if cfg.MaxAttempts == 0 {
		cfg.MaxAttempts = 5
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", handleHealth)
	r.Post("/links", handleCreate(cfg))
	r.Get("/{code}", handleRedirect(cfg))

	return r
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

type createRequest struct {
	URL string `json:"url"`
}

type createResponse struct {
	Code      string `json:"code"`
	ShortURL  string `json:"short_url"`
	CreatedAt string `json:"created_at"`
}

func handleCreate(cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		req.URL = strings.TrimSpace(req.URL)
		if req.URL == "" {
			writeError(w, http.StatusBadRequest, "url is required")
			return
		}
		if len(req.URL) > cfg.MaxURLLength {
			writeError(w, http.StatusBadRequest, "url is too long")
			return
		}
		parsed, err := url.Parse(req.URL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			writeError(w, http.StatusBadRequest, "url must be absolute with scheme and host")
			return
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			writeError(w, http.StatusBadRequest, "url scheme must be http or https")
			return
		}

		for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
			code, err := shortener.Random(cfg.CodeLength)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "could not generate code")
				return
			}
			link, err := cfg.Repo.Save(r.Context(), code, req.URL)
			if err == nil {
				host := r.Host
				if host == "" {
					host = "tinylink.local"
				}
				writeJSON(w, http.StatusCreated, createResponse{
					Code:      link.Code,
					ShortURL:  "http://" + host + "/" + link.Code,
					CreatedAt: link.CreatedAt.UTC().Format(time.RFC3339),
				})
				return
			}
			if errors.Is(err, storage.ErrCodeTaken) {
				continue
			}
			cfg.Logger.Error("save link", "error", err)
			writeError(w, http.StatusInternalServerError, "could not save link")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not allocate unique code after retries")
	}
}

func handleRedirect(cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		code := chi.URLParam(r, "code")
		link, err := cfg.Repo.Get(r.Context(), code)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				http.NotFound(w, r)
				return
			}
			cfg.Logger.Error("lookup link", "code", code, "error", err)
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}

		// Fire-and-forget hit increment so it never blocks the redirect.
		// On a single-node baseline this is acceptable; later stages will
		// batch this through an in-memory counter flushed periodically.
		go func() {
			if err := cfg.Repo.IncrementHits(r.Context(), code); err != nil {
				cfg.Logger.Warn("increment hits", "code", code, "error", err)
			}
		}()

		http.Redirect(w, r, link.URL, http.StatusFound)
	}
}

type errorResponse struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}
