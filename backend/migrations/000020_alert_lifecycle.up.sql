CREATE TABLE IF NOT EXISTS alert_rules (
    id BIGSERIAL PRIMARY KEY,
    cluster_id BIGINT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    display_name VARCHAR(128) NOT NULL,
    resource_kind VARCHAR(16) NOT NULL,
    resource_name VARCHAR(253) NOT NULL,
    metric_name VARCHAR(16) NOT NULL,
    operator VARCHAR(4) NOT NULL,
    threshold BIGINT NOT NULL,
    for_seconds INTEGER NOT NULL,
    minimum_points INTEGER NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    deleted BOOLEAN NOT NULL DEFAULT FALSE,
    last_evaluation_state VARCHAR(24) NOT NULL DEFAULT '',
    last_evaluation_at TIMESTAMPTZ,
    last_error_code VARCHAR(32) NOT NULL DEFAULT '',
    next_due_at TIMESTAMPTZ NOT NULL,
    claim_expires_at TIMESTAMPTZ,
    creator_user_id BIGINT NOT NULL,
    creator_name VARCHAR(128) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT alert_rules_resource_kind_check CHECK (resource_kind = 'Node'),
    CONSTRAINT alert_rules_metric_name_check CHECK (metric_name IN ('cpu', 'memory')),
    CONSTRAINT alert_rules_operator_check CHECK (operator IN ('gte', 'lte')),
    CONSTRAINT alert_rules_threshold_check CHECK (threshold >= 0),
    CONSTRAINT alert_rules_for_seconds_check CHECK (for_seconds BETWEEN 60 AND 21600),
    CONSTRAINT alert_rules_minimum_points_check CHECK (minimum_points BETWEEN 2 AND 360),
    CONSTRAINT alert_rules_display_name_length_check CHECK (char_length(display_name) BETWEEN 1 AND 128),
    CONSTRAINT alert_rules_resource_name_length_check CHECK (char_length(resource_name) BETWEEN 1 AND 253),
    CONSTRAINT alert_rules_creator_name_length_check CHECK (char_length(creator_name) BETWEEN 1 AND 128)
);

CREATE INDEX IF NOT EXISTS alert_rules_cluster_active_idx
    ON alert_rules (cluster_id, deleted, next_due_at, id)
    WHERE deleted = FALSE AND enabled = TRUE;

CREATE UNIQUE INDEX IF NOT EXISTS alert_rules_unique_active_name_idx
    ON alert_rules (cluster_id, LOWER(display_name))
    WHERE deleted = FALSE;

CREATE INDEX IF NOT EXISTS alert_rules_claim_idx
    ON alert_rules (next_due_at, id)
    WHERE deleted = FALSE AND enabled = TRUE AND claim_expires_at IS NULL;

CREATE TABLE IF NOT EXISTS alert_instances (
    id BIGSERIAL PRIMARY KEY,
    rule_id BIGINT NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
    diagnosis_id BIGINT NOT NULL REFERENCES diagnosis_records(id) ON DELETE CASCADE,
    state VARCHAR(16) NOT NULL,
    first_fired_at TIMESTAMPTZ NOT NULL,
    last_fired_at TIMESTAMPTZ NOT NULL,
    resolved_at TIMESTAMPTZ,
    latest_evidence_anchor JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT alert_instances_state_check CHECK (state IN ('firing', 'resolved'))
);

CREATE UNIQUE INDEX IF NOT EXISTS one_unresolved_per_rule_idx
    ON alert_instances (rule_id)
    WHERE state = 'firing';

CREATE INDEX IF NOT EXISTS alert_instances_rule_state_idx
    ON alert_instances (rule_id, state, created_at DESC);

CREATE INDEX IF NOT EXISTS alert_instances_diagnosis_idx
    ON alert_instances (diagnosis_id);
