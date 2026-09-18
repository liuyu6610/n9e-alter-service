package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"n9e-alter-service/internal/config"
	"n9e-alter-service/internal/ingest"
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

func TestWithAuth_IngestUsesPushTokenNotAPIToken(t *testing.T) {
	s := &Server{rt: runtime.New(runtime.Snapshot{Cfg: config.Config{
		APIToken: "api-tok",
		Push:     config.PushConfig{Token: "push-tok"},
	}})}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := s.withAuth(next)

	req := httptest.NewRequest(http.MethodPost, "http://example/api/v1/events/ingest", nil)
	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, req)
	if rw.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", rw.Code)
	}

	reqAPI := httptest.NewRequest(http.MethodPost, "http://example/api/v1/events/ingest", nil)
	reqAPI.Header.Set("X-Token", "api-tok")
	rwAPI := httptest.NewRecorder()
	h.ServeHTTP(rwAPI, reqAPI)
	if rwAPI.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for api token on ingest, got %d", rwAPI.Code)
	}

	reqPush := httptest.NewRequest(http.MethodPost, "http://example/api/v1/events/ingest", nil)
	reqPush.Header.Set("X-Token", "push-tok")
	rwPush := httptest.NewRecorder()
	h.ServeHTTP(rwPush, reqPush)
	if rwPush.Code != http.StatusOK {
		t.Fatalf("expected 200 for push token, got %d", rwPush.Code)
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

func TestWithAuth_EmptyAPITokenRejectsAPI(t *testing.T) {
	s := &Server{rt: runtime.New(runtime.Snapshot{Cfg: config.Config{APIToken: ""}})}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := s.withAuth(next)

	req := httptest.NewRequest(http.MethodGet, "http://example/api/v1/status", nil)
	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, req)
	if rw.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when api_token is empty, got %d", rw.Code)
	}
}

func TestWithAuth_EmptyPushTokenRejectsIngest(t *testing.T) {
	s := &Server{rt: runtime.New(runtime.Snapshot{Cfg: config.Config{
		APIToken: "api-tok",
		Push:     config.PushConfig{Token: ""},
	}})}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := s.withAuth(next)

	req := httptest.NewRequest(http.MethodPost, "http://example/api/v1/events/ingest", nil)
	req.Header.Set("X-Token", "api-tok")
	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, req)
	if rw.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when push.token is empty, got %d", rw.Code)
	}
}

func TestHandleIngest_EmptyPushTokenUnauthorized(t *testing.T) {
	cfg := config.Config{Push: config.PushConfig{Enabled: true, Token: ""}}
	s := &Server{ing: ingest.New(cfg.Push, cfg.State, nil, nil, nil, nil)}

	req := httptest.NewRequest(http.MethodPost, "http://example/api/v1/events/ingest", strings.NewReader("[]"))
	rw := httptest.NewRecorder()
	s.handleIngest(rw, req)
	if rw.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", rw.Code, rw.Body.String())
	}
}

func TestHandleIngest_RequiresMatchingPushToken(t *testing.T) {
	cfg := config.Config{Push: config.PushConfig{Enabled: true, Token: "push-tok"}}
	s := &Server{ing: ingest.New(cfg.Push, cfg.State, nil, nil, nil, nil)}

	req := httptest.NewRequest(http.MethodPost, "http://example/api/v1/events/ingest", strings.NewReader("[]"))
	rw := httptest.NewRecorder()
	s.handleIngest(rw, req)
	if rw.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", rw.Code)
	}

	reqOK := httptest.NewRequest(http.MethodPost, "http://example/api/v1/events/ingest", strings.NewReader("[]"))
	reqOK.Header.Set("Authorization", "Bearer push-tok")
	rwOK := httptest.NewRecorder()
	s.handleIngest(rwOK, reqOK)
	if rwOK.Code != http.StatusOK {
		t.Fatalf("expected 200 with push token, got %d body=%s", rwOK.Code, rwOK.Body.String())
	}
}

func TestHandleIngest_NoWorkersFailsClosed(t *testing.T) {
	cfg := config.Config{Push: config.PushConfig{Enabled: true, Token: "push-tok", QueueSize: 4, WorkerCount: 1}}
	s := &Server{ing: ingest.New(cfg.Push, cfg.State, nil, nil, nil, nil)}

	req := httptest.NewRequest(http.MethodPost, "http://example/api/v1/events/ingest", strings.NewReader(`[{"id":1,"hash":"h"}]`))
	req.Header.Set("Authorization", "Bearer push-tok")
	rw := httptest.NewRecorder()
	s.handleIngest(rw, req)
	if rw.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 with no workers, got %d body=%s", rw.Code, rw.Body.String())
	}
}
