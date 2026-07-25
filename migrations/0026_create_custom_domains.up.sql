CREATE TABLE custom_domains (
    id UUID PRIMARY KEY,

    project_id UUID NOT NULL
        REFERENCES projects(id)
        ON DELETE CASCADE,

    hostname VARCHAR(253) NOT NULL,

    status VARCHAR(40) NOT NULL
        DEFAULT 'pending_verification',

    verification_token_hash TEXT NOT NULL,

    verification_record_name VARCHAR(253) NOT NULL,

    certificate_arn TEXT,

    certificate_status VARCHAR(40),

    failure_reason TEXT,

    verified_at TIMESTAMPTZ,

    activated_at TIMESTAMPTZ,

    last_checked_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX custom_domains_hostname_unique
    ON custom_domains (LOWER(hostname))
    WHERE deleted_at IS NULL;

CREATE INDEX custom_domains_project_id_index
    ON custom_domains (project_id);

CREATE INDEX custom_domains_status_index
    ON custom_domains (status);
