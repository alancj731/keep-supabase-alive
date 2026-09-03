// Package httpapi exposes the keep-alive status, a manual trigger, and health
// endpoints. The paths and JSON field names match the Spring Boot service this
// replaced, so existing health checks and jq commands keep working.
package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alancj731/keep-supabase-alive/internal/keepalive"
)

// NextRunFunc reports when the schedule will next fire, or nil if there is no
// schedule.
type NextRunFunc func() *time.Time

// Server holds everything the handlers need.
type Server struct {
	Service  *keepalive.Service
	Cron     string
	Timezone string
	NextRun  NextRunFunc
	APIToken string
	// CronSecret is the token Vercel Cron sends; accepted alongside APIToken.
	CronSecret  string
	ShowDetails bool
	Logger      *slog.Logger
}

type scheduleView struct {
	Cron      string     `json:"cron"`
	Timezone  string     `json:"timezone"`
	NextRunAt *time.Time `json:"nextRunAt"`
}

type runView struct {
	Trigger    string    `json:"trigger"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt"`
	DurationMs int64     `json:"durationMs"`
	Total      int       `json:"total"`
	Succeeded  int       `json:"succeeded"`
	Failed     int       `json:"failed"`
}

type statusView struct {
	GeneratedAt  time.Time              `json:"generatedAt"`
	Running      bool                   `json:"running"`
	ProjectCount int                    `json:"projectCount"`
	Schedule     scheduleView           `json:"schedule"`
	LastRun      *runView               `json:"lastRun"`
	Projects     []keepalive.PingResult `json:"projects"`
}

type runResponse struct {
	Run      runView                `json:"run"`
	Projects []keepalive.PingResult `json:"projects"`
}

// Handler builds the router. The bearer-token check wraps /api/ only; the
// health endpoints stay open so platform probes keep working.
func (s *Server) Handler() http.Handler {
	api := http.NewServeMux()
	api.HandleFunc("GET /api/keepalive/status", s.handleStatus)
	api.HandleFunc("POST /api/keepalive/run", s.handleRun)
	// GET too: hosted schedulers (Vercel Cron among them) trigger with GET.
	api.HandleFunc("GET /api/keepalive/run", s.handleRun)

	mux := http.NewServeMux()
	mux.Handle("/api/", s.requireToken(api))
	mux.HandleFunc("GET /actuator/health", s.handleHealth)
	mux.HandleFunc("GET /actuator/health/liveness", s.handleLiveness)
	return mux
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	view := statusView{
		GeneratedAt:  time.Now().UTC(),
		Running:      s.Service.IsRunning(),
		ProjectCount: s.Service.ProjectCount(),
		Schedule:     scheduleView{Cron: s.Cron, Timezone: s.Timezone, NextRunAt: s.nextRun()},
		Projects:     s.Service.LastResults(),
	}
	if last := s.Service.LastRun(); last != nil {
		summary := toRunView(*last)
		view.LastRun = &summary
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	summary, err := s.Service.RunAll(r.Context(), "manual")
	if errors.Is(err, keepalive.ErrRunInProgress) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, runResponse{Run: toRunView(summary), Projects: summary.Results})
}

// handleHealth reports DOWN when any project's most recent query failed, so a
// paused or misconfigured project is visible. Container health checks should
// use /actuator/health/liveness instead.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	results := s.Service.LastResults()
	body := map[string]any{}

	if len(results) == 0 {
		body["status"] = "UNKNOWN"
		if s.ShowDetails {
			body["details"] = map[string]any{
				"message":  "no keep-alive run has completed yet",
				"projects": s.Service.ProjectCount(),
			}
		}
		writeJSON(w, http.StatusOK, body)
		return
	}

	details := make(map[string]string, len(results))
	anyFailed := false
	for _, result := range results {
		// Keyed by project id as well: two projects can share a host, user and table.
		key := result.ProjectID + " " + result.ProjectName + " [" + result.Table + "]"
		if result.Success {
			details[key] = "ok in " + strconv.FormatInt(result.DurationMs, 10) + " ms at " + result.CheckedAt.UTC().Format(time.RFC3339Nano)
		} else {
			message := ""
			if result.Error != nil {
				message = *result.Error
			}
			details[key] = "FAILED at " + result.CheckedAt.UTC().Format(time.RFC3339Nano) + ": " + message
			anyFailed = true
		}
	}

	status := http.StatusOK
	body["status"] = "UP"
	if anyFailed {
		status = http.StatusServiceUnavailable
		body["status"] = "DOWN"
	}
	if s.ShowDetails {
		body["details"] = details
	}
	writeJSON(w, status, body)
}

// handleLiveness answers while the process is up. An unreachable Supabase
// project must not restart the container.
func (s *Server) handleLiveness(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "UP"})
}

func (s *Server) requireToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accepted := make([]string, 0, 2)
		for _, token := range []string{s.APIToken, s.CronSecret} {
			if token != "" {
				accepted = append(accepted, token)
			}
		}
		if len(accepted) == 0 {
			next.ServeHTTP(w, r)
			return
		}
		const prefix = "bearer "
		header := r.Header.Get("Authorization")
		provided := ""
		if len(header) >= len(prefix) && strings.EqualFold(header[:len(prefix)], prefix) {
			provided = strings.TrimSpace(header[len(prefix):])
		}
		for _, token := range accepted {
			if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) == 1 {
				next.ServeHTTP(w, r)
				return
			}
		}
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	})
}

func (s *Server) nextRun() *time.Time {
	if s.NextRun == nil {
		return nil
	}
	return s.NextRun()
}

func toRunView(summary keepalive.RunSummary) runView {
	return runView{
		Trigger: summary.Trigger, StartedAt: summary.StartedAt, FinishedAt: summary.FinishedAt,
		DurationMs: summary.DurationMs, Total: summary.Total,
		Succeeded: summary.Succeeded, Failed: summary.Failed,
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
