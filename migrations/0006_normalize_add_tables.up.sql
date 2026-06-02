
CREATE TABLE IF NOT EXISTS policies (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name           TEXT        UNIQUE NOT NULL,
    auth_required  BOOLEAN     NOT NULL DEFAULT TRUE,
    rate_limit_rpm INT         NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);


CREATE TABLE IF NOT EXISTS route_methods (
    route_id UUID NOT NULL REFERENCES routes(id) ON DELETE CASCADE,
    method   TEXT NOT NULL CHECK (method IN ('GET','POST','PUT','DELETE','PATCH','HEAD','OPTIONS')),
    PRIMARY KEY (route_id, method)
);

ALTER TABLE routes
    ADD COLUMN IF NOT EXISTS policy_id UUID REFERENCES policies(id);

ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS admin_user_id UUID REFERENCES admin_users(id) ON DELETE SET NULL;
