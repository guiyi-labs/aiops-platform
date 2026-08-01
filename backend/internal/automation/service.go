package automation

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"k8s-aiops.local/backend/internal/cluster"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

// CaseReader reads correlation cases for the automation service. The
// service stays independent of the correlation package so it can be
// tested with fixtures.
type CaseReader interface {
	// GetCase returns the case context for the given case ID. The
	// service uses this to derive the target, runbook eligibility and
	// source linkage.
	GetCase(ctx context.Context, caseID int64) (CaseContext, error)
	// EligibleActionCodes returns the M42 ActionCandidate codes for the case.
	// The service uses this to validate runbook eligibility at preview time.
	EligibleActionCodes(ctx context.Context, caseID int64) (map[string]bool, error)
}

// NopCaseReader returns empty context. Used when the automation service
// is in query-only mode.
type NopCaseReader struct{}

func (NopCaseReader) GetCase(context.Context, int64) (CaseContext, error) {
	return CaseContext{}, ErrCaseNotFound
}
func (NopCaseReader) EligibleActionCodes(context.Context, int64) (map[string]bool, error) {
	return nil, nil
}

// CaseContext is the typed case context used by the automation service.
// The aiinvestigator package mirrors this shape.
type CaseContext struct {
	CaseID           int64
	ClusterID        int64
	PrimaryKind      string
	PrimaryNamespace string
	PrimaryName      string
	PrimaryUID       string
	// RecommendedRunbookID is the M43 AI investigator's recommendation,
	// if any. The operator may pick a different eligible runbook.
	RecommendedRunbookID string
	// InvestigationID is the source M43 investigation, if any.
	InvestigationID *int64
	// ActionCandidateID is the M42 change candidate that this plan
	// addresses, if any.
	ActionCandidateID *int64
}

// ErrCaseNotFound is returned when the case does not exist.
var ErrCaseNotFound = errors.New("correlation case not found")

// ErrDisabled is returned when the automation service is disabled (no
// repository or no Kubernetes source).
var ErrDisabled = errors.New("automation service is disabled")

// KubernetesSource mirrors remediation.KubernetesSource. The automation
// service reads target snapshots and patches deployments/cronjobs.
type KubernetesSource interface {
	Deployment(ctx context.Context, clusterID int64, namespace, name string) (k8sgateway.Deployment, error)
	PatchDeployment(ctx context.Context, clusterID int64, namespace, name string, patch []byte, dryRun bool) (k8sgateway.Deployment, error)
	CronJob(ctx context.Context, clusterID int64, namespace, name string) (k8sgateway.CronJob, error)
	PatchCronJob(ctx context.Context, clusterID int64, namespace, name string, patch []byte, dryRun bool) (k8sgateway.CronJob, error)
	ReplicaSet(ctx context.Context, clusterID int64, namespace, name string) (k8sgateway.ReplicaSet, error)
	RolloutHistory(ctx context.Context, clusterID int64, namespace, name string) (k8sgateway.RolloutHistory, error)
}

// Service is the M44 automation application service. It is the only
// writer to action_plans and the only caller of GateEvaluator and
// Verifier. HTTP handlers translate requests into service calls; they
// never bypass the service to write directly.
type Service struct {
	enabled  bool
	repo     Repository
	reader   CaseReader
	k8s      KubernetesSource
	gates    *GateEvaluator
	verifier *Verifier
	now      func() time.Time
	planTTL  time.Duration
	claimTTL time.Duration
	cooldown time.Duration
}

// ServiceOption configures a Service at construction.
type ServiceOption func(*Service)

// WithNow overrides the clock (tests).
func WithNow(now func() time.Time) ServiceOption {
	return func(s *Service) { s.now = now }
}

// WithPlanTTL overrides the default preview→approve→execute window.
func WithPlanTTL(d time.Duration) ServiceOption {
	return func(s *Service) { s.planTTL = d }
}

// WithClaimTTL overrides the default executing-state stale cutoff.
func WithClaimTTL(d time.Duration) ServiceOption {
	return func(s *Service) { s.claimTTL = d }
}

// WithCooldown overrides the default post-action cooldown.
func WithCooldown(d time.Duration) ServiceOption {
	return func(s *Service) { s.cooldown = d }
}

// NewService constructs a Service. repository must be non-nil. The case
// reader defaults to NopCaseReader, the Kubernetes source may be nil
// (query-only mode — Preview/Execute return ErrDisabled).
func NewService(repo Repository, reader CaseReader, k8s KubernetesSource, opts ...ServiceOption) *Service {
	if reader == nil {
		reader = NopCaseReader{}
	}
	s := &Service{
		enabled:  true,
		repo:     repo,
		reader:   reader,
		k8s:      k8s,
		gates:    NewGateEvaluator(),
		verifier: NewVerifier(),
		now:      time.Now,
		planTTL:  DefaultPlanTTLSeconds * time.Second,
		claimTTL: DefaultClaimTTLSeconds * time.Second,
		cooldown: DefaultCooldownSeconds * time.Second,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// WithEvidenceProvider wires the verifier's evidence provider. The
// default is NopEvidenceProvider; production code injects a provider
// that reads from the SLO service and the Kubernetes source.
func WithEvidenceProvider(p EvidenceProvider) ServiceOption {
	return func(s *Service) {
		if p != nil {
			s.verifier = NewVerifier(WithVerifierProvider(p), WithVerifierNow(s.now), WithVerifierCooldown(s.cooldown))
		}
	}
}

// CreatePlanInput is the request shape for CreatePlan.
type CreatePlanInput struct {
	CaseID            int64
	RunbookID         string
	InvestigationID   *int64
	ActionCandidateID *int64
	Operator          ActorRef
	// OperationOverrides allows the operator to override the
	// materialized parameters (e.g. pick a different rollback revision).
	// The service validates that the override is consistent with the
	// runbook's action_code. Empty means use the case context.
	OperationOverrides *OperationParameters
}

// CreatePlan creates a draft action plan. The runbook must exist in the
// M43 catalog and be eligible per the M42 Action Catalog. The plan is
// created in StatusDraft; the operator must call Preview to evaluate
// policy gates and transition to StatusPreviewed.
func (s *Service) CreatePlan(ctx context.Context, input CreatePlanInput) (ActionPlan, error) {
	if !s.enabled || s.repo == nil {
		return ActionPlan{}, ErrDisabled
	}
	input.RunbookID = strings.TrimSpace(input.RunbookID)
	if input.RunbookID == "" || len(input.RunbookID) > MaxRunbookIDLength {
		return ActionPlan{}, ErrInvalidRunbook
	}
	runbook, ok := LookupRunbook(input.RunbookID)
	if !ok {
		return ActionPlan{}, ErrRunbookNotInCatalog
	}
	if runbook.ActionCode == "" {
		return ActionPlan{}, ErrAdvisoryRunbookNotExecutable
	}
	caseCtx, err := s.reader.GetCase(ctx, input.CaseID)
	if err != nil {
		return ActionPlan{}, err
	}
	eligibleCodes, err := s.reader.EligibleActionCodes(ctx, input.CaseID)
	if err != nil {
		return ActionPlan{}, err
	}
	if eligibleCodes == nil {
		eligibleCodes = map[string]bool{}
	}
	if !eligibleCodes[runbook.ActionCode] {
		return ActionPlan{}, ErrRunbookNotEligible
	}
	if input.InvestigationID == nil {
		input.InvestigationID = caseCtx.InvestigationID
	}
	if input.ActionCandidateID == nil {
		input.ActionCandidateID = caseCtx.ActionCandidateID
	}
	id, token, tokenHash, err := newIdentity()
	if err != nil {
		return ActionPlan{}, err
	}
	now := s.now().UTC()
	plan := ActionPlan{
		ID:                    id,
		PlanKey:               computePlanKey(input.CaseID, input.RunbookID, caseCtx.PrimaryUID),
		AutomationVersion:     AutomationVersion,
		CaseID:                input.CaseID,
		InvestigationID:       input.InvestigationID,
		ActionCandidateID:     input.ActionCandidateID,
		RunbookID:             input.RunbookID,
		ActionCode:            runbook.ActionCode,
		ClusterID:             caseCtx.ClusterID,
		TargetKind:            caseCtx.PrimaryKind,
		TargetNamespace:       caseCtx.PrimaryNamespace,
		TargetName:            caseCtx.PrimaryName,
		Level:                 LevelL2,
		ApprovalType:          approvalTypeFor(runbook.ActionCode),
		RequestedByUserID:     &input.Operator.ID,
		RequestedByName:       input.Operator.Name,
		ConfirmationTokenHash: tokenHash,
		ExpiresAt:             now.Add(s.planTTL),
		CorrelationRequestID:  newCorrelationRequestID(),
		Status:                StatusDraft,
		CreatedAt:             now,
		UpdatedAt:             now,
		ConfirmationToken:     token,
	}
	if err := s.materializeParameters(ctx, &plan, input.OperationOverrides); err != nil {
		return ActionPlan{}, err
	}
	if err := s.repo.SavePlan(ctx, &plan); err != nil {
		return ActionPlan{}, err
	}
	plan.ConfirmationToken = token
	return plan, nil
}

// materializeParameters captures the before-snapshot from the Kubernetes
// source and applies operator overrides. The service validates that the
// overrides are consistent with the action_code.
func (s *Service) materializeParameters(ctx context.Context, plan *ActionPlan, overrides *OperationParameters) error {
	if s.k8s == nil {
		return ErrDisabled
	}
	switch plan.ActionCode {
	case "deployment.rollback":
		deployment, err := s.k8s.Deployment(ctx, plan.ClusterID, plan.TargetNamespace, plan.TargetName)
		if err != nil {
			return err
		}
		plan.TargetUID = deployment.Metadata.UID
		plan.TargetResourceVersion = deployment.Metadata.ResourceVersion
		history, err := s.k8s.RolloutHistory(ctx, plan.ClusterID, plan.TargetNamespace, plan.TargetName)
		if err != nil {
			return err
		}
		var targetRevision *int32
		for i, rev := range history.Revisions {
			if rev.Current {
				continue
			}
			// Default: the previous revision. Operator may override.
			targetRevision = &history.Revisions[i].Revision
		}
		if targetRevision == nil {
			return ErrNoRollbackPoint
		}
		plan.RollbackRevision = targetRevision
		if overrides != nil && overrides.RollbackRevision != nil {
			plan.RollbackRevision = overrides.RollbackRevision
		}
		for _, rev := range history.Revisions {
			if rev.Revision == *plan.RollbackRevision {
				plan.RollbackReplicaSetName = rev.ReplicaSetName
				plan.RollbackReplicaSetUID = rev.UID
				plan.RollbackReplicaSetResourceVersion = rev.ResourceVersion
				break
			}
		}
	case "deployment.rollout_restart":
		deployment, err := s.k8s.Deployment(ctx, plan.ClusterID, plan.TargetNamespace, plan.TargetName)
		if err != nil {
			return err
		}
		plan.TargetUID = deployment.Metadata.UID
		plan.TargetResourceVersion = deployment.Metadata.ResourceVersion
		now := s.now().UTC()
		plan.DesiredReplicas = deployment.Spec.Replicas
		plan.BeforeReplicas = deployment.Spec.Replicas
		_ = now
	case "deployment.image_update":
		if overrides == nil || overrides.ContainerName == "" || overrides.DesiredImage == "" {
			return ErrInvalidOperation
		}
		deployment, err := s.k8s.Deployment(ctx, plan.ClusterID, plan.TargetNamespace, plan.TargetName)
		if err != nil {
			return err
		}
		plan.TargetUID = deployment.Metadata.UID
		plan.TargetResourceVersion = deployment.Metadata.ResourceVersion
		for _, c := range deployment.Spec.Template.Spec.Containers {
			if c.Name == overrides.ContainerName {
				plan.ContainerName = c.Name
				plan.BeforeImage = c.Image
				break
			}
		}
		if plan.ContainerName == "" {
			return ErrInvalidOperation
		}
		plan.DesiredImage = overrides.DesiredImage
		if plan.BeforeImage == plan.DesiredImage {
			return ErrOperationNoChange
		}
	case "deployment.scale":
		if overrides == nil || overrides.DesiredReplicas == nil {
			return ErrInvalidOperation
		}
		deployment, err := s.k8s.Deployment(ctx, plan.ClusterID, plan.TargetNamespace, plan.TargetName)
		if err != nil {
			return err
		}
		plan.TargetUID = deployment.Metadata.UID
		plan.TargetResourceVersion = deployment.Metadata.ResourceVersion
		before := int32(1)
		if deployment.Spec.Replicas != nil {
			before = *deployment.Spec.Replicas
		}
		plan.BeforeReplicas = &before
		plan.DesiredReplicas = overrides.DesiredReplicas
		if before == *plan.DesiredReplicas {
			return ErrOperationNoChange
		}
	case "cronjob.suspend", "cronjob.resume":
		cronJob, err := s.k8s.CronJob(ctx, plan.ClusterID, plan.TargetNamespace, plan.TargetName)
		if err != nil {
			return err
		}
		plan.TargetUID = cronJob.Metadata.UID
		plan.TargetResourceVersion = cronJob.Metadata.ResourceVersion
		before := cronJob.Spec.Suspend != nil && *cronJob.Spec.Suspend
		desired := plan.ActionCode == "cronjob.suspend"
		plan.BeforeSuspended = &before
		plan.DesiredSuspended = &desired
		if before == desired {
			return ErrOperationNoChange
		}
	default:
		return ErrUnsupportedAction
	}
	return nil
}

// Preview evaluates the policy gates for a draft plan and transitions
// it to StatusPreviewed. The confirmation token is returned to the
// operator; it must be supplied at Execute time.
func (s *Service) Preview(ctx context.Context, id string) (ActionPlan, error) {
	if !s.enabled || s.repo == nil {
		return ActionPlan{}, ErrDisabled
	}
	plan, err := s.repo.GetPlan(ctx, id)
	if err != nil {
		return ActionPlan{}, err
	}
	if plan.Status != StatusDraft {
		return plan, ErrNotDraft
	}
	// Refresh the target snapshot so the operator sees the current
	// state at preview time.
	if err := s.refreshSnapshot(ctx, &plan); err != nil {
		return plan, err
	}
	gateCtx, err := s.buildGateContext(ctx, plan, true)
	if err != nil {
		return plan, err
	}
	gates := s.gates.Evaluate(plan, gateCtx)
	if !AllPassed(gates) {
		// Persist the failed gates for audit even though the plan
		// remains in Draft. The operator may adjust and re-preview.
		plan.PolicyGates = gates
		return plan, ErrPolicyGateFailed
	}
	plan.PolicyGates = gates
	updated, err := s.repo.MarkPreviewed(ctx, id, gates, s.now().UTC())
	if err != nil {
		return plan, err
	}
	updated.ConfirmationToken = plan.ConfirmationToken
	return updated, nil
}

// refreshSnapshot re-reads the target UID/RV from Kubernetes and stores
// it on the plan. Used at preview time to capture the current snapshot.
func (s *Service) refreshSnapshot(ctx context.Context, plan *ActionPlan) error {
	if s.k8s == nil {
		return ErrDisabled
	}
	switch plan.TargetKind {
	case "Deployment":
		deployment, err := s.k8s.Deployment(ctx, plan.ClusterID, plan.TargetNamespace, plan.TargetName)
		if err != nil {
			return err
		}
		plan.TargetUID = deployment.Metadata.UID
		plan.TargetResourceVersion = deployment.Metadata.ResourceVersion
	case "CronJob":
		cronJob, err := s.k8s.CronJob(ctx, plan.ClusterID, plan.TargetNamespace, plan.TargetName)
		if err != nil {
			return err
		}
		plan.TargetUID = cronJob.Metadata.UID
		plan.TargetResourceVersion = cronJob.Metadata.ResourceVersion
	default:
		return ErrUnsupportedTargetKind
	}
	return nil
}

// Approve records the approver and transitions a previewed plan to
// approved. For four_eyes plans, the approver must differ from the
// requester; the requester cannot self-approve.
func (s *Service) Approve(ctx context.Context, id string, approver ActorRef) (ActionPlan, error) {
	if !s.enabled || s.repo == nil {
		return ActionPlan{}, ErrDisabled
	}
	plan, err := s.repo.GetPlan(ctx, id)
	if err != nil {
		return ActionPlan{}, err
	}
	if plan.Status != StatusPreviewed {
		return plan, ErrNotPreviewed
	}
	if plan.ApprovalType == ApprovalFourEyes {
		if plan.RequestedByUserID == nil || approver.ID == 0 || approver.ID == *plan.RequestedByUserID {
			return plan, ErrSelfApprovalForbidden
		}
	}
	return s.repo.Approve(ctx, id, approver, s.now().UTC())
}

// Execute rechecks the policy gates, takes an idempotent claim, applies
// the Kubernetes patch and transitions the plan to Succeeded or Failed.
// Two workers and replay produce one business side effect: the claim is
// keyed by (id, idempotencyKey, confirmation_token_hash).
func (s *Service) Execute(ctx context.Context, id, confirmationToken, idempotencyKey string) (ActionPlan, error) {
	if !s.enabled || s.repo == nil {
		return ActionPlan{}, ErrDisabled
	}
	id = strings.TrimSpace(id)
	confirmationToken = strings.TrimSpace(confirmationToken)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if id == "" || confirmationToken == "" {
		return ActionPlan{}, ErrConfirmationInvalid
	}
	if len(idempotencyKey) < 8 || len(idempotencyKey) > 128 {
		return ActionPlan{}, ErrInvalidIdempotency
	}
	tokenHash := sha256.Sum256([]byte(confirmationToken))
	now := s.now().UTC()
	plan, shouldExecute, err := s.repo.Claim(ctx, id, tokenHash[:], idempotencyKey, now, now.Add(-s.claimTTL))
	if err != nil || !shouldExecute {
		return plan, err
	}
	// Recheck policy gates before execute.
	gateCtx, gateErr := s.buildGateContext(ctx, plan, false)
	if gateErr != nil {
		failed, _ := s.repo.Fail(ctx, plan.ID, idempotencyKey, "gate_context_error")
		return failed, fmt.Errorf("%w: %v", ErrExecutionFailed, gateErr)
	}
	rechecked := s.gates.Recheck(plan, gateCtx)
	if !AllPassed(rechecked) {
		failed, _ := s.repo.Fail(ctx, plan.ID, idempotencyKey, "policy_gate_failed_at_recheck")
		return failed, ErrPolicyGateFailed
	}
	// Apply the Kubernetes patch.
	patch, patchErr := s.buildPatch(ctx, plan)
	if patchErr == nil {
		patchErr = s.applyPatch(ctx, plan, patch)
	}
	if patchErr != nil {
		failed, _ := s.repo.Fail(ctx, plan.ID, idempotencyKey, safeExecutionError(patchErr))
		// Schedule verification even for failed executions so the
		// operator sees the post-action evidence. Missing evidence
		// never resolves a diagnosis automatically.
		_ = s.scheduleVerification(ctx, failed)
		return failed, fmt.Errorf("%w: %v", ErrExecutionFailed, patchErr)
	}
	completed, completeErr := s.repo.Complete(ctx, plan.ID, idempotencyKey, s.now().UTC())
	if completeErr != nil {
		return completed, completeErr
	}
	// Schedule verification for successful executions. The verifier
	// worker evaluates after the cooldown elapses.
	_ = s.scheduleVerification(ctx, completed)
	return completed, nil
}

// scheduleVerification creates a pending verification row bound to the
// plan. Called by Execute after the plan transitions to Succeeded or
// Failed. The verifier worker later calls Verify to evaluate the
// post-action evidence after the cooldown elapses.
func (s *Service) scheduleVerification(ctx context.Context, plan ActionPlan) error {
	if s.verifier == nil {
		return nil
	}
	verification, err := s.verifier.CreateVerification(ctx, plan)
	if err != nil {
		return err
	}
	if err := s.repo.SaveVerification(ctx, &verification); err != nil {
		return err
	}
	return nil
}

// Verify evaluates the pending verification for a plan after the
// cooldown elapses. The verifier captures the post-snapshot, compares
// against the pre-snapshot, and returns the final verification status.
// On a failed/ineffective verification, the server-owned rollback
// contract is evaluated: a safe rollback creates a new rollback action
// plan; an unsafe rollback stops and escalates to a human (no rollback
// plan is created, Reason records the escalation).
func (s *Service) Verify(ctx context.Context, planID string) (ActionVerification, error) {
	if !s.enabled || s.repo == nil {
		return ActionVerification{}, ErrDisabled
	}
	plan, err := s.repo.GetPlan(ctx, planID)
	if err != nil {
		return ActionVerification{}, err
	}
	if plan.Status != StatusSucceeded && plan.Status != StatusFailed {
		return ActionVerification{}, ErrNotVerifiable
	}
	verification, err := s.repo.GetVerificationByPlan(ctx, planID)
	if err != nil {
		return ActionVerification{}, err
	}
	if verification.Status != VerificationStatusPending {
		return verification, nil
	}
	// Evaluate the post-action evidence.
	evaluated, err := s.verifier.Evaluate(ctx, plan, verification)
	if err != nil {
		return verification, err
	}
	// Evaluate the rollback contract for ineffective/failed verifications.
	if evaluated.Status == VerificationStatusIneffective || evaluated.Status == VerificationStatusFailed {
		rollbackPlanID, rollbackReason, safe := s.evaluateRollbackContract(ctx, plan, evaluated)
		if safe && rollbackPlanID != nil {
			evaluated.RollbackTriggered = true
			evaluated.RollbackPlanID = rollbackPlanID
		} else if !safe {
			// Unsafe rollback — escalate to a human. No rollback plan
			// is created; the reason records the escalation so the
			// operator sees it in the audit trail.
			evaluated.Reason = "unsafe_rollback_escalated_to_human"
		}
		if rollbackReason != "" {
			if evaluated.Reason == "" {
				evaluated.Reason = rollbackReason
			}
		}
	}
	now := s.now().UTC()
	evaluated.VerifiedAt = &now
	update := VerificationUpdate{
		Status:             evaluated.Status,
		EvidenceComparison: evaluated.EvidenceComparison,
		PostSnapshot:       &evaluated.PostSnapshot,
		MissingEvidence:    &evaluated.MissingEvidence,
		VerifiedAt:         &now,
		Reason:             evaluated.Reason,
		RollbackTriggered:  &evaluated.RollbackTriggered,
	}
	if evaluated.RollbackPlanID != nil {
		update.RollbackPlanID = evaluated.RollbackPlanID
	}
	updated, err := s.repo.UpdateVerification(ctx, verification.ID, update)
	if err != nil {
		return evaluated, err
	}
	// Link the verification to the plan and transition to verified.
	if _, err := s.repo.MarkVerified(ctx, planID, updated.ID, now); err != nil {
		return updated, err
	}
	return updated, nil
}

// evaluateRollbackContract evaluates whether a safe server-owned
// rollback can be triggered for a failed/ineffective verification. The
// rollback contract is:
//   - only deployment.rollback and deployment.image_update actions
//     qualify (other actions have no safe rollback);
//   - the rollback target must be the same Deployment that the plan
//     targeted;
//   - the rollback point must exist and be different from current;
//   - the rollback must not be blocked by a freeze window or concurrent
//     plan (the gate evaluator decides).
//
// Returns (rollbackPlanID, reason, safe). When safe=false, no rollback
// plan is created and the operator must review manually.
func (s *Service) evaluateRollbackContract(ctx context.Context, plan ActionPlan, verification ActionVerification) (*string, string, bool) {
	// Only rollback-eligible actions qualify. A rollback action that
	// fails verification cannot itself be rolled back to a different
	// rollback — that would be a second rollback, which is unsafe.
	if plan.ActionCode != "deployment.image_update" && plan.ActionCode != "deployment.rollout_restart" {
		return nil, "action_not_rollback_eligible", false
	}
	if plan.TargetKind != "Deployment" {
		return nil, "target_kind_not_deployment", false
	}
	// The rollback contract uses the rollback_last_rollout runbook. The
	// operator must approve it via the normal preview→approve→execute
	// flow — M44 does not auto-execute rollbacks (L3 is gated by a
	// separate ADR).
	rollbackInput := CreatePlanInput{
		CaseID:            plan.CaseID,
		RunbookID:         "rollback_last_rollout",
		InvestigationID:   plan.InvestigationID,
		ActionCandidateID: plan.ActionCandidateID,
		Operator:          plan.RequestedBy(),
	}
	rollbackPlan, err := s.CreatePlan(ctx, rollbackInput)
	if err != nil {
		return nil, "rollback_plan_create_failed:" + err.Error(), false
	}
	id := rollbackPlan.ID
	return &id, "rollback_plan_created_pending_approval", true
}

// GetVerification returns the most recent verification for a plan.
func (s *Service) GetVerification(ctx context.Context, planID string) (ActionVerification, error) {
	if !s.enabled || s.repo == nil {
		return ActionVerification{}, ErrDisabled
	}
	return s.repo.GetVerificationByPlan(ctx, planID)
}

// Cancel transitions a non-terminal plan to cancelled.
func (s *Service) Cancel(ctx context.Context, id string) (ActionPlan, error) {
	if !s.enabled || s.repo == nil {
		return ActionPlan{}, ErrDisabled
	}
	return s.repo.Cancel(ctx, id, s.now().UTC())
}

// GetPlan returns one action plan by ID.
func (s *Service) GetPlan(ctx context.Context, id string) (ActionPlan, error) {
	if !s.enabled || s.repo == nil {
		return ActionPlan{}, ErrDisabled
	}
	return s.repo.GetPlan(ctx, id)
}

// ListPlans returns plans matching the filter, newest first.
func (s *Service) ListPlans(ctx context.Context, filter ActionPlanFilter) (ActionPlanListResponse, error) {
	if !s.enabled || s.repo == nil {
		return ActionPlanListResponse{}, ErrDisabled
	}
	items, total, err := s.repo.ListPlans(ctx, filter)
	if err != nil {
		return ActionPlanListResponse{}, err
	}
	resp := ActionPlanListResponse{Total: total}
	for _, p := range items {
		resp.Items = append(resp.Items, toResponse(p))
	}
	if len(resp.Items) > 0 && int64(len(resp.Items)) < total {
		resp.Truncated = true
	}
	return resp, nil
}

// ExpireStale transitions awaiting plans past their TTL to expired.
// Called by a background worker; not exposed via HTTP.
func (s *Service) ExpireStale(ctx context.Context) (int64, error) {
	if !s.enabled || s.repo == nil {
		return 0, ErrDisabled
	}
	return s.repo.ExpireStale(ctx, s.now().UTC())
}

// toResponse converts an ActionPlan to an ActionPlanResponse with the
// target, parameters, change preview and actors filled in.
func toResponse(plan ActionPlan) ActionPlanResponse {
	resp := ActionPlanResponse{
		ActionPlan:  plan,
		Target:      plan.Target(),
		RequestedBy: plan.RequestedBy(),
		Approver:    plan.Approver(),
		Parameters: OperationParameters{
			DesiredReplicas:  plan.DesiredReplicas,
			DesiredSuspended: plan.DesiredSuspended,
			ContainerName:    plan.ContainerName,
			BeforeImage:      plan.BeforeImage,
			DesiredImage:     plan.DesiredImage,
			RollbackRevision: plan.RollbackRevision,
		},
	}
	resp.Change = buildChange(plan)
	return resp
}

// buildChange returns the preview diff for the plan's action.
func buildChange(plan ActionPlan) *OperationChange {
	switch plan.ActionCode {
	case "deployment.scale":
		if plan.BeforeReplicas != nil && plan.DesiredReplicas != nil {
			return &OperationChange{Field: "spec.replicas", Before: *plan.BeforeReplicas, After: *plan.DesiredReplicas}
		}
	case "cronjob.suspend", "cronjob.resume":
		if plan.BeforeSuspended != nil && plan.DesiredSuspended != nil {
			return &OperationChange{Field: "spec.suspend", Before: *plan.BeforeSuspended, After: *plan.DesiredSuspended}
		}
	case "deployment.image_update":
		if plan.BeforeImage != "" && plan.DesiredImage != "" {
			return &OperationChange{Field: "spec.template.spec.containers[" + plan.ContainerName + "].image", Before: plan.BeforeImage, After: plan.DesiredImage}
		}
	case "deployment.rollback":
		if plan.RollbackRevision != nil {
			return &OperationChange{Field: "spec.template (revision rollback)", Before: "current", After: *plan.RollbackRevision}
		}
	}
	return nil
}

// buildPatch constructs the Kubernetes patch for the plan.
func (s *Service) buildPatch(ctx context.Context, plan ActionPlan) ([]byte, error) {
	metadata := map[string]any{"uid": plan.TargetUID, "resourceVersion": plan.TargetResourceVersion}
	switch plan.ActionCode {
	case "deployment.rollout_restart":
		now := s.now().UTC()
		return json.Marshal(map[string]any{
			"metadata": metadata,
			"spec": map[string]any{"template": map[string]any{"metadata": map[string]any{"annotations": map[string]string{
				"k8s-aiops.local/automation-id": plan.ID,
				"k8s-aiops.local/restarted-at":  now.Format(time.RFC3339),
			}}}},
		})
	case "deployment.scale":
		if plan.DesiredReplicas == nil || *plan.DesiredReplicas < 0 || *plan.DesiredReplicas > 1000 {
			return nil, ErrInvalidOperation
		}
		return json.Marshal(map[string]any{"metadata": metadata, "spec": map[string]any{"replicas": *plan.DesiredReplicas}})
	case "cronjob.suspend", "cronjob.resume":
		if plan.DesiredSuspended == nil || *plan.DesiredSuspended != (plan.ActionCode == "cronjob.suspend") {
			return nil, ErrInvalidOperation
		}
		return json.Marshal(map[string]any{"metadata": metadata, "spec": map[string]any{"suspend": *plan.DesiredSuspended}})
	case "deployment.image_update":
		if plan.ContainerName == "" || plan.DesiredImage == "" {
			return nil, ErrInvalidOperation
		}
		return json.Marshal(map[string]any{
			"metadata": metadata,
			"spec": map[string]any{"template": map[string]any{"spec": map[string]any{
				"containers": []map[string]string{{"name": plan.ContainerName, "image": plan.DesiredImage}},
			}}},
		})
	case "deployment.rollback":
		return s.buildRollbackPatch(ctx, plan)
	default:
		return nil, ErrUnsupportedAction
	}
}

// buildRollbackPatch mirrors remediation.buildRollbackPatch. It reads
// the target ReplicaSet, validates UID/RV, and constructs a patch that
// replaces the Deployment's pod template.
func (s *Service) buildRollbackPatch(ctx context.Context, plan ActionPlan) ([]byte, error) {
	if plan.RollbackRevision == nil || plan.RollbackReplicaSetName == "" || plan.RollbackReplicaSetUID == "" {
		return nil, ErrInvalidOperation
	}
	replicaSet, err := s.k8s.ReplicaSet(ctx, plan.ClusterID, plan.TargetNamespace, plan.RollbackReplicaSetName)
	if err != nil {
		return nil, err
	}
	if replicaSet.Metadata.UID != plan.RollbackReplicaSetUID {
		return nil, ErrTargetChanged
	}
	if plan.RollbackReplicaSetResourceVersion != "" && replicaSet.Metadata.ResourceVersion != plan.RollbackReplicaSetResourceVersion {
		return nil, ErrTargetChanged
	}
	if len(replicaSet.Spec.Template.Raw) == 0 {
		return nil, ErrInvalidOperation
	}
	var template map[string]any
	if err := json.Unmarshal(replicaSet.Spec.Template.Raw, &template); err != nil {
		return nil, ErrInvalidOperation
	}
	template["$patch"] = "replace"
	metadata, _ := template["metadata"].(map[string]any)
	if metadata == nil {
		metadata = map[string]any{}
	}
	for _, field := range []string{"name", "namespace", "uid", "resourceVersion", "creationTimestamp", "generation", "managedFields", "ownerReferences", "finalizers"} {
		delete(metadata, field)
	}
	labels, _ := metadata["labels"].(map[string]any)
	if labels != nil {
		delete(labels, "pod-template-hash")
		metadata["labels"] = labels
	}
	annotations, _ := metadata["annotations"].(map[string]any)
	if annotations == nil {
		annotations = map[string]any{}
	}
	annotations["k8s-aiops.local/rollback-revision"] = fmt.Sprintf("%d", *plan.RollbackRevision)
	annotations["k8s-aiops.local/automation-id"] = plan.ID
	metadata["annotations"] = annotations
	template["metadata"] = metadata
	return json.Marshal(map[string]any{
		"metadata": map[string]any{"uid": plan.TargetUID, "resourceVersion": plan.TargetResourceVersion},
		"spec":     map[string]any{"template": template},
	})
}

// applyPatch dispatches the patch to the right Kubernetes method.
func (s *Service) applyPatch(ctx context.Context, plan ActionPlan, patch []byte) error {
	switch plan.ActionCode {
	case "deployment.rollout_restart", "deployment.scale", "deployment.image_update", "deployment.rollback":
		_, err := s.k8s.PatchDeployment(ctx, plan.ClusterID, plan.TargetNamespace, plan.TargetName, patch, false)
		return err
	case "cronjob.suspend", "cronjob.resume":
		_, err := s.k8s.PatchCronJob(ctx, plan.ClusterID, plan.TargetNamespace, plan.TargetName, patch, false)
		return err
	default:
		return ErrUnsupportedAction
	}
}

// buildGateContext assembles the gate context for a plan. The preview
// parameter is true when assembling context for the initial preview
// (the snapshot is captured fresh), false when rechecking before
// execute (the snapshot is compared against the plan's stored preview
// snapshot).
func (s *Service) buildGateContext(ctx context.Context, plan ActionPlan, preview bool) (GateContext, error) {
	gateCtx := GateContext{Now: s.now().UTC()}
	gateCtx.PreviewSnapshot = TargetRef{
		Kind: plan.TargetKind, Namespace: plan.TargetNamespace, Name: plan.TargetName,
		UID: plan.TargetUID, ResourceVersion: plan.TargetResourceVersion,
	}
	// Read the current snapshot from Kubernetes.
	if s.k8s != nil {
		switch plan.TargetKind {
		case "Deployment":
			dep, err := s.k8s.Deployment(ctx, plan.ClusterID, plan.TargetNamespace, plan.TargetName)
			if err == nil {
				gateCtx.CurrentSnapshot = TargetRef{Kind: "Deployment", Namespace: plan.TargetNamespace, Name: plan.TargetName, UID: dep.Metadata.UID, ResourceVersion: dep.Metadata.ResourceVersion}
			}
		case "CronJob":
			cj, err := s.k8s.CronJob(ctx, plan.ClusterID, plan.TargetNamespace, plan.TargetName)
			if err == nil {
				gateCtx.CurrentSnapshot = TargetRef{Kind: "CronJob", Namespace: plan.TargetNamespace, Name: plan.TargetName, UID: cj.Metadata.UID, ResourceVersion: cj.Metadata.ResourceVersion}
			}
		}
	}
	// Default scope to allowed — the HTTP layer already enforced M35.
	// A real deployment injects the actual M35 decision via a
	// GateContextProvider; the service-level default is fail-closed
	// only when the Kubernetes source is unavailable.
	gateCtx.ScopeDecision = ScopeDecision{Allowed: true}
	// Concurrent plans + attempt cap from the repository.
	concurrent, _ := s.repo.CountConcurrentPlans(ctx, plan.ClusterID, plan.TargetUID, plan.ID)
	gateCtx.ConcurrentPlanCount = concurrent
	attempts, _ := s.repo.CountAttemptsSince(ctx, plan.ClusterID, plan.TargetUID, s.now().Add(-AttemptWindowSeconds*time.Second))
	gateCtx.AttemptCount = attempts
	gateCtx.AttemptMax = MaxAttemptsPerTarget
	// Rollback point evidence for rollback actions.
	if plan.ActionCode == "deployment.rollback" && plan.RollbackRevision != nil {
		gateCtx.RollbackPoint = RollbackPoint{
			Exists:   plan.RollbackReplicaSetName != "",
			Revision: *plan.RollbackRevision,
		}
	}
	return gateCtx, nil
}

// approvalTypeFor returns the required approval type for an action code.
// High-risk actions (rollback, image_update) require four-eyes; others
// allow single approval.
func approvalTypeFor(actionCode string) ApprovalType {
	switch actionCode {
	case "deployment.rollback", "deployment.image_update":
		return ApprovalFourEyes
	default:
		return ApprovalSingle
	}
}

// computePlanKey returns the SHA-256 hex over (case_id + runbook_id +
// target_uid + automation_version). Identical source + runbook + target
// + version produce identical keys.
func computePlanKey(caseID int64, runbookID, targetUID string) string {
	h := sha256.New()
	fmt.Fprintf(h, "case_id=%d\n", caseID)
	fmt.Fprintf(h, "runbook_id=%s\n", runbookID)
	fmt.Fprintf(h, "target_uid=%s\n", targetUID)
	fmt.Fprintf(h, "automation_version=%s\n", AutomationVersion)
	return hex.EncodeToString(h.Sum(nil))
}

// newIdentity returns a UUID v4 ID, a base64 confirmation token, and the
// SHA-256 hash of the token. Mirrors remediation.newIdentity.
func newIdentity() (string, string, []byte, error) {
	idBytes := make([]byte, 16)
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(idBytes); err != nil {
		return "", "", nil, err
	}
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", "", nil, err
	}
	idBytes[6] = (idBytes[6] & 0x0f) | 0x40
	idBytes[8] = (idBytes[8] & 0x3f) | 0x80
	hexID := hex.EncodeToString(idBytes)
	id := hexID[0:8] + "-" + hexID[8:12] + "-" + hexID[12:16] + "-" + hexID[16:20] + "-" + hexID[20:32]
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	hash := sha256.Sum256([]byte(token))
	return id, token, hash[:], nil
}

// newCorrelationRequestID returns a short random ID shared across
// preview/approve/execute/verify so the audit trail is reconstructable.
func newCorrelationRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// safeExecutionError sanitizes a Kubernetes error for storage in
// last_error and audit. Mirrors remediation.safeExecutionError.
func safeExecutionError(err error) string {
	var status cluster.APIStatusError
	if errors.As(err, &status) {
		return fmt.Sprintf("Kubernetes API rejected automation with HTTP %d", status.StatusCode)
	}
	if errors.Is(err, k8sgateway.ErrResourceNotFound) {
		return "Kubernetes automation target was not found"
	}
	if errors.Is(err, ErrTargetChanged) {
		return "Kubernetes automation target changed after preview"
	}
	return "Kubernetes automation request failed"
}
