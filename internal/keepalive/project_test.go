package keepalive

import (
	"strings"
	"testing"
)

func TestParseSupabaseURI(t *testing.T) {
	got, err := parseConnString(
		"postgresql://postgres.abcdefgh:secretpw@aws-0-us-east-1.pooler.supabase.com:5432/postgres")
	if err != nil {
		t.Fatalf("parseConnString: %v", err)
	}
	if got.username != "postgres.abcdefgh" {
		t.Errorf("username = %q", got.username)
	}
	if got.password != "secretpw" {
		t.Errorf("password = %q", got.password)
	}
	if got.host != "aws-0-us-east-1.pooler.supabase.com" {
		t.Errorf("host = %q", got.host)
	}
	if got.port != 5432 {
		t.Errorf("port = %d", got.port)
	}
	if got.database != "postgres" {
		t.Errorf("database = %q", got.database)
	}
	if !strings.Contains(got.dsn, "sslmode=require") {
		t.Errorf("dsn %q should carry sslmode=require", got.dsn)
	}
}

func TestParsePostgresSchemeAndDefaults(t *testing.T) {
	got, err := parseConnString("postgres://user:pw@db.example.com")
	if err != nil {
		t.Fatalf("parseConnString: %v", err)
	}
	if got.host != "db.example.com" {
		t.Errorf("host = %q", got.host)
	}
	if got.port != 5432 {
		t.Errorf("port = %d, want the 5432 default", got.port)
	}
	if got.database != "postgres" {
		t.Errorf("database = %q, want the postgres default", got.database)
	}
}

func TestParseHonoursExplicitPortAndDatabase(t *testing.T) {
	got, err := parseConnString("postgresql://user:pw@db.example.com:6543/appdb")
	if err != nil {
		t.Fatalf("parseConnString: %v", err)
	}
	if got.port != 6543 {
		t.Errorf("port = %d, want 6543", got.port)
	}
	if got.database != "appdb" {
		t.Errorf("database = %q, want appdb", got.database)
	}
}

func TestParseDecodesPercentEncodedCredentials(t *testing.T) {
	got, err := parseConnString("postgresql://user:p%40ss%3Aw%2Cd+x@db.example.com/postgres")
	if err != nil {
		t.Fatalf("parseConnString: %v", err)
	}
	// A '+' in a password must survive: it is a literal, not a space.
	if got.password != "p@ss:w,d+x" {
		t.Errorf("password = %q, want p@ss:w,d+x", got.password)
	}
}

func TestParseKeepsCredentialsOutOfTheDSN(t *testing.T) {
	got, err := parseConnString("postgresql://user:secretpw@db.example.com/postgres")
	if err != nil {
		t.Fatalf("parseConnString: %v", err)
	}
	if strings.Contains(got.dsn, "secretpw") {
		t.Errorf("dsn %q must not contain the password", got.dsn)
	}
	project := Project{Name: got.name, DSN: got.dsn, Username: got.username, Password: got.password}
	if strings.Contains(project.String(), "secretpw") {
		t.Errorf("String() %q must not contain the password", project.String())
	}
}

func TestParsePreservesExplicitSSLMode(t *testing.T) {
	got, err := parseConnString("postgresql://user:pw@db.example.com/postgres?sslmode=verify-full")
	if err != nil {
		t.Fatalf("parseConnString: %v", err)
	}
	if !strings.Contains(got.dsn, "sslmode=verify-full") {
		t.Errorf("dsn = %q, want sslmode=verify-full preserved", got.dsn)
	}
	if strings.Contains(got.dsn, "sslmode=require") {
		t.Errorf("dsn = %q must not also carry sslmode=require", got.dsn)
	}
}

func TestParseAcceptsJDBCForm(t *testing.T) {
	got, err := parseConnString(
		"jdbc:postgresql://db.example.com:5432/postgres?user=admin&password=secretpw")
	if err != nil {
		t.Fatalf("parseConnString: %v", err)
	}
	if got.username != "admin" || got.password != "secretpw" {
		t.Errorf("credentials = %q/%q", got.username, got.password)
	}
	if strings.Contains(got.dsn, "secretpw") || strings.Contains(got.dsn, "user=admin") {
		t.Errorf("dsn %q must not carry credentials", got.dsn)
	}
}

func TestParseRejectsBadInput(t *testing.T) {
	for name, in := range map[string]string{
		"empty":          "   ",
		"no scheme":      "db.example.com:5432/postgres",
		"wrong scheme":   "mysql://user:pw@db.example.com/db",
		"no credentials": "postgresql://db.example.com:5432/postgres",
		"no password":    "postgresql://user@db.example.com/postgres",
		"jdbc no creds":  "jdbc:postgresql://db.example.com:5432/postgres",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseConnString(in); err == nil {
				t.Errorf("parseConnString(%q) should have failed", in)
			}
		})
	}
}
