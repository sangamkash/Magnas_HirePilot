package migrate

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This test is intentionally opt-in and should only be run against a disposable PostgreSQL database.
func TestProfileMigrationBackfillPostgres17(t *testing.T) {
	url := os.Getenv("HIREPILOT_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set HIREPILOT_TEST_DATABASE_URL to a disposable PostgreSQL 17 database")
	}
	ctx := context.Background()
	base, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer base.Close()
	var schema string
	if err := base.QueryRow(ctx, `SELECT 'hp_migration_test_' || replace(gen_random_uuid()::text,'-','')`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := base.Exec(ctx, `CREATE SCHEMA `+quotedSchema); err != nil {
		t.Fatal(err)
	}
	ensureLTreeForMigrationTest(t, ctx, base)
	defer func() { _, _ = base.Exec(context.Background(), `DROP SCHEMA `+quotedSchema+` CASCADE`) }()

	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	migrations, err := Load(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	for _, migration := range migrations[:3] {
		if _, err := db.Exec(ctx, migration.Up); err != nil {
			t.Fatalf("apply fixture migration %06d: %v", migration.Version, err)
		}
	}
	var userID, profileID, searchID string
	if err := db.QueryRow(ctx, `INSERT INTO users(username,password_hash) VALUES('migration_test_user','not-a-real-password-hash') RETURNING id::text`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO profiles(user_id,full_name,email,phone,location,headline,summary) VALUES($1,'Test User','test@example.invalid','555-0100','Bengaluru','Backend Engineer','Builds reliable services') RETURNING id::text`, userID).Scan(&profileID); err != nil {
		t.Fatal(err)
	}
	statements := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO profile_skills(profile_id,category,name) VALUES($1,'programming','Go'),($1,'technology','PostgreSQL'),($1,'tool','Docker')`, []any{profileID}},
		{`INSERT INTO education(profile_id,institution,degree,field_of_study,start_date,end_date) VALUES($1,'Example University','BSc','Computer Science','2010-01-01','2014-01-01')`, []any{profileID}},
		{`INSERT INTO work_experience(profile_id,company,title,employment_type,location,start_date,is_current,responsibilities) VALUES($1,'Example Co','Engineer','Full-time','Remote','2018-01-01',true,'Built systems')`, []any{profileID}},
		{`INSERT INTO profile_languages(profile_id,language,proficiency) VALUES($1,'English','Fluent')`, []any{profileID}},
		{`INSERT INTO certificates(profile_id,name,organization,issue_date,credential_url) VALUES($1,'Cloud Cert','Example','2020-01-01','https://example.invalid/cert')`, []any{profileID}},
		{`INSERT INTO awards(profile_id,name,organization,awarded_on,description) VALUES($1,'Engineering Award','Example','2021-01-01','Recognition')`, []any{profileID}},
	}
	for _, statement := range statements {
		if _, err := db.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.QueryRow(ctx, `INSERT INTO search_configs(profile_id,name,region,minimum_salary,salary_currency,salary_period,posted_hours,posted_days,posted_months) VALUES($1,'Go roles','India',100000,'INR','month',2,1,0) RETURNING id::text`, profileID).Scan(&searchID); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO search_job_titles(search_config_id,title) VALUES($1,'Go Engineer')`,
		`INSERT INTO search_technologies(search_config_id,technology) VALUES($1,'Go'),($1,'PostgreSQL')`,
		`INSERT INTO search_work_modes(search_config_id,work_mode) VALUES($1,'Remote')`,
	} {
		if _, err := db.Exec(ctx, statement, searchID); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := db.Exec(ctx, migrations[3].Up); err != nil {
		t.Fatalf("apply profile migration: %v", err)
	}
	var checks bool
	query := `SELECT
	 profile_data->'personal'->>'name'='Test User'
	 AND profile_data->'skills'->'programming' ? 'Go'
	 AND profile_data->'education'->0->>'institution'='Example University'
	 AND profile_data->'experience'->0->>'responsibilities'='Built systems'
	 AND profile_data->'languages'->0->>'language'='English'
	 AND profile_data->'certificates'->0->>'url'='https://example.invalid/cert'
	 AND profile_data->'awards'->0->>'description'='Recognition'
	 AND profile_data->'searches'->0->>'name'='Go roles'
	 AND profile_data->'searches'->0->'titles' ? 'Go Engineer'
	 AND profile_data->'searches'->0->>'technologies'='Go, PostgreSQL'
	 AND profile_data->'searches'->0->'workMode' ? 'Remote'
	 AND name='Backend Engineer' AND target_role='Backend Engineer'
	 FROM profiles WHERE id=$1`
	if err := db.QueryRow(ctx, query, profileID).Scan(&checks); err != nil || !checks {
		t.Fatalf("profile JSONB backfill check=%v err=%v", checks, err)
	}

	downTx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := downTx.Exec(ctx, migrations[3].Down); err == nil {
		_ = downTx.Rollback(ctx)
		t.Fatal("down migration should refuse to discard backfilled profile data")
	}
	_ = downTx.Rollback(ctx)
}

func ensureLTreeForMigrationTest(t *testing.T, ctx context.Context, db *pgxpool.Pool) {
	t.Helper()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(817264105)`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS ltree WITH SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}
