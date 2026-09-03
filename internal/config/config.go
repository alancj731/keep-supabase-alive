package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the whole service configuration. Every field maps to an environment
// variable, so the service is configured entirely through .env or the platform.
type Config struct {
	URLs   []string // SUPABASE_URLS
	Tables []string // SUPABASE_TABLES

	Cron         string         // KEEPALIVE_CRON, "-" disables the schedule
	Timezone     *time.Location // KEEPALIVE_TIMEZONE
	TimezoneName string

	RunOnStartup bool // KEEPALIVE_RUN_ON_STARTUP

	RetryAttempts  int           // KEEPALIVE_RETRY_ATTEMPTS
	RetryBackoff   time.Duration // KEEPALIVE_RETRY_BACKOFF_MS
	ConnectTimeout time.Duration // KEEPALIVE_CONNECT_TIMEOUT_SECONDS
	QueryTimeout   time.Duration // KEEPALIVE_QUERY_TIMEOUT_SECONDS

	APIToken    string // KEEPALIVE_API_TOKEN, when set /api/** needs a bearer token
	CronSecret  string // CRON_SECRET, the bearer token Vercel Cron sends
	Port        int    // SERVER_PORT
	LogLevel    slog.Level
	ShowDetails bool // MANAGEMENT_HEALTH_SHOW_DETAILS

	// DotenvSource is the .env file the values came from, or "none".
	DotenvSource string
}

// Load reads the configuration from the environment, applying defaults.
func Load(dotenvSource string) (*Config, error) {
	cfg := &Config{
		URLs:         splitList(os.Getenv("SUPABASE_URLS")),
		Tables:       splitList(os.Getenv("SUPABASE_TABLES")),
		Cron:         stringOr("KEEPALIVE_CRON", "0 0 3 * * *"),
		TimezoneName: stringOr("KEEPALIVE_TIMEZONE", "UTC"),
		APIToken:     strings.TrimSpace(os.Getenv("KEEPALIVE_API_TOKEN")),
		CronSecret:   strings.TrimSpace(os.Getenv("CRON_SECRET")),
		DotenvSource: dotenvSource,
	}

	var err error
	if cfg.Timezone, err = time.LoadLocation(cfg.TimezoneName); err != nil {
		return nil, fmt.Errorf("KEEPALIVE_TIMEZONE %q is not a known timezone: %w", cfg.TimezoneName, err)
	}
	if cfg.RunOnStartup, err = boolOr("KEEPALIVE_RUN_ON_STARTUP", true); err != nil {
		return nil, err
	}
	if cfg.RetryAttempts, err = intOr("KEEPALIVE_RETRY_ATTEMPTS", 3); err != nil {
		return nil, err
	}
	if cfg.RetryAttempts < 1 {
		cfg.RetryAttempts = 1
	}
	backoffMs, err := intOr("KEEPALIVE_RETRY_BACKOFF_MS", 2000)
	if err != nil {
		return nil, err
	}
	cfg.RetryBackoff = time.Duration(max(backoffMs, 0)) * time.Millisecond

	connectSeconds, err := intOr("KEEPALIVE_CONNECT_TIMEOUT_SECONDS", 10)
	if err != nil {
		return nil, err
	}
	cfg.ConnectTimeout = time.Duration(max(connectSeconds, 1)) * time.Second

	querySeconds, err := intOr("KEEPALIVE_QUERY_TIMEOUT_SECONDS", 10)
	if err != nil {
		return nil, err
	}
	cfg.QueryTimeout = time.Duration(max(querySeconds, 1)) * time.Second

	// PORT wins when the platform injects it: Vercel, Fly, Render and Railway
	// pick the port and route to it, so honouring a stale SERVER_PORT would
	// leave the service unreachable. SERVER_PORT is the local convenience.
	if cfg.Port, err = intOr("SERVER_PORT", 8088); err != nil {
		return nil, err
	}
	if platformPort, portErr := intOr("PORT", 0); portErr == nil && platformPort > 0 {
		cfg.Port = platformPort
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return nil, fmt.Errorf("SERVER_PORT %d is not a valid port", cfg.Port)
	}

	if cfg.LogLevel, err = logLevel(stringOr("KEEPALIVE_LOG_LEVEL", "INFO")); err != nil {
		return nil, err
	}
	cfg.ShowDetails = !strings.EqualFold(stringOr("MANAGEMENT_HEALTH_SHOW_DETAILS", "always"), "never")

	return cfg, nil
}

// ScheduleEnabled reports whether a cron schedule is configured. "-" disables it.
func (c *Config) ScheduleEnabled() bool {
	trimmed := strings.TrimSpace(c.Cron)
	return trimmed != "" && trimmed != "-"
}

func splitList(raw string) []string {
	var values []string
	for _, part := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values
}

func stringOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func intOr(key string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a whole number, got %q", key, raw)
	}
	return value, nil
}

func boolOr(key string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false, got %q", key, raw)
	}
	return value, nil
}

func logLevel(name string) (slog.Level, error) {
	switch strings.ToUpper(strings.TrimSpace(name)) {
	case "DEBUG":
		return slog.LevelDebug, nil
	case "INFO":
		return slog.LevelInfo, nil
	case "WARN", "WARNING":
		return slog.LevelWarn, nil
	case "ERROR":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("KEEPALIVE_LOG_LEVEL must be DEBUG, INFO, WARN or ERROR, got %q", name)
	}
}
