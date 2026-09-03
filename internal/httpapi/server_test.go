package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alancj731/keep-supabase-alive/internal/keepalive"
)

type stubConnector struct{ err error }

func (s stubConnector) Ping(ctx context.Context, project keepalive.Project) (int, error) {
	if s.err != nil {
		return 0, s.err
	}
	return 1, nil
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError + 1}))
}

func newServer(t *testing.T, connector keepalive.Connector, token string) *Server {
	t.Helper()
	projects, err := keepalive.BuildProjects(
		[]string{
			"postgresql://postgres.aaa:pw1@aws-0-us-east-1.pooler.supabase.com:5432/postgres",
			"postgresql://postgres.bbb:pw2@aws-0-eu-west-2.pooler.supabase.com:5432/postgres",
		},
		[]string{"public.allowed_emails", "public.calls"})
	if err != nil {
		t.Fatalf("BuildProjects: %v", err)
	}
	service := keepalive.NewService(projects, connector,
		keepalive.Options{RetryAttempts: 1, RetryBackoff: 0}, quietLogger())
	next := time.Date(2026, 9, 4, 3, 0, 0, 0, time.UTC)
	return &Server{
		Service: service, Cron: "0 0 3 * * *", Timezone: "UTC",
		NextRun:     func() *time.Time { return &next },
		APIToken:    token,
		ShowDetails: true,
		Logger:      quietLogger(),
	}
}

func do(t *testing.T, server *Server, method, path, token string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	request := httptest.NewRequest(method, path, nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)

	var body map[string]any
	if recorder.Body.Len() > 0 {
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatalf("response is not JSON: %v (%s)", err, recorder.Body.String())
		}
	}
	return recorder, body
}

func TestStatusBeforeFirstRun(t *testing.T) {
	server := newServer(t, stubConnector{}, "")

	recorder, body := do(t, server, http.MethodGet, "/api/keepalive/status", "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	if body["projectCount"].(float64) != 2 {
		t.Errorf("projectCount = %v, want 2", body["projectCount"])
	}
	if body["running"].(bool) {
		t.Error("running should be false")
	}
	if body["lastRun"] != nil {
		t.Errorf("lastRun = %v, want null before the first run", body["lastRun"])
	}
	if len(body["projects"].([]any)) != 0 {
		t.Errorf("projects = %v, want empty", body["projects"])
	}
	schedule := body["schedule"].(map[string]any)
	if schedule["cron"] != "0 0 3 * * *" || schedule["timezone"] != "UTC" || schedule["nextRunAt"] == nil {
		t.Errorf("schedule = %v", schedule)
	}
}

func TestStatusAfterRunKeepsJavaFieldNames(t *testing.T) {
	server := newServer(t, stubConnector{}, "")
	if _, err := server.Service.RunAll(context.Background(), "test"); err != nil {
		t.Fatal(err)
	}

	_, body := do(t, server, http.MethodGet, "/api/keepalive/status", "")

	lastRun := body["lastRun"].(map[string]any)
	for _, key := range []string{"trigger", "startedAt", "finishedAt", "durationMs", "total", "succeeded", "failed"} {
		if _, ok := lastRun[key]; !ok {
			t.Errorf("lastRun is missing %q", key)
		}
	}
	projects := body["projects"].([]any)
	if len(projects) != 2 {
		t.Fatalf("projects = %d, want 2", len(projects))
	}
	first := projects[0].(map[string]any)
	for _, key := range []string{"projectId", "projectName", "host", "port", "database", "table",
		"success", "attempts", "durationMs", "rowsSeen", "error", "checkedAt"} {
		if _, ok := first[key]; !ok {
			t.Errorf("project payload is missing %q", key)
		}
	}
	if first["projectId"] != "p1" || first["table"] != "public.allowed_emails" {
		t.Errorf("first project = %v", first)
	}
	if first["success"] != true {
		t.Errorf("success = %v", first["success"])
	}
}

func TestRunTriggersAPass(t *testing.T) {
	server := newServer(t, stubConnector{}, "")

	recorder, body := do(t, server, http.MethodPost, "/api/keepalive/run", "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	run := body["run"].(map[string]any)
	if run["trigger"] != "manual" || run["total"].(float64) != 2 || run["succeeded"].(float64) != 2 {
		t.Errorf("run = %v", run)
	}
	if len(body["projects"].([]any)) != 2 {
		t.Errorf("projects = %v", body["projects"])
	}
}

func TestRunReturnsConflictWhileAnotherRunIsInFlight(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	server := newServer(t, blockingConnector{started: started, release: release}, "")

	go func() { _, _ = server.Service.RunAll(context.Background(), "first") }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("first run never started")
	}

	recorder, body := do(t, server, http.MethodPost, "/api/keepalive/run", "")
	close(release)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", recorder.Code)
	}
	if body["error"] != "a keep-alive run is already in progress" {
		t.Errorf("error = %v", body["error"])
	}
}

type blockingConnector struct {
	started chan struct{}
	release chan struct{}
}

func (b blockingConnector) Ping(ctx context.Context, project keepalive.Project) (int, error) {
	select {
	case <-b.started:
	default:
		close(b.started)
	}
	<-b.release
	return 1, nil
}

func TestTokenIsRequiredWhenConfigured(t *testing.T) {
	server := newServer(t, stubConnector{}, "s3cret-token")

	if recorder, body := do(t, server, http.MethodGet, "/api/keepalive/status", ""); recorder.Code != http.StatusUnauthorized {
		t.Errorf("no token: status = %d, want 401 (%v)", recorder.Code, body)
	}
	if recorder, _ := do(t, server, http.MethodGet, "/api/keepalive/status", "wrong"); recorder.Code != http.StatusUnauthorized {
		t.Errorf("wrong token: status = %d, want 401", recorder.Code)
	}
	if recorder, _ := do(t, server, http.MethodGet, "/api/keepalive/status", "s3cret-token"); recorder.Code != http.StatusOK {
		t.Errorf("correct token: status = %d, want 200", recorder.Code)
	}
}

func TestCronSecretIsAcceptedAlongsideTheApiToken(t *testing.T) {
	server := newServer(t, stubConnector{}, "manual-token")
	server.CronSecret = "vercel-cron-secret"

	for name, token := range map[string]string{
		"api token":   "manual-token",
		"cron secret": "vercel-cron-secret",
	} {
		t.Run(name, func(t *testing.T) {
			if recorder, body := do(t, server, http.MethodGet, "/api/keepalive/status", token); recorder.Code != http.StatusOK {
				t.Errorf("status = %d, want 200 (%v)", recorder.Code, body)
			}
		})
	}
	if recorder, _ := do(t, server, http.MethodGet, "/api/keepalive/status", "neither"); recorder.Code != http.StatusUnauthorized {
		t.Errorf("unknown token: status = %d, want 401", recorder.Code)
	}
}

// Hosted schedulers trigger with GET, so /run must accept both verbs.
func TestRunAcceptsGetAsWellAsPost(t *testing.T) {
	server := newServer(t, stubConnector{}, "")

	for _, method := range []string{http.MethodGet, http.MethodPost} {
		t.Run(method, func(t *testing.T) {
			recorder, body := do(t, server, method, "/api/keepalive/run", "")
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", recorder.Code)
			}
			run := body["run"].(map[string]any)
			if run["succeeded"].(float64) != 2 {
				t.Errorf("run = %v", run)
			}
		})
	}
}

func TestHealthEndpointsStayOpenWhenTokenIsSet(t *testing.T) {
	server := newServer(t, stubConnector{}, "s3cret-token")

	for _, path := range []string{"/actuator/health", "/actuator/health/liveness"} {
		if recorder, _ := do(t, server, http.MethodGet, path, ""); recorder.Code != http.StatusOK {
			t.Errorf("%s = %d, want 200 without a token", path, recorder.Code)
		}
	}
}

func TestHealthIsUnknownBeforeFirstRun(t *testing.T) {
	server := newServer(t, stubConnector{}, "")

	recorder, body := do(t, server, http.MethodGet, "/actuator/health", "")

	if recorder.Code != http.StatusOK || body["status"] != "UNKNOWN" {
		t.Errorf("status = %d / %v", recorder.Code, body["status"])
	}
}

func TestHealthIsUpWhenAllSucceed(t *testing.T) {
	server := newServer(t, stubConnector{}, "")
	_, _ = server.Service.RunAll(context.Background(), "test")

	recorder, body := do(t, server, http.MethodGet, "/actuator/health", "")

	if recorder.Code != http.StatusOK || body["status"] != "UP" {
		t.Errorf("status = %d / %v", recorder.Code, body["status"])
	}
	if len(body["details"].(map[string]any)) != 2 {
		t.Errorf("details = %v, want one entry per project", body["details"])
	}
}

func TestHealthIsDownWithServiceUnavailableWhenAnyFails(t *testing.T) {
	server := newServer(t, stubConnector{err: errors.New("nope")}, "")
	_, _ = server.Service.RunAll(context.Background(), "test")

	recorder, body := do(t, server, http.MethodGet, "/actuator/health", "")

	if recorder.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", recorder.Code)
	}
	if body["status"] != "DOWN" {
		t.Errorf("status = %v, want DOWN", body["status"])
	}

	// Liveness must stay UP: an unreachable project is not a reason to restart.
	liveness, livenessBody := do(t, server, http.MethodGet, "/actuator/health/liveness", "")
	if liveness.Code != http.StatusOK || livenessBody["status"] != "UP" {
		t.Errorf("liveness = %d / %v", liveness.Code, livenessBody["status"])
	}
}

func TestHealthDetailsCanBeHidden(t *testing.T) {
	server := newServer(t, stubConnector{}, "")
	server.ShowDetails = false
	_, _ = server.Service.RunAll(context.Background(), "test")

	_, body := do(t, server, http.MethodGet, "/actuator/health", "")

	if _, present := body["details"]; present {
		t.Errorf("details should be hidden, got %v", body["details"])
	}
	if body["status"] != "UP" {
		t.Errorf("status = %v", body["status"])
	}
}
