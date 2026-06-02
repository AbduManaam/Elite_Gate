
ALTER TABLE routes
    ADD COLUMN IF NOT EXISTS upstream_url    TEXT,
    ADD COLUMN IF NOT EXISTS protocol        TEXT NOT NULL DEFAULT 'http'
                                             CHECK (protocol IN ('http', 'grpc')),
    ADD COLUMN IF NOT EXISTS methods         TEXT[],
    ADD COLUMN IF NOT EXISTS auth_required   BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS rate_limit_rpm  INT     NOT NULL DEFAULT 0;
