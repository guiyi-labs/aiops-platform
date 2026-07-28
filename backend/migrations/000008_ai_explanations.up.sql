CREATE TABLE IF NOT EXISTS ai_explanations (
    id BIGSERIAL PRIMARY KEY,
    diagnosis_id BIGINT NOT NULL REFERENCES diagnosis_records(id) ON DELETE CASCADE,
    actor_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    actor_name VARCHAR(128) NOT NULL,
    provider VARCHAR(64) NOT NULL,
    model VARCHAR(128) NOT NULL,
    provider_response_id VARCHAR(128) NOT NULL DEFAULT '',
    summary TEXT NOT NULL,
    analysis TEXT NOT NULL,
    recommended_actions JSONB NOT NULL DEFAULT '[]'::JSONB,
    citations JSONB NOT NULL DEFAULT '[]'::JSONB,
    input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ai_explanations_token_check CHECK (input_tokens >= 0 AND output_tokens >= 0)
);

CREATE INDEX IF NOT EXISTS ai_explanations_diagnosis_idx ON ai_explanations (diagnosis_id, created_at DESC);
