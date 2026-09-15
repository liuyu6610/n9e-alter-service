package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"n9e-alter-service/internal/config"
	"n9e-alter-service/internal/engine"
	"n9e-alter-service/internal/rulesrepo"
	"n9e-alter-service/internal/runtime"
	"n9e-alter-service/internal/state"
)

func TestRestoreSecret_PreservesMaskedAndKeepsNew(t *testing.T) {
	cur := "n9e-user-token-real"
	if got := restoreSecret(maskSecret(cur), cur); got != cur {
		t.Fatalf("masked long secret: got %q", got)
	}
	if got := restoreSecret("***", "short"); got != "short" {
		t.Fatalf("masked short secret: got %q", got)
	}
	if got := restoreSecret("brand-new-token", cur); got != "brand-new-token" {
		t.Fatalf("new secret: got %q", got)
	}
	if got := restoreSecret("", cur); got != "" {
		t.Fatalf("empty should stay empty (explicit clear), got %q", got)
	}
}

func TestRestoreRuleSetSecrets_RestoresNested(t *testing.T) {
	cur := rulesrepo.RuleSet{
		N9E:      config.N9EConfig{UserToken: "n9e-user-token-real", Authorization: "Bearer real-auth"},
		Push:     config.PushConfig{Token: "push-token-real-value"},
		State:    config.StateConfig{Redis: config.RedisConfig{Password: "redis-password-real"}},
		DingTalk: config.DingTalkConfig{Webhook: "https://oapi.dingtalk.com/robot/send?access_token=dt", Secret: "dingtalk-secret-real"},
		Routes: []config.RouteConfig{{
			Name: "default",
			Notify: config.NotifyConfig{
				Webhook: config.WebhookConfig{
					URL:     "https://hooks.example.com/very-secret",
					Headers: map[string]string{"X-Hook": "header-secret-value"},
				},
				Escalations: []config.EscalationConfig{{
					Webhook: config.WebhookConfig{
						URL:     "https://hooks.example.com/esc-secret",
						Headers: map[string]string{"Authorization": "Bearer esc-token"},
					},
				}},
			},
		}},
		Robots: []config.RobotConfig{{
			ID:      "bot1",
			Webhook: "https://oapi.example.com/robot/send?access_token=real",
			Secret:  "robot-secret-real",
		}},
	}
	in := sanitizeRuleSet(cur)
	got := restoreRuleSetSecrets(in, cur)
	if got.N9E.UserToken != cur.N9E.UserToken {
		t.Fatalf("n9e.user_token: %q", got.N9E.UserToken)
	}
	if got.N9E.Authorization != cur.N9E.Authorization {
		t.Fatalf("n9e.authorization: %q", got.N9E.Authorization)
	}
	if got.Push.Token != cur.Push.Token {
		t.Fatalf("push.token: %q", got.Push.Token)
	}
	if got.State.Redis.Password != cur.State.Redis.Password {
		t.Fatalf("redis password: %q", got.State.Redis.Password)
	}
	if got.DingTalk.Webhook != cur.DingTalk.Webhook || got.DingTalk.Secret != cur.DingTalk.Secret {
		t.Fatalf("dingtalk: webhook=%q secret=%q", got.DingTalk.Webhook, got.DingTalk.Secret)
	}
	if got.Routes[0].Notify.Webhook.URL != cur.Routes[0].Notify.Webhook.URL {
		t.Fatalf("route webhook: %q", got.Routes[0].Notify.Webhook.URL)
	}
	if got.Routes[0].Notify.Webhook.Headers["X-Hook"] != "header-secret-value" {
		t.Fatalf("route header: %q", got.Routes[0].Notify.Webhook.Headers["X-Hook"])
	}
	if got.Routes[0].Notify.Escalations[0].Webhook.URL != cur.Routes[0].Notify.Escalations[0].Webhook.URL {
		t.Fatalf("escalation webhook: %q", got.Routes[0].Notify.Escalations[0].Webhook.URL)
	}
	if got.Robots[0].Webhook != cur.Robots[0].Webhook || got.Robots[0].Secret != cur.Robots[0].Secret {
		t.Fatalf("robot: webhook=%q secret=%q", got.Robots[0].Webhook, got.Robots[0].Secret)
	}
}

func TestHandleRulesPublish_DoesNotWriteRedactedSecrets(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.APIToken = "api-tok"
	cfg.DataDir = dir
	cfg.N9E.UserToken = "n9e-user-token-real"
	cfg.N9E.Authorization = "Bearer real-auth"
	cfg.Push.Token = "push-token-real-value"
	cfg.DingTalk.Secret = "dingtalk-secret-real"
	cfg.State.Redis.Password = "redis-password-real"
	cfg.Routes = []config.RouteConfig{{
		Name:    "default",
		Enabled: true,
		Match:   config.RouteMatchConfig{GroupNameRegex: ".*", RuleNameRegex: ".*"},
		Notify: config.NotifyConfig{
			Webhook: config.WebhookConfig{
				URL:     "https://hooks.example.com/very-secret",
				Headers: map[string]string{"X-Hook": "header-secret-value"},
			},
		},
	}}
	cfg.Robots = []config.RobotConfig{{
		ID:      "bot1",
		Webhook: "https://oapi.example.com/robot/send?access_token=real",
		Secret:  "robot-secret-real",
	}}

	st := state.New()
	eng, err := engine.New(cfg, st)
	if err != nil {
		t.Fatal(err)
	}
	repo := rulesrepo.New(dir)
	s := &Server{
		rt:   runtime.New(runtime.Snapshot{Cfg: cfg, Eng: eng}),
		repo: repo,
		st:   st,
	}
	h := s.Handler()

	getReq := httptest.NewRequest(http.MethodGet, "http://example/api/v1/rules/current", nil)
	getReq.Header.Set("X-Token", "api-tok")
	getRW := httptest.NewRecorder()
	h.ServeHTTP(getRW, getReq)
	if getRW.Code != http.StatusOK {
		t.Fatalf("GET current: %d %s", getRW.Code, getRW.Body.String())
	}
	var got struct {
		Rules rulesrepo.RuleSet `json:"rules"`
	}
	if err := json.Unmarshal(getRW.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Rules.N9E.UserToken == cfg.N9E.UserToken {
		t.Fatal("GET should redact n9e.user_token")
	}
	if got.Rules.Push.Token == cfg.Push.Token {
		t.Fatal("GET should redact push.token")
	}

	body, err := json.Marshal(map[string]any{"rules": got.Rules, "message": "roundtrip", "actor": "test"})
	if err != nil {
		t.Fatal(err)
	}
	pubReq := httptest.NewRequest(http.MethodPost, "http://example/api/v1/rules/publish", bytes.NewReader(body))
	pubReq.Header.Set("X-Token", "api-tok")
	pubReq.Header.Set("Content-Type", "application/json")
	pubRW := httptest.NewRecorder()
	h.ServeHTTP(pubRW, pubReq)
	if pubRW.Code != http.StatusOK {
		t.Fatalf("publish: %d %s", pubRW.Code, pubRW.Body.String())
	}

	rs, _, ok, err := repo.LoadCurrent()
	if err != nil || !ok {
		t.Fatalf("load current: ok=%v err=%v", ok, err)
	}
	if rs.N9E.UserToken != cfg.N9E.UserToken {
		t.Fatalf("user_token overwritten: %q", rs.N9E.UserToken)
	}
	if rs.N9E.Authorization != cfg.N9E.Authorization {
		t.Fatalf("authorization overwritten: %q", rs.N9E.Authorization)
	}
	if rs.Push.Token != cfg.Push.Token {
		t.Fatalf("push.token overwritten: %q", rs.Push.Token)
	}
	if rs.DingTalk.Secret != cfg.DingTalk.Secret {
		t.Fatalf("dingtalk.secret overwritten: %q", rs.DingTalk.Secret)
	}
	if rs.State.Redis.Password != cfg.State.Redis.Password {
		t.Fatalf("redis password overwritten: %q", rs.State.Redis.Password)
	}
	if rs.Routes[0].Notify.Webhook.URL != cfg.Routes[0].Notify.Webhook.URL {
		t.Fatalf("webhook url overwritten: %q", rs.Routes[0].Notify.Webhook.URL)
	}
	if rs.Routes[0].Notify.Webhook.Headers["X-Hook"] != "header-secret-value" {
		t.Fatalf("header overwritten: %q", rs.Routes[0].Notify.Webhook.Headers["X-Hook"])
	}
	if rs.Robots[0].Webhook != cfg.Robots[0].Webhook {
		t.Fatalf("robot webhook overwritten: %q", rs.Robots[0].Webhook)
	}
	if rs.Robots[0].Secret != cfg.Robots[0].Secret {
		t.Fatalf("robot secret overwritten: %q", rs.Robots[0].Secret)
	}
}
