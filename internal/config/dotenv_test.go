package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDotenvKeysCommentsAndExports(t *testing.T) {
	input := strings.Join([]string{
		"# a comment",
		"",
		"SUPABASE_TABLES=public.ping",
		"export KEEPALIVE_CRON=0 0 3 * * *",
		"   SPACED_KEY   =   spaced value   ",
		"NOT_A_PAIR",
		"=novalue",
	}, "\n")

	values, err := ParseDotenv(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseDotenv: %v", err)
	}
	want := map[string]string{
		"SUPABASE_TABLES": "public.ping",
		"KEEPALIVE_CRON":  "0 0 3 * * *",
		"SPACED_KEY":      "spaced value",
	}
	if len(values) != len(want) {
		t.Fatalf("got %d entries %v, want %d", len(values), values, len(want))
	}
	for key, expected := range want {
		if values[key] != expected {
			t.Errorf("%s = %q, want %q", key, values[key], expected)
		}
	}
}

func TestParseDotenvQuotesAndEscapes(t *testing.T) {
	input := strings.Join([]string{
		`DOUBLE="a b\nc"`,
		`SINGLE='raw \n value'`,
		`HASH_IN_QUOTES="pass#word"`,
		`TRAILING_COMMENT=value # explanation`,
	}, "\n")

	values, err := ParseDotenv(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseDotenv: %v", err)
	}
	for key, want := range map[string]string{
		"DOUBLE":           "a b\nc",
		"SINGLE":           `raw \n value`,
		"HASH_IN_QUOTES":   "pass#word",
		"TRAILING_COMMENT": "value",
	} {
		if values[key] != want {
			t.Errorf("%s = %q, want %q", key, values[key], want)
		}
	}
}

func TestParseDotenvKeepsConnectionStringIntact(t *testing.T) {
	const dsn = "postgresql://postgres.abc:p%40ss@aws-0-us-east-1.pooler.supabase.com:5432/postgres"
	values, err := ParseDotenv(strings.NewReader("SUPABASE_URLS=" + dsn))
	if err != nil {
		t.Fatalf("ParseDotenv: %v", err)
	}
	if values["SUPABASE_URLS"] != dsn {
		t.Errorf("SUPABASE_URLS = %q, want %q", values["SUPABASE_URLS"], dsn)
	}
}

func TestLoadDotenvReadsFileAndReportsPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("SUPABASE_TABLES=public.ping\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(PathVariable, path)
	t.Setenv("SUPABASE_TABLES", "")
	os.Unsetenv("SUPABASE_TABLES")

	source, err := LoadDotenv()
	if err != nil {
		t.Fatalf("LoadDotenv: %v", err)
	}
	if got := os.Getenv("SUPABASE_TABLES"); got != "public.ping" {
		t.Errorf("SUPABASE_TABLES = %q, want public.ping", got)
	}
	absolute, _ := filepath.Abs(path)
	if source != absolute {
		t.Errorf("source = %q, want %q", source, absolute)
	}
}

func TestLoadDotenvToleratesMissingFile(t *testing.T) {
	t.Setenv(PathVariable, filepath.Join(t.TempDir(), "absent.env"))

	source, err := LoadDotenv()
	if err != nil {
		t.Fatalf("LoadDotenv: %v", err)
	}
	if source != "none" {
		t.Errorf("source = %q, want none", source)
	}
}

func TestLoadDotenvDoesNotOverrideExistingEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("SUPABASE_TABLES=from.file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(PathVariable, path)
	t.Setenv("SUPABASE_TABLES", "from.environment")

	if _, err := LoadDotenv(); err != nil {
		t.Fatalf("LoadDotenv: %v", err)
	}
	if got := os.Getenv("SUPABASE_TABLES"); got != "from.environment" {
		t.Errorf("SUPABASE_TABLES = %q, want from.environment", got)
	}
}
