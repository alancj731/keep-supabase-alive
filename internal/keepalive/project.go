package keepalive

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Project is one Supabase project to keep alive, derived from a single
// SUPABASE_URLS entry.
type Project struct {
	ID       string
	Name     string
	DSN      string // safe to log: carries no password
	Username string
	Password string
	Host     string
	Port     int
	Database string
	Table    string
	Quoted   string
}

// String never includes the password, so a Project is safe to log.
func (p Project) String() string {
	return fmt.Sprintf("Project{id:%s name:%s dsn:%s user:%s password:*** table:%s}",
		p.ID, p.Name, p.DSN, p.Username, p.Table)
}

// target holds the connection details parsed out of one connection string.
type target struct {
	name     string
	dsn      string
	username string
	password string
	host     string
	port     int
	database string
}

const defaultPort = 5432
const defaultDatabase = "postgres"

// parseConnString accepts the postgresql:// URI Supabase shows in Project
// Settings, the postgres:// alias, and a jdbc:postgresql:// URL carrying user
// and password query parameters.
//
// Credentials always come back separately from the DSN so the DSN itself is
// safe to log, and sslmode=require is added when the caller did not specify one
// (Supabase requires TLS).
func parseConnString(connString string) (target, error) {
	raw := strings.TrimSpace(connString)
	if raw == "" {
		return target{}, errors.New("connection string is empty")
	}
	// Accept the Java-flavoured form so an existing .env keeps working.
	raw = strings.TrimPrefix(raw, "jdbc:")

	if !strings.HasPrefix(raw, "postgresql://") && !strings.HasPrefix(raw, "postgres://") {
		return target{}, errors.New("unsupported connection string: expected it to start with " +
			"postgresql://, postgres:// or jdbc:postgresql://")
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return target{}, errors.New("connection string is not a valid URI: percent-encode any " +
			"@ : / ? # or , characters in the password (for example @ becomes %40)")
	}

	query := parsed.Query()
	username := parsed.User.Username()
	password, _ := parsed.User.Password()
	// The jdbc: form carries credentials as query parameters instead.
	if username == "" {
		username = query.Get("user")
	}
	if password == "" {
		password = query.Get("password")
	}
	query.Del("user")
	query.Del("password")

	if username == "" || password == "" {
		return target{}, errors.New("connection string has no credentials: expected " +
			"postgresql://user:password@host:port/database")
	}
	if parsed.Hostname() == "" {
		return target{}, errors.New("connection string has no host; percent-encode any " +
			"@ : / ? # or , characters in the password (for example @ becomes %40)")
	}

	if query.Get("sslmode") == "" {
		query.Set("sslmode", "require")
	}

	database := strings.TrimPrefix(parsed.Path, "/")
	if database == "" {
		database = defaultDatabase
	}

	// Rebuild without credentials so the DSN can be logged.
	safe := url.URL{Scheme: "postgresql", Host: parsed.Host, Path: "/" + database, RawQuery: query.Encode()}
	dsn := safe.String()

	// pgx settles host, port and the sslmode semantics; it is also what will
	// actually connect, so parsing here catches a bad DSN at startup.
	pgxConfig, err := pgx.ParseConfig(dsn)
	if err != nil {
		return target{}, fmt.Errorf("connection string is not usable: %w", err)
	}
	port := int(pgxConfig.Port)
	if port == 0 {
		port = defaultPort
	}

	return target{
		name:     username + "@" + pgxConfig.Host + "/" + database,
		dsn:      dsn,
		username: username,
		password: password,
		host:     pgxConfig.Host,
		port:     port,
		database: database,
	}, nil
}
