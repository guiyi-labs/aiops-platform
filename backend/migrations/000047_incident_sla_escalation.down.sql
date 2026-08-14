DROP INDEX IF EXISTS notification_deliveries_incident_event_level_uniq;
ALTER TABLE notification_deliveries DROP CONSTRAINT IF EXISTS notification_deliveries_escalation_level_check;
ALTER TABLE notification_deliveries DROP COLUMN IF EXISTS escalation_level;
CREATE UNIQUE INDEX IF NOT EXISTS notification_deliveries_incident_event_uniq
    ON notification_deliveries (incident_id, event_type)
    WHERE incident_id IS NOT NULL;
