ALTER TABLE incidents
    ADD COLUMN IF NOT EXISTS template_id VARCHAR(64) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS incidents_template_idx ON incidents (template_id)
    WHERE template_id <> '';
