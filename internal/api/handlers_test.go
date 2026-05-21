package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/goncharovart/tinylink/internal/storage"
)

func newTestRouter(t *testing.T) (http.Handler, *storage.MemoryRepo) {
	t.Helper()
	repo := storage.NewMemoryRepo()
	r := NewRouter(Config{Repo: repo})
	return r, repo
}

func TestHealth(t *testing.T) {
	r, _ := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if w.Body.String() != "ok" {
		t.Errorf("body = %q, want %q", w.Body.String(), "ok")
	}
}

func TestCreate_HappyPath(t *testing.T) {
	r, repo := newTestRouter(t)

	body := strings.NewReader(`{"url":"https://example.com/long/path?with=query"}`)
	req := httptest.NewRequest(http.MethodPost, "/links", body)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s, want 201", w.Code, w.Body.String())
	}

	var resp createResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Code == "" {
		t.Error("response code is empty")
	}
	if !strings.HasSuffix(resp.ShortURL, "/"+resp.Code) {
		t.Errorf("short_url %q does not end with /%s", resp.ShortURL, resp.Code)
	}

	saved, err := repo.Get(req.Context(), resp.Code)
	if err != nil {
		t.Fatalf("Get after create: %v", err)
	}
	if saved.URL != "https://example.com/long/path?with=query" {
		t.Errorf("stored url mismatch: %q", saved.URL)
	}
}

func TestCreate_BadRequest(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"empty body", ``},
		{"invalid JSON", `{bad json`},
		{"missing url", `{}`},
		{"empty url", `{"url":""}`},
		{"relative url", `{"url":"/relative"}`},
		{"scheme only", `{"url":"http://"}`},
		{"ftp scheme", `{"url":"ftp://example.com"}`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, _ := newTestRouter(t)
			req := httptest.NewRequest(http.MethodPost, "/links", bytes.NewBufferString(c.body))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400 (body=%s)", w.Code, w.Body.String())
			}
		})
	}
}

func TestRedirect_HappyPath(t *testing.T) {
	r, repo := newTestRouter(t)
	if _, err := repo.Save(nil, "fixedcode", "https://destination.example/path"); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/fixedcode", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "https://destination.example/path" {
		t.Errorf("Location = %q, want destination", loc)
	}
}

func TestRedirect_NotFound(t *testing.T) {
	r, _ := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/nopeNope", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
	_, _ = io.Copy(io.Discard, w.Body)
}
