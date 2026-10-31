DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM users WHERE email IS NULL) THEN
        RAISE EXCEPTION 'cannot roll back username auth while users without email exist';
    END IF;
END $$;

ALTER TABLE users DROP CONSTRAINT users_username_format_check;
DROP INDEX users_username_lower_idx;
ALTER TABLE users ALTER COLUMN email SET NOT NULL;
ALTER TABLE users DROP COLUMN username;
