package migrate

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This test is opt-in and must use a disposable PostgreSQL database. It never
// runs against the application's configured DATABASE_URL.
func TestJobMatchingMigrationPostgres(t *testing.T) {
	url := os.Getenv("HIREPILOT_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set HIREPILOT_TEST_DATABASE_URL to a disposable PostgreSQL database")
	}
	ctx := context.Background()
	base, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer base.Close()
	var schema string
	if err = base.QueryRow(ctx, `SELECT 'hp_job_matching_test_'||replace(gen_random_uuid()::text,'-','')`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err = base.Exec(ctx, `CREATE SCHEMA `+quoted); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = base.Exec(context.Background(), `DROP SCHEMA `+quoted+` CASCADE`) }()
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	migrations, err := Load(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	var matching Migration
	for _, m := range migrations {
		if m.Version == 8 {
			matching = m
		}
		if _, err = db.Exec(ctx, m.Up); err != nil {
			t.Fatalf("apply fixture migration %06d: %v", m.Version, err)
		}
	}
	if matching.Version != 8 {
		t.Fatal("matching migration 000008 was not loaded")
	}
	var userID, profileID, searchID, sourceID, jobID, discoveryID, runID string
	if err = db.QueryRow(ctx, `INSERT INTO users(username,password_hash) VALUES('match_test_user','fixture') RETURNING id::text`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `INSERT INTO profiles(user_id,name,full_name) VALUES($1,'Profile','Candidate') RETURNING id::text`, userID).Scan(&profileID); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `INSERT INTO search_configs(profile_id,name) VALUES($1,'Go jobs') RETURNING id::text`, profileID).Scan(&searchID); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `INSERT INTO job_sources(name,provider_type) VALUES('Test source','test') RETURNING id::text`).Scan(&sourceID); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `INSERT INTO jobs(source_id,external_job_id,title,company_name) VALUES($1,'job-1','Go Engineer','Example') RETURNING id::text`, sourceID).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `INSERT INTO job_discovery_runs(search_config_id,source_id) VALUES($1,$2) RETURNING id::text`, searchID, sourceID).Scan(&discoveryID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO job_discovery_results(discovery_run_id,job_id) VALUES($1,$2)`, discoveryID, jobID); err != nil {
		t.Fatal(err)
	}
	insertRun := `INSERT INTO job_match_runs(profile_id,search_config_id,discovery_run_id,prompt_version,algorithm_version,profile_snapshot) VALUES($1,$2,$3,'prompt-v1','algo-v1','{}') RETURNING id::text`
	if err = db.QueryRow(ctx, insertRun, profileID, searchID, discoveryID).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	var otherDiscoveryID string
	if err = db.QueryRow(ctx, `INSERT INTO job_discovery_runs(search_config_id,source_id) VALUES($1,$2) RETURNING id::text`, searchID, sourceID).Scan(&otherDiscoveryID); err != nil {
		t.Fatal(err)
	}
	expectDBError(t, func() error {
		_, e := db.Exec(ctx, `INSERT INTO job_matches(match_run_id,job_id,discovery_run_id,match_score,recommendation,raw_ai_response) VALUES($1,$2,$3,91.25,'RECOMMENDED','{}')`, runID, jobID, otherDiscoveryID)
		return e
	})
	if _, err = db.Exec(ctx, `INSERT INTO job_matches(match_run_id,job_id,discovery_run_id,match_score,recommendation,raw_ai_response) VALUES($1,$2,$3,91.25,'RECOMMENDED','{}')`, runID, jobID, discoveryID); err != nil {
		t.Fatal(err)
	}
	expectDBError(t, func() error {
		_, e := db.Exec(ctx, `INSERT INTO job_matches(match_run_id,job_id,discovery_run_id,match_score,recommendation,raw_ai_response) VALUES($1,$2,$3,20,'CONSIDER','{}')`, runID, jobID, discoveryID)
		return e
	})
	expectDBError(t, func() error {
		_, e := db.Exec(ctx, insertRun, profileID, searchID, "00000000-0000-0000-0000-000000000000")
		return e
	})
	if err = db.QueryRow(ctx, insertRun, profileID, searchID, discoveryID).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	expectDBError(t, func() error {
		_, e := db.Exec(ctx, `INSERT INTO job_matches(match_run_id,job_id,discovery_run_id,match_score,recommendation,raw_ai_response) VALUES($1,$2,$3,101,'CONSIDER','{}')`, runID, jobID, discoveryID)
		return e
	})
	if _, err = db.Exec(ctx, `INSERT INTO job_matches(match_run_id,job_id,discovery_run_id,match_score,recommendation,raw_ai_response) VALUES($1,$2,$3,63,'CONSIDER','{}')`, runID, jobID, discoveryID); err != nil {
		t.Fatalf("same job should be matchable in another historical run: %v", err)
	}
	if _, err = db.Exec(ctx, matching.Down); err != nil {
		t.Fatalf("down migration: %v", err)
	}
	var existing bool
	if err = db.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, schema+`.jobs`).Scan(&existing); err != nil || !existing {
		t.Fatalf("down migration removed discovery table: %v %v", existing, err)
	}
}
