-- Migration 0029 cannot be safely rolled back.
-- It repairs production data by resuming failed ACM provisioning.
-- The previous state cannot be reconstructed automatically.

DO $$
BEGIN
    RAISE EXCEPTION 'Migration 0029 is irreversible.';
END $$;