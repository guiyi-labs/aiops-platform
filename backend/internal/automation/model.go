// Package automation implements the M44 policy-constrained automation and
// post-action verification layer. It is the safe-execution ceiling of the
// AIOps Intelligence Plane: every recommended runbook from M43 must pass
// deterministic policy gates, obtain human approval (L2 default), execute
// idempotently, and be verified against the same versioned SLI/resource
// evidence used before the action.
//
// M44 is NOT a generic workflow engine. It produces one bounded ActionPlan
// per recommended runbook:
//   - source: M42 correlation_case (+ optional M43 ai_investigation)
//   - runbook_id: a fixed server-owned catalog ID from M43
//   - action_code: an M19 controlled-operations code (deployment.rollback,
//     deployment.rollout_restart, etc.)
//   - target snapshot: kind/namespace/name/UID/resourceVersion captured at
//     preview time and rechecked before execute
//   - policy_gates: deterministic pre-execute checks (UID/RV, scope, PDB /
//     blast radius, SLO, freeze window, concurrent plans, attempt cap)
//   - approval: L2 human-confirmed by default; high-risk actions require
//     four-eyes and the requester cannot self-approve
//   - execution: idempotent claim with confirmation token, mirroring the
//     remediation/maintenance patterns
//   - verification: post-action SLI/resource comparison with explicit
//     improved/unchanged/worse/insufficient outcome; missing evidence never
//     resolves a diagnosis automatically
//
// Invariants:
//   - AI output never becomes client input directly. The recommended_runbook_id
//     is a fixed catalog ID; clients cannot supply arbitrary patches, images,
//     rollback revisions, or kubectl.
//   - The default automation ceiling is L2 (human-confirmed execution). L3
//     (pre-authorized automatic execution) requires a separate ADR, shadow
//     mode, narrow policy, canary, kill switch and user approval; it is not
//     implied by completing M43.
//   - Stale UID/RV, expired token, duplicate execution, wrong target,
//     unauthorized scope, unconfirmed plan, expired plan, exceeded attempt
//     cap, or active freeze window fail closed.
//   - Two workers and replay produce one business side effect (idempotent
//     claim with confirmation token hash and idempotency key).
//   - High-risk actions support four-eyes approval and the requester cannot
//     self-approve.
//   - Pre/post checks use the same versioned SLI/resource rules and preserve
//     evidence; missing evidence yields VerificationStatusUnknown, never
//     VerificationStatusEffective.
//   - A failed post-check follows only the server-owned rollback contract;
//     unsafe rollback stops and escalates to a human.
//   - Preview, approval, execute, verify and rollback share correlation /
//     request identities and complete safe audit metadata.
//   - Action outcome feeds offline quality evaluation but never self-modifies
//     rules, prompts or policy online.
package automation

import "time"

// AutomationVersion is the schema version of the automation contract.
// Bumped when the policy gate set, runbook eligibility, approval rules,
// verification rules or evidence schema changes. Identical source +
// policy + verification versions reproduce identical plans and
// verifications.
const AutomationVersion = "1.0"

// VerifierVersion is the schema version of the post-action verifier.
// Bumped when the SLI comparison rules, cooldown windows, evidence
// comparison, or rollback contract changes.
const VerifierVersion = "1.0"

// AutomationLevel is the L0/L1/L2/L3 automation ceiling. The default
// product closure is L2 (human-confirmed execution). L3 (pre-authorized
// automatic execution) is not enabled by completing M43; it requires a
// separate ADR with shadow mode, narrow policy, canary, kill switch and
// user approval.
type AutomationLevel string

const (
	// LevelL0 Observe only: no plan is created; the platform records
	// observation and recommends no action.
	LevelL0 AutomationLevel = "L0"
	// LevelL1 Deterministic/AI-assisted recommendation: a plan is
	// created and previewed, but never auto-executed.
	LevelL1 AutomationLevel = "L1"
	// LevelL2 Human-confirmed execution (default): a plan requires
	// explicit human approval before execute.
	LevelL2 AutomationLevel = "L2"
	// LevelL3 Pre-authorized automatic execution: gated by a separate
	// ADR. Not enabled by M44.
	LevelL3 AutomationLevel = "L3"
)

// PlanStatus is the lifecycle status of an action plan.
//
// Lifecycle:
//
//	draft → previewed → approved → executing → succeeded ──→ verified
//	                                                  ↘ failed ──→ verified
//	previewed → expired
//	previewed → cancelled
//	approved  → expired
//	approved  → cancelled
type PlanStatus string

const (
	// StatusDraft: plan created but policy gates not yet evaluated.
	StatusDraft PlanStatus = "draft"
	// StatusPreviewed: preview passed all required policy gates; awaiting
	// human approval.
	StatusPreviewed PlanStatus = "previewed"
	// StatusApproved: human approval recorded (single or four-eyes).
	StatusApproved PlanStatus = "approved"
	// StatusExecuting: idempotent claim taken; Kubernetes patch in flight.
	StatusExecuting PlanStatus = "executing"
	// StatusSucceeded: Kubernetes patch completed; awaiting verification.
	StatusSucceeded PlanStatus = "succeeded"
	// StatusFailed: Kubernetes patch failed; awaiting verification.
	StatusFailed PlanStatus = "failed"
	// StatusExpired: TTL elapsed before approval or execute.
	StatusExpired PlanStatus = "expired"
	// StatusCancelled: operator cancelled before execute.
	StatusCancelled PlanStatus = "cancelled"
	// StatusVerified: post-action verification completed (effective,
	// ineffective, failed or unknown recorded in the linked verification).
	StatusVerified PlanStatus = "verified"
)

// ApprovalType is the required approval mode for an action plan.
type ApprovalType string

const (
	// ApprovalSingle: one human approver (L2 default for low-risk actions).
	ApprovalSingle ApprovalType = "single"
	// ApprovalFourEyes: requester + a different approver (required for
	// high-risk actions such as deployment.rollback and image_update).
	ApprovalFourEyes ApprovalType = "four_eyes"
)

// GateStatus is the result of one policy gate check.
type GateStatus string

const (
	// GatePassed: the check passed.
	GatePassed GateStatus = "passed"
	// GateFailed: the check failed; the plan cannot proceed.
	GateFailed GateStatus = "failed"
	// GateSkipped: the check was not applicable (e.g. no PDB for the
	// target kind).
	GateSkipped GateStatus = "skipped"
)

// GateCode identifies a deterministic policy gate. Adding a gate is a
// contract change: the catalog and migration must be updated together.
type GateCode string

const (
	// GateUIDRVRecheck: target UID + resourceVersion must match the
	// snapshot captured at preview time. Stale target fails closed.
	GateUIDRVRecheck GateCode = "uid_rv_recheck"
	// GateScope: target cluster_id + namespace must be within the
	// requester's M35 grants.
	GateScope GateCode = "scope"
	// GatePDBBlastRadius: PodDisruptionBudget and blast-radius check
	// (only meaningful for actions that affect Pod placement).
	GatePDBBlastRadius GateCode = "pdb_blast_radius"
	// GateSLOBurn: active SLO burn state must not already be in a
	// breached window for unrelated reasons that the action would worsen.
	GateSLOBurn GateCode = "slo_burn"
	// GateFreezeWindow: no active maintenance/freeze window on the
	// target cluster or namespace.
	GateFreezeWindow GateCode = "freeze_window"
	// GateConcurrentPlans: no other non-terminal action plan targets the
	// same UID concurrently.
	GateConcurrentPlans GateCode = "concurrent_plans"
	// GateAttemptCap: per-target attempt count within the rolling window
	// must not exceed MaxAttemptsPerTarget.
	GateAttemptCap GateCode = "attempt_cap"
	// GateRollbackPoint: for rollback actions, a valid rollback point
	// (ReplicaSet revision) must exist and be different from current.
	GateRollbackPoint GateCode = "rollback_point"
)

// PolicyGate is the result of one policy gate check, captured at preview
// time and rechecked before execute. The gate set is versioned with the
// automation contract.
type PolicyGate struct {
	Code      GateCode   `json:"code"`
	Status    GateStatus `json:"status"`
	Reason    string     `json:"reason,omitempty"`
	CheckedAt time.Time  `json:"checked_at"`
	Rechecked bool       `json:"rechecked,omitempty"`
}

// ActorRef mirrors remediation.ActorRef so the automation package stays
// independent of the remediation package while preserving the same audit
// shape.
type ActorRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// TargetRef mirrors remediation.TargetRef.
type TargetRef struct {
	Kind            string `json:"kind"`
	Namespace       string `json:"namespace"`
	Name            string `json:"name"`
	UID             string `json:"uid"`
	ResourceVersion string `json:"resource_version"`
}

// OperationParameters mirrors remediation.OperationParameters. Only one
// of the action-specific fields is meaningful per action_code; the
// catalog enforces which fields are admitted.
type OperationParameters struct {
	DesiredReplicas  *int32 `json:"desired_replicas,omitempty"`
	DesiredSuspended *bool  `json:"desired_suspended,omitempty"`
	ContainerName    string `json:"container_name,omitempty"`
	BeforeImage      string `json:"before_image,omitempty"`
	DesiredImage     string `json:"desired_image,omitempty"`
	RollbackRevision *int32 `json:"rollback_revision,omitempty"`
}

// OperationChange mirrors remediation.OperationChange for the preview
// diff returned to the operator.
type OperationChange struct {
	Field  string `json:"field"`
	Before any    `json:"before"`
	After  any    `json:"after"`
}

// ActionPlan is the M44 entity. It links the M42 correlation case and the
// optional M43 AI investigation to a fixed runbook, captures the target
// snapshot, records the policy gate results, tracks approval, executes
// idempotently, and is linked to exactly one ActionVerification.
//
// The plan is the only writer-side artifact: clients cannot supply
// arbitrary patches; they request a runbook by ID and the service
// materializes the operation parameters from the case context.
type ActionPlan struct {
	ID                string `json:"id"`       // UUID v4
	PlanKey           string `json:"plan_key"` // SHA256 over (case_id + runbook_id + target_uid + automation_version)
	AutomationVersion string `json:"automation_version"`

	// Source linkage. case_id is required (M42); investigation_id is
	// optional (M43). action_candidate_id is the M42 change candidate
	// that this plan addresses (optional — the operator may pick any
	// eligible runbook, not only the candidate's).
	CaseID            int64  `json:"case_id"`
	InvestigationID   *int64 `json:"investigation_id,omitempty"`
	ActionCandidateID *int64 `json:"action_candidate_id,omitempty"`

	// Runbook + action. runbook_id must exist in the M43 catalog and be
	// eligible per the M42 Action Catalog at preview time.
	RunbookID  string `json:"runbook_id"`
	ActionCode string `json:"action_code"`
	ClusterID  int64  `json:"cluster_id"`

	// Target snapshot captured at preview time. UID/RV are rechecked
	// before execute; mismatch fails closed.
	TargetKind            string `json:"-"`
	TargetNamespace       string `json:"-"`
	TargetName            string `json:"-"`
	TargetUID             string `json:"-"`
	TargetResourceVersion string `json:"-"`

	// Operation parameters materialized from the case context.
	DesiredReplicas                   *int32 `json:"-"`
	BeforeReplicas                    *int32 `json:"-"`
	DesiredSuspended                  *bool  `json:"-"`
	BeforeSuspended                   *bool  `json:"-"`
	ContainerName                     string `json:"-"`
	BeforeImage                       string `json:"-"`
	DesiredImage                      string `json:"-"`
	RollbackRevision                  *int32 `json:"-"`
	RollbackReplicaSetName            string `json:"-"`
	RollbackReplicaSetUID             string `json:"-"`
	RollbackReplicaSetResourceVersion string `json:"-"`

	// Policy gate results captured at preview time and rechecked before
	// execute.
	PolicyGates []PolicyGate `json:"policy_gates"`

	// Status + approval.
	Status            PlanStatus      `json:"status"`
	Level             AutomationLevel `json:"level"`
	ApprovalType      ApprovalType    `json:"approval_type"`
	RequestedByUserID *int64          `json:"-"`
	RequestedByName   string          `json:"-"`
	ApproverUserID    *int64          `json:"-"`
	ApproverName      string          `json:"-"`
	ApprovedAt        *time.Time      `json:"approved_at,omitempty"`

	// Confirmation + idempotency (mirror remediation/maintenance).
	ConfirmationTokenHash []byte     `json:"-"`
	IdempotencyKey        string     `json:"-"`
	ExpiresAt             time.Time  `json:"expires_at"`
	LockedAt              *time.Time `json:"-"`
	ExecutedAt            *time.Time `json:"executed_at,omitempty"`
	AttemptCount          int        `json:"attempt_count"`
	LastError             string     `json:"last_error,omitempty"`

	// Verification linkage. Set when post-action verification completes.
	VerificationID *int64 `json:"verification_id,omitempty"`

	// Audit correlation. CorrelationRequestID is shared across
	// preview/approve/execute/verify so the audit trail is reconstructable.
	CorrelationRequestID string `json:"correlation_request_id"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// ConfirmationToken is only populated on preview responses; it is
	// never persisted and never returned after execute.
	ConfirmationToken string `json:"confirmation_token,omitempty"`
}

// ActionPlanResponse is the API shape returned to clients. It hides
// internal hashes and exposes the target, parameters, change preview and
// policy gates.
type ActionPlanResponse struct {
	ActionPlan
	Target      TargetRef           `json:"target"`
	RequestedBy ActorRef            `json:"requested_by"`
	Approver    ActorRef            `json:"approver"`
	Parameters  OperationParameters `json:"parameters"`
	Change      *OperationChange    `json:"change,omitempty"`
}

// Target returns the target ref.
func (p ActionPlan) Target() TargetRef {
	return TargetRef{Kind: p.TargetKind, Namespace: p.TargetNamespace, Name: p.TargetName, UID: p.TargetUID, ResourceVersion: p.TargetResourceVersion}
}

// RequestedBy returns the requester actor.
func (p ActionPlan) RequestedBy() ActorRef {
	id := int64(0)
	if p.RequestedByUserID != nil {
		id = *p.RequestedByUserID
	}
	return ActorRef{ID: id, Name: p.RequestedByName}
}

// Approver returns the approver actor (zero when not yet approved).
func (p ActionPlan) Approver() ActorRef {
	if p.ApproverUserID == nil {
		return ActorRef{}
	}
	return ActorRef{ID: *p.ApproverUserID, Name: p.ApproverName}
}

// VerificationStatus is the post-action verification outcome.
type VerificationStatus string

const (
	// VerificationStatusPending: verification scheduled but not yet
	// evaluated (cooldown not elapsed, or evidence not yet gathered).
	VerificationStatusPending VerificationStatus = "pending"
	// VerificationStatusEffective: post-action SLI/resource evidence
	// shows the action improved the situation and the diagnosis is
	// resolved.
	VerificationStatusEffective VerificationStatus = "effective"
	// VerificationStatusIneffective: post-action SLI/resource evidence
	// shows the action did not improve the situation.
	VerificationStatusIneffective VerificationStatus = "ineffective"
	// VerificationStatusFailed: post-action evidence gathering itself
	// failed (e.g. provider outage). The plan retains its execution
	// status; verification is retried by the operator.
	VerificationStatusFailed VerificationStatus = "failed"
	// VerificationStatusUnknown: post-action evidence is insufficient to
	// resolve the diagnosis. Missing evidence never auto-resolves a
	// diagnosis; the operator must review manually.
	VerificationStatusUnknown VerificationStatus = "unknown"
)

// EvidenceComparison records whether the post-action evidence is better,
// the same, worse or insufficient compared with the pre-action evidence.
type EvidenceComparison string

const (
	// ComparisonImproved: post-action SLI/resource state is strictly
	// better than pre-action (e.g. error rate dropped, SLO burn stopped).
	ComparisonImproved EvidenceComparison = "improved"
	// ComparisonUnchanged: post-action evidence is not materially
	// different from pre-action.
	ComparisonUnchanged EvidenceComparison = "unchanged"
	// ComparisonWorse: post-action evidence is worse than pre-action
	// (e.g. error rate increased, new pods crash-looping).
	ComparisonWorse EvidenceComparison = "worse"
	// ComparisonInsufficient: pre or post evidence is missing/partial;
	// no comparison is possible. VerificationStatus must be Unknown.
	ComparisonInsufficient EvidenceComparison = "insufficient"
)

// EvidenceSnapshot is a redacted, hash-stamped evidence snapshot used by
// the verifier. The verifier never accepts free-form evidence; only
// server-captured SLI/resource states are admitted.
type EvidenceSnapshot struct {
	// ResourceState captures the target's relevant fields at the
	// observation time (replicas, available_replicas, image, suspended,
	// etc.). Redacted.
	ResourceState map[string]any `json:"resource_state"`
	// SLOState captures the most recent SLO evaluation for the same
	// service/template, if one exists. May be nil.
	SLOState *SLOSnapshot `json:"slo_state,omitempty"`
	// CapturedAt is the server time the snapshot was taken.
	CapturedAt time.Time `json:"captured_at"`
	// ContentHash is the SHA-256 over the redacted JSON content so the
	// verification is replayable even if the underlying row is later
	// pruned.
	ContentHash string `json:"content_hash"`
}

// SLOSnapshot is a redacted slice of an SLO evaluation used by the
// verifier. The verifier never evaluates PromQL/LogQL; it only compares
// server-captured evaluation states.
type SLOSnapshot struct {
	SLOID            int64     `json:"slo_id"`
	Version          int       `json:"version"`
	Template         string    `json:"template"`
	State            string    `json:"state"`
	Coverage         string    `json:"coverage"`
	ErrorBudgetRatio float64   `json:"error_budget_ratio"`
	BurnRate         float64   `json:"burn_rate"`
	EvaluatedAt      time.Time `json:"evaluated_at"`
}

// ActionVerification is the post-action verification entity. One
// verification per action plan; it is created when the plan transitions
// to Succeeded or Failed and is updated when the cooldown elapses.
type ActionVerification struct {
	ID                 int64              `json:"id"`
	PlanID             string             `json:"plan_id"`
	VerificationKey    string             `json:"verification_key"` // SHA256 over (plan_id + verifier_version + evidence_hash)
	VerifierVersion    string             `json:"verifier_version"`
	Status             VerificationStatus `json:"status"`
	EvidenceComparison EvidenceComparison `json:"evidence_comparison"`
	PreSnapshot        EvidenceSnapshot   `json:"pre_snapshot"`
	PostSnapshot       EvidenceSnapshot   `json:"post_snapshot"`
	// SLOEvaluationBeforeID/AfterID point at slo_evaluations rows; nil
	// when no evaluation exists for the service/template.
	SLOEvaluationBeforeID *int64 `json:"slo_evaluation_before_id,omitempty"`
	SLOEvaluationAfterID  *int64 `json:"slo_evaluation_after_id,omitempty"`
	// MissingEvidence is true when pre or post evidence is unavailable
	// or partial. MissingEvidence → ComparisonInsufficient + StatusUnknown.
	MissingEvidence bool `json:"missing_evidence"`
	// CooldownSeconds is the wait between execute and post-snapshot.
	CooldownSeconds int `json:"cooldown_seconds"`
	// VerifiedAt is set when Status leaves Pending.
	VerifiedAt *time.Time `json:"verified_at,omitempty"`
	// Reason is set when Status is Failed or Unknown.
	Reason string `json:"reason,omitempty"`
	// RollbackTriggered is true when a failed/ineffective verification
	// triggered the server-owned rollback contract.
	RollbackTriggered bool `json:"rollback_triggered,omitempty"`
	// RollbackPlanID is set when RollbackTriggered is true and a
	// rollback action plan was created. Unsafe rollback stops and
	// escalates to a human (no rollback plan is created).
	RollbackPlanID *string   `json:"rollback_plan_id,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// ActionPlanFilter bounds action plan queries.
type ActionPlanFilter struct {
	CaseID    int64
	ClusterID int64
	Namespace string
	Status    PlanStatus
	RunbookID string
	Limit     int
}

// ActionPlanListResponse is the bounded paginated response.
type ActionPlanListResponse struct {
	Items     []ActionPlanResponse `json:"items"`
	Total     int64                `json:"total"`
	Truncated bool                 `json:"truncated,omitempty"`
}

// QualityReport summarizes automation quality for offline evaluation. It
// never self-modifies rules, prompts or policy online.
type QualityReport struct {
	TotalPlans               int64     `json:"total_plans"`
	ApprovedPlans            int64     `json:"approved_plans"`
	ExecutedPlans            int64     `json:"executed_plans"`
	SucceededPlans           int64     `json:"succeeded_plans"`
	FailedPlans              int64     `json:"failed_plans"`
	EffectiveVerifications   int64     `json:"effective_verifications"`
	IneffectiveVerifications int64     `json:"ineffective_verifications"`
	UnknownVerifications     int64     `json:"unknown_verifications"`
	PolicyGatePassRate       float64   `json:"policy_gate_pass_rate"`
	FourEyesApprovalRate     float64   `json:"four_eyes_approval_rate"`
	EffectiveRate            float64   `json:"effective_rate"`
	GeneratedAt              time.Time `json:"generated_at"`
}

// Bounds enforced by the catalog and migration CHECK constraints.
const (
	// MaxAttemptsPerTarget caps the number of action plans per target
	// UID within AttemptWindowSeconds. Exceeding the cap fails the
	// attempt_cap gate closed.
	MaxAttemptsPerTarget = 5
	AttemptWindowSeconds = 3600 // 1 hour

	// MaxPolicyGatesPerPlan bounds the policy_gates JSONB array.
	MaxPolicyGatesPerPlan = 16

	// MaxPlansPerCase bounds the number of non-terminal plans per case.
	MaxPlansPerCase = 8

	// MaxRunbookIDLength matches M43.
	MaxRunbookIDLength = 128

	// MaxPlanKeyLength is the SHA-256 hex length.
	MaxPlanKeyLength = 64

	// MaxVerificationKeyLength is the SHA-256 hex length.
	MaxVerificationKeyLength = 64

	// DefaultPlanTTL bounds the preview→approve→execute window.
	DefaultPlanTTLSeconds = 600 // 10 minutes

	// DefaultClaimTTL bounds the executing state before a replay is
	// allowed.
	DefaultClaimTTLSeconds = 60

	// DefaultCooldownSeconds bounds the wait between execute and
	// post-snapshot.
	DefaultCooldownSeconds = 300 // 5 minutes

	// MinCooldownSeconds is the minimum cooldown. Below this the
	// verifier returns VerificationStatusUnknown.
	MinCooldownSeconds = 60
)
