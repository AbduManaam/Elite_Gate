-- Migration: 0009_multi_tenant_rls.up.sql
-- Strategy: Option 1 — Shared DB, Row-Level Isolation (Deduplicated Safe Upgrade)

CREATE EXTENSION IF NOT EXISTS "citext";

-- ============================================================
-- 1. ENUMS (Create only if they don't exist)
-- ============================================================
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'member_role') THEN
        CREATE TYPE member_role AS ENUM ('owner', 'editor', 'viewer');
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'http_method') THEN
        CREATE TYPE http_method AS ENUM ('GET','POST','PUT','PATCH','DELETE','HEAD','OPTIONS','ANY');
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'route_status') THEN
        CREATE TYPE route_status AS ENUM ('active','inactive','draft');
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'policy_type') THEN
        CREATE TYPE policy_type AS ENUM ('rate_limit','auth','cors','ip_whitelist','transform');
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'key_status') THEN
        CREATE TYPE key_status AS ENUM ('active','revoked','expired');
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'audit_action') THEN
        CREATE TYPE audit_action AS ENUM (
            'route.create','route.update','route.delete',
            'upstream.create','upstream.update','upstream.delete',
            'policy.create','policy.update','policy.delete',
            'api_key.create','api_key.revoke',
            'member.invite','member.remove','member.role_change',
            'project.create','project.update','project.delete',
            'consumer.create','consumer.delete'
        );
    END IF;
END $$;

-- ============================================================
-- 2. PROJECTS & MEMBERS TABLES
-- ============================================================
CREATE TABLE IF NOT EXISTS projects (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT        NOT NULL,
    slug            TEXT        NOT NULL,
    description     TEXT,
    owner_id        UUID        NOT NULL REFERENCES admin_users(id) ON DELETE RESTRICT,
    is_active       BOOLEAN     NOT NULL DEFAULT TRUE,
    plan            TEXT        NOT NULL DEFAULT 'free',
    metadata        JSONB       NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ,
    CONSTRAINT projects_slug_unique UNIQUE (slug),
    CONSTRAINT projects_slug_format CHECK (slug ~ '^[a-z0-9][a-z0-9\-]{1,62}[a-z0-9]$')
);

CREATE TABLE IF NOT EXISTS project_members (
    project_id      UUID        NOT NULL REFERENCES projects(id)     ON DELETE CASCADE,
    admin_user_id   UUID        NOT NULL REFERENCES admin_users(id)  ON DELETE CASCADE,
    role            member_role NOT NULL DEFAULT 'viewer',
    invited_by      UUID        REFERENCES admin_users(id)           ON DELETE SET NULL,
    joined_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (project_id, admin_user_id)
);

-- ============================================================
-- 3. UPGRADE & BACKFILL ADMIN USERS
-- ============================================================
ALTER TABLE admin_users
   ADD COLUMN IF NOT EXISTS email CITEXT,
   ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT TRUE,
   ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
   ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

-- Backfill admin emails if null
UPDATE admin_users SET email = username || '@elitegate.local' WHERE email IS NULL;
ALTER TABLE admin_users ALTER COLUMN email SET NOT NULL;

-- Add constraints
ALTER TABLE admin_users DROP CONSTRAINT IF EXISTS admin_users_email_unique;
ALTER TABLE admin_users ADD CONSTRAINT admin_users_email_unique UNIQUE (email);
ALTER TABLE admin_users DROP CONSTRAINT IF EXISTS admin_users_email_format;
ALTER TABLE admin_users ADD CONSTRAINT admin_users_email_format CHECK (email ~* '^[^@]+@[^@]+\.[^@]+$');

-- Ensure updated_at trigger helper exists
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$;

-- Create triggers to auto-manage updated_at for global tables
DROP TRIGGER IF EXISTS trg_admin_users_updated_at ON admin_users;
CREATE TRIGGER trg_admin_users_updated_at BEFORE UPDATE ON admin_users FOR EACH ROW EXECUTE FUNCTION set_updated_at();

DROP TRIGGER IF EXISTS trg_projects_updated_at ON projects;
CREATE TRIGGER trg_projects_updated_at BEFORE UPDATE ON projects FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ============================================================
-- 4. CREATE DEFAULT PROJECT AND MEMBER BACKFILL
-- ============================================================
INSERT INTO projects (id, name, slug, owner_id)
SELECT '00000000-0000-0000-0000-000000000000', 'Default Project', 'default', id
FROM admin_users
LIMIT 1
ON CONFLICT DO NOTHING;

INSERT INTO project_members (project_id, admin_user_id, role)
SELECT '00000000-0000-0000-0000-000000000000', id, 'owner'
FROM admin_users
ON CONFLICT DO NOTHING;

-- ============================================================
-- 5. UPGRADE TENANT-SCOPED TABLES
-- ============================================================

-- Upstreams
ALTER TABLE upstreams 
    ADD COLUMN IF NOT EXISTS project_id UUID REFERENCES projects(id) ON DELETE CASCADE,
    ADD COLUMN IF NOT EXISTS connect_timeout_ms INT NOT NULL DEFAULT 5000,
    ADD COLUMN IF NOT EXISTS read_timeout_ms INT NOT NULL DEFAULT 30000,
    ADD COLUMN IF NOT EXISTS max_retries INT NOT NULL DEFAULT 2,
    ADD COLUMN IF NOT EXISTS metadata JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

UPDATE upstreams SET project_id = '00000000-0000-0000-0000-000000000000' WHERE project_id IS NULL;
ALTER TABLE upstreams ALTER COLUMN project_id SET NOT NULL;
ALTER TABLE upstreams DROP CONSTRAINT IF EXISTS upstreams_name_key;
ALTER TABLE upstreams ADD CONSTRAINT upstreams_project_name_unique UNIQUE (project_id, name);

-- Policies
ALTER TABLE policies 
    ADD COLUMN IF NOT EXISTS project_id UUID REFERENCES projects(id) ON DELETE CASCADE,
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

UPDATE policies SET project_id = '00000000-0000-0000-0000-000000000000' WHERE project_id IS NULL;
ALTER TABLE policies ALTER COLUMN project_id SET NOT NULL;
ALTER TABLE policies DROP CONSTRAINT IF EXISTS policies_name_key;
ALTER TABLE policies ADD CONSTRAINT policies_project_name_unique UNIQUE (project_id, name);

-- Routes
ALTER TABLE routes 
    ADD COLUMN IF NOT EXISTS project_id UUID REFERENCES projects(id) ON DELETE CASCADE,
    ADD COLUMN IF NOT EXISTS name TEXT,
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

-- Backfill route names using short IDs and set default project
UPDATE routes SET name = 'route_' || substring(id::text, 1, 8) WHERE name IS NULL;
UPDATE routes SET project_id = '00000000-0000-0000-0000-000000000000' WHERE project_id IS NULL;
ALTER TABLE routes ALTER COLUMN project_id SET NOT NULL, ALTER COLUMN name SET NOT NULL;
ALTER TABLE routes ADD CONSTRAINT routes_project_name_unique UNIQUE (project_id, name);

-- ============================================================
-- 6. FLATTEN ROUTE METHODS
-- ============================================================
ALTER TABLE routes ADD COLUMN IF NOT EXISTS methods http_method[] NOT NULL DEFAULT '{ANY}';

UPDATE routes r
SET methods = COALESCE(
    (SELECT array_agg(method::http_method)
     FROM route_methods rm
     WHERE rm.route_id = r.id),
    '{ANY}'::http_method[]
);

DROP TABLE IF EXISTS route_methods;

-- ============================================================
-- 7. ROW-LEVEL SECURITY
-- ============================================================
ALTER TABLE routes ENABLE ROW LEVEL SECURITY;
ALTER TABLE upstreams ENABLE ROW LEVEL SECURITY;
ALTER TABLE policies ENABLE ROW LEVEL SECURITY;

CREATE OR REPLACE FUNCTION current_project_id() RETURNS UUID
LANGUAGE sql STABLE AS $$
    SELECT NULLIF(current_setting('app.project_id', TRUE), '')::uuid;
$$;

-- Isolation Policies
DROP POLICY IF EXISTS routes_tenant_isolation ON routes;
CREATE POLICY routes_tenant_isolation ON routes
    USING      (project_id = current_project_id())
    WITH CHECK (project_id = current_project_id());

DROP POLICY IF EXISTS upstreams_tenant_isolation ON upstreams;
CREATE POLICY upstreams_tenant_isolation ON upstreams
    USING      (project_id = current_project_id())
    WITH CHECK (project_id = current_project_id());

DROP POLICY IF EXISTS policies_tenant_isolation ON policies;
CREATE POLICY policies_tenant_isolation ON policies
    USING      (project_id = current_project_id())
    WITH CHECK (project_id = current_project_id());
