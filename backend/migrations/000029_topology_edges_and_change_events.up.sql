-- M40: Temporal topology edges and change timeline.
-- topology_edges stores reviewed relationship edges, not full Kubernetes objects.
-- change_events stores normalized platform-operation outcomes from M23-M31.

CREATE TABLE IF NOT EXISTS topology_edges (
    id BIGSERIAL PRIMARY KEY,
    cluster_id BIGINT NOT NULL,
    kind VARCHAR(32) NOT NULL,
    source_kind VARCHAR(64) NOT NULL,
    source_namespace VARCHAR(63) NOT NULL DEFAULT '',
    source_name VARCHAR(253) NOT NULL,
    source_uid VARCHAR(64) NOT NULL DEFAULT '',
    source_incomplete BOOLEAN NOT NULL DEFAULT FALSE,
    target_kind VARCHAR(64) NOT NULL,
    target_namespace VARCHAR(63) NOT NULL DEFAULT '',
    target_name VARCHAR(253) NOT NULL,
    target_uid VARCHAR(64) NOT NULL DEFAULT '',
    target_incomplete BOOLEAN NOT NULL DEFAULT FALSE,
    derivation VARCHAR(32) NOT NULL,
    first_observed_at TIMESTAMPTZ NOT NULL,
    last_observed_at TIMESTAMPTZ NOT NULL,
    valid_from TIMESTAMPTZ NOT NULL,
    valid_to TIMESTAMPTZ,
    review_evidence JSONB NOT NULL DEFAULT '[]'::jsonb,
    source_hash VARCHAR(64) NOT NULL DEFAULT '',
    CHECK (kind IN ('Owns','Selects','RoutesTo','BackedBy','RunsOn','Mounts','Scales','ProtectedBy')),
    CHECK (derivation IN ('owner_reference','label_selector','endpointslice','backend_config','node_name','volume_mount','scale_target_ref','pdb_selector'))
);

-- One active edge per (cluster, kind, source_uid, target_uid, derivation).
-- When an edge is closed (valid_to set), a new row with the same identity can
-- be inserted. The partial unique index enforces at-most-one-active-edge.
CREATE UNIQUE INDEX IF NOT EXISTS uq_topology_edges_active
    ON topology_edges(cluster_id, kind, source_uid, target_uid, derivation)
    WHERE valid_to IS NULL;

CREATE INDEX IF NOT EXISTS idx_topology_edges_cluster_ns
    ON topology_edges(cluster_id, source_namespace);
CREATE INDEX IF NOT EXISTS idx_topology_edges_source
    ON topology_edges(cluster_id, source_uid) WHERE valid_to IS NULL;
CREATE INDEX IF NOT EXISTS idx_topology_edges_target
    ON topology_edges(cluster_id, target_uid) WHERE valid_to IS NULL;
CREATE INDEX IF NOT EXISTS idx_topology_edges_valid
    ON topology_edges(cluster_id, valid_from, valid_to);

CREATE TABLE IF NOT EXISTS change_events (
    id BIGSERIAL PRIMARY KEY,
    cluster_id BIGINT NOT NULL,
    namespace VARCHAR(63) NOT NULL DEFAULT '',
    kind VARCHAR(32) NOT NULL,
    plan_id VARCHAR(64) NOT NULL DEFAULT '',
    target_kind VARCHAR(64) NOT NULL,
    target_namespace VARCHAR(63) NOT NULL DEFAULT '',
    target_name VARCHAR(253) NOT NULL,
    target_uid VARCHAR(64) NOT NULL DEFAULT '',
    target_incomplete BOOLEAN NOT NULL DEFAULT FALSE,
    action VARCHAR(64) NOT NULL DEFAULT '',
    safe_diff_hash VARCHAR(64) NOT NULL DEFAULT '',
    revision VARCHAR(256) NOT NULL DEFAULT '',
    actor VARCHAR(128) NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ,
    result VARCHAR(32) NOT NULL DEFAULT 'pending',
    audit_id BIGINT,
    request_id VARCHAR(64) NOT NULL DEFAULT '',
    evidence JSONB NOT NULL DEFAULT '[]'::jsonb,
    confidence VARCHAR(8) NOT NULL DEFAULT 'high',
    source VARCHAR(32) NOT NULL DEFAULT 'platform',
    ingested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (kind IN ('promotion','backup','maintenance','restore','rollout','audit')),
    CHECK (result IN ('succeeded','failed','pending','partial')),
    CHECK (confidence IN ('high','low')),
    CHECK (source IN ('platform','k8s_event','delivery_adapter'))
);

-- Idempotent ingestion: one row per (kind, plan_id) for platform changes.
CREATE UNIQUE INDEX IF NOT EXISTS uq_change_events_plan
    ON change_events(kind, plan_id) WHERE plan_id != '';

CREATE INDEX IF NOT EXISTS idx_change_events_cluster_ns
    ON change_events(cluster_id, namespace, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_change_events_kind
    ON change_events(kind, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_change_events_target
    ON change_events(cluster_id, target_uid, started_at DESC);
