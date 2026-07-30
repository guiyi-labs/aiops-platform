ALTER TABLE backup_plans
    ADD COLUMN IF NOT EXISTS source_namespace_uid VARCHAR(253) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS source_namespace_resource_version VARCHAR(253) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS backup_uid VARCHAR(253) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS backup_resource_version VARCHAR(253) NOT NULL DEFAULT '';

ALTER TABLE backup_plans DROP CONSTRAINT IF EXISTS backup_plans_ttl_check;
ALTER TABLE backup_plans
    ADD CONSTRAINT backup_plans_ttl_check CHECK (ttl IN ('24h', '168h', '720h'));
