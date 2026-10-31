ALTER TABLE users ADD COLUMN username text;

WITH bases AS (
    SELECT id,
           regexp_replace(lower(split_part(email, '@', 1)), '[^a-z0-9_]', '', 'g') AS base
    FROM users
), ranked AS (
    SELECT id, base,
           row_number() OVER (PARTITION BY base ORDER BY id) AS duplicate_number
    FROM bases
)
UPDATE users AS u
SET username = CASE
    WHEN length(ranked.base) >= 3 THEN
        left(ranked.base, 32 - CASE WHEN ranked.duplicate_number = 1 THEN 0 ELSE length('_' || ranked.duplicate_number::text) END) ||
        CASE WHEN ranked.duplicate_number = 1 THEN '' ELSE '_' || ranked.duplicate_number::text END
    ELSE 'user_' || substring(replace(ranked.id::text, '-', '') FROM 1 FOR 12)
END
FROM ranked
WHERE u.id = ranked.id;
ALTER TABLE users ALTER COLUMN username SET NOT NULL;
ALTER TABLE users ALTER COLUMN email DROP NOT NULL;
ALTER TABLE users ALTER COLUMN email DROP DEFAULT;
CREATE UNIQUE INDEX users_username_lower_idx ON users (lower(username));
ALTER TABLE users ADD CONSTRAINT users_username_format_check
    CHECK (username ~ '^[A-Za-z0-9_]{3,32}$');
