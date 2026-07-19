-- Migration: 0023_gateway_restricted_role.down.sql
REVOKE ALL ON ALL TABLES IN SCHEMA public FROM elitegate_gateway_user;
REVOKE ALL ON SCHEMA public FROM elitegate_gateway_user;
REVOKE CONNECT ON DATABASE elitegate FROM elitegate_gateway_user;
DROP ROLE IF EXISTS elitegate_gateway_user;
