CREATE TABLE email_verification_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    admin_user_id UUID NOT NULL
        REFERENCES admin_users(id)
        ON DELETE CASCADE,

    token_hash TEXT NOT NULL UNIQUE,

    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_email_verification_tokens_admin_user_id
    ON email_verification_tokens(admin_user_id);

CREATE INDEX idx_email_verification_tokens_expires_at
    ON email_verification_tokens(expires_at);