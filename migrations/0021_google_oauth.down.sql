ALTER TABLE admin_users
    DROP CONSTRAINT IF EXISTS chk_admin_users_password_provider;

DROP INDEX IF EXISTS idx_admin_users_google_id;

ALTER TABLE admin_users
    DROP COLUMN IF EXISTS auth_provider;

ALTER TABLE admin_users
    DROP COLUMN IF EXISTS avatar_url;

ALTER TABLE admin_users
    DROP COLUMN IF EXISTS google_id;

ALTER TABLE admin_users
    ALTER COLUMN password_hash SET NOT NULL;