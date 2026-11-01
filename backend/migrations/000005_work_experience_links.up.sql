CREATE TABLE work_experience_links (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    work_experience_id uuid NOT NULL REFERENCES work_experience(id) ON DELETE CASCADE,
    url text NOT NULL CHECK (length(trim(url)) > 0),
    position integer NOT NULL DEFAULT 0 CHECK (position >= 0),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX work_experience_links_experience_position_idx
    ON work_experience_links(work_experience_id, position, id);
