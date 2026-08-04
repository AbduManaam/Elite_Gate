ALTER TABLE gateways
    ADD COLUMN IF NOT EXISTS host_port INTEGER,
    ADD COLUMN IF NOT EXISTS public_endpoint TEXT,
    ADD COLUMN IF NOT EXISTS target_group_arn TEXT,
    ADD COLUMN IF NOT EXISTS listener_rule_arn TEXT,
    ADD COLUMN IF NOT EXISTS listener_rule_priority INTEGER,
    ADD COLUMN IF NOT EXISTS provisioning_status VARCHAR(50) NOT NULL DEFAULT 'not_started',
    ADD COLUMN IF NOT EXISTS provisioning_error TEXT,
    ADD COLUMN IF NOT EXISTS retry_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS next_retry_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS locked_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS locked_by TEXT,
    ADD COLUMN IF NOT EXISTS lease_token UUID,
    ADD COLUMN IF NOT EXISTS provisioned_at TIMESTAMPTZ;

UPDATE gateways
SET host_port = public_port::INTEGER
WHERE host_port IS NULL
  AND public_port ~ '^[0-9]+$'
  AND public_port::INTEGER BETWEEN 1 AND 65535;

CREATE UNIQUE INDEX IF NOT EXISTS idx_gateways_listener_rule_priority
    ON gateways(listener_rule_priority)
    WHERE listener_rule_priority IS NOT NULL
      AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_gateways_provisioning_queue
    ON gateways(next_retry_at, provisioning_status)
    WHERE deleted_at IS NULL
      AND provisioning_status IN (
          'container_ready',
          'creating_target_group',
          'registering_target',
          'waiting_for_target_health',
          'creating_listener_rule',
          'deprovisioning',
          'retrying'
      );
