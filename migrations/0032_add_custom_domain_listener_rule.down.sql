DROP INDEX IF EXISTS idx_custom_domains_listener_rule_priority;

ALTER TABLE custom_domains
    DROP CONSTRAINT IF EXISTS custom_domains_listener_rule_priority_range;

ALTER TABLE custom_domains
    DROP COLUMN IF EXISTS listener_rule_priority,
    DROP COLUMN IF EXISTS listener_rule_arn;
