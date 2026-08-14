-- M111: bound unresolved incident SLA escalation and make each stage auditable.
-- Existing approaching/breached deliveries remain level 0. Levels 1 and 2
-- represent the first and final escalation stages respectively.

ALTER TABLE notification_deliveries
    ADD COLUMN IF NOT EXISTS escalation_level INTEGER NOT NULL DEFAULT 0;

ALTER TABLE notification_deliveries
    DROP CONSTRAINT IF EXISTS notification_deliveries_escalation_level_check;

ALTER TABLE notification_deliveries
    ADD CONSTRAINT notification_deliveries_escalation_level_check
    CHECK (escalation_level BETWEEN 0 AND 2);

DROP INDEX IF EXISTS notification_deliveries_incident_event_uniq;

CREATE UNIQUE INDEX IF NOT EXISTS notification_deliveries_incident_event_level_uniq
    ON notification_deliveries (incident_id, event_type, escalation_level)
    WHERE incident_id IS NOT NULL;
