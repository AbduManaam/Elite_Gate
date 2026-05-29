ALTER TABLE routes
    DROP COLUMN IF EXISTS upstream_id,
    DROP COLUMN IF EXISTS protocol,
    DROP COLUMN IF EXISTS match_type,
    DROP COLUMN IF EXISTS enabled,
    DROP COLUMN IF EXISTS auth_required,
    DROP COLUMN IF EXISTS rate_limit_rpm,
    DROP COLUMN IF EXISTS updated_at;