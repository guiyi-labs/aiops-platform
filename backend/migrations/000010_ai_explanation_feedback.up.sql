CREATE TABLE ai_explanation_feedback (
    id BIGSERIAL PRIMARY KEY,
    explanation_id BIGINT NOT NULL REFERENCES ai_explanations(id) ON DELETE CASCADE,
    actor_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    actor_name VARCHAR(128) NOT NULL,
    verdict VARCHAR(32) NOT NULL CHECK (verdict IN ('helpful', 'partially_helpful', 'not_helpful')),
    comment TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ai_explanation_feedback_actor_once UNIQUE (explanation_id, actor_user_id),
    CONSTRAINT ai_explanation_feedback_comment_length CHECK (char_length(comment) <= 1000)
);

CREATE INDEX ai_explanation_feedback_explanation_created_idx
    ON ai_explanation_feedback (explanation_id, created_at DESC);

CREATE INDEX ai_explanation_feedback_verdict_created_idx
    ON ai_explanation_feedback (verdict, created_at DESC);
