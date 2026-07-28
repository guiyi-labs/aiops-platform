CREATE TABLE IF NOT EXISTS roles (
    id BIGSERIAL PRIMARY KEY,
    code VARCHAR(64) NOT NULL UNIQUE,
    name VARCHAR(128) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(64) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    display_name VARCHAR(128) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    last_login_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT users_status_check CHECK (status IN ('active', 'disabled'))
);

CREATE TABLE IF NOT EXISTS user_roles (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id BIGINT NOT NULL REFERENCES roles(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, role_id)
);

CREATE TABLE IF NOT EXISTS clusters (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(128) NOT NULL UNIQUE,
    api_server VARCHAR(512) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'disabled',
    kubernetes_version VARCHAR(64),
    last_probed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT clusters_status_check CHECK (status IN ('enabled', 'disabled', 'unreachable'))
);

CREATE TABLE IF NOT EXISTS cluster_credentials (
    cluster_id BIGINT PRIMARY KEY REFERENCES clusters(id) ON DELETE CASCADE,
    encrypted_kubeconfig BYTEA NOT NULL,
    encryption_key_version VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id BIGSERIAL PRIMARY KEY,
    actor_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    cluster_id BIGINT REFERENCES clusters(id) ON DELETE SET NULL,
    action VARCHAR(128) NOT NULL,
    resource_type VARCHAR(128),
    resource_namespace VARCHAR(253),
    resource_name VARCHAR(253),
    result VARCHAR(32) NOT NULL,
    request_id VARCHAR(128) NOT NULL,
    details JSONB NOT NULL DEFAULT '{}'::JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT audit_logs_result_check CHECK (result IN ('success', 'failure', 'denied'))
);

CREATE INDEX IF NOT EXISTS audit_logs_created_at_idx ON audit_logs (created_at DESC);
CREATE INDEX IF NOT EXISTS audit_logs_cluster_id_idx ON audit_logs (cluster_id);

INSERT INTO roles (code, name) VALUES
    ('system_admin', 'System Administrator'),
    ('operations_admin', 'Operations Administrator'),
    ('security_auditor', 'Security Auditor'),
    ('viewer', 'Read-only User')
ON CONFLICT (code) DO NOTHING;
