-- Create database user/role for the app if it does not exist
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'coreguard_app') THEN
        CREATE ROLE coreguard_app WITH LOGIN PASSWORD 'coreguard_app_pass';
    END IF;
END $$;

-- Grant permissions on all existing tables and sequences
GRANT USAGE ON SCHEMA public TO coreguard_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO coreguard_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO coreguard_app;

-- Auto-grant permissions on future tables and sequences
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO coreguard_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO coreguard_app;
