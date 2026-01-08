package server

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"n9e-alter-service/internal/config"
	"n9e-alter-service/internal/dingtalk"
	"n9e-alter-service/internal/engine"
	"n9e-alter-service/internal/ingest"
	"n9e-alter-service/internal/n9e"
	"n9e-alter-service/internal/redismgr"
	"n9e-alter-service/internal/rulesrepo"
	"n9e-alter-service/internal/runtime"
	"n9e-alter-service/internal/state"
	"n9e-alter-service/internal/telemetry"
	"n9e-alter-service/internal/workers"
)

type Server struct {
	rt   *runtime.Runtime
	repo *rulesrepo.Repo
	st   *state.Store
	ing  *ingest.Manager
	rm   *redismgr.Manager
	stt  *telemetry.Stats
}

type publishRequest struct {
	RuleSet rulesrepo.RuleSet `json:"rules"`
	Message string            `json:"message"`
	Actor   string            `json:"actor"`
}

type rollbackRequest struct {
	Version string `json:"version"`
	Message string `json:"message"`
	Actor   string `json:"actor"`
}

type rulesVersionGetResponse struct {
	Time    string            `json:"time"`
	Version string            `json:"version"`
	Hash    string            `json:"hash"`
	Rules   rulesrepo.RuleSet `json:"rules"`
}

func (s *Server) handleRoutesPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	var raw any
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}

	batch := make([]n9e.CurEvent, 0, 1)
	switch v := raw.(type) {
	case []any:
		for _, it := range v {
			b, _ := json.Marshal(it)
			var ev n9e.CurEvent
			if err := json.Unmarshal(b, &ev); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid event"})
				return
			}
			batch = append(batch, ev)
		}
	case map[string]any:
		b, _ := json.Marshal(v)
		var ev n9e.CurEvent
		if err := json.Unmarshal(b, &ev); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid event"})
			return
		}
		batch = append(batch, ev)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid payload"})
		return
	}

	if len(batch) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"time": time.Now().UTC().Format(time.RFC3339), "items": []any{}})
		return
	}

	snap := s.rt.Get()
	rr := workers.NewRobotResolver(snap.Cfg)
	items := snap.Eng.PreviewInputs(batch)
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		rc, ok := findRouteConfig(snap.Cfg, it.RouteName)
		routeRobotID := ""
		dtCfg := workersMergeDingTalk(snap.Cfg.DingTalk, config.DingTalkConfig{})
		if ok {
			routeRobotID = rc.Notify.RobotID
			dtCfg = workersMergeDingTalk(snap.Cfg.DingTalk, rc.Notify.DingTalk)
		}

		rec := state.Record{
			RouteName: it.RouteName,
			GroupID:   it.GroupID,
			RuleID:    it.RuleID,
			Severity:  it.Severity,
			Tags:      it.Tags,
			GroupName: it.GroupNameAfter,
			RuleName:  it.RuleNameAfter,
			Entity:    it.EntityAfter,
		}

		robotIDs, matched := rr.ResolveIDsForRecord(rec, it.RouteName, routeRobotID, dtCfg)
		matchedNames := make([]map[string]any, 0, len(matched))
		for _, m := range matched {
			matchedNames = append(matchedNames, map[string]any{"name": m.Name, "priority": m.Priority})
		}

		out = append(out, map[string]any{
			"n9e_hash":          it.N9EHash,
			"n9e_id":            it.N9EID,
			"group_id":          it.GroupID,
			"rule_id":           it.RuleID,
			"severity":          it.Severity,
			"tags":              it.Tags,
			"route_name":        it.RouteName,
			"dedup_key":         it.DedupKey,
			"service_hash":      it.ServiceHash,
			"group_name_before": it.GroupNameBefore,
			"group_name_after":  it.GroupNameAfter,
			"rule_name_before":  it.RuleNameBefore,
			"rule_name_after":   it.RuleNameAfter,
			"entity_before":     it.EntityBefore,
			"entity_after":      it.EntityAfter,
			"matched_bindings":  matchedNames,
			"final_robot_ids":   robotIDs,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"time":  time.Now().UTC().Format(time.RFC3339),
		"items": out,
	})
}

func findRouteConfig(cfg config.Config, routeName string) (config.RouteConfig, bool) {
	routeName = strings.TrimSpace(routeName)
	if routeName == "" {
		return config.RouteConfig{}, false
	}
	for i := range cfg.Routes {
		rc := cfg.Routes[i]
		name := strings.TrimSpace(rc.Name)
		if name == "" {
			name = "route-" + strconv.Itoa(i)
		}
		if strings.EqualFold(name, routeName) {
			return rc, true
		}
	}
	return config.RouteConfig{}, false
}

func workersMergeDingTalk(global config.DingTalkConfig, override config.DingTalkConfig) dingtalk.Config {
	// 复用 workers.mergeDingTalk 的逻辑，但避免 server->workers 的非必要耦合。
	w := strings.TrimSpace(override.Webhook)
	if w == "" {
		w = strings.TrimSpace(global.Webhook)
	}
	s := strings.TrimSpace(override.Secret)
	if s == "" {
		s = strings.TrimSpace(global.Secret)
	}
	k := strings.TrimSpace(override.Keyword)
	if k == "" {
		k = strings.TrimSpace(global.Keyword)
	}
	return dingtalk.Config{Webhook: w, Secret: s, Keyword: k}
}

func New(cfg config.Config, eng *engine.Engine, st *state.Store, ing *ingest.Manager, stats *telemetry.Stats) *Server {
	rt := runtime.New(runtime.Snapshot{Cfg: cfg, Eng: eng})
	repo := rulesrepo.New(cfg.DataDir)
	return &Server{rt: rt, repo: repo, st: st, ing: ing, stt: stats}
}

func NewWithRuntime(rt *runtime.Runtime, repo *rulesrepo.Repo, st *state.Store, ing *ingest.Manager, rm *redismgr.Manager, stats *telemetry.Stats) *Server {
	return &Server{rt: rt, repo: repo, st: st, ing: ing, rm: rm, stt: stats}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/readyz", s.handleReadyz)
	mux.HandleFunc("/api/v1/status", s.handleStatus)
	mux.HandleFunc("/api/v1/metrics", s.handleMetrics)
	mux.HandleFunc("/api/v1/pull/run", s.handlePullRun)
	mux.HandleFunc("/api/v1/events/ingest", s.handleIngest)
	mux.HandleFunc("/api/v1/routes/preview", s.handleRoutesPreview)
	mux.HandleFunc("/api/v1/routes", s.handleRoutes)
	mux.HandleFunc("/api/v1/rules/current", s.handleRulesCurrent)
	mux.HandleFunc("/api/v1/rules/publish", s.handleRulesPublish)
	mux.HandleFunc("/api/v1/rules/rollback", s.handleRulesRollback)
	mux.HandleFunc("/api/v1/rules/audits", s.handleRulesAudits)
	mux.HandleFunc("/api/v1/rules/versions", s.handleRulesVersions)
	mux.HandleFunc("/api/v1/rules/version", s.handleRulesVersionGet)
	mux.HandleFunc("/api/v1/alerts", s.handleAlerts)
	mux.HandleFunc("/api/v1/alerts/get", s.handleAlertGet)

	mux.Handle("/", s.spaHandler())
	h := s.withAuth(mux)
	h = s.withAccessLog(h)
	return h
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusRecorder) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusRecorder) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(p)
	w.bytes += n
	return n, err
}

func (s *Server) withAccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r == nil {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		sw := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		if sw.status == 0 {
			sw.status = http.StatusOK
		}
		dur := time.Since(start)
		if s != nil && s.stt != nil {
			s.stt.ObserveHTTP(sw.status, dur)
		}
		// 仅记录 API/ 以及异常（>=400），避免静态资源刷屏。
		p := ""
		if r.URL != nil {
			p = r.URL.Path
		}
		if strings.HasPrefix(p, "/api/") || sw.status >= 400 {
			log.Printf("http method=%s path=%s status=%d bytes=%d dur_ms=%d", r.Method, p, sw.status, sw.bytes, dur.Milliseconds())
		}
	})
}

func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r == nil {
			next.ServeHTTP(w, r)
			return
		}
		p := r.URL.Path
		if p == "/api/v1/events/ingest" {
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(p, "/healthz") || strings.HasPrefix(p, "/readyz") {
			next.ServeHTTP(w, r)
			return
		}
		if !strings.HasPrefix(p, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		snap := s.rt.Get()
		want := strings.TrimSpace(snap.Cfg.APIToken)
		if want == "" {
			next.ServeHTTP(w, r)
			return
		}
		got := strings.TrimSpace(r.Header.Get("X-Token"))
		if got == "" {
			got = strings.TrimSpace(r.Header.Get("Authorization"))
			got = strings.TrimPrefix(got, "Bearer ")
			got = strings.TrimPrefix(got, "bearer ")
			got = strings.TrimSpace(got)
		}
		if got == "" || got != want {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func maskSecret(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if len(v) <= 6 {
		return "***"
	}
	return v[:2] + "***" + v[len(v)-2:]
}

func sanitizeRuleSet(rs rulesrepo.RuleSet) rulesrepo.RuleSet {
	rs.N9E.UserToken = maskSecret(rs.N9E.UserToken)
	rs.N9E.Authorization = maskSecret(rs.N9E.Authorization)
	rs.Push.Token = maskSecret(rs.Push.Token)
	rs.State.Redis.Password = maskSecret(rs.State.Redis.Password)
	rs.DingTalk.Webhook = maskSecret(rs.DingTalk.Webhook)
	rs.DingTalk.Secret = maskSecret(rs.DingTalk.Secret)
	routes := make([]config.RouteConfig, 0, len(rs.Routes))
	for i := range rs.Routes {
		rc := rs.Routes[i]
		rc.Notify.Webhook.URL = maskSecret(rc.Notify.Webhook.URL)
		if len(rc.Notify.Webhook.Headers) > 0 {
			h := make(map[string]string, len(rc.Notify.Webhook.Headers))
			for k, v := range rc.Notify.Webhook.Headers {
				h[k] = maskSecret(v)
			}
			rc.Notify.Webhook.Headers = h
		}
		if len(rc.Notify.Escalations) > 0 {
			es := make([]config.EscalationConfig, 0, len(rc.Notify.Escalations))
			for j := range rc.Notify.Escalations {
				e := rc.Notify.Escalations[j]
				e.Webhook.URL = maskSecret(e.Webhook.URL)
				if len(e.Webhook.Headers) > 0 {
					h2 := make(map[string]string, len(e.Webhook.Headers))
					for k, v := range e.Webhook.Headers {
						h2[k] = maskSecret(v)
					}
					e.Webhook.Headers = h2
				}
				es = append(es, e)
			}
			rc.Notify.Escalations = es
		}
		routes = append(routes, rc)
	}
	rs.Routes = routes
	robots := make([]config.RobotConfig, 0, len(rs.Robots))
	for i := range rs.Robots {
		r := rs.Robots[i]
		r.Webhook = maskSecret(r.Webhook)
		r.Secret = maskSecret(r.Secret)
		robots = append(robots, r)
	}
	rs.Robots = robots
	return rs
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	active, recovered, total := 0, 0, 0
	if s.st != nil {
		active, recovered, total = s.st.Summary()
	}

	snap := telemetry.Snapshot{}
	if s.stt != nil {
		snap = s.stt.Snapshot()
	} else {
		now := time.Now().Unix()
		snap = telemetry.Snapshot{Time: now, StartedAtUnix: now, UptimeSeconds: 0}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"time":  time.Now().UTC().Format(time.RFC3339),
		"stats": snap,
		"state": map[string]any{"active": active, "recovered": recovered, "total": total},
	})
}

func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if s.ing == nil || !s.ing.Enabled() {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "push ingest disabled"})
		return
	}

	tok := strings.TrimSpace(s.ing.Token())
	if tok != "" {
		got := strings.TrimSpace(r.Header.Get("X-Token"))
		if got == "" {
			got = strings.TrimSpace(r.Header.Get("Authorization"))
			got = strings.TrimPrefix(got, "Bearer ")
			got = strings.TrimPrefix(got, "bearer ")
			got = strings.TrimSpace(got)
		}
		if got == "" || got != tok {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
			return
		}
	}

	var raw any
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}

	batch := make([]n9e.CurEvent, 0, 1)
	switch v := raw.(type) {
	case []any:
		for _, it := range v {
			b, _ := json.Marshal(it)
			var ev n9e.CurEvent
			if err := json.Unmarshal(b, &ev); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid event"})
				return
			}
			batch = append(batch, ev)
		}
	case map[string]any:
		b, _ := json.Marshal(v)
		var ev n9e.CurEvent
		if err := json.Unmarshal(b, &ev); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid event"})
			return
		}
		batch = append(batch, ev)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid payload"})
		return
	}

	if len(batch) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "accepted": 0, "time": time.Now().UTC().Format(time.RFC3339)})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	err := s.ing.Enqueue(ctx, batch)
	if err != nil {
		if errors.Is(err, ingest.ErrQueueFull) {
			writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": "queue full"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"accepted": len(batch),
		"time":     time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	snap := s.rt.Get()

	type notifyItem struct {
		Enabled               bool `json:"enabled"`
		ObserveSeconds        int  `json:"observe_seconds"`
		RepeatIntervalSeconds int  `json:"repeat_interval_seconds"`
		SendRecovered         bool `json:"send_recovered"`
	}

	type routeItem struct {
		Name        string                   `json:"name"`
		Enabled     bool                     `json:"enabled"`
		Match       config.RouteMatchConfig  `json:"match"`
		Dedup       config.DedupConfig       `json:"dedup"`
		Processors  []config.ProcessorConfig `json:"processors"`
		Notify      notifyItem               `json:"notify"`
		DailyReport config.DailyReportConfig `json:"daily_report"`
	}

	items := make([]routeItem, 0, len(snap.Cfg.Routes))
	for i := range snap.Cfg.Routes {
		rc := snap.Cfg.Routes[i]
		name := strings.TrimSpace(rc.Name)
		if name == "" {
			name = "route-" + strconv.Itoa(i)
		}
		items = append(items, routeItem{
			Name:       name,
			Enabled:    rc.Enabled,
			Match:      rc.Match,
			Dedup:      rc.Dedup,
			Processors: rc.Processors,
			Notify: notifyItem{
				Enabled:               rc.Notify.Enabled,
				ObserveSeconds:        rc.Notify.ObserveSeconds,
				RepeatIntervalSeconds: rc.Notify.RepeatIntervalSeconds,
				SendRecovered:         rc.Notify.SendRecovered,
			},
			DailyReport: rc.DailyReport,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"time":  time.Now().UTC().Format(time.RFC3339),
		"items": items,
	})
}

func (s *Server) handleRulesVersions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	items, err := s.repo.ListVersions()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"time":  time.Now().UTC().Format(time.RFC3339),
		"items": items,
	})
}

func (s *Server) handleRulesVersionGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	ver := strings.TrimSpace(r.URL.Query().Get("version"))
	if ver == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "version is required"})
		return
	}
	rs, hash, err := s.repo.ReadVersion(ver)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, rulesVersionGetResponse{
		Time:    time.Now().UTC().Format(time.RFC3339),
		Version: ver,
		Hash:    hash,
		Rules:   sanitizeRuleSet(rs),
	})
}

func (s *Server) spaHandler() http.Handler {
	snap := s.rt.Get()
	fs := http.FileServer(http.Dir(snap.Cfg.WebDir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if p == "" {
			p = "/"
		}
		clean := path.Clean(p)
		if clean == "." {
			clean = "/"
		}
		rel := strings.TrimPrefix(clean, "/")
		if rel != "" {
			full := filepath.Join(snap.Cfg.WebDir, filepath.FromSlash(rel))
			st, err := os.Stat(full)
			if err == nil {
				if !st.IsDir() {
					fs.ServeHTTP(w, r)
					return
				}

				idx := filepath.Join(full, "index.html")
				if fi, err := os.Stat(idx); err == nil && !fi.IsDir() {
					fs.ServeHTTP(w, r)
					return
				}
			}
		}

		indexPath := filepath.Join(snap.Cfg.WebDir, "index.html")
		if _, err := os.Stat(indexPath); err == nil {
			http.ServeFile(w, r, indexPath)
			return
		}

		fs.ServeHTTP(w, r)
	})
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	snap := s.rt.Get()
	st := snap.Eng.Status()
	pd := snap.Eng.PullDebug()
	active, recovered, total := s.st.Summary()
	writeJSON(w, http.StatusOK, map[string]any{
		"time":   time.Now().UTC().Format(time.RFC3339),
		"engine": st,
		"pull_debug": pd,
		"state": map[string]any{
			"active":    active,
			"recovered": recovered,
			"total":     total,
		},
	})
}

func (s *Server) handlePullRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	snap := s.rt.Get()
	if strings.TrimSpace(snap.Cfg.N9E.BaseURL) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "n9e.base_url is empty"})
		return
	}
	res, err := snap.Eng.RunOnce(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	if err := s.st.SaveToFile(snap.Cfg.State.SnapshotFile); err != nil {
		log.Printf("save snapshot: %v", err)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"time":   time.Now().UTC().Format(time.RFC3339),
		"result": res,
		"engine": snap.Eng.Status(),
	})
}

func (s *Server) handleRulesCurrent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	snap := s.rt.Get()
	rs := sanitizeRuleSet(rulesrepo.ExtractFromConfig(snap.Cfg))
	_, hash, ok, err := s.repo.LoadCurrent()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"time":   time.Now().UTC().Format(time.RFC3339),
		"hash":   hash,
		"exists": ok,
		"rules":  rs,
	})
}

func (s *Server) handleRulesPublish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	var req publishRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	ver, hash, err := s.repo.Publish(req.RuleSet, req.Message, req.Actor)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	snap := s.rt.Get()
	newCfg := rulesrepo.ApplyToConfig(snap.Cfg, req.RuleSet)
	eng, err := engine.New(newCfg, s.st)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	s.safelySwapRuntime(newCfg, eng)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"time":    time.Now().UTC().Format(time.RFC3339),
		"version": ver,
		"hash":    hash,
	})
}

func (s *Server) handleRulesRollback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	var req rollbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	ver, hash, err := s.repo.Rollback(req.Version, req.Message, req.Actor)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	rs, _, ok, err := s.repo.LoadCurrent()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "current not found"})
		return
	}
	snap := s.rt.Get()
	newCfg := rulesrepo.ApplyToConfig(snap.Cfg, rs)
	eng, err := engine.New(newCfg, s.st)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	s.safelySwapRuntime(newCfg, eng)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"time":    time.Now().UTC().Format(time.RFC3339),
		"version": ver,
		"hash":    hash,
	})
}

func (s *Server) handleRulesAudits(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	q := r.URL.Query()
	limit := atoi(q.Get("limit"), 50)
	items, err := s.repo.ListAudits(limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"time":  time.Now().UTC().Format(time.RFC3339),
		"items": items,
	})
}

func (s *Server) safelySwapRuntime(cfg config.Config, eng *engine.Engine) {
	if s == nil || s.rt == nil {
		return
	}
	s.rt.Swap(runtime.Snapshot{Cfg: cfg, Eng: eng})
	if s.ing != nil {
		s.ing.SetEngine(eng)
		s.ing.SetConfig(cfg.Push, cfg.State)
	}
	if s.rm != nil {
		s.rm.Apply(cfg.State.Redis)
	}
}

func (s *Server) handleAlerts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	q := r.URL.Query()
	status := strings.TrimSpace(q.Get("status"))
	route := strings.TrimSpace(q.Get("route"))
	offset := atoi(q.Get("offset"), 0)
	limit := atoi(q.Get("limit"), 100)

	st := state.Status(status)
	if st != "" && st != state.StatusActive && st != state.StatusRecovered {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid status"})
		return
	}

	items, total := s.st.List(st, route, offset, limit)
	writeJSON(w, http.StatusOK, map[string]any{
		"time":   time.Now().UTC().Format(time.RFC3339),
		"total":  total,
		"offset": offset,
		"limit":  limit,
		"items":  items,
	})
}

func (s *Server) handleAlertGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	h := strings.TrimSpace(r.URL.Query().Get("hash"))
	if h == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "hash is required"})
		return
	}

	rec, ok := s.st.Get(h)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"time": time.Now().UTC().Format(time.RFC3339),
		"item": rec,
	})
}

func atoi(s string, def int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
