CREATE TABLE IF NOT EXISTS credential_reencryption_runs (
    id VARCHAR(36) PRIMARY KEY,
    target_key_version VARCHAR(64) NOT NULL,
    source_key_versions JSONB NOT NULL DEFAULT '[]'::JSONB,
    dry_run BOOLEAN NOT NULL DEFAULT TRUE,
    status VARCHAR(16) NOT NULL DEFAULT 'running',
    examined_count INTEGER NOT NULL DEFAULT 0,
    reencrypted_count INTEGER NOT NULL DEFAULT 0,
    remaining_count INTEGER NOT NULL DEFAULT 0,
    batch_count INTEGER NOT NULL DEFAULT 0,
    error_code VARCHAR(64) NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    CONSTRAINT credential_reencryption_runs_status_check
        CHECK (status IN ('running', 'succeeded', 'failed')),
    CONSTRAINT credential_reencryption_runs_counts_check
        CHECK (
            examined_count >= 0 AND reencrypted_count >= 0
            AND remaining_count >= 0 AND batch_count >= 0
        ),
    CONSTRAINT credential_reencryption_runs_completion_check
        CHECK (
            (status = 'running' AND completed_at IS NULL)
            OR (status IN ('succeeded', 'failed') AND completed_at IS NOT NULL)
        )
);

CREATE INDEX IF NOT EXISTS credential_reencryption_runs_started_idx
    ON credential_reencryption_runs (started_at DESC);
