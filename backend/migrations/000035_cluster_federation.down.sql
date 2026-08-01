-- Reverse M48: Multi-cluster federation.

DROP TABLE IF EXISTS cluster_federation_events;
DROP INDEX IF EXISTS clusters_single_host_uq;
DROP INDEX IF EXISTS clusters_federation_status_idx;
DROP INDEX IF EXISTS clusters_cluster_role_idx;
ALTER TABLE clusters
    DROP COLUMN IF EXISTS last_heartbeat_at,
    DROP COLUMN IF EXISTS registered_at,
    DROP COLUMN IF EXISTS federation_status,
    DROP COLUMN IF EXISTS cluster_role;
