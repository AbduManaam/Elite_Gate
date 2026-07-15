

CREATE INDEX IF NOT EXISTS idx_routes_project_id
    ON routes(project_id) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_upstreams_project_id
    ON upstreams(project_id) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_policies_project_id
    ON policies(project_id) WHERE deleted_at IS NULL;

-- Speeds up the 4-day audit log window used by the summary endpoint.
CREATE INDEX IF NOT EXISTS idx_audit_logs_project_created
    ON audit_logs(project_id, created_at DESC);
