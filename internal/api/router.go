package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ysqss/watchdog/internal/alerter"
	"github.com/ysqss/watchdog/internal/config"
	"github.com/ysqss/watchdog/internal/scheduler"
	"github.com/ysqss/watchdog/internal/store"
	"github.com/ysqss/watchdog/internal/target"
)

type Server struct {
	store     *store.Store
	cfg       *config.Manager
	scheduler *scheduler.Scheduler
	alerter   *alerter.Alerter
	mux       *http.ServeMux
}

func NewServer(
	st *store.Store,
	cfg *config.Manager,
	sched *scheduler.Scheduler,
	al *alerter.Alerter,
) *Server {
	s := &Server{
		store:     st,
		cfg:       cfg,
		scheduler: sched,
		alerter:   al,
		mux:       http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("/api/v1/targets", s.handleTargets)
	s.mux.HandleFunc("/api/v1/targets/", s.handleTargetByID)
	s.mux.HandleFunc("/api/v1/overview", s.handleOverview)
	s.mux.HandleFunc("/api/public/status", s.handlePublicStatus)
	s.mux.HandleFunc("/health", s.handleHealth)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	status := "ok"
	details := map[string]any{}

	if err := s.store.Ping(); err != nil {
		status = "degraded"
		details["database"] = map[string]any{
			"status": "error",
			"error":  err.Error(),
		}
	} else {
		details["database"] = map[string]any{"status": "ok"}
	}

	states := s.scheduler.GetAllStates()
	unhealthy := 0
	for _, state := range states {
		if state == target.StateUnhealthy {
			unhealthy++
		}
	}
	if len(states) > 0 && unhealthy == len(states) {
		status = "unhealthy"
		details["scheduler"] = map[string]any{
			"status":  "all_unhealthy",
			"targets": len(states),
		}
	} else {
		details["scheduler"] = map[string]any{
			"status":  "ok",
			"targets": len(states),
		}
	}

	code := http.StatusOK
	switch status {
	case "unhealthy":
		code = http.StatusServiceUnavailable
	case "degraded":
		code = http.StatusServiceUnavailable
	}

	writeJSON(w, code, map[string]any{
		"status":  status,
		"details": details,
	})
}

func (s *Server) handleTargets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		targets, err := s.store.ListTargets()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		if targets == nil {
			targets = []*target.Target{}
		}

		states := s.scheduler.GetAllStates()
		result := make([]map[string]any, 0, len(targets))
		for _, t := range targets {
			item := map[string]any{
				"id":                    t.ID,
				"name":                  t.Name,
				"type":                  string(t.Type),
				"url":                   t.URL,
				"interval_ms":           t.Interval,
				"timeout_ms":            t.Timeout,
				"degraded_threshold_ms": t.DegradedThresholdMs,
				"state":                 "unknown",
				"alert_level":           string(t.GetAlertLevel()),
				"silenced":              t.IsSilenced(),
				"labels":                t.Labels,
				"created_at":            t.CreatedAt.Format(time.RFC3339),
			}
			if state, ok := states[t.ID]; ok {
				item["state"] = state.String()
			}
			result = append(result, item)
		}

		writeJSON(w, http.StatusOK, map[string]any{"targets": result})

	case http.MethodPost:
		var t target.Target
		if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
			return
		}
		if t.ID == "" || t.Name == "" || t.URL == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "id, name, and url are required"})
			return
		}
		if t.Type != target.ProbeHTTP && t.Type != target.ProbeTCP {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "type must be http or tcp"})
			return
		}

		t.CreatedAt = time.Now()
		t.UpdatedAt = time.Now()

		if err := s.store.InsertTarget(&t); err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint") {
				writeJSON(w, http.StatusConflict, map[string]any{"error": "target id already exists"})
			} else {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			}
			return
		}

		s.scheduler.AddTarget(&t)

		writeJSON(w, http.StatusCreated, map[string]any{
			"message": "target created",
			"id":      t.ID,
		})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

func (s *Server) handleTargetByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/targets/")

	parts := strings.SplitN(path, "/", 2)
	id := parts[0]

	if len(parts) == 2 {
		action := parts[1]
		switch action {
		case "pause":
			s.handlePause(w, r, id)
			return
		case "resume":
			s.handleResume(w, r, id)
			return
		case "history":
			s.handleHistory(w, r, id)
			return
		case "events":
			s.handleEvents(w, r, id)
			return
		case "latency":
			s.handleLatency(w, r, id)
			return
		}
	}

	switch r.Method {
	case http.MethodGet:
		t, err := s.store.GetTargetByID(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "target not found"})
			return
		}

		state, _ := s.scheduler.GetTargetState(id)
		p50, p95, p99, _ := s.scheduler.GetTargetLatency(id)

		writeJSON(w, http.StatusOK, map[string]any{
			"target":      t,
			"state":       state.String(),
			"latency_p50": p50.String(),
			"latency_p95": p95.String(),
			"latency_p99": p99.String(),
		})

	case http.MethodPut:
		existing, err := s.store.GetTargetByID(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "target not found"})
			return
		}

		var t target.Target
		if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
			return
		}

		t.ID = existing.ID
		t.CreatedAt = existing.CreatedAt
		t.UpdatedAt = time.Now()

		if t.Name == "" {
			t.Name = existing.Name
		}
		if t.URL == "" {
			t.URL = existing.URL
		}
		if t.Type == "" {
			t.Type = existing.Type
		}

		if err := s.store.UpdateTarget(&t); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}

		s.scheduler.RemoveTarget(id)
		s.scheduler.AddTarget(&t)

		writeJSON(w, http.StatusOK, map[string]any{
			"message": "target updated",
			"id":      t.ID,
		})

	case http.MethodDelete:
		s.scheduler.RemoveTarget(id)
		if err := s.store.DeleteTarget(id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"message": "target deleted"})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

func (s *Server) handlePause(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	silencedUntil := time.Now().Add(24 * time.Hour)
	if err := s.store.UpdateTargetSilence(id, &silencedUntil); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"message":        "target paused",
		"silenced_until": silencedUntil.Format(time.RFC3339),
	})
}

func (s *Server) handleResume(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	if err := s.store.UpdateTargetSilence(id, nil); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "target resumed"})
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 100
	}

	records, err := s.store.ListProbeHistory(id, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if records == nil {
		records = []*store.ProbeHistoryRecord{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"history": records,
		"count":   len(records),
	})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}

	events, err := s.store.ListEvents(id, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if events == nil {
		events = []*store.EventRecord{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"events": events,
		"count":  len(events),
	})
}

func (s *Server) handleLatency(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	p50, p95, p99, ok := s.scheduler.GetTargetLatency(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "target not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"target_id": id,
		"p50_ms":    p50.Milliseconds(),
		"p95_ms":    p95.Milliseconds(),
		"p99_ms":    p99.Milliseconds(),
	})
}

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	targets, _ := s.store.ListTargets()
	states := s.scheduler.GetAllStates()

	total := len(targets)
	healthy := 0
	unhealthy := 0
	degraded := 0
	unknown := 0

	for _, t := range targets {
		switch states[t.ID] {
		case target.StateHealthy:
			healthy++
		case target.StateUnhealthy:
			unhealthy++
		case target.StateDegraded:
			degraded++
		default:
			unknown++
		}
	}

	overall := "all_healthy"
	if unhealthy > 0 && unhealthy < total {
		overall = "partial_outage"
	} else if unhealthy == total && total > 0 {
		overall = "major_outage"
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"total":     total,
		"healthy":   healthy,
		"unhealthy": unhealthy,
		"degraded":  degraded,
		"unknown":   unknown,
		"overall":   overall,
	})
}

func (s *Server) handlePublicStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	cfg := s.cfg.Get()
	if !cfg.Server.StatusPageEnabled {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "status page disabled"})
		return
	}

	targets, _ := s.store.ListTargets()
	states := s.scheduler.GetAllStates()

	services := make([]map[string]any, 0, len(targets))
	for _, t := range targets {
		state := "unknown"
		if s, ok := states[t.ID]; ok {
			state = s.String()
		}

		p50, _, _, _ := s.scheduler.GetTargetLatency(t.ID)
		stats, _ := s.store.GetTargetStats(t.ID)

		service := map[string]any{
			"id":             t.ID,
			"name":           t.Name,
			"status":         state,
			"latency_p50_ms": p50.Milliseconds(),
			"last_checked":   time.Now().Format(time.RFC3339),
		}
		if uptime, ok := stats["uptime_pct"]; ok {
			service["uptime_30d"] = uptime
		}
		if state == "unhealthy" {
			service["message"] = "Service Unhealthy"
		}
		services = append(services, service)
	}

	overall := "all_healthy"
	for _, svc := range services {
		if svc["status"] == "unhealthy" {
			overall = "partial_outage"
			break
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"timestamp": time.Now().Format(time.RFC3339),
		"overall":   overall,
		"services":  services,
	})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("failed to write JSON response", "error", err)
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func authMiddleware(next http.Handler, cfg *config.Manager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/") {
			token := r.Header.Get("Authorization")
			token = strings.TrimPrefix(token, "Bearer ")

			c := cfg.Get()
			if c.Server.AdminToken != "" && token != c.Server.AdminToken {
				writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		slog.Info("request", "method", r.Method, "path", r.URL.Path, "remote", r.RemoteAddr)
		next.ServeHTTP(w, r)
		slog.Debug("request completed", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start))
	})
}

func ApplyMiddleware(handler http.Handler, cfg *config.Manager) http.Handler {
	h := handler
	h = corsMiddleware(h)
	h = loggingMiddleware(h)
	h = authMiddleware(h, cfg)
	return h
}
