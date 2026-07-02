-- Migration: 0017_gateway_draining.up.sql
ALTER TYPE gateway_status ADD VALUE IF NOT EXISTS 'draining';

-- Tracks when a gateway entered the draining state, independent of
-- updated_at (which other transitions also touch). Used by the handler
-- to compute the *remaining* wait on a retried request, and by the
-- worker reconciler to detect gateways stuck draining too long.
ALTER TABLE gateways
    ADD COLUMN drain_started_at TIMESTAMPTZ;
