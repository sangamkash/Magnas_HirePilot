package config

import (
	"testing"
)

func TestLoadFromPostgresEnvironment(t *testing.T) {
	for key, value := range map[string]string{
		"DATABASE_URL": "", "PGHOST": "localhost", "PGPORT": "5432", "PGDATABASE": "hirepilot",
		"PGUSER": "local-user", "PGPASSWORD": "p@ss word", "PGSSLMODE": "disable", "HTTP_ADDR": "127.0.0.1:8081",
	} {
		t.Setenv(key, value)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != "127.0.0.1:8081" {
		t.Fatalf("HTTPAddr = %q", cfg.HTTPAddr)
	}
	if cfg.DatabaseURL != "postgres://local-user:p%40ss%20word@localhost:5432/hirepilot?sslmode=disable" {
		t.Fatalf("unexpected URL encoding: %q", cfg.DatabaseURL)
	}
}

func TestLoadRejectsMissingDatabaseSettings(t *testing.T) {
	for _, key := range []string{"DATABASE_URL", "PGHOST", "PGDATABASE", "PGUSER", "PGPASSWORD", "PGPORT", "PGSSLMODE", "HTTP_ADDR"} {
		t.Setenv(key, "")
	}
	if _, err := Load(); err == nil {
		t.Fatal("expected missing database configuration error")
	}
}
