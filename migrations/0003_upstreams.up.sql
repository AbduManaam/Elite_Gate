CREATE TABLE upstreams (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name        TEXT NOT NULL UNIQUE,
    target_url  TEXT NOT NULL,
    protocol    TEXT NOT NULL CHECK (protocol IN ('http', 'grpc')),
    health_path TEXT,
    enabled     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_upstreams_name ON upstreams(name);
CREATE INDEX idx_upstreams_enabled ON upstreams(enabled);