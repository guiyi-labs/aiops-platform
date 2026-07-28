CREATE TABLE IF NOT EXISTS ai_usage_reservations (
    id VARCHAR(64) PRIMARY KEY,
    diagnosis_id BIGINT NOT NULL REFERENCES diagnosis_records(id) ON DELETE CASCADE,
    reserved_tokens INTEGER NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ai_usage_reservations_tokens_check CHECK (reserved_tokens > 0)
);

CREATE INDEX IF NOT EXISTS ai_usage_reservations_expires_idx ON ai_usage_reservations (expires_at);
