CREATE TABLE project_jwt_configs (
    project_id UUID PRIMARY KEY
        REFERENCES projects(id)
        ON DELETE CASCADE,

    enabled BOOLEAN NOT NULL DEFAULT FALSE,

    algorithm TEXT NOT NULL DEFAULT 'HS256'
        CHECK (algorithm IN ('HS256')),

    -- The raw customer JWT secret is NEVER stored in PostgreSQL.
    -- Only the AWS Secrets Manager reference is stored here.
    secret_arn TEXT NOT NULL
        CHECK (BTRIM(secret_arn) <> ''),

    -- Exact AWS secret version currently used by the gateway.
    secret_version_id TEXT NOT NULL
        CHECK (BTRIM(secret_version_id) <> ''),

    -- Increment whenever any JWT configuration changes.
    -- Allows gateway runtimes to rebuild their verifier only when needed.
    config_version BIGINT NOT NULL DEFAULT 1
        CHECK (config_version > 0),

    -- Optional JWT claim validation.
    issuer TEXT,

    audiences TEXT[] NOT NULL DEFAULT '{}',

    -- Claim mappings.
    subject_claim TEXT NOT NULL DEFAULT 'sub',
    role_claim TEXT NOT NULL DEFAULT 'role',
    scopes_claim TEXT NOT NULL DEFAULT 'scope',

    -- Small allowance for clock differences between systems.
    clock_skew_seconds INTEGER NOT NULL DEFAULT 30
        CHECK (
            clock_skew_seconds >= 0
            AND clock_skew_seconds <= 300
        ),

    created_by UUID
        REFERENCES admin_users(id)
        ON DELETE SET NULL,

    updated_by UUID
        REFERENCES admin_users(id)
        ON DELETE SET NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CHECK (
        issuer IS NULL
        OR BTRIM(issuer) <> ''
    ),

    CHECK (BTRIM(subject_claim) <> ''),
    CHECK (BTRIM(role_claim) <> ''),
    CHECK (BTRIM(scopes_claim) <> '')
);


-- Reuse EliteGate's existing updated_at trigger function.
CREATE TRIGGER trg_project_jwt_configs_updated_at
    BEFORE UPDATE ON project_jwt_configs
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();


-- Tenant isolation.
ALTER TABLE project_jwt_configs
    ENABLE ROW LEVEL SECURITY;


CREATE POLICY project_jwt_configs_tenant_isolation
    ON project_jwt_configs
    USING (
        project_id = current_project_id()
    )
    WITH CHECK (
        project_id = current_project_id()
    );


-- Explicit permission for EliteGate's application DB role.
GRANT SELECT, INSERT, UPDATE, DELETE
    ON project_jwt_configs
    TO coreguard_app;