-- Migration: 0018_dashboard_summary_indexes.up.sql
-- Supports the project dashboard summary endpoint's per-table COUNT queries.
-- routes/upstreams/policies previously only had project_id as the leading
-- column of a UNIQUE(project_id, name) constraint — usable, but a dedicated
-- partial index (excluding soft-deleted rows) is smaller and faster for
-- pure count/filter queries.

CREATE INDEX IF NOT EXISTS idx_routes_project_id
    ON routes(project_id) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_upstreams_project_id
    ON upstreams(project_id) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_policies_project_id
    ON policies(project_id) WHERE deleted_at IS NULL;

-- Speeds up the 4-day audit log window used by the summary endpoint.
CREATE INDEX IF NOT EXISTS idx_audit_logs_project_created
    ON audit_logs(project_id, created_at DESC);
