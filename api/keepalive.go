// Package handler is the Vercel serverless entry point.
//
// Vercel runs functions, not long-running processes, so the in-process cron
// schedule in main.go cannot be used there. Vercel Cron calls this endpoint
// instead (see vercel.json), and each invocation pings every configured project
// once and reports the outcome.
//
// The service logic itself is shared with the standalone binary: this file only
// handles authentication, wiring and the response.
package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/alancj731/keep-supabase-alive/internal/config"
	"github.com/alancj731/keep-supabase-alive/internal/keepalive"
)

type response struct {
	Trigger    string                 `json:"trigger"`
	Total      int                    `json:"total"`
	Succeeded  int                    `json:"succeeded"`
	Failed     int                    `json:"failed"`
	DurationMs int64                  `json:"durationMs"`
	Projects   []keepalive.PingResult `json:"projects"`
}

// Handler is the entry point Vercel invokes.
func Handler(w http.ResponseWriter, r *http.Request) {
	if !authorized(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	// There is no .env on Vercel; configuration comes from the project's
	// environment variables. LoadDotenv is a no-op when the file is absent, so
	// the same code path works when running this handler locally.
	source, err := config.LoadDotenv()
	if err != nil {
		fail(w, "reading .env", err)
		return
	}
	cfg, err := config.Load(source)
	if err != nil {
		fail(w, "reading configuration", err)
		return
	}

	projects, err := keepalive.BuildProjects(cfg.URLs, cfg.Tables)
	if err != nil {
		fail(w, "invalid configuration", err)
		return
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	connector := &keepalive.PgxConnector{
		ConnectTimeout: cfg.ConnectTimeout,
		QueryTimeout:   cfg.QueryTimeout,
	}
	service := keepalive.NewService(projects, connector, keepalive.Options{
		RetryAttempts: cfg.RetryAttempts,
		RetryBackoff:  cfg.RetryBackoff,
	}, logger)

	// r.Context() carries the platform's deadline, so a slow project cannot
	// outlive the invocation.
	summary, err := service.RunAll(r.Context(), "vercel-cron")
	if err != nil {
		fail(w, "keep-alive run", err)
		return
	}

	// A non-2xx marks the invocation as failed in the Vercel dashboard, which
	// is the signal you want when a project could not be reached.
	status := http.StatusOK
	if summary.Failed > 0 {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, response{
		Trigger: summary.Trigger, Total: summary.Total, Succeeded: summary.Succeeded,
		Failed: summary.Failed, DurationMs: summary.DurationMs, Projects: summary.Results,
	})
}

// authorized accepts the bearer token Vercel Cron sends as CRON_SECRET, or
// KEEPALIVE_API_TOKEN for manual calls. With neither set the endpoint is open,
// which is why the README tells you to set CRON_SECRET.
func authorized(r *http.Request) bool {
	secrets := make([]string, 0, 2)
	for _, key := range []string{"CRON_SECRET", "KEEPALIVE_API_TOKEN"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			secrets = append(secrets, value)
		}
	}
	if len(secrets) == 0 {
		return true
	}

	const prefix = "bearer "
	header := r.Header.Get("Authorization")
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return false
	}
	provided := strings.TrimSpace(header[len(prefix):])
	for _, secret := range secrets {
		if provided == secret {
			return true
		}
	}
	return false
}

func fail(w http.ResponseWriter, what string, err error) {
	// Config errors name the entry number, never the connection string.
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": what + ": " + err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
