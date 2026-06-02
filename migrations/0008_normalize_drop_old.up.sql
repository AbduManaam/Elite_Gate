
ALTER TABLE routes
    DROP COLUMN IF EXISTS upstream_url,
    DROP COLUMN IF EXISTS protocol,
    DROP COLUMN IF EXISTS methods,
    DROP COLUMN IF EXISTS auth_required,
    DROP COLUMN IF EXISTS rate_limit_rpm;
