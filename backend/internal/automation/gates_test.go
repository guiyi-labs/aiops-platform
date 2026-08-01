package automation

import (
	"reflect"
	"testing"
	"time"
)

// fixedTime is the deterministic clock used by the gate tests.
var fixedTime = time.Date(2025, 3, 14, 9, 26, 53, 0, time.UTC)

// gateCodes is a helper that extracts the GateCode slice from a PolicyGate
// slice so the test can compare gate sets without caring about order-via
// RequiredGates (which is itself order-stable).
func gateCodes(gates []PolicyGate) []GateCode {
	out := make([]GateCode, len(gates))
	for i, g := range gates {
		out[i] = g.Code
	}
	return out
}

// startGate returns a PolicyGate seed matching the per-gate dispatch in
// evaluateGate. Using this helper keeps each unit test focused on the
// gate's own logic rather than the dispatch.
func startGate(code GateCode, ctx GateContext) PolicyGate {
	return PolicyGate{Code: code, CheckedAt: ctx.Now}
}

func TestRequiredGates(t *testing.T) {
	t.Parallel()
	core := []GateCode{GateUIDRVRecheck, GateScope, GateFreezeWindow, GateConcurrentPlans, GateAttemptCap}
	cases := []struct {
		name       string
		actionCode string
		want       []GateCode
	}{
		{
			name:       "rollback",
			actionCode: "deployment.rollback",
			want:       append(append([]GateCode{}, core...), GateRollbackPoint, GateSLOBurn, GatePDBBlastRadius),
		},
		{
			name:       "rollout_restart",
			actionCode: "deployment.rollout_restart",
			want:       append(append([]GateCode{}, core...), GateSLOBurn, GatePDBBlastRadius),
		},
		{
			name:       "image_update",
			actionCode: "deployment.image_update",
			want:       append(append([]GateCode{}, core...), GateSLOBurn, GatePDBBlastRadius),
		},
		{
			name:       "scale",
			actionCode: "deployment.scale",
			want:       append(append([]GateCode{}, core...), GatePDBBlastRadius),
		},
		{
			name:       "cronjob_suspend",
			actionCode: "cronjob.suspend",
			want:       append(append([]GateCode{}, core...), GateSLOBurn),
		},
		{
			name:       "cronjob_resume",
			actionCode: "cronjob.resume",
			want:       append(append([]GateCode{}, core...), GateSLOBurn),
		},
		{
			name:       "unknown_action_returns_core_only",
			actionCode: "unknown.action",
			want:       append([]GateCode{}, core...),
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := RequiredGates(tc.actionCode)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("RequiredGates(%q) = %v, want %v", tc.actionCode, got, tc.want)
			}
		})
	}
}

func TestEvaluateUIDRV(t *testing.T) {
	t.Parallel()
	previewSnapshot := TargetRef{Kind: "Deployment", Namespace: "default", Name: "api", UID: "uid-1", ResourceVersion: "rv-1"}
	cases := []struct {
		name   string
		ctx    GateContext
		plan   ActionPlan
		want   GateStatus
		reason string
	}{
		{
			name:   "preview_snapshot_missing_skips",
			ctx:    GateContext{Now: fixedTime, PreviewSnapshot: TargetRef{}, CurrentSnapshot: previewSnapshot},
			want:   GateSkipped,
			reason: "preview_snapshot_missing",
		},
		{
			name:   "target_no_longer_exists_fails",
			ctx:    GateContext{Now: fixedTime, PreviewSnapshot: previewSnapshot, CurrentSnapshot: TargetRef{Kind: "Deployment", Namespace: "default", Name: "api", UID: ""}},
			want:   GateFailed,
			reason: "target_no_longer_exists",
		},
		{
			name:   "uid_changed_fails",
			ctx:    GateContext{Now: fixedTime, PreviewSnapshot: previewSnapshot, CurrentSnapshot: TargetRef{Kind: "Deployment", Namespace: "default", Name: "api", UID: "uid-2", ResourceVersion: "rv-1"}},
			want:   GateFailed,
			reason: "target_uid_changed",
		},
		{
			name:   "resource_version_changed_fails",
			ctx:    GateContext{Now: fixedTime, PreviewSnapshot: previewSnapshot, CurrentSnapshot: TargetRef{Kind: "Deployment", Namespace: "default", Name: "api", UID: "uid-1", ResourceVersion: "rv-2"}},
			want:   GateFailed,
			reason: "target_resource_version_changed",
		},
		{
			name:   "all_match_passes",
			ctx:    GateContext{Now: fixedTime, PreviewSnapshot: previewSnapshot, CurrentSnapshot: previewSnapshot},
			want:   GatePassed,
			reason: "",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gate := evaluateUIDRV(ActionPlan{}, tc.ctx, startGate(GateUIDRVRecheck, tc.ctx))
			if gate.Status != tc.want {
				t.Fatalf("status = %q, want %q (reason=%q)", gate.Status, tc.want, gate.Reason)
			}
			if gate.Reason != tc.reason {
				t.Fatalf("reason = %q, want %q", gate.Reason, tc.reason)
			}
			if !gate.CheckedAt.Equal(fixedTime) {
				t.Fatalf("CheckedAt = %v, want %v", gate.CheckedAt, fixedTime)
			}
		})
	}
}

func TestEvaluateScope(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		decision   ScopeDecision
		want       GateStatus
		wantReason string
	}{
		{
			name:       "allowed_passes",
			decision:   ScopeDecision{Allowed: true},
			want:       GatePassed,
			wantReason: "",
		},
		{
			name:       "denied_with_reason_fails",
			decision:   ScopeDecision{Allowed: false, Reason: "no_grant"},
			want:       GateFailed,
			wantReason: "no_grant",
		},
		{
			name:       "denied_without_reason_defaults",
			decision:   ScopeDecision{Allowed: false},
			want:       GateFailed,
			wantReason: "scope_denied",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := GateContext{Now: fixedTime, ScopeDecision: tc.decision}
			gate := evaluateScope(ActionPlan{}, ctx, startGate(GateScope, ctx))
			if gate.Status != tc.want {
				t.Fatalf("status = %q, want %q", gate.Status, tc.want)
			}
			if gate.Reason != tc.wantReason {
				t.Fatalf("reason = %q, want %q", gate.Reason, tc.wantReason)
			}
		})
	}
}

func TestEvaluateFreezeWindow(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		freeze     FreezeWindow
		want       GateStatus
		wantReason string
	}{
		{
			name:   "not_active_passes",
			freeze: FreezeWindow{Active: false},
			want:   GatePassed,
		},
		{
			name:       "active_with_reason_fails",
			freeze:     FreezeWindow{Active: true, Reason: "change_freeze"},
			want:       GateFailed,
			wantReason: "change_freeze",
		},
		{
			name:       "active_without_reason_defaults",
			freeze:     FreezeWindow{Active: true},
			want:       GateFailed,
			wantReason: "freeze_window_active",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := GateContext{Now: fixedTime, FreezeWindow: tc.freeze}
			gate := evaluateFreezeWindow(ActionPlan{}, ctx, startGate(GateFreezeWindow, ctx))
			if gate.Status != tc.want {
				t.Fatalf("status = %q, want %q", gate.Status, tc.want)
			}
			if gate.Reason != tc.wantReason {
				t.Fatalf("reason = %q, want %q", gate.Reason, tc.wantReason)
			}
		})
	}
}

func TestEvaluateConcurrentPlans(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		count int
		want  GateStatus
	}{
		{name: "zero_passes", count: 0, want: GatePassed},
		{name: "one_fails", count: 1, want: GateFailed},
		{name: "many_fail", count: 5, want: GateFailed},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := GateContext{Now: fixedTime, ConcurrentPlanCount: tc.count}
			gate := evaluateConcurrentPlans(ActionPlan{}, ctx, startGate(GateConcurrentPlans, ctx))
			if gate.Status != tc.want {
				t.Fatalf("status = %q, want %q", gate.Status, tc.want)
			}
			if tc.want == GateFailed && gate.Reason != "concurrent_plan_active" {
				t.Fatalf("reason = %q, want concurrent_plan_active", gate.Reason)
			}
		})
	}
}

func TestEvaluateAttemptCap(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		count int
		max   int
		want  GateStatus
	}{
		{name: "below_default_max_passes", count: 0, max: 0, want: GatePassed},
		{name: "just_below_max_passes", count: MaxAttemptsPerTarget - 1, max: 0, want: GatePassed},
		{name: "at_max_fails", count: MaxAttemptsPerTarget, max: 0, want: GateFailed},
		{name: "exceeding_max_fails", count: MaxAttemptsPerTarget + 3, max: 0, want: GateFailed},
		{name: "custom_max_below_passes", count: 2, max: 5, want: GatePassed},
		{name: "custom_max_at_fails", count: 5, max: 5, want: GateFailed},
		{name: "custom_max_exceeding_fails", count: 6, max: 5, want: GateFailed},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := GateContext{Now: fixedTime, AttemptCount: tc.count, AttemptMax: tc.max}
			gate := evaluateAttemptCap(ActionPlan{}, ctx, startGate(GateAttemptCap, ctx))
			if gate.Status != tc.want {
				t.Fatalf("status = %q, want %q (count=%d max=%d)", gate.Status, tc.want, tc.count, tc.max)
			}
			if tc.want == GateFailed && gate.Reason != "attempt_cap_exceeded" {
				t.Fatalf("reason = %q, want attempt_cap_exceeded", gate.Reason)
			}
		})
	}
}

func TestEvaluateRollbackPoint(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		point      RollbackPoint
		want       GateStatus
		wantReason string
	}{
		{
			name:       "not_exists_fails",
			point:      RollbackPoint{Exists: false},
			want:       GateFailed,
			wantReason: "no_rollback_point",
		},
		{
			name:       "current_fails",
			point:      RollbackPoint{Exists: true, Current: true, Revision: 3},
			want:       GateFailed,
			wantReason: "rollback_point_is_current",
		},
		{
			name:  "valid_rollback_point_passes",
			point: RollbackPoint{Exists: true, Current: false, Revision: 2},
			want:  GatePassed,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := GateContext{Now: fixedTime, RollbackPoint: tc.point}
			gate := evaluateRollbackPoint(ActionPlan{}, ctx, startGate(GateRollbackPoint, ctx))
			if gate.Status != tc.want {
				t.Fatalf("status = %q, want %q", gate.Status, tc.want)
			}
			if gate.Reason != tc.wantReason {
				t.Fatalf("reason = %q, want %q", gate.Reason, tc.wantReason)
			}
		})
	}
}

func TestEvaluateSLOBurn(t *testing.T) {
	t.Parallel()
	before := int32(5)
	down := int32(2) // scale-down: desired < before
	up := int32(8)   // scale-up: desired > before
	cases := []struct {
		name       string
		plan       ActionPlan
		state      string
		want       GateStatus
		wantReason string
	}{
		{
			name:       "healthy_passes",
			plan:       ActionPlan{ActionCode: "deployment.rollout_restart"},
			state:      "healthy",
			want:       GatePassed,
			wantReason: "",
		},
		{
			name:       "burning_slow_passes_with_reason",
			plan:       ActionPlan{ActionCode: "deployment.rollout_restart"},
			state:      "burning_slow",
			want:       GatePassed,
			wantReason: "burning_slow",
		},
		{
			name:       "burning_fast_passes_with_reason",
			plan:       ActionPlan{ActionCode: "deployment.image_update"},
			state:      "burning_fast",
			want:       GatePassed,
			wantReason: "burning_fast",
		},
		{
			name:       "breached_rollback_passes_remedy",
			plan:       ActionPlan{ActionCode: "deployment.rollback"},
			state:      "breached",
			want:       GatePassed,
			wantReason: "breached_action_is_remedy",
		},
		{
			name:       "breached_scale_down_fails",
			plan:       ActionPlan{ActionCode: "deployment.scale", BeforeReplicas: &before, DesiredReplicas: &down},
			state:      "breached",
			want:       GateFailed,
			wantReason: "scale_down_during_breach",
		},
		{
			name:       "breached_scale_up_passes_remedy",
			plan:       ActionPlan{ActionCode: "deployment.scale", BeforeReplicas: &before, DesiredReplicas: &up},
			state:      "breached",
			want:       GatePassed,
			wantReason: "breached_action_is_remedy",
		},
		{
			name:       "breached_image_update_fails",
			plan:       ActionPlan{ActionCode: "deployment.image_update"},
			state:      "breached",
			want:       GateFailed,
			wantReason: "image_update_during_breach",
		},
		{
			name:       "unavailable_skips",
			plan:       ActionPlan{ActionCode: "deployment.rollback"},
			state:      "unavailable",
			want:       GateSkipped,
			wantReason: "slo_state_unavailable",
		},
		{
			name:       "empty_state_skips",
			plan:       ActionPlan{ActionCode: "deployment.rollback"},
			state:      "",
			want:       GateSkipped,
			wantReason: "slo_state_unavailable",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := GateContext{Now: fixedTime, SLOBurnState: SLOBurnState{State: tc.state}}
			gate := evaluateSLOBurn(tc.plan, ctx, startGate(GateSLOBurn, ctx))
			if gate.Status != tc.want {
				t.Fatalf("status = %q, want %q (reason=%q)", gate.Status, tc.want, gate.Reason)
			}
			if gate.Reason != tc.wantReason {
				t.Fatalf("reason = %q, want %q", gate.Reason, tc.wantReason)
			}
		})
	}
}

func TestEvaluatePDBBlastRadius(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		pdb        PDBEvidence
		blast      BlastRadius
		want       GateStatus
		wantReason string
	}{
		{
			name:       "unavailable_skips",
			pdb:        PDBEvidence{Available: false},
			blast:      BlastRadius{PodsAffected: 10, Max: 1},
			want:       GateSkipped,
			wantReason: "pdb_evidence_unavailable",
		},
		{
			name:       "blast_radius_exceeds_cap_fails",
			pdb:        PDBEvidence{Available: true, DisruptionsAllowed: 5},
			blast:      BlastRadius{PodsAffected: 10, Max: 1},
			want:       GateFailed,
			wantReason: "blast_radius_exceeds_cap",
		},
		{
			name:  "blast_radius_at_cap_passes",
			pdb:   PDBEvidence{Available: true, DisruptionsAllowed: 5},
			blast: BlastRadius{PodsAffected: 1, Max: 1},
			want:  GatePassed,
		},
		{
			name:  "no_cap_passes",
			pdb:   PDBEvidence{Available: true, DisruptionsAllowed: 5},
			blast: BlastRadius{PodsAffected: 99, Max: 0},
			want:  GatePassed,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := GateContext{Now: fixedTime, PDBEvidence: tc.pdb, BlastRadius: tc.blast}
			gate := evaluatePDBBlastRadius(ActionPlan{}, ctx, startGate(GatePDBBlastRadius, ctx))
			if gate.Status != tc.want {
				t.Fatalf("status = %q, want %q", gate.Status, tc.want)
			}
			if gate.Reason != tc.wantReason {
				t.Fatalf("reason = %q, want %q", gate.Reason, tc.wantReason)
			}
		})
	}
}

func TestAllPassed(t *testing.T) {
	t.Parallel()
	ctx := GateContext{Now: fixedTime}
	mk := func(code GateCode, status GateStatus) PolicyGate {
		return PolicyGate{Code: code, Status: status, CheckedAt: ctx.Now}
	}
	cases := []struct {
		name  string
		gates []PolicyGate
		want  bool
	}{
		{name: "empty_passes", gates: nil, want: true},
		{name: "all_passed_passes", gates: []PolicyGate{mk(GateUIDRVRecheck, GatePassed), mk(GateScope, GatePassed)}, want: true},
		{name: "skipped_passes", gates: []PolicyGate{mk(GateUIDRVRecheck, GatePassed), mk(GatePDBBlastRadius, GateSkipped)}, want: true},
		{name: "one_failed_fails", gates: []PolicyGate{mk(GateUIDRVRecheck, GatePassed), mk(GateScope, GateFailed)}, want: false},
		{name: "all_failed_fails", gates: []PolicyGate{mk(GateUIDRVRecheck, GateFailed), mk(GateScope, GateFailed)}, want: false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := AllPassed(tc.gates); got != tc.want {
				t.Fatalf("AllPassed = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRecheck(t *testing.T) {
	t.Parallel()
	plan := ActionPlan{
		ActionCode:             "deployment.rollback",
		TargetKind:             "Deployment",
		TargetNamespace:        "default",
		TargetName:             "api",
		TargetUID:              "uid-1",
		TargetResourceVersion:  "rv-1",
		RollbackRevision:       int32Ptr(2),
		RollbackReplicaSetName: "api-abc",
	}
	ctx := GateContext{
		Now:             fixedTime,
		PreviewSnapshot: TargetRef{Kind: "Deployment", Namespace: "default", Name: "api", UID: "uid-1", ResourceVersion: "rv-1"},
		CurrentSnapshot: TargetRef{Kind: "Deployment", Namespace: "default", Name: "api", UID: "uid-1", ResourceVersion: "rv-1"},
		ScopeDecision:   ScopeDecision{Allowed: true},
		RollbackPoint:   RollbackPoint{Exists: true, Current: false, Revision: 2},
		SLOBurnState:    SLOBurnState{State: "healthy"},
		PDBEvidence:     PDBEvidence{Available: true, DisruptionsAllowed: 5},
		BlastRadius:     BlastRadius{PodsAffected: 1, Max: 5},
	}
	evaluator := NewGateEvaluator()
	gates := evaluator.Recheck(plan, ctx)
	if len(gates) == 0 {
		t.Fatalf("Recheck returned no gates")
	}
	// Every gate must be flagged Rechecked=true and the codes must match
	// RequiredGates (rollback has the full set).
	wantCodes := RequiredGates(plan.ActionCode)
	if !reflect.DeepEqual(gateCodes(gates), wantCodes) {
		t.Fatalf("gate codes = %v, want %v", gateCodes(gates), wantCodes)
	}
	for _, g := range gates {
		if !g.Rechecked {
			t.Errorf("gate %s: Rechecked = false, want true", g.Code)
		}
		if !g.CheckedAt.Equal(fixedTime) {
			t.Errorf("gate %s: CheckedAt = %v, want %v", g.Code, g.CheckedAt, fixedTime)
		}
	}
	if !AllPassed(gates) {
		t.Errorf("expected all rechecked gates to pass, got failures: %v", FailedGates(gates))
	}

	// Compare against the preview evaluation: the same context must
	// produce the same per-gate Status, only Rechecked differs.
	preview := evaluator.Evaluate(plan, ctx)
	if len(preview) != len(gates) {
		t.Fatalf("preview gate count = %d, recheck gate count = %d", len(preview), len(gates))
	}
	for i := range preview {
		if preview[i].Status != gates[i].Status {
			t.Errorf("gate %s: preview status %q != recheck status %q", preview[i].Code, preview[i].Status, gates[i].Status)
		}
		if preview[i].Rechecked {
			t.Errorf("preview gate %s: Rechecked should be false", preview[i].Code)
		}
	}
}

// int32Ptr is a small helper for building *int32 fields in test tables.
func int32Ptr(v int32) *int32 { return &v }
