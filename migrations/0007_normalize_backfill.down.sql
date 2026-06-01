-- ============================================================
-- Rollback for 0007: Clear all backfilled data
-- WARNING: This removes derived data only.
-- The original columns (methods[], auth_required, etc.) are
-- still present and contain the source of truth.
-- ============================================================

DELETE FROM route_methods;

UPDATE routes SET policy_id = NULL;

DELETE FROM policies;
