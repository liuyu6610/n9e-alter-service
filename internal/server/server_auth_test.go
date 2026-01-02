package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"n9e-alter-service/internal/config"
	"n9e-alter-service/internal/runtime"
)

func TestWithAuth_AllowsHealthzWithoutToken(t *testing.T) {
	s := &Server{rt: runtime.New(runtime.Snapshot{Cfg: config.Config{APIToken: "tok"}})}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := s.withAuth(next)

	req := httptest.NewRequest(http.MethodGet, "http://example/healthz", nil)
	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, req)
	if rw.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rw.Code)
	}
}

func TestWithAuth_AllowsIngestWithoutAPIToken(t *testing.T) {
	s := &Server{rt: runtime.New(runtime.Snapshot{Cfg: config.Config{APIToken: "tok"}})}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := s.withAuth(next)

	req := httptest.NewRequest(http.MethodPost, "http://example/api/v1/events/ingest", nil)
	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, req)
	if rw.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rw.Code)
	}
}

func TestWithAuth_RequiresTokenForAPI(t *testing.T) {
	s := &Server{rt: runtime.New(runtime.Snapshot{Cfg: config.Config{APIToken: "tok"}})}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := s.withAuth(next)

	req := httptest.NewRequest(http.MethodGet, "http://example/api/v1/status", nil)
	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, req)
	if rw.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rw.Code)
	}
}

func TestWithAuth_AcceptsBearerAndXToken(t *testing.T) {
	s := &Server{rt: runtime.New(runtime.Snapshot{Cfg: config.Config{APIToken: "tok"}})}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := s.withAuth(next)

	req1 := httptest.NewRequest(http.MethodGet, "http://example/api/v1/status", nil)
	req1.Header.Set("Authorization", "Bearer tok")
	rw1 := httptest.NewRecorder()
	h.ServeHTTP(rw1, req1)
	if rw1.Code != http.StatusOK {
		t.Fatalf("expected 200 for bearer, got %d", rw1.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "http://example/api/v1/status", nil)
	req2.Header.Set("X-Token", "tok")
	rw2 := httptest.NewRecorder()
	h.ServeHTTP(rw2, req2)
	if rw2.Code != http.StatusOK {
		t.Fatalf("expected 200 for x-token, got %d", rw2.Code)
	}
}

func TestWithAuth_DisabledWhenTokenEmpty(t *testing.T) {
	s := &Server{rt: runtime.New(runtime.Snapshot{Cfg: config.Config{APIToken: ""}})}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := s.withAuth(next)

	req := httptest.NewRequest(http.MethodGet, "http://example/api/v1/status", nil)
	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, req)
	if rw.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rw.Code)
	}
}
