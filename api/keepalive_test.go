package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func call(t *testing.T, token string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/keepalive", nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	Handler(recorder, request)

	var body map[string]any
	if recorder.Body.Len() > 0 {
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatalf("response is not JSON: %v (%s)", err, recorder.Body.String())
		}
	}
	return recorder, body
}

func TestRejectsMissingOrWrongCronSecret(t *testing.T) {
	t.Setenv("CRON_SECRET", "s3cret")

	for name, token := range map[string]string{"missing": "", "wrong": "nope"} {
		t.Run(name, func(t *testing.T) {
			recorder, body := call(t, token)
			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", recorder.Code)
			}
			if body["error"] != "unauthorized" {
				t.Errorf("body = %v", body)
			}
		})
	}
}

func TestAcceptsTheKeepaliveTokenToo(t *testing.T) {
	t.Setenv("KEEPALIVE_API_TOKEN", "manual-token")
	// No projects configured, so it gets past auth and fails on configuration —
	// which is exactly what proves the token was accepted.
	t.Setenv("SUPABASE_URLS", "")
	t.Setenv("SUPABASE_TABLES", "")

	recorder, body := call(t, "manual-token")

	if recorder.Code == http.StatusUnauthorized {
		t.Fatalf("a valid token was rejected: %v", body)
	}
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 for missing configuration", recorder.Code)
	}
}

func TestReportsMissingConfigurationWithoutLeakingIt(t *testing.T) {
	t.Setenv("CRON_SECRET", "s3cret")
	t.Setenv("SUPABASE_URLS", "postgresql://nopassword@host.example.com/db")
	t.Setenv("SUPABASE_TABLES", "public.ping")

	recorder, body := call(t, "s3cret")

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
	message, _ := body["error"].(string)
	if message == "" {
		t.Fatal("expected an error message")
	}
	if contains(message, "nopassword") {
		t.Errorf("error %q must not echo the connection string", message)
	}
	if !contains(message, "entry #1") {
		t.Errorf("error %q should name the offending entry", message)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
