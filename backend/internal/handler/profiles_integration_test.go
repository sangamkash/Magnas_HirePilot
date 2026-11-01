package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/magnas/hirepilot/internal/migrate"
)

// This test is opt-in and must use a disposable PostgreSQL 17 database.
func TestProfileImportRetryIsIdempotentPostgres17(t *testing.T) {
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
	if err := base.QueryRow(ctx, `SELECT 'hp_profile_test_' || replace(gen_random_uuid()::text,'-','')`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := base.Exec(ctx, `CREATE SCHEMA `+quotedSchema); err != nil {
		t.Fatal(err)
	}
	ensureLTree(t, ctx, base)
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
	migrations, err := migrate.Load(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	for _, migration := range migrations {
		if _, err := db.Exec(ctx, migration.Up); err != nil {
			t.Fatalf("migration %06d: %v", migration.Version, err)
		}
	}
	var userID string
	if err := db.QueryRow(ctx, `INSERT INTO users(username,password_hash) VALUES('import_test_user','not-a-real-password-hash') RETURNING id::text`).Scan(&userID); err != nil {
		t.Fatal(err)
	}

	h := &profileHandler{db: db}
	requestBody := map[string]any{
		"name": "Backend Engineer", "legacy_import_id": "legacy-stable-profile-key",
		"data": map[string]any{
			"personal":     map[string]string{"name": "Test Import", "headline": "Backend Engineer"},
			"skills":       map[string][]string{"programming": {"Go"}, "technologies": {"PostgreSQL"}, "tools": {"Docker"}},
			"education":    []map[string]string{{"institution": "Example University", "degree": "BSc", "field": "CS", "startDate": "2010-01", "endDate": "2014-01"}},
			"experience":   []map[string]any{{"company": "Example Co", "title": "Engineer", "employmentType": "Full-time", "startDate": "2018-01", "current": true, "responsibilities": "Built systems", "links": []map[string]string{{"url": "https://example.com/project", "description": "Project portfolio"}, {"url": "https://github.com/example", "description": "Source code"}}}},
			"languages":    []map[string]string{{"language": "English", "proficiency": "Fluent"}},
			"certificates": []map[string]string{{"name": "Cloud Cert", "organization": "Example", "issueDate": "2020-01", "url": "https://example.invalid/cert"}},
			"awards":       []map[string]string{{"name": "Engineering Award", "organization": "Example", "date": "2021-01", "description": "Recognition"}},
			"searches":     []map[string]any{{"id": "5ce66c1b-53e5-497c-94d5-60e94e9ad2bc", "name": "Go roles", "titles": []string{"Go Engineer"}, "technologies": "Go, PostgreSQL", "region": "India", "workMode": []string{"Remote"}, "salary": "100000", "currency": "INR", "period": "month", "hours": "2", "days": 1, "months": 0}},
		},
	}
	body, err := json.Marshal(requestBody)
	if err != nil {
		t.Fatal(err)
	}
	post := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/profiles", bytes.NewReader(body))
		r = r.WithContext(context.WithValue(r.Context(), userIDKey{}, userID))
		w := httptest.NewRecorder()
		h.collection(w, r)
		return w
	}
	first, second := post(), post()
	if first.Code != http.StatusCreated || second.Code != http.StatusOK {
		t.Fatalf("POST statuses first=%d second=%d body=%s", first.Code, second.Code, second.Body.String())
	}
	var firstProfile, secondProfile profileRecord
	if err := json.Unmarshal(first.Body.Bytes(), &firstProfile); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(second.Body.Bytes(), &secondProfile); err != nil {
		t.Fatal(err)
	}
	if firstProfile.ID == "" || firstProfile.ID != secondProfile.ID {
		t.Fatalf("retry IDs differ: %q / %q", firstProfile.ID, secondProfile.ID)
	}
	var profileCount, skillCount, educationCount, experienceCount, languageCount, certificateCount, awardCount, searchCount, titleCount, technologyCount, modeCount int
	var responsibilities, searchTechnology string
	query := `SELECT
	 (SELECT count(*) FROM profiles WHERE user_id=$1),
	 (SELECT count(*) FROM profile_skills WHERE profile_id=$2),
	 (SELECT count(*) FROM education WHERE profile_id=$2),
	 (SELECT count(*) FROM work_experience WHERE profile_id=$2),
	 (SELECT count(*) FROM profile_languages WHERE profile_id=$2),
	 (SELECT count(*) FROM certificates WHERE profile_id=$2),
	 (SELECT count(*) FROM awards WHERE profile_id=$2),
	 (SELECT count(*) FROM search_configs WHERE profile_id=$2),
	 (SELECT count(*) FROM search_job_titles t JOIN search_configs s ON s.id=t.search_config_id WHERE s.profile_id=$2),
	 (SELECT count(*) FROM search_technologies t JOIN search_configs s ON s.id=t.search_config_id WHERE s.profile_id=$2),
	 (SELECT count(*) FROM search_work_modes t JOIN search_configs s ON s.id=t.search_config_id WHERE s.profile_id=$2),
	 (SELECT responsibilities FROM work_experience WHERE profile_id=$2 LIMIT 1),
	 (SELECT string_agg(technology, ', ' ORDER BY technology) FROM search_technologies t JOIN search_configs s ON s.id=t.search_config_id WHERE s.profile_id=$2)`
	if err := db.QueryRow(ctx, query, userID, firstProfile.ID).Scan(&profileCount, &skillCount, &educationCount, &experienceCount, &languageCount, &certificateCount, &awardCount, &searchCount, &titleCount, &technologyCount, &modeCount, &responsibilities, &searchTechnology); err != nil {
		t.Fatal(err)
	}
	if profileCount != 1 || skillCount != 3 || educationCount != 1 || experienceCount != 1 || languageCount != 1 || certificateCount != 1 || awardCount != 1 || searchCount != 1 || titleCount != 1 || technologyCount != 2 || modeCount != 1 {
		t.Fatalf("unexpected profile/relation counts: %d %d %d %d %d %d %d %d %d %d %d", profileCount, skillCount, educationCount, experienceCount, languageCount, certificateCount, awardCount, searchCount, titleCount, technologyCount, modeCount)
	}
	if responsibilities != "Built systems" || searchTechnology != "Go, PostgreSQL" {
		t.Fatalf("relational field mappings mismatch: responsibilities=%q technologies=%q", responsibilities, searchTechnology)
	}
	var experienceLinkCount int
	var orderedLinks, orderedDescriptions string
	if err := db.QueryRow(ctx, `SELECT count(*), string_agg(url, ', ' ORDER BY position, id), string_agg(description, ', ' ORDER BY position, id) FROM work_experience_links l JOIN work_experience e ON e.id=l.work_experience_id WHERE e.profile_id=$1`, firstProfile.ID).Scan(&experienceLinkCount, &orderedLinks, &orderedDescriptions); err != nil {
		t.Fatal(err)
	}
	if experienceLinkCount != 2 || orderedLinks != "https://example.com/project, https://github.com/example" || orderedDescriptions != "Project portfolio, Source code" {
		t.Fatalf("experience links mismatch: count=%d links=%q descriptions=%q", experienceLinkCount, orderedLinks, orderedDescriptions)
	}
	var savedData []byte
	if err := db.QueryRow(ctx, `SELECT profile_data FROM profiles WHERE id=$1`, firstProfile.ID).Scan(&savedData); err != nil {
		t.Fatal(err)
	}
	var savedProfile struct {
		Experience []struct {
			Links []struct {
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"links"`
		} `json:"experience"`
	}
	if err := json.Unmarshal(savedData, &savedProfile); err != nil {
		t.Fatal(err)
	}
	if len(savedProfile.Experience) != 1 || len(savedProfile.Experience[0].Links) != 2 || savedProfile.Experience[0].Links[0].Description != "Project portfolio" {
		t.Fatalf("profile JSON did not preserve associated link descriptions: %+v", savedProfile.Experience)
	}
}

func TestProfileCreateLimitCannotBeBypassedWithLegacyImportIDPostgres17(t *testing.T) {
	db, userID := profileTestDB(t)
	h := &profileHandler{db: db}
	if _, err := db.Exec(context.Background(), `INSERT INTO profiles(user_id,name,full_name) SELECT $1,'Existing '||n,'Test User' FROM generate_series(1,10) n`, userID); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"name":"Imported","legacy_import_id":"legacy-over-limit","data":{"personal":{"name":"Test User"}}}`)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/profiles", bytes.NewReader(body)).WithContext(context.WithValue(context.Background(), userIDKey{}, userID))
	w := httptest.NewRecorder()
	h.collection(w, r)
	if w.Code != http.StatusConflict {
		t.Fatalf("got status %d, want 409: %s", w.Code, w.Body.String())
	}
}

func TestConcurrentProfileCreatesRespectLimitPostgres17(t *testing.T) {
	db, userID := profileTestDB(t)
	h := &profileHandler{db: db}
	if _, err := db.Exec(context.Background(), `INSERT INTO profiles(user_id,name,full_name) SELECT $1,'Existing '||n,'Test User' FROM generate_series(1,9) n`, userID); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	statuses := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body := []byte(`{"name":"Concurrent","data":{"personal":{"name":"Test User"}}}`)
			r := httptest.NewRequest(http.MethodPost, "/api/v1/profiles", bytes.NewReader(body)).WithContext(context.WithValue(context.Background(), userIDKey{}, userID))
			w := httptest.NewRecorder()
			h.collection(w, r)
			statuses <- w.Code
		}(i)
	}
	wg.Wait()
	close(statuses)
	created, rejected := 0, 0
	for status := range statuses {
		if status == http.StatusCreated {
			created++
		} else if status == http.StatusConflict {
			rejected++
		} else {
			t.Errorf("unexpected status %d", status)
		}
	}
	if created != 1 || rejected != 1 {
		t.Fatalf("created=%d rejected=%d, want one each", created, rejected)
	}
}

func profileTestDB(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
	url := os.Getenv("HIREPILOT_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set HIREPILOT_TEST_DATABASE_URL to a disposable PostgreSQL 17 database")
	}
	ctx := context.Background()
	base, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(base.Close)
	var schema string
	if err := base.QueryRow(ctx, `SELECT 'hp_profile_test_' || replace(gen_random_uuid()::text,'-','')`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := base.Exec(ctx, `CREATE SCHEMA `+quoted); err != nil {
		t.Fatal(err)
	}
	ensureLTree(t, ctx, base)
	t.Cleanup(func() { _, _ = base.Exec(context.Background(), `DROP SCHEMA `+quoted+` CASCADE`) })
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	migrations, err := migrate.Load(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	for _, migration := range migrations {
		if _, err := db.Exec(ctx, migration.Up); err != nil {
			t.Fatalf("fixture migration %06d: %v", migration.Version, err)
		}
	}
	var userID string
	if err := db.QueryRow(ctx, `INSERT INTO users(username,password_hash) VALUES('p_'||substring(replace(gen_random_uuid()::text,'-','') from 1 for 16),'fixture') RETURNING id::text`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	return db, userID
}

func ensureLTree(t *testing.T, ctx context.Context, db *pgxpool.Pool) {
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
