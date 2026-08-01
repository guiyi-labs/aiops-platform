-- M46: Workspace multi-tenancy (KubeSphere-style lightweight Workspace layer)
--
-- Introduces the Workspace aggregation dimension modeled after KubeSphere's
-- Workspace CRD, but implemented in SQL rather than as a Kubernetes CRD. A
-- Workspace groups cluster namespaces across the fleet for UI grouping, quota
-- display and cross-cluster namespace attribution. It also carries its own
-- three-role model (workspace_admin / workspace_editor / workspace_viewer) that
-- is independent of the four platform roles.
--
-- Design invariants (per ADR 0061):
-- * Workspace is an aggregation dimension only. The existing 2D authorization
--   matrix (four platform roles x ClusterGrant/NamespaceGrant) is unchanged.
-- * A WorkspaceRole does NOT grant namespace read access. Namespace reads still
--   require the corresponding ClusterGrant or NamespaceGrant. WorkspaceRole
--   only authorizes workspace metadata/membership/quota/role-binding edits.
-- * SystemAdmin bypasses all grants (including WorkspaceGrant).
-- * 404 > 403 anti-leakage invariant still applies: an unauthorized workspace
--   is indistinguishable from a missing one.
-- * Custom workspace roles are deferred; only the three fixed roles are
--   permitted (workspace_admin / workspace_editor / workspace_viewer).
-- * Workspace quota is display-only; it is NOT enforced against actual cluster
--   ResourceQuota. Multi-cluster Workspace propagation controller is deferred;
--   membership is expressed in SQL only.

CREATE TABLE IF NOT EXISTS workspaces (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(63) NOT NULL,
    display_name VARCHAR(128) NOT NULL,
    owner_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    metadata_json JSONB NOT NULL DEFAULT '{}'::JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT workspaces_name_unique UNIQUE (name),
    CONSTRAINT workspaces_name_chk CHECK (name ~ '^[a-z0-9]([-a-z0-9]*[a-z0-9])?$'),
    CONSTRAINT workspaces_metadata_chk CHECK (jsonb_typeof(metadata_json) = 'object')
);

CREATE INDEX IF NOT EXISTS workspaces_owner_idx ON workspaces (owner_user_id);

-- workspace_memberships binds a workspace to a (cluster_id, namespace) tuple.
-- Mirrors the KubeSphere `kubesphere.io/workspace` label binding but in SQL.
-- A (cluster_id, namespace) pair may belong to at most one workspace.
CREATE TABLE IF NOT EXISTS workspace_memberships (
    id BIGSERIAL PRIMARY KEY,
    workspace_id BIGINT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    cluster_id BIGINT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    namespace VARCHAR(253) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT workspace_memberships_ws_cluster_ns_unique UNIQUE (cluster_id, namespace),
    CONSTRAINT workspace_memberships_ws_unique UNIQUE (workspace_id, cluster_id, namespace),
    CONSTRAINT workspace_memberships_namespace_chk CHECK (namespace ~ '^[a-z0-9]([-a-z0-9]*[a-z0-9])?$')
);

CREATE INDEX IF NOT EXISTS workspace_memberships_workspace_idx ON workspace_memberships (workspace_id);
CREATE INDEX IF NOT EXISTS workspace_memberships_cluster_ns_idx ON workspace_memberships (cluster_id, namespace);

-- workspace_quotas holds the soft quota display for a workspace (one row per
-- workspace). Mirrors KubeSphere Workspace ResourceQuota but is display-only;
-- the platform does NOT enforce it against cluster ResourceQuota. All hard_*
-- columns are nullable so a workspace can omit fields it does not track.
CREATE TABLE IF NOT EXISTS workspace_quotas (
    workspace_id BIGINT PRIMARY KEY REFERENCES workspaces(id) ON DELETE CASCADE,
    hard_cpu_cores NUMERIC(12,3),
    hard_memory_mib BIGINT,
    hard_pod_count BIGINT,
    hard_namespace_count BIGINT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT workspace_quotas_cpu_chk CHECK (hard_cpu_cores IS NULL OR hard_cpu_cores >= 0),
    CONSTRAINT workspace_quotas_mem_chk CHECK (hard_memory_mib IS NULL OR hard_memory_mib >= 0),
    CONSTRAINT workspace_quotas_pod_chk CHECK (hard_pod_count IS NULL OR hard_pod_count >= 0),
    CONSTRAINT workspace_quotas_ns_chk CHECK (hard_namespace_count IS NULL OR hard_namespace_count >= 0)
);

-- user_workspace_grants binds a user to a workspace with a fixed workspace role
-- (workspace_admin / workspace_editor / workspace_viewer). One role per
-- (user_id, workspace_id) pair.
CREATE TABLE IF NOT EXISTS user_workspace_grants (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    workspace_id BIGINT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    role VARCHAR(32) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT user_workspace_grants_unique UNIQUE (user_id, workspace_id),
    CONSTRAINT user_workspace_grants_role_chk CHECK (
        role IN ('workspace_admin', 'workspace_editor', 'workspace_viewer')
    )
);

CREATE INDEX IF NOT EXISTS user_workspace_grants_user_idx ON user_workspace_grants (user_id);
CREATE INDEX IF NOT EXISTS user_workspace_grants_workspace_idx ON user_workspace_grants (workspace_id);

-- workspace_role_bindings_audit is an append-only audit trail of workspace
-- role binding changes. Mirrors the platform audit pattern but is scoped to
-- workspace role management actions (grant / revoke / role change).
CREATE TABLE IF NOT EXISTS workspace_role_bindings_audit (
    id BIGSERIAL PRIMARY KEY,
    workspace_id BIGINT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role VARCHAR(32) NOT NULL,
    action VARCHAR(16) NOT NULL,
    granted_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    granted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT workspace_role_bindings_audit_role_chk CHECK (
        role IN ('workspace_admin', 'workspace_editor', 'workspace_viewer')
    ),
    CONSTRAINT workspace_role_bindings_audit_action_chk CHECK (
        action IN ('granted', 'revoked', 'changed')
    )
);

CREATE INDEX IF NOT EXISTS workspace_role_bindings_audit_workspace_idx
    ON workspace_role_bindings_audit (workspace_id, granted_at DESC);
CREATE INDEX IF NOT EXISTS workspace_role_bindings_audit_user_idx
    ON workspace_role_bindings_audit (user_id, granted_at DESC);
