ALTER TABLE audit_logs
    DROP COLUMN IF EXISTS entity_label,
    DROP COLUMN IF EXISTS ip_address,
    DROP COLUMN IF EXISTS status;

