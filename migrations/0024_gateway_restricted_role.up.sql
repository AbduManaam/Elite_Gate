-- Migration: 0024_gateway_restricted_role.up.sql
-- Restricted, read-only role for tenant gateway containers.
-- Isolation is enforced by RLS (see 0009/0015) PLUS this role's limited
-- grants — this migration alone is not sufficient without the application
-- reliably setting `app.project_id` on every connection.

DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'elitegate_gateway_user') THEN
        CREATE ROLE elitegate_gateway_user WITH LOGIN PASSWORD 'gateway_restricted_pass';
    END IF;
END $$;

GRANT CONNECT ON DATABASE elitegate_db TO elitegate_gateway_user;
GRANT USAGE ON SCHEMA public TO elitegate_gateway_user;

-- Read-only, and only on tables the data plane actually needs.
GRANT SELECT ON routes, upstreams, upstream_targets, policies, api_keys, projects, gateways
    TO elitegate_gateway_user;

-- Explicit deny, defense-in-depth even though these were never granted above.
REVOKE ALL ON admin_users, refresh_tokens, audit_logs, project_members
    FROM elitegate_gateway_user;

-- Belt-and-braces: force RLS even for table owner semantics on this role.
ALTER TABLE routes            FORCE ROW LEVEL SECURITY;
ALTER TABLE upstreams         FORCE ROW LEVEL SECURITY;
ALTER TABLE policies          FORCE ROW LEVEL SECURITY;
ALTER TABLE api_keys          FORCE ROW LEVEL SECURITY;
