-- =============================================================================
-- Migration 0010: Rollback
-- =============================================================================

-- -----------------------------------------------------------------------------
-- 3. Drop audit_logs
-- -----------------------------------------------------------------------------

ALTER TABLE IF EXISTS audit_logs DISABLE ROW LEVEL SECURITY;
DROP POLICY  IF EXISTS audit_logs_tenant_isolation ON audit_logs;
DROP TABLE   IF EXISTS audit_logs;


-- -----------------------------------------------------------------------------
-- 2. Drop gateways
-- -----------------------------------------------------------------------------

ALTER TABLE IF EXISTS gateways DISABLE ROW LEVEL SECURITY;
DROP POLICY  IF EXISTS gateways_tenant_isolation ON gateways;
DROP TABLE   IF EXISTS gateways;


-- -----------------------------------------------------------------------------
-- 1. Revert api_keys to its pre-0010 state
-- -----------------------------------------------------------------------------

ALTER TABLE IF EXISTS api_keys DISABLE ROW LEVEL SECURITY;
DROP POLICY  IF EXISTS api_keys_tenant_isolation ON api_keys;

DROP INDEX IF EXISTS idx_api_keys_project_id;
DROP INDEX IF EXISTS idx_api_keys_key_hash;
DROP INDEX IF EXISTS idx_api_keys_status;

ALTER TABLE api_keys
    DROP COLUMN IF EXISTS project_id,
    DROP COLUMN IF EXISTS name,
    DROP COLUMN IF EXISTS status,
    DROP COLUMN IF EXISTS expires_at,
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS deleted_at;


-- -----------------------------------------------------------------------------
-- 0. Drop ENUMs added by this migration
--    Only drop if no other table still references them.
-- -----------------------------------------------------------------------------

DROP TYPE IF EXISTS gateway_status;
DROP TYPE IF EXISTS gateway_plan;
-- Do NOT drop key_status or audit_action here if earlier migrations created them.
-- Uncomment the lines below only if 0010 was the migration that first created them:
-- DROP TYPE IF EXISTS key_status;
-- DROP TYPE IF EXISTS audit_action;
