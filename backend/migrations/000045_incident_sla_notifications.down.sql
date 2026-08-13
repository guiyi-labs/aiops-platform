DROP INDEX IF EXISTS notification_deliveries_incident_idx;
DROP INDEX IF EXISTS notification_deliveries_incident_event_uniq;
ALTER TABLE notification_deliveries DROP COLUMN IF EXISTS incident_id;
DELETE FROM notification_deliveries WHERE diagnosis_id IS NULL;
ALTER TABLE notification_deliveries ALTER COLUMN diagnosis_id SET NOT NULL;
