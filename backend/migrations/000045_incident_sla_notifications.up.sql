-- M107: SLA reminders reuse the notification outbox. Deliveries previously
-- only referenced diagnosis_records; incident SLA events may have no diagnosis
-- (finding/alert/inspection/signal sources), so the outbox gains a nullable
-- incident_id column and diagnosis_id becomes optional. A partial unique index
-- guarantees at most one delivery per (incident, event_type), which lets the
-- SLA monitor re-run safely without duplicate reminders.

ALTER TABLE notification_deliveries ALTER COLUMN diagnosis_id DROP NOT NULL;

ALTER TABLE notification_deliveries
    ADD COLUMN IF NOT EXISTS incident_id BIGINT REFERENCES incidents(id) ON DELETE CASCADE;

CREATE UNIQUE INDEX IF NOT EXISTS notification_deliveries_incident_event_uniq
    ON notification_deliveries (incident_id, event_type)
    WHERE incident_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS notification_deliveries_incident_idx
    ON notification_deliveries (incident_id, created_at DESC);
