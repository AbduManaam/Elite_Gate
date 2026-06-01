-- ============================================================
-- Rollback for 0006: Remove normalized tables
-- Safe to run — drops only what 0006 added.
-- ============================================================

ALTER TABLE api_keys  DROP COLUMN IF EXISTS admin_user_id;
ALTER TABLE routes    DROP COLUMN IF EXISTS policy_id;

DROP TABLE IF EXISTS route_methods;
DROP TABLE IF EXISTS policies;
