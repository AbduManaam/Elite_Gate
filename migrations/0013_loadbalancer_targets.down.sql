DROP TRIGGER IF EXISTS trg_upstream_targets_updated_at ON upstream_targets;
DROP TABLE IF EXISTS upstream_targets;

ALTER TABLE upstreams DROP COLUMN IF EXISTS lb_strategy;

DROP TYPE IF EXISTS lb_strategy;