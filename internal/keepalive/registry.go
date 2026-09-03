package keepalive

import (
	"errors"
	"fmt"
	"strconv"
)

// BuildProjects pairs the configured connection strings with the configured
// tables.
//
// A single table is broadcast to every project; otherwise the number of tables
// must match the number of URLs and the two lists are aligned by position. Any
// problem is returned here, at startup, so a misconfigured service never boots
// into a state where it silently pings nothing.
//
// Errors name the entry number but never the entry itself, which contains the
// database password.
func BuildProjects(urls, tables []string) ([]Project, error) {
	if len(urls) == 0 {
		return nil, errors.New("SUPABASE_URLS is empty: set at least one Supabase connection " +
			"string (comma-separated) in .env")
	}
	if len(tables) == 0 {
		return nil, errors.New("SUPABASE_TABLES is empty: set one table name for all projects, " +
			"or one per project (comma-separated) in .env")
	}
	if len(tables) != 1 && len(tables) != len(urls) {
		return nil, fmt.Errorf("SUPABASE_TABLES has %d entries but SUPABASE_URLS has %d: supply "+
			"either one table for all projects or exactly one per project, in the same order",
			len(tables), len(urls))
	}

	projects := make([]Project, 0, len(urls))
	for i, connString := range urls {
		table := tables[0]
		if len(tables) > 1 {
			table = tables[i]
		}

		parsed, err := parseConnString(connString)
		if err != nil {
			return nil, fmt.Errorf("SUPABASE_URLS entry #%d is invalid: %w", i+1, err)
		}
		quoted, err := QuoteTable(table)
		if err != nil {
			return nil, fmt.Errorf("SUPABASE_TABLES entry for project #%d is invalid: %w", i+1, err)
		}

		projects = append(projects, Project{
			ID:       "p" + strconv.Itoa(i+1),
			Name:     parsed.name,
			DSN:      parsed.dsn,
			Username: parsed.username,
			Password: parsed.password,
			Host:     parsed.host,
			Port:     parsed.port,
			Database: parsed.database,
			Table:    table,
			Quoted:   quoted,
		})
	}
	return projects, nil
}
