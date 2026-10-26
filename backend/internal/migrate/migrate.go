// Package migrate runs paired, versioned SQL migrations against PostgreSQL.
package migrate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const trackingTable = "hirepilot_schema_migrations"
const lockID int64 = 723498112

var migrationName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
var fileName = regexp.MustCompile(`^(\d{6})_([a-z0-9][a-z0-9_-]*)\.(up|down)\.sql$`)

// Migration represents a versioned SQL migration pair. A SQL file containing
// "-- migrate:no-transaction" runs without a transaction; use this only for
// PostgreSQL statements that cannot run inside one (for example CREATE INDEX CONCURRENTLY).
type Migration struct {
	Version int64
	Name    string
	Up      string
	Down    string
}

// Store is the small persistence boundary used by Runner and its unit tests.
type Store interface {
	Applied(context.Context) (map[int64]string, error)
	Apply(context.Context, Migration) error
	Rollback(context.Context, Migration) error
}

// Runner applies files from Dir through Store.
type Runner struct {
	Dir   string
	Store Store
}

func Load(dir string) ([]Migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations directory: %w", err)
	}
	type pair struct {
		name     string
		up, down string
	}
	pairs := map[int64]*pair{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := fileName.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		version, _ := strconv.ParseInt(match[1], 10, 64)
		p := pairs[version]
		if p == nil {
			p = &pair{name: match[2]}
			pairs[version] = p
		}
		if p.name != match[2] {
			return nil, fmt.Errorf("version %06d has inconsistent names", version)
		}
		body, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", entry.Name(), err)
		}
		if match[3] == "up" {
			p.up = string(body)
		} else {
			p.down = string(body)
		}
	}
	versions := make([]int64, 0, len(pairs))
	for version := range pairs {
		versions = append(versions, version)
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i] < versions[j] })
	result := make([]Migration, 0, len(versions))
	for _, version := range versions {
		p := pairs[version]
		if strings.TrimSpace(p.up) == "" || strings.TrimSpace(p.down) == "" {
			return nil, fmt.Errorf("migration %06d_%s must have non-empty up and down SQL files", version, p.name)
		}
		result = append(result, Migration{Version: version, Name: p.name, Up: p.up, Down: p.down})
	}
	return result, nil
}

func (r Runner) Up(ctx context.Context) error {
	migrations, err := Load(r.Dir)
	if err != nil {
		return err
	}
	applied, err := r.Store.Applied(ctx)
	if err != nil {
		return err
	}
	if err := validateApplied(migrations, applied); err != nil {
		return err
	}
	for _, m := range migrations {
		if _, ok := applied[m.Version]; ok {
			continue
		}
		if err := r.Store.Apply(ctx, m); err != nil {
			return fmt.Errorf("apply migration %06d_%s: %w", m.Version, m.Name, err)
		}
	}
	return nil
}

func (r Runner) Down(ctx context.Context, count int) error {
	if count < 1 {
		return errors.New("rollback count must be positive")
	}
	migrations, err := Load(r.Dir)
	if err != nil {
		return err
	}
	applied, err := r.Store.Applied(ctx)
	if err != nil {
		return err
	}
	if err := validateApplied(migrations, applied); err != nil {
		return err
	}
	byVersion := map[int64]Migration{}
	for _, m := range migrations {
		byVersion[m.Version] = m
	}
	versions := make([]int64, 0, len(applied))
	for v := range applied {
		versions = append(versions, v)
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i] > versions[j] })
	if count > len(versions) {
		count = len(versions)
	}
	for _, v := range versions[:count] {
		if err := r.Store.Rollback(ctx, byVersion[v]); err != nil {
			return fmt.Errorf("rollback migration %06d_%s: %w", v, byVersion[v].Name, err)
		}
	}
	return nil
}

func (r Runner) Status(ctx context.Context) ([]Migration, map[int64]string, error) {
	migrations, err := Load(r.Dir)
	if err != nil {
		return nil, nil, err
	}
	applied, err := r.Store.Applied(ctx)
	if err != nil {
		return nil, nil, err
	}
	if err := validateApplied(migrations, applied); err != nil {
		return nil, nil, err
	}
	return migrations, applied, nil
}

func validateApplied(migrations []Migration, applied map[int64]string) error {
	known := map[int64]string{}
	for _, m := range migrations {
		known[m.Version] = m.Name
	}
	for v, name := range applied {
		if expected, ok := known[v]; !ok || expected != name {
			return fmt.Errorf("database contains unknown migration %06d_%s; local files do not match", v, name)
		}
	}
	return nil
}

// PostgresStore uses a pinned pool connection and a PostgreSQL advisory lock,
// serializing runner instances even when their statements use transactions.
type PostgresStore struct{ Pool *pgxpool.Pool }

func (s PostgresStore) withLock(ctx context.Context, fn func(*pgxpool.Conn) error) (err error) {
	conn, err := s.Pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err = conn.Exec(ctx, "SELECT pg_advisory_lock($1)", lockID); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		_, unlockErr := conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", lockID)
		if err == nil && unlockErr != nil {
			err = fmt.Errorf("release migration lock: %w", unlockErr)
		}
	}()
	if _, err = conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS `+trackingTable+` (version bigint PRIMARY KEY, name text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return fmt.Errorf("ensure migration tracking table: %w", err)
	}
	return fn(conn)
}

func (s PostgresStore) Applied(ctx context.Context) (map[int64]string, error) {
	result := map[int64]string{}
	err := s.withLock(ctx, func(conn *pgxpool.Conn) error {
		rows, err := conn.Query(ctx, "SELECT version, name FROM "+trackingTable+" ORDER BY version")
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var v int64
			var n string
			if err := rows.Scan(&v, &n); err != nil {
				return err
			}
			result[v] = n
		}
		return rows.Err()
	})
	return result, err
}

func (s PostgresStore) execute(ctx context.Context, m Migration, sql string, up bool) error {
	return s.withLock(ctx, func(conn *pgxpool.Conn) error {
		var existingName string
		checkErr := conn.QueryRow(ctx, "SELECT name FROM "+trackingTable+" WHERE version=$1", m.Version).Scan(&existingName)
		if up && checkErr == nil {
			if existingName != m.Name {
				return fmt.Errorf("version %06d is already recorded as %s", m.Version, existingName)
			}
			return nil
		}
		if !up && checkErr != nil {
			return nil
		}
		if checkErr != nil && checkErr != pgx.ErrNoRows {
			return checkErr
		}
		if strings.Contains(sql, "-- migrate:no-transaction") {
			if _, err := conn.Exec(ctx, sql); err != nil {
				return err
			}
			if up {
				_, err := conn.Exec(ctx, "INSERT INTO "+trackingTable+" (version,name) VALUES ($1,$2)", m.Version, m.Name)
				return err
			}
			_, err := conn.Exec(ctx, "DELETE FROM "+trackingTable+" WHERE version=$1", m.Version)
			return err
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, sql); err != nil {
			return err
		}
		if up {
			_, err = tx.Exec(ctx, "INSERT INTO "+trackingTable+" (version,name) VALUES ($1,$2)", m.Version, m.Name)
		} else {
			_, err = tx.Exec(ctx, "DELETE FROM "+trackingTable+" WHERE version=$1", m.Version)
		}
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	})
}
func (s PostgresStore) Apply(ctx context.Context, m Migration) error {
	return s.execute(ctx, m, m.Up, true)
}
func (s PostgresStore) Rollback(ctx context.Context, m Migration) error {
	return s.execute(ctx, m, m.Down, false)
}

// IsNonTransactional reports whether SQL contains the opt-out directive.
func IsNonTransactional(sql string) bool { return strings.Contains(sql, "-- migrate:no-transaction") }
