CREATE TABLE IF NOT EXISTS backup_plans (
    id VARCHAR(36) PRIMARY KEY,
    cluster_id BIGINT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    status VARCHAR(24) NOT NULL DEFAULT 'awaiting_confirmation',
    backup_name VARCHAR(253) NOT NULL,
    backup_namespace VARCHAR(63) NOT NULL,
    included_namespaces TEXT[] NOT NULL DEFAULT '{}',
    storage_location VARCHAR(253) NOT NULL,
    ttl VARCHAR(16) NOT NULL DEFAULT '720h',
    include_cluster_resources BOOLEAN NOT NULL DEFAULT FALSE,
    snapshot_volumes BOOLEAN NOT NULL DEFAULT FALSE,
    label_selector JSONB NOT NULL DEFAULT '{}'::jsonb,
    velero_version VARCHAR(16) NOT NULL DEFAULT '',
    confirmation_token_hash BYTEA NOT NULL,
    requested_by_user_id BIGINT,
    requested_by_name VARCHAR(128) NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    idempotency_key VARCHAR(128) NOT NULL DEFAULT '',
    locked_at TIMESTAMPTZ,
    executed_at TIMESTAMPTZ,
    last_error VARCHAR(500) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT backup_plans_status_check CHECK (status IN (
        'awaiting_confirmation', 'executing', 'succeeded', 'failed', 'expired'
    )),
    CONSTRAINT backup_plans_ttl_check CHECK (ttl ~ '^[0-9]+(h|m|s)$'),
    CONSTRAINT backup_plans_backup_name_check CHECK (char_length(backup_name) BETWEEN 1 AND 253),
    CONSTRAINT backup_plans_backup_namespace_check CHECK (char_length(backup_namespace) BETWEEN 1 AND 63),
    CONSTRAINT backup_plans_storage_location_check CHECK (char_length(storage_location) BETWEEN 1 AND 253),
    CONSTRAINT backup_plans_requested_by_name_check CHECK (char_length(requested_by_name) BETWEEN 1 AND 128)
);

CREATE INDEX IF NOT EXISTS backup_plans_cluster_idx
    ON backup_plans (cluster_id, created_at DESC);

CREATE INDEX IF NOT EXISTS backup_plans_claim_idx
    ON backup_plans (status, expires_at, locked_at);
