// Package keepalive pings every configured Supabase project so the project is
// never idle long enough to be paused.
package keepalive

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var tablePart = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_$]{0,62}$`)

// QuoteTable validates a table name and returns it double-quoted, turning
// public.users into "public"."users".
//
// A table cannot be a bind parameter, so the name is whitelisted against a
// strict pattern and then quoted before it is ever interpolated into SQL.
// Anything outside the pattern fails at startup rather than reaching the
// database. Quoting makes the name case-sensitive, so it must match the table
// as it was created.
func QuoteTable(table string) (string, error) {
	trimmed := strings.TrimSpace(table)
	if trimmed == "" {
		return "", errors.New("table name must not be blank")
	}
	parts := strings.Split(trimmed, ".")
	if len(parts) > 2 {
		return "", fmt.Errorf("invalid table name %q: expected 'table' or 'schema.table'", trimmed)
	}
	quoted := make([]string, 0, len(parts))
	for _, part := range parts {
		if !tablePart.MatchString(part) {
			return "", fmt.Errorf("invalid table name %q: each part must start with a letter or "+
				"underscore and contain only letters, digits, underscore or $", trimmed)
		}
		quoted = append(quoted, `"`+part+`"`)
	}
	return strings.Join(quoted, "."), nil
}
