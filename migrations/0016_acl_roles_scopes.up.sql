ALTER TABLE policies
    ADD COLUMN IF NOT EXISTS allowed_roles   TEXT[],
    ADD COLUMN IF NOT EXISTS allowed_scopes  TEXT[];

ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS roles   TEXT[],
    ADD COLUMN IF NOT EXISTS scopes  TEXT[];

-- GIN indexes for fast array-contains (@>) queries
CREATE INDEX IF NOT EXISTS idx_policies_allowed_roles  ON policies  USING GIN (allowed_roles);
CREATE INDEX IF NOT EXISTS idx_policies_allowed_scopes ON policies  USING GIN (allowed_scopes);
CREATE INDEX IF NOT EXISTS idx_api_keys_roles          ON api_keys  USING GIN (roles);
CREATE INDEX IF NOT EXISTS idx_api_keys_scopes         ON api_keys  USING GIN (scopes);
