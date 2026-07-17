ALTER TABLE admin_users
    ALTER COLUMN password_hash DROP NOT NULL;

ALTER TABLE admin_users
    ADD COLUMN IF NOT EXISTS google_id TEXT,
    ADD COLUMN IF NOT EXISTS avatar_url TEXT,
    ADD COLUMN IF NOT EXISTS auth_provider TEXT NOT NULL DEFAULT 'password';

CREATE UNIQUE INDEX IF NOT EXISTS idx_admin_users_google_id
    ON admin_users (google_id)
    WHERE google_id IS NOT NULL;

ALTER TABLE admin_users
    ADD CONSTRAINT chk_admin_users_password_provider
    CHECK (
        auth_provider <> 'password'
        OR password_hash IS NOT NULL
    );