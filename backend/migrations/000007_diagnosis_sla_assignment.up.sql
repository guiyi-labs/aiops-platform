ALTER TABLE diagnosis_records
    ADD COLUMN IF NOT EXISTS sla_due_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS resolved_at TIMESTAMPTZ;

UPDATE diagnosis_records
SET sla_due_at = observed_at + CASE severity
    WHEN 'critical' THEN INTERVAL '1 hour'
    WHEN 'high' THEN INTERVAL '4 hours'
    WHEN 'warning' THEN INTERVAL '24 hours'
    ELSE INTERVAL '72 hours'
END
WHERE sla_due_at IS NULL;

ALTER TABLE diagnosis_records ALTER COLUMN sla_due_at SET NOT NULL;

CREATE TABLE IF NOT EXISTS diagnosis_assignments (
    id BIGSERIAL PRIMARY KEY,
    diagnosis_id BIGINT NOT NULL REFERENCES diagnosis_records(id) ON DELETE CASCADE,
    actor_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    actor_name VARCHAR(128) NOT NULL,
    from_assignee_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    from_assignee_name VARCHAR(128) NOT NULL DEFAULT '',
    to_assignee_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    to_assignee_name VARCHAR(128) NOT NULL,
    comment TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS diagnosis_records_sla_idx ON diagnosis_records (status, sla_due_at);
CREATE INDEX IF NOT EXISTS diagnosis_assignments_diagnosis_idx ON diagnosis_assignments (diagnosis_id, created_at DESC);
