package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Config contains process configuration sourced from environment variables.
type Config struct {
	HTTPAddr      string
	DatabaseURL   string
	MigrationsDir string
}

func Load() (Config, error) {
	addr := strings.TrimSpace(os.Getenv("HTTP_ADDR"))
	if addr == "" {
		addr = ":8080"
	}
	databaseURL, err := postgresURL()
	if err != nil {
		return Config{}, err
	}
	migrationsDir := strings.TrimSpace(os.Getenv("MIGRATIONS_DIR"))
	if migrationsDir == "" {
		migrationsDir = "migrations"
	}
	return Config{HTTPAddr: addr, DatabaseURL: databaseURL, MigrationsDir: migrationsDir}, nil
}

func postgresURL() (string, error) {
	if databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL")); databaseURL != "" {
		u, err := url.Parse(databaseURL)
		if err != nil || u.Scheme != "postgres" && u.Scheme != "postgresql" || u.Host == "" || u.Path == "/" {
			return "", errors.New("DATABASE_URL must be a valid PostgreSQL connection URL")
		}
		return databaseURL, nil
	}

	host := strings.TrimSpace(os.Getenv("PGHOST"))
	database := strings.TrimSpace(os.Getenv("PGDATABASE"))
	user := strings.TrimSpace(os.Getenv("PGUSER"))
	password := os.Getenv("PGPASSWORD")
	if host == "" || database == "" || user == "" {
		return "", errors.New("set DATABASE_URL or PGHOST, PGDATABASE, and PGUSER")
	}
	port := strings.TrimSpace(os.Getenv("PGPORT"))
	if port == "" {
		port = "5432"
	}
	if _, err := strconv.Atoi(port); err != nil {
		return "", fmt.Errorf("PGPORT must be an integer: %w", err)
	}
	sslmode := strings.TrimSpace(os.Getenv("PGSSLMODE"))
	if sslmode == "" {
		sslmode = "prefer"
	}
	u := &url.URL{Scheme: "postgres", User: url.UserPassword(user, password), Host: host + ":" + port, Path: "/" + database}
	query := url.Values{}
	query.Set("sslmode", sslmode)
	u.RawQuery = query.Encode()
	return u.String(), nil
}
