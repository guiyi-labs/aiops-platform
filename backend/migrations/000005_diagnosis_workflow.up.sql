ALTER TABLE diagnosis_records
    ADD COLUMN IF NOT EXISTS assigned_to_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

CREATE TABLE IF NOT EXISTS diagnosis_activities (
    id BIGSERIAL PRIMARY KEY,
    diagnosis_id BIGINT NOT NULL REFERENCES diagnosis_records(id) ON DELETE CASCADE,
    actor_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    actor_name VARCHAR(128) NOT NULL,
    from_status VARCHAR(32) NOT NULL,
    to_status VARCHAR(32) NOT NULL,
    comment TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT diagnosis_activities_from_status_check CHECK (from_status IN ('open', 'confirmed', 'resolved', 'dismissed')),
    CONSTRAINT diagnosis_activities_to_status_check CHECK (to_status IN ('open', 'confirmed', 'resolved', 'dismissed')),
    CONSTRAINT diagnosis_activities_status_changed_check CHECK (from_status <> to_status)
);

CREATE TABLE IF NOT EXISTS diagnosis_feedback (
    id BIGSERIAL PRIMARY KEY,
    diagnosis_id BIGINT NOT NULL REFERENCES diagnosis_records(id) ON DELETE CASCADE,
    actor_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    actor_name VARCHAR(128) NOT NULL,
    verdict VARCHAR(32) NOT NULL,
    comment TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT diagnosis_feedback_verdict_check CHECK (verdict IN ('accurate', 'inaccurate', 'uncertain'))
);

CREATE INDEX IF NOT EXISTS diagnosis_records_assignee_idx ON diagnosis_records (assigned_to_user_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS diagnosis_activities_diagnosis_idx ON diagnosis_activities (diagnosis_id, created_at DESC);
CREATE INDEX IF NOT EXISTS diagnosis_feedback_diagnosis_idx ON diagnosis_feedback (diagnosis_id, created_at DESC);
