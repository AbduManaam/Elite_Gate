create extention if not exists "uuid-ossp";

create table admin_users(
    id uuid primary key  default uuid_generate_v4(),
    username text unique not null,
    password_hash text not null,
    failed_attempts int not null default 0,
    locked_until timestamp,
    created_at  timestamp not null default now(),
);

create table refresh_tokens(
    id uuid primary key default uuid_generate_v4(),
    admin_id uuid not null references admin_users(id),
    token_hash TEXT NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    revoked BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);