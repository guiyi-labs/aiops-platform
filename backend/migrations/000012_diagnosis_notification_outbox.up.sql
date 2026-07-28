CREATE TABLE IF NOT EXISTS notification_settings (
    id SMALLINT PRIMARY KEY CHECK (id = 1),
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO notification_settings (id, enabled)
VALUES (1, FALSE)
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS notification_deliveries (
    id BIGSERIAL PRIMARY KEY,
    diagnosis_id BIGINT NOT NULL REFERENCES diagnosis_records(id) ON DELETE CASCADE,
    event_type VARCHAR(64) NOT NULL,
    payload JSONB NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'delivering', 'delivered', 'dead')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    locked_at TIMESTAMPTZ,
    delivered_at TIMESTAMPTZ,
    last_error VARCHAR(500) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS notification_deliveries_pending_idx
    ON notification_deliveries (next_attempt_at, id)
    WHERE status IN ('pending', 'delivering');
CREATE INDEX IF NOT EXISTS notification_deliveries_diagnosis_idx
    ON notification_deliveries (diagnosis_id, created_at DESC);

CREATE OR REPLACE FUNCTION enqueue_diagnosis_notification()
RETURNS TRIGGER AS $$
DECLARE
    event_payload JSONB;
BEGIN
    IF NOT COALESCE((SELECT enabled FROM notification_settings WHERE id = 1), FALSE) THEN
        RETURN NEW;
    END IF;

    IF TG_OP = 'INSERT' THEN
        event_payload := jsonb_build_object(
            'diagnosis_id', NEW.id,
            'cluster_id', NEW.cluster_id,
            'rule_id', NEW.rule_id,
            'severity', NEW.severity,
            'resource', jsonb_build_object(
                'kind', NEW.resource_kind,
                'namespace', NEW.resource_namespace,
                'name', NEW.resource_name
            ),
            'status', NEW.status,
            'previous_status', NULL,
            'assignee_user_id', NEW.assigned_to_user_id,
            'summary', NEW.summary,
            'observed_at', NEW.observed_at,
            'sla_due_at', NEW.sla_due_at,
            'changed_at', NEW.updated_at
        );
        INSERT INTO notification_deliveries (diagnosis_id, event_type, payload)
        VALUES (NEW.id, 'diagnosis.created', event_payload);
        RETURN NEW;
    END IF;

    IF OLD.status IS DISTINCT FROM NEW.status THEN
        event_payload := jsonb_build_object(
            'diagnosis_id', NEW.id,
            'cluster_id', NEW.cluster_id,
            'rule_id', NEW.rule_id,
            'severity', NEW.severity,
            'resource', jsonb_build_object(
                'kind', NEW.resource_kind,
                'namespace', NEW.resource_namespace,
                'name', NEW.resource_name
            ),
            'status', NEW.status,
            'previous_status', OLD.status,
            'assignee_user_id', NEW.assigned_to_user_id,
            'summary', NEW.summary,
            'observed_at', NEW.observed_at,
            'sla_due_at', NEW.sla_due_at,
            'changed_at', NEW.updated_at
        );
        INSERT INTO notification_deliveries (diagnosis_id, event_type, payload)
        VALUES (NEW.id, 'diagnosis.status_changed', event_payload);
    END IF;

    IF OLD.assigned_to_user_id IS DISTINCT FROM NEW.assigned_to_user_id THEN
        event_payload := jsonb_build_object(
            'diagnosis_id', NEW.id,
            'cluster_id', NEW.cluster_id,
            'rule_id', NEW.rule_id,
            'severity', NEW.severity,
            'resource', jsonb_build_object(
                'kind', NEW.resource_kind,
                'namespace', NEW.resource_namespace,
                'name', NEW.resource_name
            ),
            'status', NEW.status,
            'previous_status', NEW.status,
            'assignee_user_id', NEW.assigned_to_user_id,
            'summary', NEW.summary,
            'observed_at', NEW.observed_at,
            'sla_due_at', NEW.sla_due_at,
            'changed_at', NEW.updated_at
        );
        INSERT INTO notification_deliveries (diagnosis_id, event_type, payload)
        VALUES (NEW.id, 'diagnosis.assigned', event_payload);
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS diagnosis_notification_outbox ON diagnosis_records;
CREATE TRIGGER diagnosis_notification_outbox
AFTER INSERT OR UPDATE OF status, assigned_to_user_id ON diagnosis_records
FOR EACH ROW EXECUTE FUNCTION enqueue_diagnosis_notification();
