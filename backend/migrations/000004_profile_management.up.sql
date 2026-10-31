-- Expand the existing profile model without discarding its relational data.
ALTER TABLE profiles DROP CONSTRAINT profiles_user_id_key;
ALTER TABLE profiles ADD COLUMN name text NOT NULL DEFAULT 'My profile';
ALTER TABLE profiles ADD COLUMN target_role text NOT NULL DEFAULT '';
ALTER TABLE profiles ADD COLUMN profile_data jsonb NOT NULL DEFAULT '{}'::jsonb;

-- Preserve the existing scalar profile fields in the API's JSONB representation.
UPDATE profiles
SET profile_data = jsonb_build_object(
    'personal', jsonb_strip_nulls(jsonb_build_object(
        'name', full_name,
        'email', email,
        'phone', phone,
        'location', location,
        'headline', headline,
        'summary', summary
    )),
    'skills', jsonb_build_object(
        'programming', COALESCE((SELECT jsonb_agg(name ORDER BY name) FROM profile_skills WHERE profile_id=profiles.id AND category='programming'), '[]'::jsonb),
        'technologies', COALESCE((SELECT jsonb_agg(name ORDER BY name) FROM profile_skills WHERE profile_id=profiles.id AND category='technology'), '[]'::jsonb),
        'tools', COALESCE((SELECT jsonb_agg(name ORDER BY name) FROM profile_skills WHERE profile_id=profiles.id AND category='tool'), '[]'::jsonb)
    ),
    'education', COALESCE((SELECT jsonb_agg(jsonb_strip_nulls(jsonb_build_object('institution', institution, 'degree', degree, 'field', field_of_study, 'startDate', start_date, 'endDate', end_date)) ORDER BY start_date NULLS LAST, id) FROM education WHERE profile_id=profiles.id), '[]'::jsonb),
    'experience', COALESCE((SELECT jsonb_agg(jsonb_strip_nulls(jsonb_build_object('company', company, 'title', title, 'employmentType', employment_type, 'location', location, 'startDate', start_date, 'endDate', end_date, 'current', is_current, 'responsibilities', responsibilities)) ORDER BY start_date NULLS LAST, id) FROM work_experience WHERE profile_id=profiles.id), '[]'::jsonb),
    'languages', COALESCE((SELECT jsonb_agg(jsonb_build_object('language', language, 'proficiency', proficiency) ORDER BY language) FROM profile_languages WHERE profile_id=profiles.id), '[]'::jsonb),
    'certificates', COALESCE((SELECT jsonb_agg(jsonb_strip_nulls(jsonb_build_object('name', name, 'organization', organization, 'issueDate', issue_date, 'expirationDate', expiration_date, 'url', credential_url)) ORDER BY issue_date NULLS LAST, id) FROM certificates WHERE profile_id=profiles.id), '[]'::jsonb),
    'awards', COALESCE((SELECT jsonb_agg(jsonb_strip_nulls(jsonb_build_object('name', name, 'organization', organization, 'date', awarded_on, 'description', description)) ORDER BY awarded_on NULLS LAST, id) FROM awards WHERE profile_id=profiles.id), '[]'::jsonb),
    'searches', COALESCE((SELECT jsonb_agg(jsonb_build_object(
        'id', sc.id,
        'name', sc.name,
        'titles', COALESCE((SELECT jsonb_agg(title ORDER BY title) FROM search_job_titles WHERE search_config_id=sc.id), '[]'::jsonb),
        'technologies', COALESCE((SELECT string_agg(technology, ', ' ORDER BY technology) FROM search_technologies WHERE search_config_id=sc.id), ''),
        'region', COALESCE(sc.region, ''),
        'workMode', COALESCE((SELECT jsonb_agg(work_mode ORDER BY work_mode) FROM search_work_modes WHERE search_config_id=sc.id), '[]'::jsonb),
        'salary', COALESCE(sc.minimum_salary::text, ''),
        'currency', COALESCE(trim(sc.salary_currency), ''),
        'period', COALESCE(sc.salary_period, ''),
        'hours', sc.posted_hours,
        'days', sc.posted_days,
        'months', sc.posted_months
    ) ORDER BY sc.created_at, sc.id) FROM search_configs sc WHERE sc.profile_id=profiles.id), '[]'::jsonb)
);
UPDATE profiles SET name = COALESCE(NULLIF(trim(headline), ''), NULLIF(trim(full_name), ''), 'My profile');
UPDATE profiles SET target_role = COALESCE(headline, '');

-- Give imported client profiles a per-profile idempotency marker.
ALTER TABLE profiles ADD COLUMN legacy_import_id text;
CREATE UNIQUE INDEX profiles_user_legacy_import_id_idx
    ON profiles(user_id, legacy_import_id) WHERE legacy_import_id IS NOT NULL;

ALTER TABLE profiles ADD CONSTRAINT profiles_id_user_id_unique UNIQUE (id, user_id);

-- Keep the active profile relationship ownership-safe at the database level.
ALTER TABLE users ADD COLUMN active_profile_id uuid;
ALTER TABLE users ADD CONSTRAINT users_active_profile_owner_fk
    FOREIGN KEY (active_profile_id, id) REFERENCES profiles(id, user_id)
    ON DELETE SET NULL (active_profile_id) DEFERRABLE INITIALLY DEFERRED;
CREATE INDEX profiles_user_id_created_at_idx ON profiles(user_id, created_at);
