CREATE TABLE job_sources (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name varchar(100) NOT NULL CHECK (length(trim(name)) > 0),
    provider_type varchar(50) NOT NULL CHECK (length(trim(provider_type)) > 0),
    base_url text,
    is_active boolean NOT NULL DEFAULT true,
    configuration jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(configuration) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX job_sources_name_lower_idx ON job_sources (lower(name));
COMMENT ON COLUMN job_sources.configuration IS 'Non-secret provider settings only; credentials belong in environment configuration or a secrets manager.';

CREATE TABLE jobs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id uuid NOT NULL REFERENCES job_sources(id) ON DELETE RESTRICT,
    external_job_id varchar(255) NOT NULL CHECK (length(trim(external_job_id)) > 0),
    title text NOT NULL CHECK (length(trim(title)) > 0),
    company_name text NOT NULL CHECK (length(trim(company_name)) > 0),
    description text,
    location text,
    country varchar(2) CHECK (country IS NULL OR country ~ '^[A-Z]{2}$'),
    employment_type varchar(100) CHECK (employment_type IS NULL OR length(trim(employment_type)) > 0),
    work_mode varchar(50) CHECK (work_mode IS NULL OR length(trim(work_mode)) > 0),
    salary_min numeric(14, 2) CHECK (salary_min IS NULL OR salary_min >= 0),
    salary_max numeric(14, 2) CHECK (salary_max IS NULL OR salary_max >= 0),
    salary_currency varchar(3) CHECK (salary_currency IS NULL OR salary_currency ~ '^[A-Z]{3}$'),
    salary_period varchar(20) CHECK (salary_period IS NULL OR salary_period IN ('hour', 'month', 'year')),
    application_url text,
    posted_at timestamptz,
    expires_at timestamptz,
    content_hash varchar(128),
    raw_data jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(raw_data) = 'object'),
    status varchar(20) NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'CLOSED', 'EXPIRED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (source_id, external_job_id),
    CHECK (salary_min IS NULL OR salary_max IS NULL OR salary_max >= salary_min),
    CHECK (expires_at IS NULL OR posted_at IS NULL OR expires_at >= posted_at)
);
CREATE INDEX jobs_title_idx ON jobs (title);
CREATE INDEX jobs_company_name_idx ON jobs (company_name);
CREATE INDEX jobs_country_idx ON jobs (country);
CREATE INDEX jobs_work_mode_idx ON jobs (work_mode);
CREATE INDEX jobs_posted_at_idx ON jobs (posted_at DESC);
CREATE INDEX jobs_status_idx ON jobs (status);
COMMENT ON TABLE jobs IS 'Globally shared normalized provider jobs; user association is recorded through discovery runs.';

CREATE TABLE job_discovery_runs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    search_config_id uuid NOT NULL REFERENCES search_configs(id) ON DELETE CASCADE,
    source_id uuid NOT NULL REFERENCES job_sources(id) ON DELETE RESTRICT,
    status varchar(20) NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'RUNNING', 'COMPLETED', 'FAILED')),
    started_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    jobs_found integer NOT NULL DEFAULT 0 CHECK (jobs_found >= 0),
    jobs_new integer NOT NULL DEFAULT 0 CHECK (jobs_new >= 0),
    jobs_updated integer NOT NULL DEFAULT 0 CHECK (jobs_updated >= 0),
    error_message text,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (completed_at IS NULL OR completed_at >= started_at),
    CHECK (jobs_new + jobs_updated <= jobs_found),
    CHECK ((status IN ('COMPLETED', 'FAILED')) = (completed_at IS NOT NULL))
);
CREATE INDEX job_discovery_runs_search_config_id_idx ON job_discovery_runs (search_config_id, started_at DESC);
CREATE INDEX job_discovery_runs_source_id_idx ON job_discovery_runs (source_id, started_at DESC);
CREATE INDEX job_discovery_runs_status_idx ON job_discovery_runs (status);

CREATE TABLE job_discovery_results (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    discovery_run_id uuid NOT NULL REFERENCES job_discovery_runs(id) ON DELETE CASCADE,
    job_id uuid NOT NULL REFERENCES jobs(id) ON DELETE RESTRICT,
    discovered_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (discovery_run_id, job_id)
);
CREATE INDEX job_discovery_results_job_id_idx ON job_discovery_results (job_id);
CREATE INDEX job_discovery_results_discovered_at_idx ON job_discovery_results (discovered_at DESC);

-- Keep jobs_found equal to the number of jobs recorded for the run. This avoids
-- application-side counters drifting from the provenance rows.
CREATE FUNCTION sync_job_discovery_found_count() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    target_run_id uuid;
BEGIN
    target_run_id := COALESCE(NEW.discovery_run_id, OLD.discovery_run_id);
    UPDATE job_discovery_runs
       SET jobs_found = (SELECT count(*)::integer FROM job_discovery_results WHERE discovery_run_id = target_run_id)
     WHERE id = target_run_id;
    RETURN NULL;
END;
$$;
CREATE TRIGGER job_discovery_results_sync_found_count
AFTER INSERT OR DELETE ON job_discovery_results
FOR EACH ROW EXECUTE FUNCTION sync_job_discovery_found_count();
