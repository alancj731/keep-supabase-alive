package keepalive

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Connector opens a short-lived connection to one project and runs the
// keep-alive query. It is an interface so the retry, redaction and concurrency
// logic can be tested without a database.
type Connector interface {
	// Ping runs the keep-alive query and reports how many rows came back.
	Ping(ctx context.Context, project Project) (int, error)
}

// PgxConnector opens a plain connection per ping. A pool would keep idle
// connections open against every project all day for the sake of one query,
// which is the opposite of what this service is for.
type PgxConnector struct {
	ConnectTimeout time.Duration
	QueryTimeout   time.Duration
	// OnConnect is called once a connection is established, for debug logging.
	OnConnect func(project Project, sql string)
}

// Ping connects, runs "select * from <table> limit 1" and counts the rows. The
// result is thrown away — the point is the database activity.
func (c *PgxConnector) Ping(ctx context.Context, project Project) (int, error) {
	config, err := pgx.ParseConfig(project.DSN)
	if err != nil {
		return 0, fmt.Errorf("bad connection string: %w", err)
	}
	config.User = project.Username
	config.Password = project.Password
	config.RuntimeParams["application_name"] = "supabase-keepalive"

	connectCtx, cancelConnect := context.WithTimeout(ctx, c.ConnectTimeout)
	defer cancelConnect()

	conn, err := pgx.ConnectConfig(connectCtx, config)
	if err != nil {
		return 0, err
	}
	defer func() {
		closeCtx, cancelClose := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancelClose()
		_ = conn.Close(closeCtx)
	}()

	sql := "select * from " + project.Quoted + " limit 1"
	if c.OnConnect != nil {
		c.OnConnect(project, sql)
	}

	queryCtx, cancelQuery := context.WithTimeout(ctx, c.QueryTimeout)
	defer cancelQuery()

	rows, err := conn.Query(queryCtx, sql)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	seen := 0
	for rows.Next() {
		seen++
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return seen, nil
}
