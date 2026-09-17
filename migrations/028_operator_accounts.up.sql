CREATE TABLE admin_accounts (
    username text PRIMARY KEY,
    password_hash text NOT NULL,
    role text NOT NULL CHECK (role IN ('owner', 'admin')),
    disabled boolean NOT NULL DEFAULT false,
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE admin_sessions (
    token_hash text PRIMARY KEY,
    username text NOT NULL REFERENCES admin_accounts(username),
    account_version bigint NOT NULL,
    expires_at timestamptz NOT NULL
);
CREATE INDEX admin_sessions_expiry ON admin_sessions(expires_at);
CREATE TABLE admin_service_tokens (
    name text PRIMARY KEY,
    token_hash text NOT NULL UNIQUE,
    permissions text[] NOT NULL,
    namespaces text[] NOT NULL,
    revoked boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE admin_login_buckets (
    bucket text PRIMARY KEY,
    attempts integer NOT NULL,
    expires_at timestamptz NOT NULL
);
CREATE TABLE admin_audit (
    id bigserial PRIMARY KEY,
    actor text NOT NULL,
    action text NOT NULL,
    target text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE namespace_provisions (
    namespace text PRIMARY KEY REFERENCES namespace_configs(namespace) ON DELETE CASCADE,
    desired_hash text NOT NULL
);
