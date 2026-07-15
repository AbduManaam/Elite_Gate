

ALTER TABLE admin_users
    ADD COLUMN IF NOT EXISTS is_super_admin BOOLEAN NOT NULL DEFAULT FALSE;

-- Mark the very first admin (bootstrap operator) as super-admin automatically.
-- No manual DB edit required after running this migration.
UPDATE admin_users
SET    is_super_admin = TRUE
WHERE  created_at = (SELECT MIN(created_at) FROM admin_users);
