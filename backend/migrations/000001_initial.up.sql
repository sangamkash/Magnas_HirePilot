CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email text NOT NULL UNIQUE CHECK (length(trim(email)) > 0),
    password_hash text NOT NULL CHECK (length(password_hash) > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE profiles (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    full_name text NOT NULL CHECK (length(trim(full_name)) > 0),
    email text,
    phone text,
    location text,
    headline text,
    summary text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE profile_skills (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    profile_id uuid NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    category text NOT NULL CHECK (category IN ('programming', 'technology', 'tool')),
    name text NOT NULL CHECK (length(trim(name)) > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (profile_id, category, name)
);
CREATE INDEX profile_skills_profile_id_idx ON profile_skills(profile_id);

CREATE TABLE education (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    profile_id uuid NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    institution text NOT NULL CHECK (length(trim(institution)) > 0),
    degree text,
    field_of_study text,
    start_date date,
    end_date date,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (start_date IS NULL OR end_date IS NULL OR end_date >= start_date)
);
CREATE INDEX education_profile_id_idx ON education(profile_id);

CREATE TABLE work_experience (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    profile_id uuid NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    company text NOT NULL CHECK (length(trim(company)) > 0),
    title text NOT NULL CHECK (length(trim(title)) > 0),
    employment_type text,
    location text,
    start_date date,
    end_date date,
    is_current boolean NOT NULL DEFAULT false,
    responsibilities text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (start_date IS NULL OR end_date IS NULL OR end_date >= start_date),
    CHECK (NOT is_current OR end_date IS NULL)
);
CREATE INDEX work_experience_profile_id_idx ON work_experience(profile_id);

CREATE TABLE profile_languages (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    profile_id uuid NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    language text NOT NULL CHECK (length(trim(language)) > 0),
    proficiency text NOT NULL CHECK (proficiency IN ('Beginner', 'Elementary', 'Intermediate', 'Upper-intermediate', 'Advanced', 'Fluent', 'Native', 'Bilingual')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (profile_id, language)
);
CREATE INDEX profile_languages_profile_id_idx ON profile_languages(profile_id);

CREATE TABLE certificates (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    profile_id uuid NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    name text NOT NULL CHECK (length(trim(name)) > 0),
    organization text,
    issue_date date,
    expiration_date date,
    credential_url text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (issue_date IS NULL OR expiration_date IS NULL OR expiration_date >= issue_date)
);
CREATE INDEX certificates_profile_id_idx ON certificates(profile_id);

CREATE TABLE awards (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    profile_id uuid NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    name text NOT NULL CHECK (length(trim(name)) > 0),
    organization text,
    awarded_on date,
    description text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX awards_profile_id_idx ON awards(profile_id);

CREATE TABLE search_configs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    profile_id uuid NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    name text NOT NULL CHECK (length(trim(name)) > 0),
    region text,
    minimum_salary numeric(14, 2) CHECK (minimum_salary IS NULL OR minimum_salary >= 0),
    salary_currency char(3) CHECK (salary_currency IS NULL OR salary_currency ~ '^[A-Z]{3}$'),
    salary_period text CHECK (salary_period IS NULL OR salary_period IN ('hour', 'month', 'year')),
    posted_hours integer NOT NULL DEFAULT 0 CHECK (posted_hours >= 0),
    posted_days integer NOT NULL DEFAULT 0 CHECK (posted_days >= 0),
    posted_months integer NOT NULL DEFAULT 0 CHECK (posted_months >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((minimum_salary IS NULL AND salary_currency IS NULL AND salary_period IS NULL) OR
           (minimum_salary IS NOT NULL AND salary_currency IS NOT NULL AND salary_period IS NOT NULL))
);
CREATE INDEX search_configs_profile_id_idx ON search_configs(profile_id);

CREATE TABLE search_job_titles (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    search_config_id uuid NOT NULL REFERENCES search_configs(id) ON DELETE CASCADE,
    title text NOT NULL CHECK (length(trim(title)) > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (search_config_id, title)
);
CREATE INDEX search_job_titles_config_id_idx ON search_job_titles(search_config_id);

CREATE TABLE search_technologies (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    search_config_id uuid NOT NULL REFERENCES search_configs(id) ON DELETE CASCADE,
    technology text NOT NULL CHECK (length(trim(technology)) > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (search_config_id, technology)
);
CREATE INDEX search_technologies_config_id_idx ON search_technologies(search_config_id);

CREATE TABLE search_work_modes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    search_config_id uuid NOT NULL REFERENCES search_configs(id) ON DELETE CASCADE,
    work_mode text NOT NULL CHECK (work_mode IN ('Remote', 'Hybrid', 'On-site')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (search_config_id, work_mode)
);
CREATE INDEX search_work_modes_config_id_idx ON search_work_modes(search_config_id);

