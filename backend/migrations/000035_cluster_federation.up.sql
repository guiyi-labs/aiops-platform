-- M48: Multi-cluster federation (KubeSphere-style host/member model)
--
-- Extends the existing clusters table with KubeSphere host/member semantics
-- and introduces an append-only federation event log. The platform keeps the
-- kubeconfig-direct-connection model: there is no Cluster Agent / Tower side
-- channel, and no inter-cluster resource sync controller. Federation state is
-- a SQL aggregation view over the existing clusters table — cross-cluster
-- operations still go through explicit cluster_id + fixed GVR whitelist.
--
-- Design invariants (per ADR 0063):
-- * No Cluster Agent / Tower: every member cluster is still reached via its
--   stored kubeconfig (M17 encrypted cluster_credentials). Registration reuses
--   an existing cluster row; it does not provision a new one.
-- * No inter-cluster resource sync controller. Cross-cluster summaries are
--   computed on read from the existing cluster/fleet packages.
-- * The host cluster is identified by cluster_role = 'host'. There is at most
--   one host cluster (enforced by a partial unique index). A standalone
--   cluster (the default) is not part of any federation.
-- * 404 > 403 anti-leakage is preserved: an unauthorized cluster is
--   indistinguishable from a missing one. Federation reads require
--   SystemAdmin or operations_admin (rolesSystemOpsAdmin); the role gate is
--   enforced at the route layer.
-- * Federation events are append-only. There is no UPDATE/DELETE path; the
--   audit pattern mirrors ADR 0008.
-- * cluster_role and federation_status are bounded enums; the CHECK
--   constraints are part of the public contract.

-- Extend the existing clusters table with federation semantics. The existing
-- status column (enabled/disabled/unreachable) is unchanged; federation_status
-- is an orthogonal dimension that describes federation-level health
-- (registered/healthy/degraded/disconnected). A newly created cluster defaults
-- to cluster_role = 'standalone' and federation_status = 'registered' so the
-- migration is a no-op for pre-existing rows: every existing cluster becomes
-- a standalone cluster that is "registered" with the federation view (i.e.
-- visible in the overview) but not a host or member.
ALTER TABLE clusters
    ADD COLUMN IF NOT EXISTS cluster_role VARCHAR(16) NOT NULL DEFAULT 'standalone'
        CHECK (cluster_role IN ('host', 'member', 'standalone')),
    ADD COLUMN IF NOT EXISTS federation_status VARCHAR(16) NOT NULL DEFAULT 'registered'
        CHECK (federation_status IN ('registered', 'healthy', 'degraded', 'disconnected')),
    ADD COLUMN IF NOT EXISTS registered_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_heartbeat_at TIMESTAMPTZ;

-- Backfill pre-existing rows so the default takes effect for them too. The
-- registered_at is set to created_at so the overview's "registered" badge is
-- not null for legacy clusters.
UPDATE clusters
SET cluster_role = 'standalone',
    federation_status = 'registered',
    registered_at = created_at
WHERE cluster_role IS NULL OR registered_at IS NULL;

CREATE INDEX IF NOT EXISTS clusters_cluster_role_idx ON clusters (cluster_role);
CREATE INDEX IF NOT EXISTS clusters_federation_status_idx ON clusters (federation_status);

-- At most one host cluster. A partial unique index on cluster_role = 'host'
-- enforces the single-host invariant without restricting member/standalone
-- rows. NULLs are not an issue because cluster_role is NOT NULL.
CREATE UNIQUE INDEX IF NOT EXISTS clusters_single_host_uq
    ON clusters (cluster_role)
    WHERE cluster_role = 'host';

-- cluster_federation_events is the append-only audit trail of federation
-- state transitions and operator actions (register / deregister / heartbeat /
-- status_change). Mirrors the platform audit pattern (ADR 0008): no UPDATE or
-- DELETE path is exposed by the repository.
CREATE TABLE IF NOT EXISTS cluster_federation_events (
    id BIGSERIAL PRIMARY KEY,
    cluster_id BIGINT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    event_type VARCHAR(32) NOT NULL,
    status VARCHAR(16) NOT NULL,
    message VARCHAR(1024) NOT NULL DEFAULT '',
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT cluster_federation_events_event_type_chk CHECK (
        event_type IN ('registered', 'deregistered', 'heartbeat', 'status_change', 'role_change')
    ),
    CONSTRAINT cluster_federation_events_status_chk CHECK (
        status IN ('registered', 'healthy', 'degraded', 'disconnected')
    )
);

CREATE INDEX IF NOT EXISTS cluster_federation_events_cluster_idx
    ON cluster_federation_events (cluster_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS cluster_federation_events_occurred_at_idx
    ON cluster_federation_events (occurred_at DESC);
