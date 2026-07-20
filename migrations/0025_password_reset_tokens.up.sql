CREATE TABLE password_reset_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    admin_user_id UUID NOT NULL
        REFERENCES admin_users(id)
        ON DELETE CASCADE,

    token_hash TEXT NOT NULL UNIQUE,

    expires_at TIMESTAMPTZ NOT NULL,

    used_at TIMESTAMPTZ NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_password_reset_tokens_admin_user_id
    ON password_reset_tokens(admin_user_id);

CREATE INDEX idx_password_reset_tokens_expires_at
    ON password_reset_tokens(expires_at);

-- Unique index guaranteeing at most one active (unused) reset token per user
CREATE UNIQUE INDEX uq_password_reset_tokens_active_user
    ON password_reset_tokens(admin_user_id)
    WHERE used_at IS NULL;
