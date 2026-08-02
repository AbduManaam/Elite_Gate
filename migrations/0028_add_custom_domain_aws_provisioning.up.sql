-- Migration 0028: Add columns for automated AWS ACM provisioning, leases, and certificate ownership

-- Backfill existing NULL certificate_status values to 'not_requested' and set default
UPDATE custom_domains SET certificate_status = 'not_requested' WHERE certificate_status IS NULL;
ALTER TABLE custom_domains ALTER COLUMN certificate_status SET DEFAULT 'not_requested';

ALTER TABLE custom_domains
    ADD COLUMN certificate_managed_by_elitegate BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN provisioning_status VARCHAR(50) NOT NULL DEFAULT 'not_started',
    ADD COLUMN certificate_validation_name VARCHAR(253),
    ADD COLUMN certificate_validation_value TEXT,
    ADD COLUMN certificate_requested_at TIMESTAMPTZ,
    ADD COLUMN certificate_issued_at TIMESTAMPTZ,
    ADD COLUMN certificate_attached_at TIMESTAMPTZ,
    ADD COLUMN provisioning_started_at TIMESTAMPTZ,
    ADD COLUMN provisioning_completed_at TIMESTAMPTZ,
    ADD COLUMN deprovisioning_started_at TIMESTAMPTZ,
    ADD COLUMN provisioning_error TEXT,
    ADD COLUMN provisioning_attempts INT NOT NULL DEFAULT 0,
    ADD COLUMN next_retry_at TIMESTAMPTZ,
    ADD COLUMN locked_at TIMESTAMPTZ,
    ADD COLUMN locked_by VARCHAR(100),
    ADD COLUMN lease_token UUID;

CREATE INDEX idx_custom_domains_provisioning_queue
    ON custom_domains (next_retry_at, provisioning_status)
    WHERE deleted_at IS NULL AND provisioning_status IN (
        'requesting_certificate',
        'waiting_for_validation_record',
        'waiting_for_dns',
        'waiting_for_certificate',
        'attaching_certificate',
        'deprovisioning'
    );
