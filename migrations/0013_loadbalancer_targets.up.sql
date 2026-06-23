DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'lb_strategy') THEN
        CREATE TYPE lb_strategy AS ENUM ('round_robin', 'least_conn');
    END IF;
END $$;

ALTER TABLE upstreams
    ADD COLUMN IF NOT EXISTS lb_strategy lb_strategy NOT NULL DEFAULT 'round_robin';

-- 2. Target pool table
CREATE TABLE IF NOT EXISTS upstream_targets (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    upstream_id UUID        NOT NULL REFERENCES upstreams(id) ON DELETE CASCADE,
    target_url  TEXT        NOT NULL,
    weight      INT         NOT NULL DEFAULT 1 CHECK (weight > 0),
    enabled     BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ,
    CONSTRAINT upstream_targets_unique UNIQUE (upstream_id, target_url)
);

CREATE INDEX IF NOT EXISTS idx_upstream_targets_upstream_id
    ON upstream_targets(upstream_id) WHERE deleted_at IS NULL;

-- 3. Backfill:-- Migrate/Backfill existing target_url values into upstream_targets.

INSERT INTO upstream_targets (upstream_id, target_url, weight, enabled)
SELECT id, target_url, 1, enabled
FROM   upstreams
WHERE  deleted_at IS NULL
ON CONFLICT (upstream_id, target_url) DO NOTHING;

-- Auto-manage updated_at, reusing the helper from migration 0009.
DROP TRIGGER IF EXISTS trg_upstream_targets_updated_at ON upstream_targets;
CREATE TRIGGER trg_upstream_targets_updated_at
    BEFORE UPDATE ON upstream_targets
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
