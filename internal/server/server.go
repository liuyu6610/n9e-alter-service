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
	"n9e-alter-service/internal/state"
	"n9e-alter-service/internal/telemetry"
	"n9e-alter-service/internal/workers"
)

type Server struct {
	cfg config.Config
	eng *engine.Engine
	st  *state.Store
	ing *ingest.Manager
	stt *telemetry.Stats
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

	rr := workers.NewRobotResolver(s.cfg)
	items := s.eng.PreviewInputs(batch)
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		rc, ok := findRouteConfig(s.cfg, it.RouteName)
		routeRobotID := ""
		dtCfg := workersMergeDingTalk(s.cfg.DingTalk, config.DingTalkConfig{})
		if ok {
			routeRobotID = rc.Notify.RobotID
			dtCfg = workersMergeDingTalk(s.cfg.DingTalk, rc.Notify.DingTalk)
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
			"n9e_hash":           it.N9EHash,
			"n9e_id":             it.N9EID,
			"group_id":           it.GroupID,
			"rule_id":            it.RuleID,
			"severity":           it.Severity,
			"tags":               it.Tags,
			"route_name":         it.RouteName,
			"dedup_key":           it.DedupKey,
			"service_hash":        it.ServiceHash,
			"group_name_before":   it.GroupNameBefore,
			"group_name_after":    it.GroupNameAfter,
			"rule_name_before":    it.RuleNameBefore,
			"rule_name_after":     it.RuleNameAfter,
			"entity_before":       it.EntityBefore,
			"entity_after":        it.EntityAfter,
			"matched_bindings":    matchedNames,
			"final_robot_ids":     robotIDs,
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
	return &Server{cfg: cfg, eng: eng, st: st, ing: ing, stt: stats}
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
	mux.HandleFunc("/api/v1/alerts", s.handleAlerts)
	mux.HandleFunc("/api/v1/alerts/get", s.handleAlertGet)

	mux.Handle("/", s.spaHandler())
	return mux
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
		"time": time.Now().UTC().Format(time.RFC3339),
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
		Notify      notifyItem               `json:"notify"`
		DailyReport config.DailyReportConfig `json:"daily_report"`
	}

	items := make([]routeItem, 0, len(s.cfg.Routes))
	for i := range s.cfg.Routes {
		rc := s.cfg.Routes[i]
		name := strings.TrimSpace(rc.Name)
		if name == "" {
			name = "route-" + strconv.Itoa(i)
		}
		items = append(items, routeItem{
			Name:    name,
			Enabled: rc.Enabled,
			Match:   rc.Match,
			Dedup:   rc.Dedup,
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

func (s *Server) spaHandler() http.Handler {
	fs := http.FileServer(http.Dir(s.cfg.WebDir))
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
			full := filepath.Join(s.cfg.WebDir, filepath.FromSlash(rel))
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

		indexPath := filepath.Join(s.cfg.WebDir, "index.html")
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
	st := s.eng.Status()
	active, recovered, total := s.st.Summary()
	writeJSON(w, http.StatusOK, map[string]any{
		"time":   time.Now().UTC().Format(time.RFC3339),
		"engine": st,
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

	res, err := s.eng.RunOnce(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	if err := s.st.SaveToFile(s.cfg.State.SnapshotFile); err != nil {
		log.Printf("save snapshot: %v", err)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"time":   time.Now().UTC().Format(time.RFC3339),
		"result": res,
		"engine": s.eng.Status(),
	})
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
