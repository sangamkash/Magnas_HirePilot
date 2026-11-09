package migrate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This test is opt-in and must use a disposable PostgreSQL 17 database.
func TestJobDiscoveryMigrationPostgres17(t *testing.T) {
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
	if err := base.QueryRow(ctx, `SELECT 'hp_job_discovery_test_' || replace(gen_random_uuid()::text,'-','')`).Scan(&schema); err != nil {
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
	var discoveryMigration Migration
	for _, migration := range migrations {
		if migration.Version == 7 {
			discoveryMigration = migration
			break
		}
		if _, err := db.Exec(ctx, migration.Up); err != nil {
			t.Fatalf("apply fixture migration %06d: %v", migration.Version, err)
		}
	}
	if discoveryMigration.Version != 7 {
		t.Fatal("migration 000007_job_discovery was not loaded")
	}
	if _, err := db.Exec(ctx, discoveryMigration.Up); err != nil {
		t.Fatalf("apply job discovery migration: %v", err)
	}

	var profileID, searchID string
	var userID string
	if err := db.QueryRow(ctx, `INSERT INTO users(username,password_hash) VALUES('job_test_user','fixture') RETURNING id::text`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO profiles(user_id,name,full_name) VALUES($1,'Engineer profile','Test User') RETURNING id::text`, userID).Scan(&profileID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO search_configs(profile_id,name) VALUES($1,'Go roles') RETURNING id::text`, profileID).Scan(&searchID); err != nil {
		t.Fatal(err)
	}

	var sourceA, sourceB string
	if err := db.QueryRow(ctx, `INSERT INTO job_sources(name,provider_type,base_url,configuration) VALUES('Example Jobs','api','https://jobs.example.invalid','{"region":"IN"}') RETURNING id::text`).Scan(&sourceA); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO job_sources(name,provider_type) VALUES('Other Jobs','rss') RETURNING id::text`).Scan(&sourceB); err != nil {
		t.Fatal(err)
	}
	expectDBError(t, func() error {
		_, err := db.Exec(ctx, `INSERT INTO job_sources(name,provider_type) VALUES('example jobs','api')`)
		return err
	})

	var jobA, jobB string
	jobInsert := `INSERT INTO jobs(source_id,external_job_id,title,company_name,description,country,work_mode,content_hash,raw_data)
		VALUES($1,'external-1','Go Engineer','Acme','Build services','IN','Remote','hash-1','{"source":"payload"}') RETURNING id::text`
	expectDBError(t, func() error {
		_, err := db.Exec(ctx, `INSERT INTO jobs(source_id,external_job_id,title,company_name) VALUES('00000000-0000-0000-0000-000000000000','orphan','Engineer','Acme')`)
		return err
	})
	if err := db.QueryRow(ctx, jobInsert, sourceA).Scan(&jobA); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, jobInsert, sourceB).Scan(&jobB); err != nil {
		t.Fatalf("same external ID from another provider should be allowed: %v", err)
	}
	expectDBError(t, func() error {
		_, err := db.Exec(ctx, jobInsert, sourceA)
		return err
	})
	if _, err := db.Exec(ctx, `INSERT INTO jobs(source_id,external_job_id,title,company_name,description,content_hash,raw_data,updated_at)
		VALUES($1,'external-1','Go Engineer II','Acme','Updated description','hash-2','{"source":"updated"}',now())
		ON CONFLICT (source_id,external_job_id) DO UPDATE SET title=EXCLUDED.title,description=EXCLUDED.description,content_hash=EXCLUDED.content_hash,raw_data=EXCLUDED.raw_data,updated_at=EXCLUDED.updated_at`, sourceA); err != nil {
		t.Fatalf("provider job refresh upsert failed: %v", err)
	}
	var title, contentHash, rawSource string
	if err := db.QueryRow(ctx, `SELECT title,content_hash,raw_data->>'source' FROM jobs WHERE id=$1`, jobA).Scan(&title, &contentHash, &rawSource); err != nil {
		t.Fatal(err)
	}
	if title != "Go Engineer II" || contentHash != "hash-2" || rawSource != "updated" {
		t.Fatalf("job refresh did not replace normalized and raw content: %q %q %q", title, contentHash, rawSource)
	}

	var runA, runB string
	runInsert := `INSERT INTO job_discovery_runs(search_config_id,source_id,metadata)
		VALUES($1,$2,'{"next_page":"cursor-2"}') RETURNING id::text`
	if err := db.QueryRow(ctx, runInsert, searchID, sourceA).Scan(&runA); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, runInsert, searchID, sourceB).Scan(&runB); err != nil {
		t.Fatal(err)
	}
	expectDBError(t, func() error {
		_, err := db.Exec(ctx, `UPDATE job_discovery_runs SET jobs_found=99 WHERE id=$1`, runA)
		return err
	})
	expectDBError(t, func() error {
		_, err := db.Exec(ctx, `INSERT INTO job_discovery_runs(search_config_id,source_id,jobs_found) VALUES($1,$2,1)`, searchID, sourceA)
		return err
	})
	expectDBError(t, func() error {
		_, err := db.Exec(ctx, runInsert, "00000000-0000-0000-0000-000000000000", sourceA)
		return err
	})
	expectDBError(t, func() error {
		_, err := db.Exec(ctx, `INSERT INTO job_discovery_runs(search_config_id,source_id,status,completed_at,jobs_found,jobs_new) VALUES($1,$2,'DONE',now(),0,0)`, searchID, sourceA)
		return err
	})
	expectDBError(t, func() error {
		_, err := db.Exec(ctx, `INSERT INTO job_discovery_runs(search_config_id,source_id,status,completed_at,jobs_found,jobs_new) VALUES($1,$2,'COMPLETED',now(),1,2)`, searchID, sourceA)
		return err
	})
	if _, err := db.Exec(ctx, `INSERT INTO job_discovery_results(discovery_run_id,job_id) VALUES($1,$2)`, runA, jobA); err != nil {
		t.Fatal(err)
	}
	var foundCount int
	if err := db.QueryRow(ctx, `SELECT jobs_found FROM job_discovery_runs WHERE id=$1`, runA).Scan(&foundCount); err != nil || foundCount != 1 {
		t.Fatalf("jobs_found not synchronized to result rows: count=%d err=%v", foundCount, err)
	}
	expectDBError(t, func() error {
		_, err := db.Exec(ctx, `INSERT INTO job_discovery_results(discovery_run_id,job_id) VALUES($1,$2)`, runA, jobA)
		return err
	})
	expectDBError(t, func() error {
		_, err := db.Exec(ctx, `INSERT INTO job_discovery_results(discovery_run_id,job_id) VALUES($1,$2)`, runA, jobB)
		return err
	})
	if _, err := db.Exec(ctx, `INSERT INTO job_discovery_results(discovery_run_id,job_id) VALUES($1,$2)`, runB, jobB); err != nil {
		t.Fatalf("job should be recorded under its source's run: %v", err)
	}
	if _, err := db.Exec(ctx, `UPDATE job_discovery_runs SET jobs_new=1 WHERE id=$1`, runA); err != nil {
		t.Fatal(err)
	}
	expectDBError(t, func() error {
		_, err := db.Exec(ctx, `UPDATE job_discovery_runs SET jobs_new=2 WHERE id=$1`, runA)
		return err
	})
	expectDBErrorContains(t, "job discovery result records are immutable", func() error {
		_, err := db.Exec(ctx, `DELETE FROM job_discovery_results WHERE discovery_run_id=$1 AND job_id=$2`, runA, jobA)
		return err
	})
	updateCases := []struct {
		name string
		sql  string
		args []any
	}{
		{"unchanged values", `UPDATE job_discovery_results SET discovered_at=discovered_at WHERE discovery_run_id=$1 AND job_id=$2`, []any{runA, jobA}},
		{"discovery run", `UPDATE job_discovery_results SET discovery_run_id=$2 WHERE discovery_run_id=$1 AND job_id=$3`, []any{runA, runB, jobA}},
		{"source", `UPDATE job_discovery_results SET source_id=$2 WHERE discovery_run_id=$1 AND job_id=$3`, []any{runA, sourceB, jobA}},
		{"job", `UPDATE job_discovery_results SET job_id=$2 WHERE discovery_run_id=$1 AND job_id=$3`, []any{runA, jobB, jobA}},
	}
	for _, tc := range updateCases {
		t.Run("immutable/"+tc.name, func(t *testing.T) {
			expectDBErrorContains(t, "job discovery result records are immutable", func() error {
				_, err := db.Exec(ctx, tc.sql, tc.args...)
				return err
			})
		})
	}
	if err := db.QueryRow(ctx, `SELECT jobs_found FROM job_discovery_runs WHERE id=$1`, runA).Scan(&foundCount); err != nil || foundCount != 1 {
		t.Fatalf("rejected result mutations changed jobs_found: count=%d err=%v", foundCount, err)
	}

	// Concurrent result writes for a single run must produce an exact atomic count.
	const concurrentJobs = 24
	jobIDs := make([]string, concurrentJobs)
	for i := range jobIDs {
		externalID := fmt.Sprintf("concurrent-%d", i)
		if err := db.QueryRow(ctx, `INSERT INTO jobs(source_id,external_job_id,title,company_name) VALUES($1,$2,'Engineer','Acme') RETURNING id::text`, sourceA, externalID).Scan(&jobIDs[i]); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	errCh := make(chan error, concurrentJobs)
	for _, jobID := range jobIDs {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			_, err := db.Exec(ctx, `INSERT INTO job_discovery_results(discovery_run_id,job_id) VALUES($1,$2)`, runA, id)
			errCh <- err
		}(jobID)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	var recordedCount int
	if err := db.QueryRow(ctx, `SELECT jobs_found, (SELECT count(*) FROM job_discovery_results WHERE discovery_run_id=$1) FROM job_discovery_runs WHERE id=$1`, runA).Scan(&foundCount, &recordedCount); err != nil || foundCount != concurrentJobs+1 || recordedCount != foundCount {
		t.Fatalf("concurrent jobs_found mismatch: jobs_found=%d rows=%d err=%v", foundCount, recordedCount, err)
	}
	expectDBErrorContains(t, "job discovery result records are immutable", func() error {
		_, err := db.Exec(ctx, `UPDATE job_discovery_results SET discovered_at=discovered_at WHERE discovery_run_id=$1`, runA)
		return err
	})
	if err := db.QueryRow(ctx, `SELECT jobs_found, (SELECT count(*) FROM job_discovery_results WHERE discovery_run_id=$1) FROM job_discovery_runs WHERE id=$1`, runA).Scan(&foundCount, &recordedCount); err != nil || foundCount != recordedCount {
		t.Fatalf("rejected multi-row update changed counter/provenance rows: jobs_found=%d rows=%d err=%v", foundCount, recordedCount, err)
	}
	if _, err := db.Exec(ctx, `DELETE FROM job_discovery_runs WHERE id=$1`, runB); err != nil {
		t.Fatalf("deleting a run should cascade its immutable result rows: %v", err)
	}
	var cascadeRows int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM job_discovery_results WHERE discovery_run_id=$1`, runB).Scan(&cascadeRows); err != nil || cascadeRows != 0 {
		t.Fatalf("parent-run delete did not cascade result rows: rows=%d err=%v", cascadeRows, err)
	}
	expectDBError(t, func() error {
		_, err := db.Exec(ctx, `INSERT INTO job_discovery_runs(search_config_id,source_id) VALUES($1,$2)`, searchID, "00000000-0000-0000-0000-000000000000")
		return err
	})
	expectDBError(t, func() error {
		_, err := db.Exec(ctx, `INSERT INTO job_discovery_results(discovery_run_id,job_id) VALUES($1,'00000000-0000-0000-0000-000000000000')`, runA)
		return err
	})
	var indexCount, foreignKeyCount int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM pg_indexes WHERE schemaname=$1 AND indexname IN (
		'job_sources_name_lower_idx','jobs_title_idx','jobs_company_name_idx','jobs_country_idx','jobs_work_mode_idx','jobs_posted_at_idx','jobs_status_idx',
		'job_discovery_runs_search_config_id_idx','job_discovery_runs_source_id_idx','job_discovery_runs_status_idx',
		'job_discovery_results_job_id_idx','job_discovery_results_discovered_at_idx','job_discovery_results_run_source_idx','job_discovery_results_job_source_idx')`, schema).Scan(&indexCount); err != nil {
		t.Fatal(err)
	}
	if indexCount != 14 {
		t.Fatalf("found %d of 14 expected discovery indexes", indexCount)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM pg_constraint c JOIN pg_class r ON r.oid=c.conrelid JOIN pg_namespace n ON n.oid=r.relnamespace WHERE n.nspname=$1 AND c.contype='f' AND r.relname IN ('jobs','job_discovery_runs','job_discovery_results')`, schema).Scan(&foreignKeyCount); err != nil {
		t.Fatal(err)
	}
	if foreignKeyCount != 5 {
		t.Fatalf("found %d of 5 expected discovery foreign keys", foreignKeyCount)
	}

	if _, err := db.Exec(ctx, discoveryMigration.Down); err != nil {
		t.Fatalf("down migration: %v", err)
	}
	var remainingTables int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema=$1 AND table_name IN ('job_sources','jobs','job_discovery_runs','job_discovery_results')`, schema).Scan(&remainingTables); err != nil {
		t.Fatal(err)
	}
	if remainingTables != 0 {
		t.Fatalf("down migration left %d milestone tables", remainingTables)
	}
	var searchConfigExists bool
	if err := db.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, schema+`.search_configs`).Scan(&searchConfigExists); err != nil || !searchConfigExists {
		t.Fatalf("down migration affected existing search_configs table: exists=%v err=%v", searchConfigExists, err)
	}
	if _, err := db.Exec(ctx, discoveryMigration.Up); err != nil {
		t.Fatalf("migration could not be applied again after rollback: %v", err)
	}
}

func expectDBError(t *testing.T, operation func() error) {
	t.Helper()
	if err := operation(); err == nil {
		t.Fatal("operation unexpectedly succeeded; expected a database constraint error")
	}
}

func expectDBErrorContains(t *testing.T, expected string, operation func() error) {
	t.Helper()
	err := operation()
	if err == nil || !strings.Contains(err.Error(), expected) {
		t.Fatalf("expected database error containing %q, got %v", expected, err)
	}
}
