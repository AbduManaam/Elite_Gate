DROP INDEX IF EXISTS idx_gateways_provisioning_queue;
DROP INDEX IF EXISTS idx_gateways_listener_rule_priority;

ALTER TABLE gateways
    DROP COLUMN IF EXISTS provisioned_at,
    DROP COLUMN IF EXISTS next_retry_at,
    DROP COLUMN IF EXISTS retry_count,
    DROP COLUMN IF EXISTS provisioning_error,
    DROP COLUMN IF EXISTS provisioning_status,
    DROP COLUMN IF EXISTS listener_rule_priority,
    DROP COLUMN IF EXISTS listener_rule_arn,
    DROP COLUMN IF EXISTS target_group_arn,
    DROP COLUMN IF EXISTS public_endpoint,
    DROP COLUMN IF EXISTS host_port;
