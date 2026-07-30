ALTER TABLE backup_plans DROP CONSTRAINT IF EXISTS backup_plans_ttl_check;
ALTER TABLE backup_plans
    ADD CONSTRAINT backup_plans_ttl_check CHECK (ttl ~ '^[0-9]+(h|m|s)$');

ALTER TABLE backup_plans
    DROP COLUMN IF EXISTS backup_resource_version,
    DROP COLUMN IF EXISTS backup_uid,
    DROP COLUMN IF EXISTS source_namespace_resource_version,
    DROP COLUMN IF EXISTS source_namespace_uid;
