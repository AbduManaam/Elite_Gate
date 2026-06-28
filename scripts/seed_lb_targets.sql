-- ── Step 1: Enable round_robin on both upstreams ─────────────────────────────
UPDATE upstreams
SET    lb_strategy = 'round_robin'
WHERE  name IN ('http-user-service', 'http-order-service')
  AND  deleted_at IS NULL;

-- ── It replaces the old single backend (user-service) with three new backend instances (user-service-1, 2, 3) in EliteGate's load-balancing pool.
DELETE FROM upstream_targets
WHERE  target_url = 'http://user-service:9001'
  AND  upstream_id = (
      SELECT id FROM upstreams
      WHERE  name = 'http-user-service' AND deleted_at IS NULL
  );  

INSERT INTO upstream_targets (upstream_id, target_url, weight, enabled)
SELECT u.id, 'http://user-service-1:9001', 1, TRUE
FROM   upstreams u
WHERE  u.name = 'http-user-service' AND u.deleted_at IS NULL
ON CONFLICT (upstream_id, target_url) DO NOTHING;

INSERT INTO upstream_targets (upstream_id, target_url, weight, enabled)
SELECT u.id, 'http://user-service-2:9001', 1, TRUE
FROM   upstreams u
WHERE  u.name = 'http-user-service' AND u.deleted_at IS NULL
ON CONFLICT (upstream_id, target_url) DO NOTHING;

INSERT INTO upstream_targets (upstream_id, target_url, weight, enabled)
SELECT u.id, 'http://user-service-3:9001', 1, TRUE
FROM   upstreams u
WHERE  u.name = 'http-user-service' AND u.deleted_at IS NULL
ON CONFLICT (upstream_id, target_url) DO NOTHING;  

-- ── Step 3: Register order-service instances ──────────────────────────────────
DELETE FROM upstream_targets
WHERE  target_url = 'http://order-service:9002'
  AND  upstream_id = (
      SELECT id FROM upstreams
      WHERE  name = 'http-order-service' AND deleted_at IS NULL
  );

INSERT INTO upstream_targets (upstream_id, target_url, weight, enabled)
SELECT u.id, 'http://order-service-1:9002', 1, TRUE
FROM   upstreams u
WHERE  u.name = 'http-order-service' AND u.deleted_at IS NULL
ON CONFLICT (upstream_id, target_url) DO NOTHING;

INSERT INTO upstream_targets (upstream_id, target_url, weight, enabled)
SELECT u.id, 'http://order-service-2:9002', 1, TRUE
FROM   upstreams u
WHERE  u.name = 'http-order-service' AND u.deleted_at IS NULL
ON CONFLICT (upstream_id, target_url) DO NOTHING;

INSERT INTO upstream_targets (upstream_id, target_url, weight, enabled)
SELECT u.id, 'http://order-service-3:9002', 1, TRUE
FROM   upstreams u
WHERE  u.name = 'http-order-service' AND u.deleted_at IS NULL
ON CONFLICT (upstream_id, target_url) DO NOTHING;


-- ── Verify ────────────────────────────────────────────────────────────────────
-- This query checks that the load-balancing targets were added correctly and that both upstreams are using the round_robin strategy with three service instances each.
SELECT
    u.name        AS upstream,
    u.lb_strategy,
    ut.target_url,
    ut.weight,
    ut.enabled
FROM   upstream_targets ut
JOIN   upstreams u ON u.id = ut.upstream_id
WHERE  u.name IN ('http-user-service', 'http-order-service')
  AND  ut.deleted_at IS NULL
ORDER  BY u.name, ut.created_at;