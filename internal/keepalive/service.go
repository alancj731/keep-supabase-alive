package keepalive

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ErrRunInProgress is returned when a run is requested while one is already in
// flight. The HTTP layer turns it into a 409.
var ErrRunInProgress = errors.New("a keep-alive run is already in progress")

// PingResult is the outcome of a single keep-alive query against one project.
// The JSON field names match the Spring Boot service this replaced.
type PingResult struct {
	ProjectID   string    `json:"projectId"`
	ProjectName string    `json:"projectName"`
	Host        string    `json:"host"`
	Port        int       `json:"port"`
	Database    string    `json:"database"`
	Table       string    `json:"table"`
	Success     bool      `json:"success"`
	Attempts    int       `json:"attempts"`
	DurationMs  int64     `json:"durationMs"`
	RowsSeen    *int      `json:"rowsSeen"`
	Error       *string   `json:"error"`
	CheckedAt   time.Time `json:"checkedAt"`
}

// RunSummary is the aggregate outcome of one pass over every project.
type RunSummary struct {
	Trigger    string       `json:"trigger"`
	StartedAt  time.Time    `json:"startedAt"`
	FinishedAt time.Time    `json:"finishedAt"`
	DurationMs int64        `json:"durationMs"`
	Total      int          `json:"total"`
	Succeeded  int          `json:"succeeded"`
	Failed     int          `json:"failed"`
	Results    []PingResult `json:"-"`
}

// Options configures the retry behaviour of a Service.
type Options struct {
	RetryAttempts int
	RetryBackoff  time.Duration
}

// Service runs the keep-alive query against every configured project and
// remembers the latest outcome, in memory only.
type Service struct {
	projects  []Project
	connector Connector
	options   Options
	logger    *slog.Logger

	running atomic.Bool

	mu          sync.RWMutex
	lastResults map[string]PingResult
	lastRun     *RunSummary
}

// NewService creates a Service over an already-validated project list.
func NewService(projects []Project, connector Connector, options Options, logger *slog.Logger) *Service {
	if options.RetryAttempts < 1 {
		options.RetryAttempts = 1
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		projects:    projects,
		connector:   connector,
		options:     options,
		logger:      logger,
		lastResults: make(map[string]PingResult, len(projects)),
	}
}

// RunAll pings every project concurrently and records the outcome. It returns
// ErrRunInProgress if another run has not finished yet.
func (s *Service) RunAll(ctx context.Context, trigger string) (RunSummary, error) {
	if !s.running.CompareAndSwap(false, true) {
		return RunSummary{}, ErrRunInProgress
	}
	defer s.running.Store(false)

	startedAt := time.Now().UTC()
	results := make([]PingResult, len(s.projects))

	var wg sync.WaitGroup
	for i, project := range s.projects {
		wg.Add(1)
		go func(index int, project Project) {
			defer wg.Done()
			results[index] = s.ping(ctx, project)
		}(i, project)
	}
	wg.Wait()

	summary := RunSummary{
		Trigger:    trigger,
		StartedAt:  startedAt,
		FinishedAt: time.Now().UTC(),
		Total:      len(results),
		Results:    results,
	}
	summary.DurationMs = summary.FinishedAt.Sub(summary.StartedAt).Milliseconds()
	for _, result := range results {
		if result.Success {
			summary.Succeeded++
		}
	}
	summary.Failed = summary.Total - summary.Succeeded

	s.mu.Lock()
	for _, result := range results {
		s.lastResults[result.ProjectID] = result
	}
	stored := summary
	s.lastRun = &stored
	s.mu.Unlock()

	s.logger.Info("Keep-alive run finished",
		"trigger", trigger, "ok", summary.Succeeded, "total", summary.Total, "durationMs", summary.DurationMs)
	return summary, nil
}

func (s *Service) ping(ctx context.Context, project Project) PingResult {
	start := time.Now()
	lastError := "unknown error"

	for attempt := 1; attempt <= s.options.RetryAttempts; attempt++ {
		s.logger.Debug("Connecting to project",
			"dsn", project.DSN, "user", project.Username, "attempt", attempt, "of", s.options.RetryAttempts)

		rows, err := s.connector.Ping(ctx, project)
		if err == nil {
			durationMs := time.Since(start).Milliseconds()
			s.logger.Info("Keep-alive OK",
				"project", project.Name, "table", project.Table, "rows", rows,
				"durationMs", durationMs, "attempt", attempt, "of", s.options.RetryAttempts)
			seen := rows
			return PingResult{
				ProjectID: project.ID, ProjectName: project.Name, Host: project.Host, Port: project.Port,
				Database: project.Database, Table: project.Table, Success: true, Attempts: attempt,
				DurationMs: durationMs, RowsSeen: &seen, CheckedAt: time.Now().UTC(),
			}
		}

		lastError = redact(err.Error(), project.Password)
		if attempt < s.options.RetryAttempts {
			s.logger.Warn("Keep-alive attempt failed",
				"project", project.Name, "attempt", attempt, "of", s.options.RetryAttempts, "error", lastError)
			if !s.backoff(ctx, attempt) {
				break
			}
			continue
		}
		s.logger.Error("Keep-alive FAILED",
			"project", project.Name, "table", project.Table,
			"attempts", s.options.RetryAttempts, "error", lastError)
	}

	message := lastError
	return PingResult{
		ProjectID: project.ID, ProjectName: project.Name, Host: project.Host, Port: project.Port,
		Database: project.Database, Table: project.Table, Success: false, Attempts: s.options.RetryAttempts,
		DurationMs: time.Since(start).Milliseconds(), Error: &message, CheckedAt: time.Now().UTC(),
	}
}

// backoff waits before the next attempt, reporting false if the wait was cut
// short by shutdown.
func (s *Service) backoff(ctx context.Context, attempt int) bool {
	wait := s.options.RetryBackoff * time.Duration(attempt)
	if wait <= 0 {
		return true
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// redact scrubs the password out of a driver message, just in case. Very short
// passwords are left alone so unrelated words are not mangled.
func redact(message, password string) string {
	if len(password) >= 4 {
		message = strings.ReplaceAll(message, password, "***")
	}
	return strings.Join(strings.Fields(message), " ")
}

// LastResults returns the latest result per project, in configured order.
// Projects never pinged yet are omitted.
func (s *Service) LastResults() []PingResult {
	s.mu.RLock()
	defer s.mu.RUnlock()

	results := make([]PingResult, 0, len(s.projects))
	for _, project := range s.projects {
		if result, ok := s.lastResults[project.ID]; ok {
			results = append(results, result)
		}
	}
	return results
}

// LastRun returns the most recent run summary, or nil before the first run.
func (s *Service) LastRun() *RunSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.lastRun == nil {
		return nil
	}
	summary := *s.lastRun
	return &summary
}

// IsRunning reports whether a run is in flight.
func (s *Service) IsRunning() bool { return s.running.Load() }

// ProjectCount is the number of configured projects.
func (s *Service) ProjectCount() int { return len(s.projects) }
