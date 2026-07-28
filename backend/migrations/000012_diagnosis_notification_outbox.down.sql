DROP TRIGGER IF EXISTS diagnosis_notification_outbox ON diagnosis_records;
DROP FUNCTION IF EXISTS enqueue_diagnosis_notification();
DROP TABLE IF EXISTS notification_deliveries;
DROP TABLE IF EXISTS notification_settings;
