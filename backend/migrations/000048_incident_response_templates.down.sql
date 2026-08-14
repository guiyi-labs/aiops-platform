DROP INDEX IF EXISTS incidents_template_idx;

ALTER TABLE incidents
    DROP COLUMN IF EXISTS template_id;
