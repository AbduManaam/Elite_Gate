-- ============================================================
-- Migration 0008: Drop old redundant columns from routes
-- ⚠ Run this ONLY after:
--   1. Migration 0007 backfill has been verified
--   2. Application code reads from new schema (policy_id, route_methods)
--   3. No code still reads upstream_url, protocol, methods[],
--      auth_required, or rate_limit_rpm directly from routes
-- ============================================================

ALTER TABLE routes
    DROP COLUMN IF EXISTS upstream_url,
    DROP COLUMN IF EXISTS protocol,
    DROP COLUMN IF EXISTS methods,
    DROP COLUMN IF EXISTS auth_required,
    DROP COLUMN IF EXISTS rate_limit_rpm;
