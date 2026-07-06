ALTER TABLE projects DROP CONSTRAINT IF EXISTS dashboard_allowed_origins_max_len;
ALTER TABLE projects DROP COLUMN IF EXISTS dashboard_allowed_origins;
