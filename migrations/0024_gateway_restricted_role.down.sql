-- Migration: 0023_gateway_restricted_role.down.sql
REVOKE ALL ON ALL TABLES IN SCHEMA public FROM elitegate_gateway_user;
REVOKE ALL ON SCHEMA public FROM elitegate_gateway_user;
DO $$
BEGIN
    EXECUTE format('REVOKE CONNECT ON DATABASE %I FROM elitegate_gateway_user', current_database());
END $$;
DROP ROLE IF EXISTS elitegate_gateway_user;
