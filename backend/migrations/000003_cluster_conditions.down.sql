DROP TABLE IF EXISTS cluster_conditions;
ALTER TABLE clusters DROP COLUMN IF EXISTS enabled;
ALTER TABLE clusters DROP CONSTRAINT IF EXISTS clusters_status_check;
ALTER TABLE clusters ADD CONSTRAINT clusters_status_check
    CHECK (status IN ('enabled', 'disabled', 'unreachable'));
