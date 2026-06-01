-- ============================================================
-- Migration 0007: Backfill normalized tables from old columns
-- Reads existing data and populates the new structure.
-- Old columns are still present and untouched.
-- ============================================================

-- ── Step 1: Build policies from distinct rule combinations ──
-- Each unique (auth_required, rate_limit_rpm) pair becomes
-- one named policy row. Name is auto-generated from the values.
-- You can rename them later via the admin UI.
INSERT INTO policies (name, auth_required, rate_limit_rpm)
SELECT DISTINCT
    CASE
        WHEN auth_required = false THEN 'public'
        WHEN rate_limit_rpm = 0    THEN 'authenticated_unlimited'
        ELSE 'authenticated_' || rate_limit_rpm::TEXT || '_rpm'
    END,
    auth_required,
    rate_limit_rpm
FROM routes
ON CONFLICT (name) DO NOTHING;

-- ── Step 2: Link each route to its matching policy ──
-- Matches on both fields to find the correct policy row.
UPDATE routes r
SET policy_id = p.id
FROM policies p
WHERE p.auth_required  = r.auth_required
  AND p.rate_limit_rpm = r.rate_limit_rpm;

-- ── Step 3: Populate route_methods from methods[] array ──
-- unnest() expands {GET, POST, DELETE} into individual rows.
-- ON CONFLICT DO NOTHING protects against re-running this migration.
-- Only insert recognised HTTP methods.
-- gRPC routes use {*} as a wildcard which is not an HTTP method
-- and would violate the CHECK constraint on route_methods.method.
INSERT INTO route_methods (route_id, method)
SELECT id, m
FROM   routes,
       unnest(methods) AS m
WHERE  methods IS NOT NULL
  AND  array_length(methods, 1) > 0
  AND  m IN ('GET','POST','PUT','DELETE','PATCH','HEAD','OPTIONS')
ON CONFLICT DO NOTHING;
