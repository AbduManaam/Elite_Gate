-- Rollback migration 0028
DROP INDEX IF EXISTS idx_custom_domains_provisioning_queue;

ALTER TABLE custom_domains ALTER COLUMN certificate_status DROP DEFAULT;

ALTER TABLE custom_domains
    DROP COLUMN IF EXISTS lease_token,
    DROP COLUMN IF EXISTS locked_by,
    DROP COLUMN IF EXISTS locked_at,
    DROP COLUMN IF EXISTS next_retry_at,
    DROP COLUMN IF EXISTS provisioning_attempts,
    DROP COLUMN IF EXISTS provisioning_error,
    DROP COLUMN IF EXISTS deprovisioning_started_at,
    DROP COLUMN IF EXISTS provisioning_completed_at,
    DROP COLUMN IF EXISTS provisioning_started_at,
    DROP COLUMN IF EXISTS certificate_attached_at,
    DROP COLUMN IF EXISTS certificate_issued_at,
    DROP COLUMN IF EXISTS certificate_requested_at,
    DROP COLUMN IF EXISTS certificate_validation_value,
    DROP COLUMN IF EXISTS certificate_validation_name,
    DROP COLUMN IF EXISTS provisioning_status,
    DROP COLUMN IF EXISTS certificate_managed_by_elitegate;
