ALTER TABLE audit_logs
    ADD COLUMN IF NOT EXISTS entity_label TEXT,
    ADD COLUMN IF NOT EXISTS ip_address   INET,
    ADD COLUMN IF NOT EXISTS status       TEXT NOT NULL DEFAULT 'success';
