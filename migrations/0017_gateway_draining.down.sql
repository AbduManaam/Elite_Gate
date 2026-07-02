-- Migration: 0017_gateway_draining.down.sql
-- PostgreSQL does not support dropping enum values — this part of the
-- rollback remains a no-op.
ALTER TABLE gateways
    DROP COLUMN IF EXISTS drain_started_at;
