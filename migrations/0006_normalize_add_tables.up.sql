-- ============================================================
-- Migration 0006: Add normalized tables (backward compatible)
-- Old columns are kept untouched. New tables are added alongside.
-- No data is moved in this migration.
-- ============================================================

-- Step 1: Create the policies table.
-- Extracts auth_required + rate_limit_rpm out of routes
-- so multiple routes can share one named policy.
CREATE TABLE IF NOT EXISTS policies (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name           TEXT        UNIQUE NOT NULL,
    auth_required  BOOLEAN     NOT NULL DEFAULT TRUE,
    rate_limit_rpm INT         NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Step 2: Create the route_methods junction table.
-- Replaces the TEXT[] methods array on routes with atomic rows.
-- Composite PK prevents duplicate method entries per route.
CREATE TABLE IF NOT EXISTS route_methods (
    route_id UUID NOT NULL REFERENCES routes(id) ON DELETE CASCADE,
    method   TEXT NOT NULL CHECK (method IN ('GET','POST','PUT','DELETE','PATCH','HEAD','OPTIONS')),
    PRIMARY KEY (route_id, method)
);

-- Step 3: Add policy_id to routes.
-- NULL is allowed here — will be filled in migration 0007 backfill.
ALTER TABLE routes
    ADD COLUMN IF NOT EXISTS policy_id UUID REFERENCES policies(id);

-- Step 4: Add admin_user_id to api_keys.
-- NULL is allowed here — will be filled in migration 0007 backfill.
ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS admin_user_id UUID REFERENCES admin_users(id) ON DELETE SET NULL;
