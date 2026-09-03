// Command supabase-keepalive keeps free-tier Supabase projects awake by running
// a trivial query against each one on a schedule. The query result is thrown
// away — the point is the database activity, which is what stops Supabase
// pausing a project after ~7 days of inactivity.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/alancj731/keep-supabase-alive/internal/config"
	"github.com/alancj731/keep-supabase-alive/internal/httpapi"
	"github.com/alancj731/keep-supabase-alive/internal/keepalive"
	"github.com/robfig/cron/v3"
)

func main() {
	if err := run(); err != nil {
		// The logger may not exist yet, so fail loudly on stderr either way.
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Load .env before anything reads configuration, including the log level.
	dotenvSource, err := config.LoadDotenv()
	if err != nil {
		return err
	}

	cfg, err := config.Load(dotenvSource)
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)

	projects, err := keepalive.BuildProjects(cfg.URLs, cfg.Tables)
	if err != nil {
		return fmt.Errorf("%w (.env file: %s)", err, describeSource(cfg.DotenvSource))
	}

	connector := &keepalive.PgxConnector{
		ConnectTimeout: cfg.ConnectTimeout,
		QueryTimeout:   cfg.QueryTimeout,
		OnConnect: func(project keepalive.Project, sql string) {
			logger.Debug("Connected", "project", project.Name, "sql", sql)
		},
	}
	service := keepalive.NewService(projects, connector, keepalive.Options{
		RetryAttempts: cfg.RetryAttempts,
		RetryBackoff:  cfg.RetryBackoff,
	}, logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	scheduler, nextRun, err := startScheduler(ctx, cfg, service, logger)
	if err != nil {
		return err
	}
	if scheduler != nil {
		defer func() {
			<-scheduler.Stop().Done()
		}()
	}

	server := &httpapi.Server{
		Service: service, Cron: cfg.Cron, Timezone: cfg.TimezoneName, NextRun: nextRun,
		APIToken: cfg.APIToken, ShowDetails: cfg.ShowDetails, Logger: logger,
	}
	httpServer := &http.Server{
		Addr:              net.JoinHostPort("", strconv.Itoa(cfg.Port)),
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	logger.Info("Keep-alive configured",
		"projects", len(projects), "cron", cfg.Cron, "timezone", cfg.TimezoneName,
		"port", cfg.Port, "dotenv", describeSource(cfg.DotenvSource))

	if cfg.RunOnStartup {
		// Off the startup path so the HTTP server comes up immediately.
		go func() {
			if _, err := service.RunAll(ctx, "startup"); err != nil {
				logger.Error("Startup keep-alive run failed", "error", err)
			}
		}()
	}

	errs := make(chan error, 1)
	go func() {
		logger.Info("Listening", "addr", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- err
			return
		}
		errs <- nil
	}()

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
		logger.Info("Shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	}
}

// startScheduler wires the cron schedule. KEEPALIVE_CRON="-" disables it, in
// which case runs only happen at startup or on demand.
func startScheduler(ctx context.Context, cfg *config.Config, service *keepalive.Service,
	logger *slog.Logger) (*cron.Cron, httpapi.NextRunFunc, error) {

	if !cfg.ScheduleEnabled() {
		logger.Info("Schedule disabled; runs happen at startup or via POST /api/keepalive/run")
		return nil, func() *time.Time { return nil }, nil
	}

	// WithSeconds keeps the existing 6-field expressions working unchanged.
	scheduler := cron.New(cron.WithSeconds(), cron.WithLocation(cfg.Timezone))
	entryID, err := scheduler.AddFunc(cfg.Cron, func() {
		if _, err := service.RunAll(ctx, "scheduled"); err != nil {
			logger.Error("Scheduled keep-alive run failed", "error", err)
		}
	})
	if err != nil {
		return nil, nil, fmt.Errorf("KEEPALIVE_CRON %q is not a valid 6-field cron expression "+
			"(second minute hour day month weekday), or \"-\" to disable: %w", cfg.Cron, err)
	}
	scheduler.Start()

	nextRun := func() *time.Time {
		entry := scheduler.Entry(entryID)
		if entry.Next.IsZero() {
			return nil
		}
		next := entry.Next.UTC()
		return &next
	}
	return scheduler, nextRun, nil
}

func describeSource(source string) string {
	if source == "none" {
		// In a container the values normally arrive as environment variables.
		return "none (using environment variables)"
	}
	return source
}
