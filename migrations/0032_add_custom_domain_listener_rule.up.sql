ALTER TABLE custom_domains
    ADD COLUMN listener_rule_arn TEXT,
    ADD COLUMN listener_rule_priority INTEGER;

ALTER TABLE custom_domains
    ADD CONSTRAINT custom_domains_listener_rule_priority_range
    CHECK (
        listener_rule_priority IS NULL
        OR (
            listener_rule_priority >= 40001
            AND listener_rule_priority <= 50000
        )
    );

CREATE UNIQUE INDEX idx_custom_domains_listener_rule_priority
    ON custom_domains(listener_rule_priority)
    WHERE listener_rule_priority IS NOT NULL
      AND deleted_at IS NULL;

-- Backfill active domains missing host rule
UPDATE custom_domains
SET
    provisioning_status = 'attaching_certificate',
    next_retry_at = NOW(),
    provisioning_error = NULL,
    locked_at = NULL,
    locked_by = NULL,
    lease_token = NULL,
    updated_at = NOW()
WHERE deleted_at IS NULL
  AND status = 'active'
  AND provisioning_status = 'completed'
  AND certificate_managed_by_elitegate = true
  AND certificate_arn IS NOT NULL
  AND listener_rule_arn IS NULL;
