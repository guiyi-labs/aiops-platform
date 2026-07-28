DROP INDEX IF EXISTS diagnosis_assignments_diagnosis_idx;
DROP INDEX IF EXISTS diagnosis_records_sla_idx;
DROP TABLE IF EXISTS diagnosis_assignments;
ALTER TABLE diagnosis_records DROP COLUMN IF EXISTS resolved_at;
ALTER TABLE diagnosis_records DROP COLUMN IF EXISTS sla_due_at;
