ALTER TABLE custom_domains
    DROP COLUMN IF EXISTS routing_target,
    DROP COLUMN IF EXISTS routing_status,
    DROP COLUMN IF EXISTS routing_checked_at,
    DROP COLUMN IF EXISTS routing_error;
