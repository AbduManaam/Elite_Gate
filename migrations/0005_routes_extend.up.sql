ALTER TABLE routes
    ADD COLUMN IF NOT EXISTS upstream_id UUID REFERENCES upstreams(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS protocol TEXT NOT NULL DEFAULT 'http'
        CHECK (protocol IN ('http', 'grpc')),
    ADD COLUMN IF NOT EXISTS match_type TEXT NOT NULL DEFAULT 'prefix'
        CHECK (match_type IN ('exact', 'prefix')),
    ADD COLUMN IF NOT EXISTS enabled BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS auth_required BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS rate_limit_rpm INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

CREATE INDEX IF NOT EXISTS idx_routes_path ON routes(path);
CREATE INDEX IF NOT EXISTS idx_routes_enabled ON routes(enabled);


-- How incoming requests should be matched and handled

-- Client request path
--         ↓
-- Which backend should receive it?
--         ↓
-- What rules apply?

-- | Incoming Request | Route Match   | Backend              |
-- | ---------------- | ------------- | -------------------- |
-- | `/api/users/42`  | `/api/users`  | `http-user-service`  |
-- | `/api/orders/99` | `/api/orders` | `http-order-service` |
