DROP INDEX IF EXISTS diagnosis_feedback_diagnosis_idx;
DROP INDEX IF EXISTS diagnosis_activities_diagnosis_idx;
DROP INDEX IF EXISTS diagnosis_records_assignee_idx;
DROP TABLE IF EXISTS diagnosis_feedback;
DROP TABLE IF EXISTS diagnosis_activities;
ALTER TABLE diagnosis_records DROP COLUMN IF EXISTS updated_at;
ALTER TABLE diagnosis_records DROP COLUMN IF EXISTS assigned_to_user_id;
