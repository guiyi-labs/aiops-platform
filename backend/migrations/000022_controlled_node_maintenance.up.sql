CREATE TABLE IF NOT EXISTS maintenance_plans (
    id VARCHAR(36) PRIMARY KEY,
    cluster_id BIGINT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    status VARCHAR(24) NOT NULL DEFAULT 'awaiting_confirmation',
    action VARCHAR(16) NOT NULL,
    node_name VARCHAR(253) NOT NULL,
    node_uid VARCHAR(64) NOT NULL DEFAULT '',
    node_resource_version VARCHAR(64) NOT NULL DEFAULT '',
    node_unschedulable BOOLEAN NOT NULL DEFAULT FALSE,
    preview_evidence JSONB NOT NULL DEFAULT '{}'::jsonb,
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
    CONSTRAINT maintenance_plans_status_check CHECK (status IN (
        'awaiting_confirmation', 'executing', 'succeeded', 'failed', 'expired'
    )),
    CONSTRAINT maintenance_plans_action_check CHECK (action IN (
        'cordon', 'uncordon', 'drain'
    )),
    CONSTRAINT maintenance_plans_node_name_check CHECK (char_length(node_name) BETWEEN 1 AND 253),
    CONSTRAINT maintenance_plans_requested_by_name_check CHECK (char_length(requested_by_name) BETWEEN 1 AND 128)
);

CREATE INDEX IF NOT EXISTS maintenance_plans_cluster_idx
    ON maintenance_plans (cluster_id, created_at DESC);

CREATE INDEX IF NOT EXISTS maintenance_plans_claim_idx
    ON maintenance_plans (status, expires_at, locked_at);
