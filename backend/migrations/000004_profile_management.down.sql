DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM profiles WHERE profile_data <> '{}'::jsonb OR legacy_import_id IS NOT NULL)
       OR EXISTS (SELECT 1 FROM users WHERE active_profile_id IS NOT NULL)
       OR EXISTS (SELECT 1 FROM profiles GROUP BY user_id HAVING count(*) > 1) THEN
        RAISE EXCEPTION '000004 is forward-only after profile data, imports, active selections, or multiple profiles exist';
    END IF;
END $$;

DROP INDEX IF EXISTS profiles_user_id_created_at_idx;
DROP INDEX IF EXISTS profiles_user_legacy_import_id_idx;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_active_profile_owner_fk;
ALTER TABLE users DROP COLUMN IF EXISTS active_profile_id;
ALTER TABLE profiles DROP CONSTRAINT IF EXISTS profiles_id_user_id_unique;
ALTER TABLE profiles DROP COLUMN IF EXISTS profile_data;
ALTER TABLE profiles DROP COLUMN IF EXISTS target_role;
ALTER TABLE profiles DROP COLUMN IF EXISTS name;
ALTER TABLE profiles DROP COLUMN IF EXISTS legacy_import_id;
ALTER TABLE profiles ADD CONSTRAINT profiles_user_id_key UNIQUE (user_id);
