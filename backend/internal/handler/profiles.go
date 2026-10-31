package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

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
			writeError(w, 400, "Profile name and personal full name are required; profile data must be a JSON object.")
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
			writeError(w, 400, "Profile name and personal full name are required; profile data must be a JSON object.")
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
	for _, v := range profile.Experience {
		if strings.TrimSpace(v.Company) == "" || strings.TrimSpace(v.Title) == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO work_experience(profile_id,company,title,employment_type,location,start_date,end_date,is_current,responsibilities) VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,''),NULLIF($6,'')::date,NULLIF($7,'')::date,$8,NULLIF($9,''))`, id, v.Company, v.Title, v.EmploymentType, v.Location, normalizeProfileDate(v.Start), normalizeProfileDate(v.End), v.Current, v.Description); err != nil {
			return err
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
		Education    []json.RawMessage `json:"education"`
		Experience   []json.RawMessage `json:"experience"`
		Languages    []json.RawMessage `json:"languages"`
		Certificates []json.RawMessage `json:"certificates"`
		Awards       []json.RawMessage `json:"awards"`
		Searches     []struct {
			Titles       []string        `json:"titles"`
			WorkMode     []string        `json:"workMode"`
			Technologies string          `json:"technologies"`
			Salary       json.RawMessage `json:"salary"`
			Hours        json.RawMessage `json:"hours"`
			Days         json.RawMessage `json:"days"`
			Months       json.RawMessage `json:"months"`
		} `json:"searches"`
	}
	if err := json.Unmarshal(data, &profile); err != nil {
		return err
	}
	for _, group := range []struct {
		entries  []json.RawMessage
		required []string
	}{
		{profile.Education, []string{"institution"}},
		{profile.Experience, []string{"company", "title"}},
		{profile.Languages, []string{"language"}},
		{profile.Certificates, []string{"name"}},
		{profile.Awards, []string{"name"}},
	} {
		for _, raw := range group.entries {
			var entry map[string]json.RawMessage
			if err := json.Unmarshal(raw, &entry); err != nil || entry == nil {
				return fmt.Errorf("invalid nested profile entry")
			}
			for _, key := range group.required {
				var value string
				if json.Unmarshal(entry[key], &value) != nil || strings.TrimSpace(value) == "" {
					return fmt.Errorf("nested profile entry is missing %s", key)
				}
			}
		}
	}
	for _, search := range profile.Searches {
		for _, raw := range []json.RawMessage{search.Hours, search.Days, search.Months} {
			if _, err := decodeSearchInteger(raw); err != nil {
				return err
			}
		}
	}
	return nil
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
