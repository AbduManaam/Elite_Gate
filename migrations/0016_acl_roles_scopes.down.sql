DROP INDEX IF EXISTS idx_policies_allowed_roles;
DROP INDEX IF EXISTS idx_policies_allowed_scopes;
DROP INDEX IF EXISTS idx_api_keys_roles;
DROP INDEX IF EXISTS idx_api_keys_scopes;

ALTER TABLE policies
    DROP COLUMN IF EXISTS allowed_roles,
    DROP COLUMN IF EXISTS allowed_scopes;

ALTER TABLE api_keys
    DROP COLUMN IF EXISTS roles,
    DROP COLUMN IF EXISTS scopes;
