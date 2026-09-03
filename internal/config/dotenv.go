// Package config loads the service configuration from the environment, optionally
// seeded by a .env file.
package config

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// PathVariable overrides the location of the .env file.
const PathVariable = "DOTENV_PATH"

const defaultFilename = ".env"

// LoadDotenv reads a .env file into the process environment.
//
// Values already present in the environment are never overwritten, so variables
// injected by Docker Compose or a hosting platform always win over the file. A
// missing file is not an error. It returns the absolute path that was read, or
// "none".
//
// This must run before anything reads configuration — including the log level.
func LoadDotenv() (string, error) {
	path := dotenvPath()
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "none", nil
		}
		return "none", fmt.Errorf("read %s: %w", path, err)
	}
	defer file.Close()

	values, err := ParseDotenv(file)
	if err != nil {
		return "none", fmt.Errorf("parse %s: %w", path, err)
	}
	for key, value := range values {
		if _, present := os.LookupEnv(key); !present {
			if err := os.Setenv(key, value); err != nil {
				return "none", fmt.Errorf("set %s: %w", key, err)
			}
		}
	}

	absolute, err := filepath.Abs(path)
	if err != nil {
		return path, nil
	}
	return absolute, nil
}

func dotenvPath() string {
	if configured := strings.TrimSpace(os.Getenv(PathVariable)); configured != "" {
		return configured
	}
	return defaultFilename
}

// ParseDotenv parses dotenv syntax: KEY=VALUE, an optional "export " prefix,
// "#" comments, and single- or double-quoted values. Escapes are only expanded
// inside double quotes.
func ParseDotenv(reader io.Reader) (map[string]string, error) {
	values := make(map[string]string)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		line = strings.TrimSpace(line)

		separator := strings.Index(line, "=")
		if separator <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:separator])
		if key == "" {
			continue
		}
		values[key] = unquote(strings.TrimSpace(line[separator+1:]))
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

func unquote(value string) string {
	if len(value) >= 2 && strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
		return unescape(value[1 : len(value)-1])
	}
	if len(value) >= 2 && strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") {
		return value[1 : len(value)-1]
	}
	// An unquoted value ends at an inline " #" comment; quote the value to keep a literal '#'.
	if comment := strings.Index(value, " #"); comment >= 0 {
		return strings.TrimRight(value[:comment], " \t")
	}
	return value
}

func unescape(value string) string {
	var out strings.Builder
	out.Grow(len(value))
	for i := 0; i < len(value); i++ {
		if value[i] == '\\' && i+1 < len(value) {
			i++
			switch value[i] {
			case 'n':
				out.WriteByte('\n')
			case 'r':
				out.WriteByte('\r')
			case 't':
				out.WriteByte('\t')
			case '"':
				out.WriteByte('"')
			case '\\':
				out.WriteByte('\\')
			default:
				out.WriteByte('\\')
				out.WriteByte(value[i])
			}
			continue
		}
		out.WriteByte(value[i])
	}
	return out.String()
}
