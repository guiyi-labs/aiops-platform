CREATE TABLE IF NOT EXISTS restore_plans (
    id VARCHAR(36) PRIMARY KEY,
    cluster_id BIGINT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    status VARCHAR(24) NOT NULL DEFAULT 'awaiting_confirmation',
    source_backup_name VARCHAR(253) NOT NULL,
    source_backup_namespace VARCHAR(63) NOT NULL,
    source_backup_uid VARCHAR(64) NOT NULL DEFAULT '',
    source_backup_resource_version VARCHAR(64) NOT NULL DEFAULT '',
    source_backup_phase VARCHAR(32) NOT NULL DEFAULT '',
    destination_namespace VARCHAR(63) NOT NULL,
    destination_namespace_uid VARCHAR(64) NOT NULL DEFAULT '',
    velero_restore_name VARCHAR(253) NOT NULL DEFAULT '',
    velero_restore_namespace VARCHAR(63) NOT NULL DEFAULT '',
    velero_restore_uid VARCHAR(64) NOT NULL DEFAULT '',
    quarantine_status JSONB NOT NULL DEFAULT '{}'::jsonb,
    execution_result JSONB,
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
    CONSTRAINT restore_plans_status_check CHECK (status IN (
        'awaiting_confirmation', 'executing', 'succeeded', 'failed', 'expired'
    )),
    CONSTRAINT restore_plans_source_backup_name_check CHECK (char_length(source_backup_name) BETWEEN 1 AND 253),
    CONSTRAINT restore_plans_source_backup_namespace_check CHECK (char_length(source_backup_namespace) BETWEEN 1 AND 63),
    CONSTRAINT restore_plans_destination_namespace_check CHECK (char_length(destination_namespace) BETWEEN 1 AND 63),
    CONSTRAINT restore_plans_velero_restore_namespace_check CHECK (char_length(velero_restore_namespace) BETWEEN 0 AND 63),
    CONSTRAINT restore_plans_requested_by_name_check CHECK (char_length(requested_by_name) BETWEEN 1 AND 128)
);

CREATE INDEX IF NOT EXISTS restore_plans_cluster_idx
    ON restore_plans (cluster_id, created_at DESC);

CREATE INDEX IF NOT EXISTS restore_plans_claim_idx
    ON restore_plans (status, expires_at, locked_at);

CREATE UNIQUE INDEX IF NOT EXISTS restore_plans_source_active_idx
    ON restore_plans (cluster_id, source_backup_name, source_backup_namespace)
    WHERE status IN ('awaiting_confirmation', 'executing');
