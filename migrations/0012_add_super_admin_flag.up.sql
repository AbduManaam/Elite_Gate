-- Migration: 0012_add_super_admin_flag.up.sql
--
-- Adds is_super_admin column to admin_users.
-- Only the bootstrap operator admin should ever be TRUE.
-- Every tenant account created via POST /admin/signup is FALSE.
--
-- Future platform-operator features (tenant suspension, impersonation,
-- secret rotation, system-wide audit) attach to this flag without a
-- new schema migration -- the SuperAdminOnly middleware gates on it.

ALTER TABLE admin_users
    ADD COLUMN IF NOT EXISTS is_super_admin BOOLEAN NOT NULL DEFAULT FALSE;

-- Mark the very first admin (bootstrap operator) as super-admin automatically.
-- No manual DB edit required after running this migration.
UPDATE admin_users
SET    is_super_admin = TRUE
WHERE  created_at = (SELECT MIN(created_at) FROM admin_users);
