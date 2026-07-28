CREATE TABLE IF NOT EXISTS remediation_plans (
    id VARCHAR(36) PRIMARY KEY,
    diagnosis_id BIGINT NOT NULL REFERENCES diagnosis_records(id) ON DELETE CASCADE,
    cluster_id BIGINT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    action VARCHAR(64) NOT NULL CHECK (action IN ('deployment.rollout_restart')),
    status VARCHAR(32) NOT NULL DEFAULT 'awaiting_confirmation'
        CHECK (status IN ('awaiting_confirmation', 'executing', 'succeeded', 'failed', 'expired')),
    target_kind VARCHAR(64) NOT NULL,
    target_namespace VARCHAR(253) NOT NULL,
    target_name VARCHAR(253) NOT NULL,
    target_uid VARCHAR(128) NOT NULL,
    target_resource_version VARCHAR(128) NOT NULL,
    restart_at TIMESTAMPTZ NOT NULL,
    confirmation_token_hash BYTEA NOT NULL,
    requested_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    requested_by_name VARCHAR(128) NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    idempotency_key VARCHAR(128),
    locked_at TIMESTAMPTZ,
    executed_at TIMESTAMPTZ,
    last_error VARCHAR(500) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS remediation_plans_diagnosis_idx
    ON remediation_plans (diagnosis_id, created_at DESC);
CREATE INDEX IF NOT EXISTS remediation_plans_claim_idx
    ON remediation_plans (status, expires_at, locked_at);
