package workers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"n9e-alter-service/internal/config"
	"n9e-alter-service/internal/state"
)

func TestIsSilenced_ByRouteAndTags(t *testing.T) {
	n := &Notifier{}
	now := time.Now().Unix()

	cfg := config.Config{
		Silences: []config.SilenceRule{
			{
				Name:          "s1",
				Enabled:       true,
				RouteName:     "default",
				Tags:          map[string]string{"cluster": "prod"},
				TagRegex:      nil,
				ExpiresAtUnix: now + 3600,
			},
		},
	}

	rec := state.Record{RouteName: "default", Tags: map[string]string{"cluster": "prod", "app": "api"}}
	if !n.isSilenced(cfg, "default", rec, now) {
		t.Fatalf("expected silenced")
	}

	rec2 := state.Record{RouteName: "default", Tags: map[string]string{"cluster": "dev"}}
	if n.isSilenced(cfg, "default", rec2, now) {
		t.Fatalf("expected not silenced")
	}
}

func TestIsSilenced_ByRegexAndExpiry(t *testing.T) {
	n := &Notifier{}
	now := time.Now().Unix()

	cfg := config.Config{
		Silences: []config.SilenceRule{
			{
				Name:          "s2",
				Enabled:       true,
				RouteName:     "",
				Tags:          nil,
				TagRegex:      map[string]string{"app": "^api-.*"},
				ExpiresAtUnix: now + 3600,
			},
			{
				Name:          "expired",
				Enabled:       true,
				RouteName:     "default",
				Tags:          map[string]string{"cluster": "prod"},
				ExpiresAtUnix: now - 1,
			},
		},
	}

	rec := state.Record{RouteName: "x", Tags: map[string]string{"app": "api-foo"}}
	if !n.isSilenced(cfg, "x", rec, now) {
		t.Fatalf("expected silenced by regex")
	}

	rec2 := state.Record{RouteName: "default", Tags: map[string]string{"cluster": "prod"}}
	if n.isSilenced(cfg, "default", rec2, now) {
		t.Fatalf("expected not silenced by expired rule")
	}
}

func TestSendWebhook_StatusHandling(t *testing.T) {
	n := &Notifier{}

	okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer okSrv.Close()

	errSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer errSrv.Close()

	cfgOK := config.WebhookConfig{Enabled: true, URL: okSrv.URL, TimeoutSeconds: 2, Headers: map[string]string{"X-Test": "1"}}
	if err := n.sendWebhook(cfgOK, map[string]any{"a": 1}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	cfgErr := config.WebhookConfig{Enabled: true, URL: errSrv.URL, TimeoutSeconds: 2}
	if err := n.sendWebhook(cfgErr, map[string]any{"a": 1}); err == nil {
		t.Fatalf("expected error")
	}
}
