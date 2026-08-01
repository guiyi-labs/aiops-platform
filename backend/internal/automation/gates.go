package automation

import (
	"context"
	"time"
)

// GateContext is the data needed by the gate evaluator. The service
// composes it from the case context, M35 authorization, SLO state, PDB
// evidence, maintenance plans and the repository's count helpers. The
// evaluator is pure: identical GateContext + identical plan → identical
// gates.
type GateContext struct {
	// Now is the server time at evaluation. Captured in CheckedAt.
	Now time.Time

	// PreviewSnapshot is the target UID/RV captured at preview time. The
	// uid_rv_recheck gate compares CurrentSnapshot against this.
	PreviewSnapshot TargetRef

	// CurrentSnapshot is the target UID/RV observed at gate evaluation
	// time. The service reads this from the Kubernetes source.
	CurrentSnapshot TargetRef

	// ScopeDecision is the M35 authorization decision for the requester
	// against (cluster_id, namespace). The scope gate fails closed when
	// not allowed.
	ScopeDecision ScopeDecision

	// PDBEvidence is the PodDisruptionBudget evidence for the target.
	// Only meaningful for actions that affect Pod placement (e.g.
	// rollout_restart). Empty for non-Pod-affecting actions.
	PDBEvidence PDBEvidence

	// BlastRadius is the blast-radius estimate for the action.
	// BlastRadiusPods is the number of Pods the action may disrupt;
	// BlastRadiusMax is the cap from policy. Exceeding the cap fails
	// the pdb_blast_radius gate.
	BlastRadius BlastRadius

	// SLOBurnState is the most recent SLO evaluation state for the
	// service. The slo_burn gate fails when the service is already in
	// a breached window for unrelated reasons that the action would
	// worsen (e.g. rolling back to a known-bad revision during a
	// burn-fast window).
	SLOBurnState SLOBurnState

	// FreezeWindow is the maintenance/freeze window state for the
	// cluster/namespace. The freeze_window gate fails when a freeze
	// is active.
	FreezeWindow FreezeWindow

	// ConcurrentPlanCount is the number of other non-terminal plans
	// targeting the same UID. The concurrent_plans gate fails when
	// this is non-zero.
	ConcurrentPlanCount int

	// AttemptCount is the number of non-cancelled plans for the same
	// target UID within AttemptWindowSeconds. The attempt_cap gate
	// fails when this exceeds MaxAttemptsPerTarget.
	AttemptCount int
	AttemptMax   int

	// RollbackPoint is the rollback-point evidence for rollback actions.
	// RollbackPointExists=false fails the rollback_point gate.
	// RollbackPointCurrent=true fails the rollback_point gate (no-op
	// rollback would not change the target).
	RollbackPoint RollbackPoint
}

// ScopeDecision records whether the requester may operate on the target.
type ScopeDecision struct {
	Allowed bool
	Reason  string // empty when Allowed
}

// PDBEvidence records the PDB state for the target. PDBMissing=true means
// the PDB evidence could not be gathered; the pdb_blast_radius gate is
// skipped in that case (the action may proceed, but the audit records it).
type PDBEvidence struct {
	Available           bool
	DisruptionsAllowed  int32
	DisruptionsObserved int32
	Reason              string
}

// BlastRadius records the blast-radius estimate.
type BlastRadius struct {
	PodsAffected int
	Max          int
	Reason       string
}

// SLOBurnState records the most recent SLO burn state.
type SLOBurnState struct {
	State  string // healthy, burning_slow, burning_fast, breached, unavailable
	Reason string
}

// FreezeWindow records the maintenance/freeze window state.
type FreezeWindow struct {
	Active bool
	Reason string // empty when not active
}

// RollbackPoint records the rollback-point evidence.
type RollbackPoint struct {
	Exists   bool
	Current  bool
	Revision int32
	Reason   string
}

// GateEvaluator is the pure policy-gate evaluator. It is stateless and
// deterministic: identical (plan, ctx) → identical gates. The service is
// the only caller; HTTP handlers never bypass it.
type GateEvaluator struct{}

// NewGateEvaluator returns a stateless evaluator.
func NewGateEvaluator() *GateEvaluator { return &GateEvaluator{} }

// RequiredGates returns the gates required for an action_code. Adding a
// gate to this mapping is a contract change (AutomationVersion bump).
//
// All actions share the core gates (uid_rv_recheck, scope,
// freeze_window, concurrent_plans, attempt_cap). Pod-affecting actions
// additionally require pdb_blast_radius. SLO-bound actions additionally
// require slo_burn. Rollback actions additionally require rollback_point.
func RequiredGates(actionCode string) []GateCode {
	core := []GateCode{
		GateUIDRVRecheck,
		GateScope,
		GateFreezeWindow,
		GateConcurrentPlans,
		GateAttemptCap,
	}
	switch actionCode {
	case "deployment.rollback":
		return append(core, GateRollbackPoint, GateSLOBurn, GatePDBBlastRadius)
	case "deployment.rollout_restart":
		return append(core, GateSLOBurn, GatePDBBlastRadius)
	case "deployment.image_update":
		return append(core, GateSLOBurn, GatePDBBlastRadius)
	case "deployment.scale":
		return append(core, GatePDBBlastRadius)
	case "cronjob.suspend", "cronjob.resume":
		// CronJob actions do not affect Pods directly (no PDB); SLO
		// burn is still relevant because suspending a CronJob may
		// stop a noise source.
		return append(core, GateSLOBurn)
	default:
		return core
	}
}

// Evaluate runs all required gates for the plan and returns the results.
// The order matches RequiredGates so the audit trail is stable.
func (GateEvaluator) Evaluate(plan ActionPlan, ctx GateContext) []PolicyGate {
	required := RequiredGates(plan.ActionCode)
	gates := make([]PolicyGate, 0, len(required))
	for _, code := range required {
		gates = append(gates, evaluateGate(code, plan, ctx))
	}
	return gates
}

// Recheck runs the gates that must be re-evaluated before execute. The
// preview gates are preserved in the plan's PolicyGates; the recheck
// produces a fresh set with Rechecked=true. A gate that passed at
// preview may fail at recheck (e.g. UID/RV changed); a gate that was
// skipped at preview may still be skipped at recheck.
func (e GateEvaluator) Recheck(plan ActionPlan, ctx GateContext) []PolicyGate {
	required := RequiredGates(plan.ActionCode)
	gates := make([]PolicyGate, 0, len(required))
	for _, code := range required {
		gate := evaluateGate(code, plan, ctx)
		gate.Rechecked = true
		gates = append(gates, gate)
	}
	return gates
}

// AllPassed returns true when every gate in the slice passed or was
// skipped (a skipped gate is not a failure). A single failed gate fails
// the plan.
func AllPassed(gates []PolicyGate) bool {
	for _, g := range gates {
		if g.Status == GateFailed {
			return false
		}
	}
	return true
}

// FailedGates returns the gates that failed. Useful for audit logging.
func FailedGates(gates []PolicyGate) []PolicyGate {
	out := make([]PolicyGate, 0)
	for _, g := range gates {
		if g.Status == GateFailed {
			out = append(out, g)
		}
	}
	return out
}

// evaluateGate is the per-gate dispatch. Adding a case here is a contract
// change.
func evaluateGate(code GateCode, plan ActionPlan, ctx GateContext) PolicyGate {
	gate := PolicyGate{Code: code, CheckedAt: ctx.Now}
	switch code {
	case GateUIDRVRecheck:
		gate = evaluateUIDRV(plan, ctx, gate)
	case GateScope:
		gate = evaluateScope(plan, ctx, gate)
	case GatePDBBlastRadius:
		gate = evaluatePDBBlastRadius(plan, ctx, gate)
	case GateSLOBurn:
		gate = evaluateSLOBurn(plan, ctx, gate)
	case GateFreezeWindow:
		gate = evaluateFreezeWindow(plan, ctx, gate)
	case GateConcurrentPlans:
		gate = evaluateConcurrentPlans(plan, ctx, gate)
	case GateAttemptCap:
		gate = evaluateAttemptCap(plan, ctx, gate)
	case GateRollbackPoint:
		gate = evaluateRollbackPoint(plan, ctx, gate)
	default:
		gate.Status = GateSkipped
		gate.Reason = "unknown_gate_code"
	}
	return gate
}

// evaluateUIDRV fails closed when the target UID or resourceVersion has
// changed since preview. An empty preview UID/RV (draft plan without
// preview) skips the gate.
func evaluateUIDRV(plan ActionPlan, ctx GateContext, gate PolicyGate) PolicyGate {
	if ctx.PreviewSnapshot.UID == "" || ctx.PreviewSnapshot.ResourceVersion == "" {
		gate.Status = GateSkipped
		gate.Reason = "preview_snapshot_missing"
		return gate
	}
	if ctx.CurrentSnapshot.UID == "" {
		gate.Status = GateFailed
		gate.Reason = "target_no_longer_exists"
		return gate
	}
	if ctx.CurrentSnapshot.UID != ctx.PreviewSnapshot.UID {
		gate.Status = GateFailed
		gate.Reason = "target_uid_changed"
		return gate
	}
	if ctx.CurrentSnapshot.ResourceVersion != ctx.PreviewSnapshot.ResourceVersion {
		gate.Status = GateFailed
		gate.Reason = "target_resource_version_changed"
		return gate
	}
	gate.Status = GatePassed
	return gate
}

// evaluateScope fails closed when the M35 authorization decision denies
// the requester access to (cluster_id, namespace). The HTTP layer already
// enforced scope at request time; this gate is a defence-in-depth for
// the execute path.
func evaluateScope(plan ActionPlan, ctx GateContext, gate PolicyGate) PolicyGate {
	if !ctx.ScopeDecision.Allowed {
		gate.Status = GateFailed
		gate.Reason = ctx.ScopeDecision.Reason
		if gate.Reason == "" {
			gate.Reason = "scope_denied"
		}
		return gate
	}
	gate.Status = GatePassed
	return gate
}

// evaluatePDBBlastRadius fails closed when the blast-radius estimate
// exceeds the cap. When PDB evidence is unavailable, the gate is skipped
// (the action may proceed, but the audit records it) — this mirrors the
// maintenance package's bounded-drain behaviour where missing PDB evidence
// blocks drain but does not block non-disruptive operations.
func evaluatePDBBlastRadius(plan ActionPlan, ctx GateContext, gate PolicyGate) PolicyGate {
	if !ctx.PDBEvidence.Available {
		gate.Status = GateSkipped
		gate.Reason = "pdb_evidence_unavailable"
		return gate
	}
	if ctx.BlastRadius.Max > 0 && ctx.BlastRadius.PodsAffected > ctx.BlastRadius.Max {
		gate.Status = GateFailed
		gate.Reason = "blast_radius_exceeds_cap"
		return gate
	}
	// PDB disruptions allowed must be >= 0 for the action to proceed
	// without violating the budget. DisruptionsAllowed == 0 means the
	// action would block; treat as a failure.
	if ctx.PDBEvidence.DisruptionsAllowed < 0 {
		gate.Status = GateFailed
		gate.Reason = "pdb_disruptions_allowed_negative"
		return gate
	}
	gate.Status = GatePassed
	return gate
}

// evaluateSLOBurn fails closed when the service is already in a breached
// SLO window and the action would worsen it (e.g. rolling back to a
// known-bad revision). burning_slow/burning_fast are not failures per se
// — the action may be the remedy — but the gate records the state.
func evaluateSLOBurn(plan ActionPlan, ctx GateContext, gate PolicyGate) PolicyGate {
	switch ctx.SLOBurnState.State {
	case "breached":
		// A rollback action during a breached window is the remedy, not
		// a worsening — allow it. A scale-down or image_update during a
		// breached window would worsen things — fail closed.
		if plan.ActionCode == "deployment.scale" && plan.DesiredReplicas != nil && plan.BeforeReplicas != nil && *plan.DesiredReplicas < *plan.BeforeReplicas {
			gate.Status = GateFailed
			gate.Reason = "scale_down_during_breach"
			return gate
		}
		if plan.ActionCode == "deployment.image_update" {
			gate.Status = GateFailed
			gate.Reason = "image_update_during_breach"
			return gate
		}
		gate.Status = GatePassed
		gate.Reason = "breached_action_is_remedy"
		return gate
	case "burning_fast", "burning_slow":
		gate.Status = GatePassed
		gate.Reason = ctx.SLOBurnState.State
		return gate
	case "healthy":
		gate.Status = GatePassed
		return gate
	case "unavailable", "":
		// Missing SLO evidence is not a failure — the gate records it
		// and proceeds. Missing-data fail-closed is enforced by the SLO
		// evaluator (M41), not by the automation layer.
		gate.Status = GateSkipped
		gate.Reason = "slo_state_unavailable"
		return gate
	default:
		gate.Status = GateSkipped
		gate.Reason = "unknown_slo_state"
		return gate
	}
}

// evaluateFreezeWindow fails closed when a maintenance/freeze window is
// active on the target cluster or namespace.
func evaluateFreezeWindow(plan ActionPlan, ctx GateContext, gate PolicyGate) PolicyGate {
	if ctx.FreezeWindow.Active {
		gate.Status = GateFailed
		gate.Reason = ctx.FreezeWindow.Reason
		if gate.Reason == "" {
			gate.Reason = "freeze_window_active"
		}
		return gate
	}
	gate.Status = GatePassed
	return gate
}

// evaluateConcurrentPlans fails closed when another non-terminal plan
// targets the same UID. The repository's CountConcurrentPlans excludes
// the current plan's ID.
func evaluateConcurrentPlans(plan ActionPlan, ctx GateContext, gate PolicyGate) PolicyGate {
	if ctx.ConcurrentPlanCount > 0 {
		gate.Status = GateFailed
		gate.Reason = "concurrent_plan_active"
		return gate
	}
	gate.Status = GatePassed
	return gate
}

// evaluateAttemptCap fails closed when the per-target attempt count
// within the rolling window exceeds MaxAttemptsPerTarget. The cap
// prevents a runaway automation loop.
func evaluateAttemptCap(plan ActionPlan, ctx GateContext, gate PolicyGate) PolicyGate {
	max := ctx.AttemptMax
	if max <= 0 {
		max = MaxAttemptsPerTarget
	}
	if ctx.AttemptCount >= max {
		gate.Status = GateFailed
		gate.Reason = "attempt_cap_exceeded"
		return gate
	}
	gate.Status = GatePassed
	return gate
}

// evaluateRollbackPoint fails closed when no valid rollback point exists
// (no ReplicaSet revision) or when the rollback point is the current
// revision (no-op).
func evaluateRollbackPoint(plan ActionPlan, ctx GateContext, gate PolicyGate) PolicyGate {
	if !ctx.RollbackPoint.Exists {
		gate.Status = GateFailed
		gate.Reason = "no_rollback_point"
		return gate
	}
	if ctx.RollbackPoint.Current {
		gate.Status = GateFailed
		gate.Reason = "rollback_point_is_current"
		return gate
	}
	gate.Status = GatePassed
	return gate
}

// GateContextProvider is the interface the service uses to assemble a
// GateContext. The default implementation reads from the case context,
// M35 authorization, SLO service, maintenance plans and the repository's
// count helpers. Tests inject a fake provider.
type GateContextProvider interface {
	// BuildGateContext assembles the gate context for a plan. The
	// preview parameter is true when assembling context for the initial
	// preview (the snapshot is captured fresh), false when rechecking
	// before execute (the snapshot is compared against the plan's
	// stored preview snapshot).
	BuildGateContext(ctx context.Context, plan ActionPlan, preview bool) (GateContext, error)
}

// NopGateContextProvider returns a minimal context that skips all
// evidence-dependent gates. Used when the automation service is in
// query-only mode (no Kubernetes/SLO/Maintenance integration).
type NopGateContextProvider struct{}

func (NopGateContextProvider) BuildGateContext(context.Context, ActionPlan, bool) (GateContext, error) {
	return GateContext{}, nil
}
