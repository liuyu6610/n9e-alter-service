package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"n9e-alter-service/internal/config"
	"n9e-alter-service/internal/engine"
	"n9e-alter-service/internal/state"
)

type Server struct {
	cfg config.Config
	eng *engine.Engine
	st  *state.Store
}

func New(cfg config.Config, eng *engine.Engine, st *state.Store) *Server {
	return &Server{cfg: cfg, eng: eng, st: st}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/readyz", s.handleReadyz)
	mux.HandleFunc("/api/v1/status", s.handleStatus)
	mux.HandleFunc("/api/v1/pull/run", s.handlePullRun)
	mux.HandleFunc("/api/v1/routes", s.handleRoutes)
	mux.HandleFunc("/api/v1/alerts", s.handleAlerts)
	mux.HandleFunc("/api/v1/alerts/get", s.handleAlertGet)

	fs := http.FileServer(http.Dir(s.cfg.WebDir))
	mux.Handle("/", fs)
	return mux
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	type routeItem struct {
		Name               string `json:"name"`
		Enabled            bool   `json:"enabled"`
		NotifyEnabled      bool   `json:"notify_enabled"`
		DailyReportEnabled bool   `json:"daily_report_enabled"`
	}

	items := make([]routeItem, 0, len(s.cfg.Routes))
	for i := range s.cfg.Routes {
		rc := s.cfg.Routes[i]
		name := strings.TrimSpace(rc.Name)
		if name == "" {
			name = "route-" + strconv.Itoa(i)
		}
		items = append(items, routeItem{
			Name:               name,
			Enabled:            rc.Enabled,
			NotifyEnabled:      rc.Notify.Enabled,
			DailyReportEnabled: rc.DailyReport.Enabled,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"time":  time.Now().UTC().Format(time.RFC3339),
		"items": items,
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
