-- Composite keys let the database enforce profile/search/discovery ownership.
ALTER TABLE search_configs ADD CONSTRAINT search_configs_id_profile_id_unique UNIQUE (id, profile_id);
ALTER TABLE job_discovery_runs ADD CONSTRAINT job_discovery_runs_id_search_config_id_unique UNIQUE (id, search_config_id);

CREATE TABLE job_match_runs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    profile_id uuid NOT NULL,
    search_config_id uuid NOT NULL,
    discovery_run_id uuid NOT NULL,
    status varchar(20) NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','RUNNING','COMPLETED','FAILED','CANCELLED')),
    provider_name varchar(50),
    model_name varchar(255),
    prompt_version varchar(100) NOT NULL,
    algorithm_version varchar(100) NOT NULL,
    profile_snapshot jsonb NOT NULL CHECK (jsonb_typeof(profile_snapshot) = 'object'),
    jobs_processed integer NOT NULL DEFAULT 0 CHECK (jobs_processed >= 0),
    matches_found integer NOT NULL DEFAULT 0 CHECK (matches_found >= 0 AND matches_found <= jobs_processed),
    total_tokens bigint NOT NULL DEFAULT 0 CHECK (total_tokens >= 0),
    total_cost numeric(14,8) CHECK (total_cost IS NULL OR total_cost >= 0),
    error_message text,
    started_at timestamptz,
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (search_config_id, profile_id) REFERENCES search_configs(id, profile_id) ON DELETE CASCADE,
    FOREIGN KEY (discovery_run_id, search_config_id) REFERENCES job_discovery_runs(id, search_config_id) ON DELETE RESTRICT,
    CHECK (completed_at IS NULL OR started_at IS NULL OR completed_at >= started_at),
    CHECK ((status IN ('COMPLETED','FAILED','CANCELLED')) = (completed_at IS NOT NULL))
);
ALTER TABLE job_match_runs ADD CONSTRAINT job_match_runs_id_discovery_run_id_unique UNIQUE (id, discovery_run_id);
CREATE INDEX job_match_runs_profile_created_idx ON job_match_runs(profile_id, created_at DESC);
CREATE INDEX job_match_runs_search_config_created_idx ON job_match_runs(search_config_id, created_at DESC);
CREATE INDEX job_match_runs_discovery_run_idx ON job_match_runs(discovery_run_id);
CREATE INDEX job_match_runs_status_idx ON job_match_runs(status);

CREATE TABLE job_matches (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    match_run_id uuid NOT NULL REFERENCES job_match_runs(id) ON DELETE CASCADE,
    job_id uuid NOT NULL REFERENCES jobs(id) ON DELETE RESTRICT,
    discovery_run_id uuid NOT NULL,
    match_score numeric(5,2) NOT NULL CHECK (match_score >= 0 AND match_score <= 100),
    summary text,
    strengths jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(strengths) = 'array'),
    gaps jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(gaps) = 'array'),
    score_breakdown jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(score_breakdown) = 'object'),
    explanation jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(explanation) = 'object'),
    recommendation varchar(40) NOT NULL CHECK (recommendation IN ('HIGHLY_RECOMMENDED','RECOMMENDED','CONSIDER','NOT_RECOMMENDED','UNKNOWN')),
    raw_ai_response jsonb NOT NULL CHECK (jsonb_typeof(raw_ai_response) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (match_run_id, job_id)
);
ALTER TABLE job_matches ADD CONSTRAINT job_matches_run_provenance_fk
    FOREIGN KEY (match_run_id, discovery_run_id) REFERENCES job_match_runs(id, discovery_run_id) ON DELETE CASCADE;
ALTER TABLE job_matches ADD CONSTRAINT job_matches_discovery_job_fk
    FOREIGN KEY (discovery_run_id, job_id) REFERENCES job_discovery_results(discovery_run_id, job_id) ON DELETE RESTRICT;
CREATE INDEX job_matches_job_id_idx ON job_matches(job_id);
CREATE INDEX job_matches_score_idx ON job_matches(match_score DESC);
