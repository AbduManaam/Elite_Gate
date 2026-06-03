-- Migration Rollback: 0009_multi_tenant_rls.down.sql
-- Strategy: Option 1 — Shared DB, Row-Level Isolation (Clean Rollback)


-- 1. DROP DATABASE APP ROLE PERMISSIONS

REVOKE ALL PRIVILEGES ON TABLE admin_users, projects, project_members, routes, upstreams, policies FROM coreguard_app;
REVOKE ALL PRIVILEGES ON SCHEMA public FROM coreguard_app;
DROP ROLE IF EXISTS coreguard_app;


-- 2. DROP RLS POLICIES & DISABLE RLS

DROP POLICY IF EXISTS routes_tenant_isolation ON routes;
DROP POLICY IF EXISTS upstreams_tenant_isolation ON upstreams;
DROP POLICY IF EXISTS policies_tenant_isolation ON policies;

ALTER TABLE routes NO FORCE ROW LEVEL SECURITY;
ALTER TABLE routes DISABLE ROW LEVEL SECURITY;

ALTER TABLE upstreams NO FORCE ROW LEVEL SECURITY;
ALTER TABLE upstreams DISABLE ROW LEVEL SECURITY;

ALTER TABLE policies NO FORCE ROW LEVEL SECURITY;
ALTER TABLE policies DISABLE ROW LEVEL SECURITY;

DROP FUNCTION IF EXISTS current_project_id();


-- 3. DROP PERFORMANCE INDEXES

DROP INDEX IF EXISTS idx_policies_project_id;
DROP INDEX IF EXISTS idx_upstreams_project_id;
DROP INDEX IF EXISTS idx_routes_project_path;
DROP INDEX IF EXISTS idx_routes_project_id;
DROP INDEX IF EXISTS idx_project_members_project;
DROP INDEX IF EXISTS idx_project_members_user;
DROP INDEX IF EXISTS idx_projects_slug;
DROP INDEX IF EXISTS idx_projects_owner;
DROP INDEX IF EXISTS idx_admin_users_email;


-- 4. RECREATE ROUTE_METHODS JOIN TABLE & BACKFILL FROM ARRAY

CREATE TABLE IF NOT EXISTS route_methods (
    route_id UUID NOT NULL REFERENCES routes(id) ON DELETE CASCADE,
    method   TEXT NOT NULL,
    PRIMARY KEY (route_id, method)
);

-- Extract methods array back into route_methods rows
INSERT INTO route_methods (route_id, method)
SELECT id, unnest(methods)::text
FROM routes;


-- 5. REVERT TABLES (REMOVE COLUMNS AND RESTORE ORIGINAL CONSTRAINTS)


-- Routes
ALTER TABLE routes 
    DROP COLUMN IF EXISTS methods,
    DROP COLUMN IF EXISTS project_id,
    DROP COLUMN IF EXISTS name,
    DROP COLUMN IF EXISTS deleted_at;

-- Policies
ALTER TABLE policies 
    DROP COLUMN IF EXISTS project_id,
    DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE policies DROP CONSTRAINT IF EXISTS policies_project_name_unique;
ALTER TABLE policies ADD CONSTRAINT policies_name_key UNIQUE (name);

-- Upstreams
ALTER TABLE upstreams 
    DROP COLUMN IF EXISTS project_id,
    DROP COLUMN IF EXISTS connect_timeout_ms,
    DROP COLUMN IF EXISTS read_timeout_ms,
    DROP COLUMN IF EXISTS max_retries,
    DROP COLUMN IF EXISTS metadata,
    DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE upstreams DROP CONSTRAINT IF EXISTS upstreams_project_name_unique;
ALTER TABLE upstreams ADD CONSTRAINT upstreams_name_key UNIQUE (name);


-- 6. DROP GLOBAL TABLES & TRIGGERS

DROP TRIGGER IF EXISTS trg_projects_updated_at ON projects;
DROP TABLE IF EXISTS project_members;
DROP TABLE IF EXISTS projects;


-- 7. REVERT ADMIN USERS

DROP TRIGGER IF EXISTS trg_admin_users_updated_at ON admin_users;
ALTER TABLE admin_users 
    DROP COLUMN IF EXISTS email,
    DROP COLUMN IF EXISTS is_active,
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS updated_at;


-- 8. DROP HELPER FUNCTIONS & TYPES

DROP FUNCTION IF EXISTS set_updated_at();

DROP TYPE IF EXISTS audit_action;
DROP TYPE IF EXISTS key_status;
DROP TYPE IF EXISTS policy_type;
DROP TYPE IF EXISTS route_status;
DROP TYPE IF EXISTS http_method;
DROP TYPE IF EXISTS member_role;
