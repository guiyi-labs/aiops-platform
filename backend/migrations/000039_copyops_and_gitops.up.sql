-- M58: DevOps read-only + interactive cross-cluster copy + backup/restore GUI.
-- Stores interactive cross-cluster copy plans. A plan captures a bounded set
-- of namespaced source resources (fixed GVR whitelist: Deployments,
-- StatefulSets, DaemonSets, CronJobs, ConfigMaps, Secrets, Services,
-- Ingresses, ServiceAccounts; ≤20 resources per plan) selected from a
-- source cluster/namespace, a target cluster/namespace, and the M19
-- controlled-operation confirmation_token_hash + idempotency_key + Claim
-- state machine fields. Preview fetches the source manifests, strips
-- cluster-specific fields (uid, resourceVersion, creationTimestamp,
-- status, nodeName, clusterIP, etc.), runs a server-side dry-run create on
-- the target, and persists the plan. Execute claims the plan and applies
-- the stripped manifests verbatim (deterministic, no re-rendering).
-- Source/destination namespace identity is captured at preview and
-- re-verified at execute as a CompareAnd-Swap gate (mirrors M28 backup
-- and M19 promotion contracts).
--
-- GitOps (ArgoCD Application read-only) uses the M49 generic CRD browser
-- with a compile-time whitelist entry for argoproj.io/v1alpha1/applications;
-- no database persistence needed.
--
-- Backup/restore GUI convenience APIs use the existing kubernetes.Service
-- Backups/Restores/VeleroRestore typed projection methods over the
-- Velero CRs; no new tables.

CREATE TABLE IF NOT EXISTS copy_plans (
    id VARCHAR(36) PRIMARY KEY,
    status VARCHAR(32) NOT NULL,
    source_cluster_id BIGINT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    source_namespace VARCHAR(63) NOT NULL,
    source_namespace_uid VARCHAR(128) NOT NULL,
    source_namespace_resource_version VARCHAR(64) NOT NULL,
    target_cluster_id BIGINT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    target_namespace VARCHAR(63) NOT NULL,
    -- resource_items captures the selected source resources at preview time
    -- so execute applies exactly the same set. Each item has group/version/
    -- resource/name plus the stripped manifest JSON.
    resource_items JSONB NOT NULL DEFAULT '[]'::jsonb,
    -- copy_summary is a read-friendly array of {kind, name, namespace}
    -- projections shown in the GUI without parsing resource_items.
    copy_summary JSONB NOT NULL DEFAULT '[]'::jsonb,
    -- diff captures the typed preview summary: {resource_count,
    -- target_namespace_exists, will_create, will_skip, dry_run_errors[]}
    diff JSONB NOT NULL DEFAULT '{}'::jsonb,
    confirmation_token_hash BYTEA NOT NULL,
    requested_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    requested_by_name VARCHAR(128) NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ NOT NULL,
    idempotency_key VARCHAR(128) NOT NULL DEFAULT '',
    locked_at TIMESTAMPTZ,
    executed_at TIMESTAMPTZ,
    last_error VARCHAR(500) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT copy_plans_status_check
        CHECK (status IN ('awaiting_confirmation', 'executing', 'succeeded', 'failed', 'expired')),
    CONSTRAINT copy_plans_resource_count_check
        CHECK (jsonb_array_length(resource_items) BETWEEN 1 AND 20),
    CONSTRAINT copy_plans_namespace_length_check
        CHECK (
            char_length(source_namespace) BETWEEN 1 AND 63
            AND char_length(target_namespace) BETWEEN 1 AND 63
        ),
    CONSTRAINT copy_plans_token_present_check
        CHECK (char_length(id) = 36 AND char_length(requested_by_name) BETWEEN 1 AND 128)
);

CREATE INDEX IF NOT EXISTS copy_plans_claim_idx
    ON copy_plans (status, expires_at, locked_at);
CREATE INDEX IF NOT EXISTS copy_plans_source_idx
    ON copy_plans (source_cluster_id, source_namespace, created_at DESC);
CREATE INDEX IF NOT EXISTS copy_plans_target_idx
    ON copy_plans (target_cluster_id, target_namespace, created_at DESC);
CREATE INDEX IF NOT EXISTS copy_plans_requested_by_idx
    ON copy_plans (requested_by_user_id, created_at DESC)
    WHERE requested_by_user_id IS NOT NULL;
