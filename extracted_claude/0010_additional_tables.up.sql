-- =============================================================================
-- Migration 0010: Additional Tables (api_keys, gateways, audit_logs)
-- =============================================================================

-- -----------------------------------------------------------------------------
-- 0. Create ENUMs first (safe to re-run)
-- -----------------------------------------------------------------------------

DO $$ BEGIN
    CREATE TYPE gateway_plan AS ENUM ('shared', 'dedicated');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TYPE gateway_status AS ENUM ('provisioning', 'active', 'failed', 'decommissioned');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- key_status enum may already exist from a prior migration; guard it too
DO $$ BEGIN
    CREATE TYPE key_status AS ENUM ('active', 'revoked', 'expired');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- audit_action enum may already exist; guard it too
DO $$ BEGIN
    CREATE TYPE audit_action AS ENUM (
        'created', 'updated', 'deleted',
        'enabled', 'disabled',
        'rotated', 'revoked',
        'assigned', 'removed',
        'provisioned', 'decommissioned',
        'login', 'logout'
    );
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;


-- -----------------------------------------------------------------------------
-- 1. Enhance api_keys with project scoping
-- -----------------------------------------------------------------------------

ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS project_id  UUID        REFERENCES projects(id) ON DELETE CASCADE,
    ADD COLUMN IF NOT EXISTS name        TEXT,
    ADD COLUMN IF NOT EXISTS status      key_status  NOT NULL DEFAULT 'active',
    ADD COLUMN IF NOT EXISTS expires_at  TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ADD COLUMN IF NOT EXISTS deleted_at  TIMESTAMPTZ;

-- Backfill: tie existing rows to the default project before enforcing NOT NULL
UPDATE api_keys
SET project_id = '00000000-0000-0000-0000-000000000000'
WHERE project_id IS NULL;

UPDATE api_keys
SET name = 'seeded-key'
WHERE name IS NULL;

-- Now enforce NOT NULL so future inserts cannot omit project_id
ALTER TABLE api_keys
    ALTER COLUMN project_id SET NOT NULL,
    ALTER COLUMN name        SET NOT NULL;

-- Indexes for common lookup patterns
CREATE INDEX IF NOT EXISTS idx_api_keys_project_id ON api_keys(project_id);
CREATE INDEX IF NOT EXISTS idx_api_keys_key_hash   ON api_keys(key_hash);
CREATE INDEX IF NOT EXISTS idx_api_keys_status     ON api_keys(status) WHERE deleted_at IS NULL;

-- RLS: each session can only see/write keys belonging to its project
ALTER TABLE api_keys ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS api_keys_tenant_isolation ON api_keys;
CREATE POLICY api_keys_tenant_isolation ON api_keys
    USING      (project_id = current_project_id())
    WITH CHECK (project_id = current_project_id());

-- Super-user / service-role bypass so FindByHash (auth path) still works
-- without a tenant context.  Grant this only to the service DB role.
-- ALTER TABLE api_keys FORCE ROW LEVEL SECURITY;  -- uncomment to also apply to table owner


-- -----------------------------------------------------------------------------
-- 2. Create gateways table
-- -----------------------------------------------------------------------------
-- NOTE: id is UUID (consistent with every other table).
--       The Go layer generates a human-readable label (e.g. "gw_abc123") and
--       stores it in external_id for display; the UUID is the real PK.
-- -----------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS gateways (
    id           UUID           PRIMARY KEY DEFAULT gen_random_uuid(),
    external_id  TEXT           NOT NULL UNIQUE,          -- e.g. "gw_a1b2c3d4"
    project_id   UUID           NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    endpoint_ip  TEXT           NOT NULL DEFAULT '0.0.0.0',
    gateway_port TEXT           NOT NULL DEFAULT '8080',
    plan         gateway_plan   NOT NULL DEFAULT 'shared',
    status       gateway_status NOT NULL DEFAULT 'provisioning',
    created_at   TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    deleted_at   TIMESTAMPTZ                              -- soft-delete
);

CREATE INDEX IF NOT EXISTS idx_gateways_project_id  ON gateways(project_id);
CREATE INDEX IF NOT EXISTS idx_gateways_status      ON gateways(status) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_gateways_external_id ON gateways(external_id);

ALTER TABLE gateways ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS gateways_tenant_isolation ON gateways;
CREATE POLICY gateways_tenant_isolation ON gateways
    USING      (project_id = current_project_id())
    WITH CHECK (project_id = current_project_id());


-- -----------------------------------------------------------------------------
-- 3. Create audit_logs table
-- -----------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS audit_logs (
    id            UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id    UUID         NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    admin_user_id UUID         REFERENCES admin_users(id) ON DELETE SET NULL,
    action        audit_action NOT NULL,
    entity_type   TEXT         NOT NULL,   -- e.g. 'route', 'upstream', 'api_key'
    entity_id     TEXT         NOT NULL,   -- UUID or external_id of the changed entity
    changes       JSONB        NOT NULL DEFAULT '{}',
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
    -- audit_logs are append-only: no updated_at / deleted_at needed
);

-- audit_logs grow large; indexes are critical for dashboard queries
CREATE INDEX IF NOT EXISTS idx_audit_logs_project_id   ON audit_logs(project_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at   ON audit_logs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_entity       ON audit_logs(entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_admin_user   ON audit_logs(admin_user_id) WHERE admin_user_id IS NOT NULL;

ALTER TABLE audit_logs ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS audit_logs_tenant_isolation ON audit_logs;
CREATE POLICY audit_logs_tenant_isolation ON audit_logs
    USING      (project_id = current_project_id())
    WITH CHECK (project_id = current_project_id());


-- -----------------------------------------------------------------------------
-- 4. Seed the test-key for local development
--    Raw key  : "test-key"
--    SHA-256  : 62af8704764faf8ea82fc61ce9c4c3908b6cb97d463a634e9e587d7c885db0ef
--    Project  : 00000000-0000-0000-0000-000000000000  (default / shared project)
-- -----------------------------------------------------------------------------

INSERT INTO api_keys (id, project_id, name, key_hash, status)
VALUES (
    '11111111-1111-1111-1111-111111111111',
    '00000000-0000-0000-0000-000000000000',
    'test-key-local',
    '62af8704764faf8ea82fc61ce9c4c3908b6cb97d463a634e9e587d7c885db0ef',
    'active'
)
ON CONFLICT DO NOTHING;
