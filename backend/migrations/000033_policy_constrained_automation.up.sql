-- M44: Policy-Constrained Automation And Post-Action Verification
--
-- Introduces action_plans (the M44 entity linking M42 correlation_cases +
-- optional M43 ai_investigations to a fixed runbook) and action_verifications
-- (the post-action SLI/resource comparison entity).
--
-- Design invariants:
-- * The default automation ceiling is L2 (human-confirmed execution). L3
--   (pre-authorized automatic execution) is not enabled by this migration; it
--   requires a separate ADR with shadow mode, narrow policy, canary, kill
--   switch and user approval.
-- * AI output never becomes client input directly. The runbook_id is a fixed
--   catalog ID from M43; clients cannot supply arbitrary patches, images,
--   rollback revisions or kubectl.
-- * Stale UID/RV, expired token, duplicate execution, wrong target,
--   unauthorized scope, unconfirmed plan, exceeded attempt cap or active
--   freeze window fail closed.
-- * Two workers and replay produce one business side effect (idempotent claim
--   with confirmation_token_hash and idempotency_key).
-- * High-risk actions require four-eyes approval and the requester cannot
--   self-approve (enforced in the service + DB CHECK).
-- * Pre/post checks use the same versioned SLI/resource rules and preserve
--   evidence; missing evidence yields VerificationStatusUnknown, never
--   VerificationStatusEffective.
-- * A failed post-check follows only the server-owned rollback contract;
--   unsafe rollback stops and escalates to a human (no rollback plan is
--   created).
-- * plan_key = SHA256 over (case_id + runbook_id + target_uid + automation_version)
--   so identical source + runbook + target + version reproduce identical plans.
-- * verification_key = SHA256 over (plan_id + verifier_version + evidence_hash)
--   so identical evidence reproduces identical verifications.

CREATE TABLE IF NOT EXISTS action_plans (
    id VARCHAR(36) PRIMARY KEY,
    plan_key VARCHAR(64) NOT NULL,
    automation_version VARCHAR(16) NOT NULL DEFAULT '1.0',

    -- Source linkage. case_id is required (M42); investigation_id and
    -- action_candidate_id are optional.
    case_id BIGINT NOT NULL,
    investigation_id BIGINT,
    action_candidate_id BIGINT,

    -- Runbook + action. runbook_id must exist in the M43 catalog and be
    -- eligible per the M42 Action Catalog at preview time.
    runbook_id VARCHAR(128) NOT NULL,
    action_code VARCHAR(64) NOT NULL,
    cluster_id BIGINT NOT NULL,

    -- Target snapshot captured at preview time. UID/RV are rechecked before
    -- execute; mismatch fails closed.
    target_kind VARCHAR(64) NOT NULL,
    target_namespace VARCHAR(253) NOT NULL,
    target_name VARCHAR(253) NOT NULL,
    target_uid VARCHAR(128) NOT NULL DEFAULT '',
    target_resource_version VARCHAR(128) NOT NULL DEFAULT '',

    -- Operation parameters materialized from the case context. Only one
    -- action-specific group is meaningful per action_code; the catalog
    -- enforces which fields are admitted.
    desired_replicas INTEGER,
    before_replicas INTEGER,
    desired_suspended BOOLEAN,
    before_suspended BOOLEAN,
    container_name VARCHAR(253) NOT NULL DEFAULT '',
    before_image VARCHAR(512) NOT NULL DEFAULT '',
    desired_image VARCHAR(512) NOT NULL DEFAULT '',
    rollback_revision INTEGER,
    rollback_replicaset_name VARCHAR(253) NOT NULL DEFAULT '',
    rollback_replicaset_uid VARCHAR(128) NOT NULL DEFAULT '',
    rollback_replicaset_resource_version VARCHAR(128) NOT NULL DEFAULT '',

    -- Policy gate results (JSONB array of {code,status,reason,checked_at,
    -- rechecked}). Bounded by MaxPolicyGatesPerPlan in the catalog.
    policy_gates JSONB NOT NULL DEFAULT '[]'::JSONB,

    -- Status + approval.
    status VARCHAR(16) NOT NULL DEFAULT 'draft',
    level VARCHAR(4) NOT NULL DEFAULT 'L2',
    approval_type VARCHAR(16) NOT NULL DEFAULT 'single',
    requested_by_user_id BIGINT,
    requested_by_name VARCHAR(128) NOT NULL DEFAULT '',
    approver_user_id BIGINT,
    approver_name VARCHAR(128) NOT NULL DEFAULT '',
    approved_at TIMESTAMPTZ,

    -- Confirmation + idempotency (mirror remediation/maintenance).
    confirmation_token_hash BYTEA NOT NULL,
    idempotency_key VARCHAR(128) NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ NOT NULL,
    locked_at TIMESTAMPTZ,
    executed_at TIMESTAMPTZ,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    last_error VARCHAR(500) NOT NULL DEFAULT '',

    -- Verification linkage. Set when post-action verification completes.
    verification_id BIGINT,

    -- Audit correlation. CorrelationRequestID is shared across
    -- preview/approve/execute/verify so the audit trail is reconstructable.
    correlation_request_id VARCHAR(64) NOT NULL DEFAULT '',

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT action_plans_status_chk CHECK (status IN (
        'draft', 'previewed', 'approved', 'executing',
        'succeeded', 'failed', 'expired', 'cancelled', 'verified'
    )),
    CONSTRAINT action_plans_level_chk CHECK (level IN ('L0','L1','L2','L3')),
    CONSTRAINT action_plans_approval_type_chk CHECK (approval_type IN ('single','four_eyes')),
    CONSTRAINT action_plans_automation_version_chk CHECK (automation_version = '1.0'),
    CONSTRAINT action_plans_plan_key_chk CHECK (plan_key ~ '^[0-9a-f]{64}$'),
    CONSTRAINT action_plans_runbook_id_chk CHECK (char_length(runbook_id) BETWEEN 1 AND 128),
    CONSTRAINT action_plans_action_code_chk CHECK (action_code <> ''),
    CONSTRAINT action_plans_target_kind_chk CHECK (target_kind <> ''),
    CONSTRAINT action_plans_target_name_chk CHECK (target_name <> ''),
    CONSTRAINT action_plans_attempt_count_chk CHECK (attempt_count >= 0 AND attempt_count <= 100),
    CONSTRAINT action_plans_desired_replicas_chk CHECK (
        desired_replicas IS NULL OR (desired_replicas >= 0 AND desired_replicas <= 1000)
    ),
    CONSTRAINT action_plans_rollback_revision_chk CHECK (
        rollback_revision IS NULL OR rollback_revision >= 1
    ),
    -- approved plans must record an approver
    CONSTRAINT action_plans_approved_approver_chk CHECK (
        status != 'approved' OR (approver_user_id IS NOT NULL AND approver_name <> '')
    ),
    -- executing/succeeded/failed plans must have an idempotency key
    CONSTRAINT action_plans_executing_idempotency_chk CHECK (
        status NOT IN ('executing','succeeded','failed') OR char_length(idempotency_key) BETWEEN 8 AND 128
    ),
    -- four_eyes approval requires approver != requester (the service
    -- enforces this at approve time; the DB CHECK is a defence-in-depth
    -- using (approver_user_id IS NULL OR approver_user_id <> requested_by_user_id
    -- OR requested_by_user_id IS NULL). The service is authoritative because
    -- the DB cannot express "different users" without allowing the case
    -- where one of them is NULL.
    CONSTRAINT action_plans_four_eyes_distinct_chk CHECK (
        approval_type != 'four_eyes' OR approver_user_id IS NULL
        OR requested_by_user_id IS NULL OR approver_user_id <> requested_by_user_id
    ),
    -- verified plans must reference a verification row
    CONSTRAINT action_plans_verified_link_chk CHECK (
        status != 'verified' OR verification_id IS NOT NULL
    )
);

-- At most one non-terminal plan per (case_id, runbook_id, target_uid).
-- Resolved/expired/cancelled plans retain their rows so historical queries
-- remain possible; a new non-terminal plan for the same key is admitted only
-- after the previous one is terminal.
CREATE UNIQUE INDEX IF NOT EXISTS uq_action_plans_active
    ON action_plans (case_id, runbook_id, target_uid)
    WHERE status IN ('draft','previewed','approved','executing','succeeded','failed');

-- Idempotency: at most one plan per (id, idempotency_key) once executing.
CREATE UNIQUE INDEX IF NOT EXISTS uq_action_plans_idempotency
    ON action_plans (id, idempotency_key)
    WHERE idempotency_key <> '';

-- Query indexes.
CREATE INDEX IF NOT EXISTS idx_action_plans_case
    ON action_plans (case_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_action_plans_cluster_namespace
    ON action_plans (cluster_id, target_namespace, target_kind, target_name);
CREATE INDEX IF NOT EXISTS idx_action_plans_target_uid
    ON action_plans (cluster_id, target_uid)
    WHERE target_uid <> '';
CREATE INDEX IF NOT EXISTS idx_action_plans_status
    ON action_plans (status, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_action_plans_investigation
    ON action_plans (investigation_id)
    WHERE investigation_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_action_plans_action_candidate
    ON action_plans (action_candidate_id)
    WHERE action_candidate_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_action_plans_claim
    ON action_plans (status, expires_at, locked_at);

-- FK to correlation_cases. A case can have multiple plans over time
-- (re-runs after new evidence); only one is non-terminal per (case, runbook,
-- target_uid).
ALTER TABLE action_plans
    ADD CONSTRAINT fk_action_plans_case
    FOREIGN KEY (case_id) REFERENCES correlation_cases (id) ON DELETE CASCADE;

CREATE TABLE IF NOT EXISTS action_verifications (
    id BIGSERIAL PRIMARY KEY,
    plan_id VARCHAR(36) NOT NULL,
    verification_key VARCHAR(64) NOT NULL,
    verifier_version VARCHAR(16) NOT NULL DEFAULT '1.0',
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    evidence_comparison VARCHAR(16) NOT NULL DEFAULT 'insufficient',
    pre_snapshot JSONB NOT NULL DEFAULT '{}'::JSONB,
    post_snapshot JSONB NOT NULL DEFAULT '{}'::JSONB,
    slo_evaluation_before_id BIGINT,
    slo_evaluation_after_id BIGINT,
    missing_evidence BOOLEAN NOT NULL DEFAULT FALSE,
    cooldown_seconds INTEGER NOT NULL DEFAULT 300,
    verified_at TIMESTAMPTZ,
    reason VARCHAR(500) NOT NULL DEFAULT '',
    rollback_triggered BOOLEAN NOT NULL DEFAULT FALSE,
    rollback_plan_id VARCHAR(36),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT action_verifications_status_chk CHECK (status IN (
        'pending','effective','ineffective','failed','unknown'
    )),
    CONSTRAINT action_verifications_evidence_comparison_chk CHECK (evidence_comparison IN (
        'improved','unchanged','worse','insufficient'
    )),
    CONSTRAINT action_verifications_verifier_version_chk CHECK (verifier_version = '1.0'),
    CONSTRAINT action_verifications_verification_key_chk CHECK (verification_key ~ '^[0-9a-f]{64}$'),
    CONSTRAINT action_verifications_cooldown_chk CHECK (
        cooldown_seconds >= 0 AND cooldown_seconds <= 3600
    ),
    -- missing evidence must yield insufficient comparison + unknown status
    CONSTRAINT action_verifications_missing_evidence_chk CHECK (
        NOT missing_evidence OR (evidence_comparison = 'insufficient' AND status = 'unknown')
    ),
    -- effective requires improved or unchanged (unchanged is only effective
    -- when the pre-state was already healthy, which the service validates)
    CONSTRAINT action_verifications_effective_chk CHECK (
        status != 'effective' OR evidence_comparison IN ('improved','unchanged')
    ),
    -- ineffective requires worse or unchanged (unchanged is ineffective when
    -- the pre-state was unhealthy, which the service validates)
    CONSTRAINT action_verifications_ineffective_chk CHECK (
        status != 'ineffective' OR evidence_comparison IN ('worse','unchanged')
    ),
    -- unknown requires insufficient
    CONSTRAINT action_verifications_unknown_chk CHECK (
        status != 'unknown' OR evidence_comparison = 'insufficient'
    ),
    -- failed is reserved for evidence-gathering failures (provider outage);
    -- reason must be non-empty
    CONSTRAINT action_verifications_failed_reason_chk CHECK (
        status != 'failed' OR reason <> ''
    ),
    -- rollback_plan_id is only set when rollback_triggered
    CONSTRAINT action_verifications_rollback_chk CHECK (
        NOT rollback_triggered OR rollback_plan_id IS NOT NULL OR reason <> ''
    )
);

-- One verification per plan. A plan can be re-verified after a retry; the
-- most recent non-pending verification is the authoritative one.
CREATE UNIQUE INDEX IF NOT EXISTS uq_action_verifications_plan
    ON action_verifications (plan_id, verification_key);

CREATE INDEX IF NOT EXISTS idx_action_verifications_plan
    ON action_verifications (plan_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_action_verifications_status
    ON action_verifications (status, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_action_verifications_rollback
    ON action_verifications (rollback_plan_id)
    WHERE rollback_plan_id IS NOT NULL;

-- FK to action_plans. Deleting a plan cascades to its verifications.
ALTER TABLE action_verifications
    ADD CONSTRAINT fk_action_verifications_plan
    FOREIGN KEY (plan_id) REFERENCES action_plans (id) ON DELETE CASCADE;

-- FK from action_plans.verification_id → action_verifications.id is intentionally
-- NOT added as a hard FK because the verification row is created after the
-- plan transitions to Succeeded/Failed; the service-level invariant plus the
-- status='verified' CHECK is the source of truth.

-- FK from action_plans.investigation_id → ai_investigations.id.
ALTER TABLE action_plans
    ADD CONSTRAINT fk_action_plans_investigation
    FOREIGN KEY (investigation_id) REFERENCES ai_investigations (id) ON DELETE SET NULL;
