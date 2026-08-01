-- M52: Intelligent Inspection (KubeEye-style) + Service Mesh read-only.
-- Inspection stores compile-time rule catalog metadata (runtime overrides),
-- scheduled plans, task executions, and normalized findings. Findings are
-- normalized into the M39 signal model via inspection.signal_code mapping;
-- the raw inspection result is retained for evidence links.

-- inspection_rules — per-cluster runtime overrides for the compile-time rule
-- catalog. A row existing here means "cluster_id + rule_code has explicit
-- enabled/severity override"; absence means use the compiled-in defaults.
CREATE TABLE IF NOT EXISTS inspection_rules (
    id BIGSERIAL PRIMARY KEY,
    cluster_id BIGINT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    rule_code VARCHAR(128) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    -- severity override; empty means use the catalog default.
    severity_override VARCHAR(16) NOT NULL DEFAULT '',
    updated_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (cluster_id, rule_code)
);
CREATE INDEX idx_inspection_rules_cluster ON inspection_rules(cluster_id);

-- inspection_plans — scheduled inspection runs. A plan defines which clusters
-- to inspect, which rule codes (empty = all enabled), and the cron schedule.
-- Per M52 only "all clusters" and "immediate run" are exposed via the API;
-- the full cron scheduler is implemented but gated to operations_admin.
CREATE TABLE IF NOT EXISTS inspection_plans (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(128) NOT NULL,
    creator_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- scope: cluster_ids NULL means "all reachable clusters"; otherwise the
    -- explicit list. At M52 scope the API exposes only NULL scope; per-cluster
    -- ad-hoc runs go through RunInspectOnce.
    cluster_ids BIGINT[] NOT NULL DEFAULT '{}',
    -- rule_codes: empty array means "all rules enabled for each cluster"
    rule_codes VARCHAR(128)[] NOT NULL DEFAULT '{}',
    -- cron spec (5-field standard cron, UTC). Empty string = manual only.
    cron_spec VARCHAR(64) NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    last_run_at TIMESTAMPTZ,
    next_run_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_inspection_plans_creator ON inspection_plans(creator_id);
CREATE INDEX idx_inspection_plans_enabled ON inspection_plans(enabled) WHERE enabled = TRUE;
CREATE INDEX idx_inspection_plans_next_run ON inspection_plans(next_run_at) WHERE next_run_at IS NOT NULL;

-- inspection_tasks — one execution of a plan (or ad-hoc via RunInspectOnce).
-- A task fans out into per-cluster runs; each cluster run produces 0..N
-- inspection_results rows.
CREATE TABLE IF NOT EXISTS inspection_tasks (
    id BIGSERIAL PRIMARY KEY,
    plan_id BIGINT REFERENCES inspection_plans(id) ON DELETE SET NULL,
    plan_name_snapshot VARCHAR(128) NOT NULL DEFAULT '',
    triggered_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    trigger_reason VARCHAR(64) NOT NULL DEFAULT 'manual', -- 'manual' | 'schedule'
    cluster_ids BIGINT[] NOT NULL DEFAULT '{}',
    rule_codes VARCHAR(128)[] NOT NULL DEFAULT '{}',
    status VARCHAR(16) NOT NULL DEFAULT 'pending', -- pending | running | completed | partial | failed
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    total_clusters INT NOT NULL DEFAULT 0,
    completed_clusters INT NOT NULL DEFAULT 0,
    finding_count INT NOT NULL DEFAULT 0,
    error_summary VARCHAR(500) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_inspection_tasks_status ON inspection_tasks(status);
CREATE INDEX idx_inspection_tasks_triggered_by ON inspection_tasks(triggered_by);
CREATE INDEX idx_inspection_tasks_created ON inspection_tasks(created_at DESC);

-- inspection_results — one normalized finding. Maps 1:1 into the M39 signal
-- model via signal_code. The evidence payload is a redacted JSON snapshot
-- (no secrets, no raw telemetry) used for the diagnosis explorer.
CREATE TABLE IF NOT EXISTS inspection_results (
    id BIGSERIAL PRIMARY KEY,
    task_id BIGINT NOT NULL REFERENCES inspection_tasks(id) ON DELETE CASCADE,
    cluster_id BIGINT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    rule_code VARCHAR(128) NOT NULL,
    signal_code VARCHAR(128) NOT NULL, -- M39 normalized signal code
    severity VARCHAR(16) NOT NULL,     -- critical | warning | info
    state VARCHAR(16) NOT NULL DEFAULT 'active', -- active | resolved (at time of run)
    namespace VARCHAR(253) NOT NULL DEFAULT '',
    resource_kind VARCHAR(64) NOT NULL DEFAULT '',
    resource_name VARCHAR(253) NOT NULL DEFAULT '',
    resource_uid VARCHAR(128) NOT NULL DEFAULT '',
    -- SHA256 fingerprint of (cluster_id, rule_code, namespace, resource_uid)
    -- used for de-duplication across runs and M39 signal identity.
    fingerprint VARCHAR(64) NOT NULL,
    -- Evidence snapshot: redacted rule output. JSON stored as text to avoid
    -- postgres jsonb indexing costs; callers unmarshal on read.
    evidence_snapshot TEXT NOT NULL DEFAULT '{}',
    observed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_inspection_results_task ON inspection_results(task_id);
CREATE INDEX idx_inspection_results_cluster ON inspection_results(cluster_id);
CREATE INDEX idx_inspection_results_signal ON inspection_results(signal_code);
CREATE INDEX idx_inspection_results_fingerprint ON inspection_results(fingerprint);
CREATE INDEX idx_inspection_results_observed ON inspection_results(observed_at DESC);
