ALTER TABLE audit_logs
    ADD COLUMN IF NOT EXISTS actor_name VARCHAR(128) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS status_code INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS ip_address VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS user_agent VARCHAR(512) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS audit_logs_action_created_idx ON audit_logs (action, created_at DESC);
CREATE INDEX IF NOT EXISTS audit_logs_result_created_idx ON audit_logs (result, created_at DESC);
CREATE INDEX IF NOT EXISTS audit_logs_actor_created_idx ON audit_logs (actor_user_id, created_at DESC);
