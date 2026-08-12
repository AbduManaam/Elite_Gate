
CREATE TABLE email_verification_token (
     id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

     admin_user_id UUID NOT NULL
         REFERENCES admin_users(id) ON DELETE CASCADE,

         token_hash TEXT NOT NULL UNIQUE,
         expires_at TIMESTAMPZ NOT NULL,
         user_at TIMESTAMPZ NULL,

         created_at TIMESTAMPZ NOT NULL DEFAULT NOW()

);

CREATE INDEX inx_email_verification_token_admin_user_id
ON email_verification_token(admin_user_id);

CREATE INDEX inx_email_verification_token_expires_at
ON email_verification_token(expires_at);

CREATE INDEX inx_email_verification_token_token_hash
ON email_verification_token(token_hash);