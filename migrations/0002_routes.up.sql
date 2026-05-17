CREATE TABLE routes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    path TEXT NOT NULL,
    upstream_url TEXT NOT NULL,
    methods TEXT[] NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);