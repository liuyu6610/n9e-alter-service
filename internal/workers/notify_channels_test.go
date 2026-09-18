package workers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"n9e-alter-service/internal/config"
	"n9e-alter-service/internal/dingtalk"
	"n9e-alter-service/internal/state"
)

func dingHandler(ok *int32, fail *int32, succeed bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		if succeed {
			atomic.AddInt32(ok, 1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
			return
		}
		atomic.AddInt32(fail, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func hookHandler(n *int32, status int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		atomic.AddInt32(n, 1)
		w.WriteHeader(status)
	}
}

func seedActive(t *testing.T, st *state.Store, hash string) {
	t.Helper()
	st.ApplyIngest(time.Now(), []state.InputEvent{
		{ServiceHash: hash, RouteName: "default", DedupKey: "dk-" + hash, RuleName: "rule", Entity: "host-1", Severity: 2},
	})
}

func TestTick_PartialChannelSuccessRetriesFailedOnly(t *testing.T) {
	var hookHits, dingOK, dingFail int32
	hookSrv := httptest.NewServer(hookHandler(&hookHits, http.StatusOK))
	defer hookSrv.Close()
	dingSrv := httptest.NewServer(dingHandler(&dingOK, &dingFail, false))
	defer dingSrv.Close()

	st := state.New()
	seedActive(t, st, "h1")

	cfg := config.Config{
		Robots: []config.RobotConfig{{ID: "r1", Webhook: dingSrv.URL}},
		Routes: []config.RouteConfig{{
			Name:    "default",
			Enabled: true,
			Notify: config.NotifyConfig{
				Enabled:               true,
				RobotID:               "r1",
				ObserveSeconds:        0,
				RepeatIntervalSeconds: 3600,
				Webhook:               config.WebhookConfig{Enabled: true, URL: hookSrv.URL, TimeoutSeconds: 2},
			},
			DailyReport: config.DailyReportConfig{MaxLines: 50, MaxChars: 8000},
		}},
	}
	n := NewNotifier(cfg, st, nil, nil)

	n.tick()
	if atomic.LoadInt32(&hookHits) != 1 {
		t.Fatalf("webhook hits=%d want 1", hookHits)
	}
	if atomic.LoadInt32(&dingFail) != 1 {
		t.Fatalf("ding fail hits=%d want 1", dingFail)
	}
	rec, _ := st.Get("h1")
	if rec.ChannelNotifiedAt(state.ChannelWebhook) == 0 {
		t.Fatalf("webhook should be marked notified")
	}
	if rec.ChannelNotifiedAt(state.ChannelRobot("r1")) != 0 {
		t.Fatalf("failed robot should not be marked notified")
	}
	if !rec.ChannelDue(state.ChannelRobot("r1"), time.Now().Unix(), 3600, false) {
		t.Fatalf("failed robot should remain due")
	}

	n.tick()
	if atomic.LoadInt32(&hookHits) != 1 {
		t.Fatalf("webhook re-notified: hits=%d want 1", hookHits)
	}
	if atomic.LoadInt32(&dingFail) != 2 {
		t.Fatalf("failed robot should retry: hits=%d want 2", dingFail)
	}
}

func TestTick_AllChannelsSuccessSkipsUntilRepeat(t *testing.T) {
	var hookHits, dingOK, dingFail int32
	hookSrv := httptest.NewServer(hookHandler(&hookHits, http.StatusOK))
	defer hookSrv.Close()
	dingSrv := httptest.NewServer(dingHandler(&dingOK, &dingFail, true))
	defer dingSrv.Close()

	st := state.New()
	seedActive(t, st, "h1")

	cfg := config.Config{
		Robots: []config.RobotConfig{{ID: "r1", Webhook: dingSrv.URL}},
		Routes: []config.RouteConfig{{
			Name:    "default",
			Enabled: true,
			Notify: config.NotifyConfig{
				Enabled:               true,
				RobotID:               "r1",
				ObserveSeconds:        0,
				RepeatIntervalSeconds: 3600,
				Webhook:               config.WebhookConfig{Enabled: true, URL: hookSrv.URL, TimeoutSeconds: 2},
			},
			DailyReport: config.DailyReportConfig{MaxLines: 50, MaxChars: 8000},
		}},
	}
	n := NewNotifier(cfg, st, nil, nil)
	n.tick()
	n.tick()
	if atomic.LoadInt32(&hookHits) != 1 {
		t.Fatalf("webhook hits=%d want 1", hookHits)
	}
	if atomic.LoadInt32(&dingOK) != 1 {
		t.Fatalf("ding hits=%d want 1", dingOK)
	}
}

func TestTick_LegacyLastNotifiedDoesNotStorm(t *testing.T) {
	var hookHits int32
	hookSrv := httptest.NewServer(hookHandler(&hookHits, http.StatusOK))
	defer hookSrv.Close()

	st := state.New()
	seedActive(t, st, "h1")
	st.MarkNotified("h1", time.Now().Unix())

	cfg := config.Config{
		Routes: []config.RouteConfig{{
			Name:    "default",
			Enabled: true,
			Notify: config.NotifyConfig{
				Enabled:               true,
				ObserveSeconds:        0,
				RepeatIntervalSeconds: 3600,
				Webhook:               config.WebhookConfig{Enabled: true, URL: hookSrv.URL, TimeoutSeconds: 2},
			},
			DailyReport: config.DailyReportConfig{MaxLines: 50, MaxChars: 8000},
		}},
	}
	n := NewNotifier(cfg, st, nil, nil)
	n.tick()
	if atomic.LoadInt32(&hookHits) != 0 {
		t.Fatalf("legacy LastNotified should skip, hits=%d", hookHits)
	}
}

func TestTick_RecoveredPartialChannelRetriesFailedOnly(t *testing.T) {
	var hookHits, dingOK, dingFail int32
	hookSrv := httptest.NewServer(hookHandler(&hookHits, http.StatusOK))
	defer hookSrv.Close()
	dingSrv := httptest.NewServer(dingHandler(&dingOK, &dingFail, false))
	defer dingSrv.Close()

	st := state.New()
	seedActive(t, st, "h1")
	st.ApplyPull(time.Now().Add(time.Second), nil, state.ApplyOptions{RecoverMissCount: 1, RetainRecoveredSeconds: 86400})
	rec, _ := st.Get("h1")
	if rec.Status != state.StatusRecovered {
		t.Fatalf("status=%s want recovered", rec.Status)
	}

	cfg := config.Config{
		Robots: []config.RobotConfig{{ID: "r1", Webhook: dingSrv.URL}},
		Routes: []config.RouteConfig{{
			Name:    "default",
			Enabled: true,
			Notify: config.NotifyConfig{
				Enabled:               true,
				RobotID:               "r1",
				ObserveSeconds:        0,
				RepeatIntervalSeconds: 3600,
				SendRecovered:         true,
				Webhook:               config.WebhookConfig{Enabled: true, URL: hookSrv.URL, TimeoutSeconds: 2},
			},
			DailyReport: config.DailyReportConfig{MaxLines: 50, MaxChars: 8000},
		}},
	}
	n := NewNotifier(cfg, st, nil, nil)
	n.tick()
	n.tick()
	if atomic.LoadInt32(&hookHits) != 1 {
		t.Fatalf("recovered webhook hits=%d want 1", hookHits)
	}
	if atomic.LoadInt32(&dingFail) != 2 {
		t.Fatalf("recovered robot should retry until success, hits=%d want 2", dingFail)
	}
	got, _ := st.Get("h1")
	if got.ChannelNotifiedAt(state.ChannelWebhook) == 0 {
		t.Fatalf("recovered webhook should be marked")
	}
	if got.ChannelNotifiedAt(state.ChannelRobot("r1")) != 0 {
		t.Fatalf("failed recovered robot should not be marked")
	}
}

func TestSendByRobots_WebhookOnlyMarksWebhook(t *testing.T) {
	var hookHits int32
	hookSrv := httptest.NewServer(hookHandler(&hookHits, http.StatusOK))
	defer hookSrv.Close()

	st := state.New()
	seedActive(t, st, "h1")
	rec, _ := st.Get("h1")
	now := time.Now().Unix()

	n := &Notifier{st: st, dt: dingtalk.New(2 * time.Second)}
	webhook := config.WebhookConfig{Enabled: true, URL: hookSrv.URL, TimeoutSeconds: 2}
	n.sendByRobots(config.Config{}, NewRobotResolver(config.Config{}), "default", "", dingtalk.Config{}, webhook, []state.Record{rec}, true, 1, 50, 8000, now, 3600)

	got, _ := st.Get("h1")
	if got.ChannelNotifiedAt(state.ChannelWebhook) != now {
		t.Fatalf("webhook ts=%d want %d", got.ChannelNotifiedAt(state.ChannelWebhook), now)
	}
	if atomic.LoadInt32(&hookHits) != 1 {
		t.Fatalf("hits=%d want 1", hookHits)
	}
}
