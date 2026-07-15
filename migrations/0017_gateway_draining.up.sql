-- Migration: 0017_gateway_draining.up.sql
ALTER TYPE gateway_status ADD VALUE IF NOT EXISTS 'draining';

ALTER TABLE gateways
    ADD COLUMN drain_started_at TIMESTAMPTZ;
