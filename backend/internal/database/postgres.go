package database

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Open creates a context-aware PostgreSQL connection pool. Connectivity is
// checked by the readiness handler so the HTTP server can still report health
// when the database is temporarily unavailable.
func Open(connectionURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(connectionURL)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 10
	cfg.MinConns = 0
	return pgxpool.NewWithConfig(context.Background(), cfg)
}
