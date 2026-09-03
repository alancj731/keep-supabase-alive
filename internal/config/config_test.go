package config

import (
	"log/slog"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("SUPABASE_URLS", "postgresql://u:pw@h.example.com:5432/postgres")
	t.Setenv("SUPABASE_TABLES", "public.ping")

	cfg, err := Load("none")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Cron != "0 0 3 * * *" {
		t.Errorf("Cron = %q", cfg.Cron)
	}
	if !cfg.RunOnStartup {
		t.Error("RunOnStartup should default to true")
	}
	if cfg.RetryAttempts != 3 {
		t.Errorf("RetryAttempts = %d, want 3", cfg.RetryAttempts)
	}
	if cfg.RetryBackoff != 2*time.Second {
		t.Errorf("RetryBackoff = %v, want 2s", cfg.RetryBackoff)
	}
	if cfg.ConnectTimeout != 10*time.Second || cfg.QueryTimeout != 10*time.Second {
		t.Errorf("timeouts = %v/%v, want 10s/10s", cfg.ConnectTimeout, cfg.QueryTimeout)
	}
	if cfg.Port != 8088 {
		t.Errorf("Port = %d, want 8088", cfg.Port)
	}
	if cfg.LogLevel != slog.LevelInfo {
		t.Errorf("LogLevel = %v, want INFO", cfg.LogLevel)
	}
	if !cfg.ShowDetails {
		t.Error("ShowDetails should default to true")
	}
	if !cfg.ScheduleEnabled() {
		t.Error("schedule should be enabled by default")
	}
}

func TestLoadSplitsCommaSeparatedListsAndTrimsBlanks(t *testing.T) {
	t.Setenv("SUPABASE_URLS", " postgresql://a:pw@h1/postgres , , postgresql://b:pw@h2/postgres ")
	t.Setenv("SUPABASE_TABLES", "public.a, public.b")

	cfg, err := Load("none")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.URLs) != 2 {
		t.Errorf("URLs = %v, want 2 entries", cfg.URLs)
	}
	if len(cfg.Tables) != 2 || cfg.Tables[1] != "public.b" {
		t.Errorf("Tables = %v", cfg.Tables)
	}
}

func TestScheduleDisabledByDash(t *testing.T) {
	t.Setenv("KEEPALIVE_CRON", "-")
	cfg, err := Load("none")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ScheduleEnabled() {
		t.Error(`cron "-" should disable the schedule`)
	}
}

func TestLoadRejectsBadValues(t *testing.T) {
	for _, tc := range []struct{ key, value, wantIn string }{
		{"KEEPALIVE_RETRY_ATTEMPTS", "many", "whole number"},
		{"KEEPALIVE_RUN_ON_STARTUP", "yes please", "true or false"},
		{"SERVER_PORT", "70000", "not a valid port"},
		{"KEEPALIVE_LOG_LEVEL", "CHATTY", "DEBUG, INFO, WARN or ERROR"},
		{"KEEPALIVE_TIMEZONE", "Mars/Olympus", "not a known timezone"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			t.Setenv(tc.key, tc.value)
			if _, err := Load("none"); err == nil {
				t.Fatalf("expected an error for %s=%s", tc.key, tc.value)
			} else if !contains(err.Error(), tc.wantIn) {
				t.Errorf("error %q should mention %q", err, tc.wantIn)
			}
		})
	}
}

func TestPortPrecedence(t *testing.T) {
	t.Run("PORT is used when SERVER_PORT is absent", func(t *testing.T) {
		t.Setenv("PORT", "3000")
		cfg, err := Load("none")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.Port != 3000 {
			t.Errorf("Port = %d, want the platform-injected 3000", cfg.Port)
		}
	})

	// The platform picks the port and routes to it, so PORT must win over a
	// stale SERVER_PORT or the service is unreachable.
	t.Run("PORT wins over SERVER_PORT", func(t *testing.T) {
		t.Setenv("PORT", "3000")
		t.Setenv("SERVER_PORT", "9999")
		cfg, err := Load("none")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.Port != 3000 {
			t.Errorf("Port = %d, want the platform-injected 3000", cfg.Port)
		}
	})

	t.Run("falls back to 8088", func(t *testing.T) {
		cfg, err := Load("none")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.Port != 8088 {
			t.Errorf("Port = %d, want 8088", cfg.Port)
		}
	})
}

func TestShowDetailsCanBeDisabled(t *testing.T) {
	t.Setenv("MANAGEMENT_HEALTH_SHOW_DETAILS", "never")
	cfg, err := Load("none")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ShowDetails {
		t.Error("ShowDetails should be false when set to never")
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}
