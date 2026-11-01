package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type profileHandler struct{ db *pgxpool.Pool }

type profileRecord struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	TargetRole     string          `json:"target_role"`
	Summary        string          `json:"summary"`
	Active         bool            `json:"active"`
	Data           json.RawMessage `json:"data"`
	LegacyImportID string          `json:"legacy_import_id,omitempty"`
}

func (h *profileHandler) routes(mux *http.ServeMux, auth func(http.Handler) http.Handler) {
	mux.Handle("/api/v1/profiles", auth(http.HandlerFunc(h.collection)))
	mux.Handle("/api/v1/profiles/", auth(http.HandlerFunc(h.item)))
}

func (h *profileHandler) collection(w http.ResponseWriter, r *http.Request) {
	uid := r.Context().Value(userIDKey{}).(string)
	switch r.Method {
	case http.MethodGet:
		rows, err := h.db.Query(r.Context(), `SELECT p.id::text,p.name,p.target_role,COALESCE(p.profile_data->'personal'->>'summary',p.summary,''),u.active_profile_id=p.id,p.profile_data,COALESCE(p.legacy_import_id,'') FROM profiles p JOIN users u ON u.id=p.user_id WHERE p.user_id=$1 ORDER BY p.created_at,p.id`, uid)
		if err != nil {
			writeError(w, 500, "Unable to load profiles.")
			return
		}
		defer rows.Close()
		items := []profileRecord{}
		for rows.Next() {
			var p profileRecord
			if rows.Scan(&p.ID, &p.Name, &p.TargetRole, &p.Summary, &p.Active, &p.Data, &p.LegacyImportID) != nil {
				writeError(w, 500, "Unable to load profiles.")
				return
			}
			items = append(items, p)
		}
		writeJSON(w, 200, items)
	case http.MethodPost:
		var p profileRecord
		if !decodeJSON(w, r, &p) {
			return
		}
		if !validProfile(p) {
			writeError(w, 400, profileValidationMessage(p))
			return
		}
		p.LegacyImportID = strings.TrimSpace(p.LegacyImportID)
		if len(p.LegacyImportID) > 200 {
			writeError(w, 400, "Invalid legacy import identifier.")
			return
		}
		tx, err := h.db.Begin(r.Context())
		if err != nil {
			writeError(w, 500, "Unable to save profile.")
			return
		}
		defer tx.Rollback(r.Context())
		// Serialize creations for this user to enforce the cap under concurrency.
		var locked string
		if err = tx.QueryRow(r.Context(), `SELECT id::text FROM users WHERE id=$1 FOR UPDATE`, uid).Scan(&locked); err != nil {
			writeError(w, 500, "Unable to save profile.")
			return
		}
		if p.LegacyImportID != "" {
			var existing profileRecord
			err = tx.QueryRow(r.Context(), `SELECT id::text,name,target_role,COALESCE(profile_data->'personal'->>'summary',summary,''),false,profile_data,legacy_import_id FROM profiles WHERE user_id=$1 AND legacy_import_id=$2`, uid, p.LegacyImportID).Scan(&existing.ID, &existing.Name, &existing.TargetRole, &existing.Summary, &existing.Active, &existing.Data, &existing.LegacyImportID)
			if err == nil {
				if err = tx.Commit(r.Context()); err != nil {
					writeError(w, 500, "Unable to save profile.")
					return
				}
				writeJSON(w, 200, existing)
				return
			}
			if err != pgx.ErrNoRows {
				writeError(w, 500, "Unable to save profile.")
				return
			}
		}
		var count int
		if err = tx.QueryRow(r.Context(), `SELECT count(*) FROM profiles WHERE user_id=$1`, uid).Scan(&count); err != nil {
			writeError(w, 500, "Unable to save profile.")
			return
		}
		if count >= 10 {
			writeError(w, 409, "Maximum of 10 profiles reached.")
			return
		}
		p.TargetRole = targetRole(p.Data)
		p.Summary = profileSummary(p.Data)
		err = tx.QueryRow(r.Context(), `INSERT INTO profiles(user_id,name,target_role,profile_data,legacy_import_id,full_name,email,phone,location,headline,summary) VALUES($1,$2,$3,$4::jsonb,NULLIF($5,''),$6,$7,$8,$9,$10,$11) RETURNING id::text`, uid, p.Name, p.TargetRole, string(p.Data), p.LegacyImportID, personal(p.Data, "name"), personal(p.Data, "email"), personal(p.Data, "phone"), personal(p.Data, "location"), personal(p.Data, "headline"), p.Summary).Scan(&p.ID)
		if err != nil {
			writeProfilePersistenceError(w, "save", err)
			return
		}
		if err = h.syncRelational(r.Context(), tx, p.ID, p.Data); err != nil {
			writeProfilePersistenceError(w, "save", err)
			return
		}
		if err = tx.Commit(r.Context()); err != nil {
			writeError(w, 500, "Unable to save profile.")
			return
		}
		p.Active = false
		writeJSON(w, 201, p)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, 405, "Method not allowed.")
	}
}

func (h *profileHandler) item(w http.ResponseWriter, r *http.Request) {
	uid := r.Context().Value(userIDKey{}).(string)
	suffix := strings.TrimPrefix(r.URL.Path, "/api/v1/profiles/")
	if suffix == "active" {
		h.active(w, r, uid)
		return
	}
	if strings.HasSuffix(suffix, "/activate") {
		if r.Method != http.MethodPut {
			writeError(w, 405, "Method not allowed.")
			return
		}
		id := strings.TrimSuffix(suffix, "/activate")
		tag, err := h.db.Exec(r.Context(), `UPDATE users SET active_profile_id=$1 WHERE id=$2 AND EXISTS(SELECT 1 FROM profiles WHERE id=$1 AND user_id=$2)`, id, uid)
		if err != nil {
			writeError(w, 400, "Invalid profile.")
			return
		}
		if tag.RowsAffected() == 0 {
			writeError(w, 404, "Profile not found.")
			return
		}
		writeJSON(w, 200, map[string]string{"status": "active"})
		return
	}
	id := suffix
	switch r.Method {
	case http.MethodGet:
		p, err := h.get(r, id, uid)
		if err != nil {
			writeError(w, 404, "Profile not found.")
			return
		}
		writeJSON(w, 200, p)
	case http.MethodPut:
		var p profileRecord
		if !decodeJSON(w, r, &p) {
			return
		}
		if !validProfile(p) {
			writeError(w, 400, profileValidationMessage(p))
			return
		}
		p.TargetRole = targetRole(p.Data)
		p.Summary = profileSummary(p.Data)
		tx, err := h.db.Begin(r.Context())
		if err != nil {
			writeError(w, 500, "Unable to update profile.")
			return
		}
		defer tx.Rollback(r.Context())
		tag, err := tx.Exec(r.Context(), `UPDATE profiles SET name=$1,target_role=$2,profile_data=$3::jsonb,full_name=$4,email=$5,phone=$6,location=$7,headline=$8,summary=$9,updated_at=now() WHERE id=$10 AND user_id=$11`, p.Name, p.TargetRole, string(p.Data), personal(p.Data, "name"), personal(p.Data, "email"), personal(p.Data, "phone"), personal(p.Data, "location"), personal(p.Data, "headline"), p.Summary, id, uid)
		if err != nil {
			writeProfilePersistenceError(w, "update", err)
			return
		}
		if tag.RowsAffected() == 0 {
			writeError(w, 404, "Profile not found.")
			return
		}
		if err = h.syncRelational(r.Context(), tx, id, p.Data); err != nil {
			writeProfilePersistenceError(w, "update", err)
			return
		}
		if err = tx.Commit(r.Context()); err != nil {
			writeError(w, 500, "Unable to update profile.")
			return
		}
		p.ID = id
		writeJSON(w, 200, p)
	case http.MethodDelete:
		tx, err := h.db.Begin(r.Context())
		if err != nil {
			writeError(w, 500, "Unable to delete profile.")
			return
		}
		defer tx.Rollback(r.Context())
		if _, err = tx.Exec(r.Context(), `UPDATE users SET active_profile_id=NULL WHERE id=$1 AND active_profile_id=$2`, uid, id); err != nil {
			writeError(w, 500, "Unable to delete profile.")
			return
		}
		tag, err := tx.Exec(r.Context(), `DELETE FROM profiles WHERE id=$1 AND user_id=$2`, id, uid)
		if err != nil {
			writeError(w, 500, "Unable to delete profile.")
			return
		}
		if tag.RowsAffected() == 0 {
			writeError(w, 404, "Profile not found.")
			return
		}
		if err = tx.Commit(r.Context()); err != nil {
			writeError(w, 500, "Unable to delete profile.")
			return
		}
		writeJSON(w, 200, map[string]string{"status": "deleted"})
	default:
		w.Header().Set("Allow", "GET, PUT, DELETE")
		writeError(w, 405, "Method not allowed.")
	}
}

func (h *profileHandler) active(w http.ResponseWriter, r *http.Request, uid string) {
	if r.Method != http.MethodGet {
		writeError(w, 405, "Method not allowed.")
		return
	}
	var id string
	err := h.db.QueryRow(r.Context(), `SELECT active_profile_id::text FROM users WHERE id=$1 AND active_profile_id IS NOT NULL`, uid).Scan(&id)
	if err != nil {
		writeError(w, 404, "No active profile selected.")
		return
	}
	p, err := h.get(r, id, uid)
	if err != nil {
		writeError(w, 404, "No active profile selected.")
		return
	}
	p.Active = true
	writeJSON(w, 200, p)
}
func (h *profileHandler) get(r *http.Request, id, uid string) (profileRecord, error) {
	var p profileRecord
	err := h.db.QueryRow(r.Context(), `SELECT p.id::text,p.name,p.target_role,COALESCE(p.profile_data->'personal'->>'summary',p.summary,''),u.active_profile_id=p.id,p.profile_data,COALESCE(p.legacy_import_id,'') FROM profiles p JOIN users u ON u.id=p.user_id WHERE p.id=$1 AND p.user_id=$2`, id, uid).Scan(&p.ID, &p.Name, &p.TargetRole, &p.Summary, &p.Active, &p.Data, &p.LegacyImportID)
	return p, err
}

// syncRelational keeps the legacy relational representation in step with profile_data.
func (h *profileHandler) syncRelational(ctx context.Context, tx pgx.Tx, id string, data json.RawMessage) error {
	// Child rows are replaced inside the same transaction as the profile JSONB update.
	for _, table := range []string{"profile_skills", "education", "work_experience", "profile_languages", "certificates", "awards", "search_configs"} {
		if _, err := tx.Exec(ctx, "DELETE FROM "+table+" WHERE profile_id=$1", id); err != nil {
			return err
		}
	}
	// Relational child-field synchronization is handled by typed JSON decoding below.
	return h.writeRelationalChildren(ctx, tx, id, data)
}

func (h *profileHandler) writeRelationalChildren(ctx context.Context, tx pgx.Tx, id string, data json.RawMessage) error {
	type experienceLink struct {
		URL         string `json:"url"`
		Description string `json:"description"`
	}
	// Older clients stored links as strings; newer clients send a URL and description.
	var legacyLinkShape struct {
		Experience []struct {
			Links []json.RawMessage `json:"links"`
		} `json:"experience"`
	}
	if err := json.Unmarshal(data, &legacyLinkShape); err != nil {
		return err
	}
	var profile struct {
		Skills struct {
			Programming  []string `json:"programming"`
			Technologies []string `json:"technologies"`
			Tools        []string `json:"tools"`
		} `json:"skills"`
		Education []struct {
			Institution string `json:"institution"`
			Degree      string `json:"degree"`
			Field       string `json:"field"`
			Start       string `json:"startDate"`
			End         string `json:"endDate"`
		} `json:"education"`
		Experience []struct {
			Company        string `json:"company"`
			Title          string `json:"title"`
			EmploymentType string `json:"employmentType"`
			Location       string `json:"location"`
			Start          string `json:"startDate"`
			End            string `json:"endDate"`
			Current        bool   `json:"current"`
			Description    string `json:"responsibilities"`
		} `json:"experience"`
		Languages []struct {
			Name        string `json:"language"`
			Proficiency string `json:"proficiency"`
		} `json:"languages"`
		Certificates []struct {
			Name         string `json:"name"`
			Organization string `json:"organization"`
			Issue        string `json:"issueDate"`
			Expiration   string `json:"expirationDate"`
			URL          string `json:"url"`
		} `json:"certificates"`
		Awards []struct {
			Name         string `json:"name"`
			Organization string `json:"organization"`
			Date         string `json:"date"`
			Description  string `json:"description"`
		} `json:"awards"`
		Searches []struct {
			ID           string          `json:"id"`
			Name         string          `json:"name"`
			Titles       []string        `json:"titles"`
			Technologies string          `json:"technologies"`
			Region       string          `json:"region"`
			WorkMode     []string        `json:"workMode"`
			Salary       json.RawMessage `json:"salary"`
			Currency     string          `json:"currency"`
			Period       string          `json:"period"`
			Hours        json.RawMessage `json:"hours"`
			Days         json.RawMessage `json:"days"`
			Months       json.RawMessage `json:"months"`
		} `json:"searches"`
	}
	if err := json.Unmarshal(data, &profile); err != nil {
		return err
	}
	if len(profile.Experience) != len(legacyLinkShape.Experience) {
		return fmt.Errorf("invalid experience links")
	}
	linksByExperience := make([][]experienceLink, len(profile.Experience))
	for i, experience := range legacyLinkShape.Experience {
		for _, raw := range experience.Links {
			var link experienceLink
			if err := json.Unmarshal(raw, &link); err == nil && link.URL != "" {
				linksByExperience[i] = append(linksByExperience[i], link)
				continue
			}
			var url string
			if err := json.Unmarshal(raw, &url); err != nil {
				return fmt.Errorf("invalid experience link")
			}
			linksByExperience[i] = append(linksByExperience[i], experienceLink{URL: url})
		}
	}
	for category, values := range map[string][]string{"programming": profile.Skills.Programming, "technology": profile.Skills.Technologies, "tool": profile.Skills.Tools} {
		for _, value := range values {
			if strings.TrimSpace(value) == "" {
				continue
			}
			if _, err := tx.Exec(ctx, `INSERT INTO profile_skills(profile_id,category,name) VALUES($1,$2,$3)`, id, category, strings.TrimSpace(value)); err != nil {
				return err
			}
		}
	}
	for _, v := range profile.Education {
		if strings.TrimSpace(v.Institution) == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO education(profile_id,institution,degree,field_of_study,start_date,end_date) VALUES($1,$2,NULLIF($3,''),NULLIF($4,''),NULLIF($5,'')::date,NULLIF($6,'')::date)`, id, v.Institution, v.Degree, v.Field, normalizeProfileDate(v.Start), normalizeProfileDate(v.End)); err != nil {
			return err
		}
	}
	for experienceIndex, v := range profile.Experience {
		if strings.TrimSpace(v.Company) == "" || strings.TrimSpace(v.Title) == "" {
			continue
		}
		var experienceID string
		if err := tx.QueryRow(ctx, `INSERT INTO work_experience(profile_id,company,title,employment_type,location,start_date,end_date,is_current,responsibilities) VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,''),NULLIF($6,'')::date,NULLIF($7,'')::date,$8,NULLIF($9,'')) RETURNING id::text`, id, v.Company, v.Title, v.EmploymentType, v.Location, normalizeProfileDate(v.Start), normalizeProfileDate(v.End), v.Current, v.Description).Scan(&experienceID); err != nil {
			return err
		}
		for position, link := range linksByExperience[experienceIndex] {
			link.URL = strings.TrimSpace(link.URL)
			link.Description = strings.TrimSpace(link.Description)
			if link.URL == "" {
				continue
			}
			if _, err := tx.Exec(ctx, `INSERT INTO work_experience_links(work_experience_id,url,description,position) VALUES($1,$2,NULLIF($3,''),$4)`, experienceID, link.URL, link.Description, position); err != nil {
				return err
			}
		}
	}
	for _, v := range profile.Languages {
		if strings.TrimSpace(v.Name) == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO profile_languages(profile_id,language,proficiency) VALUES($1,$2,$3)`, id, v.Name, v.Proficiency); err != nil {
			return err
		}
	}
	for _, v := range profile.Certificates {
		if strings.TrimSpace(v.Name) == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO certificates(profile_id,name,organization,issue_date,expiration_date,credential_url) VALUES($1,$2,NULLIF($3,''),NULLIF($4,'')::date,NULLIF($5,'')::date,NULLIF($6,''))`, id, v.Name, v.Organization, normalizeProfileDate(v.Issue), normalizeProfileDate(v.Expiration), v.URL); err != nil {
			return err
		}
	}
	for _, v := range profile.Awards {
		if strings.TrimSpace(v.Name) == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO awards(profile_id,name,organization,awarded_on,description) VALUES($1,$2,NULLIF($3,''),NULLIF($4,'')::date,NULLIF($5,''))`, id, v.Name, v.Organization, normalizeProfileDate(v.Date), v.Description); err != nil {
			return err
		}
	}
	for _, v := range profile.Searches {
		var searchID string
		var salary any
		if len(v.Salary) > 0 && string(v.Salary) != "null" && string(v.Salary) != `""` {
			var text string
			if json.Unmarshal(v.Salary, &text) == nil {
				salary = text
			} else if json.Valid(v.Salary) {
				salary = string(v.Salary)
			} else {
				return fmt.Errorf("invalid search salary")
			}
		}
		var currency, period any
		if salary != nil {
			currency, period = v.Currency, v.Period
		}
		hours, err := decodeSearchInteger(v.Hours)
		if err != nil {
			return err
		}
		days, err := decodeSearchInteger(v.Days)
		if err != nil {
			return err
		}
		months, err := decodeSearchInteger(v.Months)
		if err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO search_configs(id,profile_id,name,region,minimum_salary,salary_currency,salary_period,posted_hours,posted_days,posted_months) VALUES(COALESCE(NULLIF($1,'')::uuid,gen_random_uuid()),$2,$3,NULLIF($4,''),$5,$6,$7,$8,$9,$10) RETURNING id::text`, v.ID, id, v.Name, v.Region, salary, currency, period, hours, days, months).Scan(&searchID); err != nil {
			return err
		}
		for _, title := range v.Titles {
			if strings.TrimSpace(title) == "" {
				continue
			}
			if _, err := tx.Exec(ctx, `INSERT INTO search_job_titles(search_config_id,title) VALUES($1,$2)`, searchID, strings.TrimSpace(title)); err != nil {
				return err
			}
		}
		for _, technology := range strings.Split(v.Technologies, ",") {
			if strings.TrimSpace(technology) == "" {
				continue
			}
			if _, err := tx.Exec(ctx, `INSERT INTO search_technologies(search_config_id,technology) VALUES($1,$2)`, searchID, strings.TrimSpace(technology)); err != nil {
				return err
			}
		}
		for _, mode := range v.WorkMode {
			if _, err := tx.Exec(ctx, `INSERT INTO search_work_modes(search_config_id,work_mode) VALUES($1,$2)`, searchID, mode); err != nil {
				return err
			}
		}
	}
	return nil
}

func normalizeProfileDate(value string) string {
	if len(value) == 7 && value[4] == '-' {
		return value + "-01"
	}
	return value
}

func validProfile(p profileRecord) bool {
	var data map[string]json.RawMessage
	if strings.TrimSpace(p.Name) == "" || len(p.Data) == 0 || json.Unmarshal(p.Data, &data) != nil || data == nil || strings.TrimSpace(personal(p.Data, "name")) == "" {
		return false
	}
	return validateProfileEntries(p.Data) == nil
}

func profileValidationMessage(p profileRecord) string {
	if strings.TrimSpace(p.Name) == "" || len(p.Data) == 0 || strings.TrimSpace(personal(p.Data, "name")) == "" {
		return "Profile name and personal full name are required; profile data must be a JSON object."
	}
	if err := validateProfileEntries(p.Data); err != nil {
		return "Profile contains an invalid or duplicate value. Check dates, skills, languages, proficiency, work mode, and salary settings."
	}
	return "Profile data must be a JSON object."
}

// writeProfilePersistenceError gives clients a safe, actionable reason for common
// profile persistence failures without returning SQL, schema, or user data.
func writeProfilePersistenceError(w http.ResponseWriter, action string, err error) {
	message := "The profile could not be " + action + "d because the database rejected one of its values. Check dates, duplicate skills or languages, and search settings, then try again."
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			message = "The profile could not be " + action + "d because it contains a duplicate value. Remove duplicate skills, languages, job titles, or technologies and try again."
		case "23514":
			message = "The profile could not be " + action + "d because one or more values are outside the allowed options. Check proficiency, work mode, salary, and date ranges."
		case "22P02", "22007", "22008":
			message = "The profile could not be " + action + "d because a date, number, or identifier has an invalid format. Review the entered values and try again."
		case "23502":
			message = "The profile could not be " + action + "d because a required value is missing. Check the highlighted profile fields and try again."
		case "23503":
			message = "The profile could not be " + action + "d because a related profile entry is invalid. Reload the page and try again."
		case "40001", "40P01":
			message = "The profile could not be " + action + "d because the database had a temporary conflict. Please try again."
		}
	}
	writeError(w, http.StatusBadRequest, message)
}

func decodeSearchInteger(raw json.RawMessage) (int, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, nil
	}
	var value any
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return 0, fmt.Errorf("invalid search number")
	}
	var n json.Number
	switch value := value.(type) {
	case json.Number:
		n = value
	case string:
		n = json.Number(strings.TrimSpace(value))
	default:
		return 0, fmt.Errorf("invalid search number")
	}
	i, err := n.Int64()
	if err != nil || i < 0 || i > int64(^uint(0)>>1) {
		return 0, fmt.Errorf("invalid search number")
	}
	return int(i), nil
}

func validateProfileEntries(data json.RawMessage) error {
	var profile struct {
		Skills struct {
			Programming  []string `json:"programming"`
			Technologies []string `json:"technologies"`
			Tools        []string `json:"tools"`
		} `json:"skills"`
		Searches []struct {
			Name         string          `json:"name"`
			Titles       []string        `json:"titles"`
			WorkMode     []string        `json:"workMode"`
			Technologies string          `json:"technologies"`
			Salary       json.RawMessage `json:"salary"`
			Hours        json.RawMessage `json:"hours"`
			Days         json.RawMessage `json:"days"`
			Months       json.RawMessage `json:"months"`
			Currency     string          `json:"currency"`
			Period       string          `json:"period"`
		} `json:"searches"`
	}
	// Decode the row collections separately so malformed field types fail validation
	// before they reach PostgreSQL casts or constraints.
	var raw struct {
		Education []struct {
			Institution string `json:"institution"`
			Start       string `json:"startDate"`
			End         string `json:"endDate"`
		} `json:"education"`
		Experience []struct {
			Company string `json:"company"`
			Title   string `json:"title"`
			Start   string `json:"startDate"`
			End     string `json:"endDate"`
			Current bool   `json:"current"`
		} `json:"experience"`
		Languages []struct {
			Name        string `json:"language"`
			Proficiency string `json:"proficiency"`
		} `json:"languages"`
		Certificates []struct {
			Name       string `json:"name"`
			Issue      string `json:"issueDate"`
			Expiration string `json:"expirationDate"`
		} `json:"certificates"`
		Awards []struct {
			Name string `json:"name"`
			Date string `json:"date"`
		} `json:"awards"`
	}
	if err := json.Unmarshal(data, &profile); err != nil {
		return err
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for _, entry := range raw.Education {
		if strings.TrimSpace(entry.Institution) == "" || !validDateRange(entry.Start, entry.End) {
			return fmt.Errorf("invalid education entry")
		}
	}
	for _, entry := range raw.Experience {
		if strings.TrimSpace(entry.Company) == "" || strings.TrimSpace(entry.Title) == "" || !validDateRange(entry.Start, entry.End) || (entry.Current && entry.End != "") {
			return fmt.Errorf("invalid experience entry")
		}
	}
	for _, entry := range raw.Certificates {
		if strings.TrimSpace(entry.Name) == "" || !validDateRange(entry.Issue, entry.Expiration) {
			return fmt.Errorf("invalid certificate entry")
		}
	}
	for _, entry := range raw.Awards {
		if !validDate(entry.Date) {
			return fmt.Errorf("invalid award date")
		}
	}
	for _, entry := range raw.Languages {
		if strings.TrimSpace(entry.Name) == "" || !oneOf(entry.Proficiency, "Beginner", "Elementary", "Intermediate", "Upper-intermediate", "Advanced", "Fluent", "Native", "Bilingual") {
			return fmt.Errorf("invalid language entry")
		}
	}
	if hasDuplicates(profile.Skills.Programming) || hasDuplicates(profile.Skills.Technologies) || hasDuplicates(profile.Skills.Tools) {
		return fmt.Errorf("duplicate skill")
	}
	if duplicateFolded(raw.Languages, func(v struct {
		Name        string `json:"language"`
		Proficiency string `json:"proficiency"`
	}) string {
		return v.Name
	}) {
		return fmt.Errorf("duplicate language")
	}
	for _, search := range profile.Searches {
		if strings.TrimSpace(search.Name) == "" || hasDuplicates(search.Titles) || hasDuplicates(splitValues(search.Technologies)) || hasDuplicates(search.WorkMode) {
			return fmt.Errorf("invalid or duplicate search values")
		}
		for _, mode := range search.WorkMode {
			if !oneOf(mode, "Remote", "Hybrid", "On-site") {
				return fmt.Errorf("invalid work mode")
			}
		}
		for _, raw := range []json.RawMessage{search.Hours, search.Days, search.Months} {
			if _, err := decodeSearchInteger(raw); err != nil {
				return err
			}
		}
		if len(search.Salary) > 0 && string(search.Salary) != "null" && string(search.Salary) != `""` {
			var salary string
			if json.Unmarshal(search.Salary, &salary) != nil {
				salary = string(search.Salary)
			}
			amount, err := strconv.ParseFloat(strings.TrimSpace(salary), 64)
			if err != nil || amount < 0 || !oneOf(search.Currency, "INR", "USD", "EUR", "GBP", "CAD", "AUD") || !oneOf(search.Period, "hour", "month", "year") {
				return fmt.Errorf("invalid salary settings")
			}
		}
	}
	return nil
}

func validDate(value string) bool {
	if value == "" {
		return true
	}
	value = strings.TrimSpace(value)
	if len(value) == 7 && value[4] == '-' {
		if _, err := time.Parse("2006-01", value); err != nil {
			return false
		}
		return true
	}
	_, err := time.Parse("2006-01-02", value)
	return err == nil
}
func validDateRange(start, end string) bool {
	if !validDate(start) || !validDate(end) {
		return false
	}
	if start == "" || end == "" {
		return true
	}
	return normalizeProfileDate(end) >= normalizeProfileDate(start)
}
func oneOf(value string, options ...string) bool {
	for _, option := range options {
		if value == option {
			return true
		}
	}
	return false
}
func hasDuplicates(values []string) bool {
	seen := map[string]bool{}
	for _, value := range values {
		key := strings.ToLower(strings.TrimSpace(value))
		if key == "" {
			continue
		}
		if seen[key] {
			return true
		}
		seen[key] = true
	}
	return false
}
func splitValues(value string) []string {
	values := strings.Split(value, ",")
	out := values[:0]
	for _, item := range values {
		if strings.TrimSpace(item) != "" {
			out = append(out, item)
		}
	}
	return out
}
func duplicateFolded[T any](values []T, key func(T) string) bool {
	seen := map[string]bool{}
	for _, value := range values {
		k := strings.ToLower(strings.TrimSpace(key(value)))
		if k == "" {
			continue
		}
		if seen[k] {
			return true
		}
		seen[k] = true
	}
	return false
}
func personal(data json.RawMessage, key string) string {
	var x struct {
		Personal map[string]string `json:"personal"`
	}
	_ = json.Unmarshal(data, &x)
	return x.Personal[key]
}
func profileSummary(data json.RawMessage) string { return personal(data, "summary") }
func targetRole(data json.RawMessage) string     { return personal(data, "headline") }
