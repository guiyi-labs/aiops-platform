package automation

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeEvidenceProvider is a test double for EvidenceProvider. It returns
// canned pre/post snapshots and errors so the verifier can be exercised
// without a real SLO service or Kubernetes source.
type fakeEvidenceProvider struct {
	pre      EvidenceSnapshot
	sloPre   *SLOSnapshot
	sloPreID *int64
	preErr   error

	post      EvidenceSnapshot
	sloPost   *SLOSnapshot
	sloPostID *int64
	postErr   error

	preCalls  int
	postCalls int
}

func (f *fakeEvidenceProvider) CapturePreSnapshot(context.Context, ActionPlan) (EvidenceSnapshot, *SLOSnapshot, *int64, error) {
	f.preCalls++
	return f.pre, f.sloPre, f.sloPreID, f.preErr
}

func (f *fakeEvidenceProvider) CapturePostSnapshot(context.Context, ActionPlan) (EvidenceSnapshot, *SLOSnapshot, *int64, error) {
	f.postCalls++
	return f.post, f.sloPost, f.sloPostID, f.postErr
}

// validSnapshot builds an EvidenceSnapshot with a non-zero CapturedAt and
// a real ContentHash so compareEvidence does not mark it missing.
func validSnapshot(state map[string]any, slo *SLOSnapshot) EvidenceSnapshot {
	s := EvidenceSnapshot{
		ResourceState: state,
		SLOState:      slo,
		CapturedAt:    fixedTime,
	}
	s.ContentHash = HashSnapshot(s)
	return s
}

func sloSnapshot(state string) *SLOSnapshot {
	return &SLOSnapshot{SLOID: 1, Version: 1, Template: "api", State: state, EvaluatedAt: fixedTime}
}

func int64Ptr(v int64) *int64 { return &v }

func TestCreateVerification(t *testing.T) {
	t.Parallel()
	plan := ActionPlan{ID: "plan-1", ActionCode: "deployment.rollout_restart"}
	pre := validSnapshot(map[string]any{"replicas": int32(3), "available_replicas": int32(3)}, sloSnapshot("burning_fast"))
	sloPreID := int64(99)
	provider := &fakeEvidenceProvider{pre: pre, sloPre: pre.SLOState, sloPreID: &sloPreID}
	verifier := NewVerifier(WithVerifierProvider(provider), WithVerifierNow(func() time.Time { return fixedTime }), WithVerifierCooldown(120*time.Second))

	verification, err := verifier.CreateVerification(context.Background(), plan)
	if err != nil {
		t.Fatalf("CreateVerification returned error: %v", err)
	}
	if verification.PlanID != plan.ID {
		t.Errorf("PlanID = %q, want %q", verification.PlanID, plan.ID)
	}
	if verification.Status != VerificationStatusPending {
		t.Errorf("Status = %q, want %q", verification.Status, VerificationStatusPending)
	}
	if verification.EvidenceComparison != ComparisonInsufficient {
		t.Errorf("EvidenceComparison = %q, want %q", verification.EvidenceComparison, ComparisonInsufficient)
	}
	if verification.VerifierVersion != VerifierVersion {
		t.Errorf("VerifierVersion = %q, want %q", verification.VerifierVersion, VerifierVersion)
	}
	if verification.VerificationKey == "" {
		t.Error("VerificationKey is empty, want a SHA-256 hex")
	}
	if len(verification.VerificationKey) != MaxVerificationKeyLength {
		t.Errorf("VerificationKey length = %d, want %d", len(verification.VerificationKey), MaxVerificationKeyLength)
	}
	if verification.CooldownSeconds != 120 {
		t.Errorf("CooldownSeconds = %d, want 120", verification.CooldownSeconds)
	}
	if verification.PreSnapshot.ContentHash != pre.ContentHash {
		t.Errorf("PreSnapshot.ContentHash = %q, want %q", verification.PreSnapshot.ContentHash, pre.ContentHash)
	}
	if verification.PreSnapshot.SLOState == nil || verification.PreSnapshot.SLOState.State != "burning_fast" {
		t.Errorf("PreSnapshot.SLOState not propagated, got %+v", verification.PreSnapshot.SLOState)
	}
	if verification.SLOEvaluationBeforeID == nil || *verification.SLOEvaluationBeforeID != 99 {
		t.Errorf("SLOEvaluationBeforeID = %v, want 99", verification.SLOEvaluationBeforeID)
	}
	if verification.MissingEvidence {
		t.Error("MissingEvidence = true, want false (pre snapshot is complete)")
	}
	if !verification.CreatedAt.Equal(fixedTime) {
		t.Errorf("CreatedAt = %v, want %v", verification.CreatedAt, fixedTime)
	}
	if !verification.UpdatedAt.Equal(fixedTime) {
		t.Errorf("UpdatedAt = %v, want %v", verification.UpdatedAt, fixedTime)
	}
	if provider.preCalls != 1 {
		t.Errorf("CapturePreSnapshot called %d times, want 1", provider.preCalls)
	}
}

func TestCreateVerificationClampsCooldownToMinimum(t *testing.T) {
	t.Parallel()
	provider := &fakeEvidenceProvider{pre: validSnapshot(map[string]any{"replicas": int32(1)}, nil)}
	verifier := NewVerifier(WithVerifierProvider(provider), WithVerifierNow(func() time.Time { return fixedTime }), WithVerifierCooldown(10*time.Second))
	verification, err := verifier.CreateVerification(context.Background(), ActionPlan{ID: "p"})
	if err != nil {
		t.Fatalf("CreateVerification returned error: %v", err)
	}
	if verification.CooldownSeconds != MinCooldownSeconds {
		t.Errorf("CooldownSeconds = %d, want %d (clamped to minimum)", verification.CooldownSeconds, MinCooldownSeconds)
	}
}

func TestCreateVerificationPropagatesPreSnapshotError(t *testing.T) {
	t.Parallel()
	provider := &fakeEvidenceProvider{preErr: errors.New("slo service down")}
	verifier := NewVerifier(WithVerifierProvider(provider), WithVerifierNow(func() time.Time { return fixedTime }))
	_, err := verifier.CreateVerification(context.Background(), ActionPlan{ID: "p"})
	if err == nil || !strings.Contains(err.Error(), "capture pre-snapshot") {
		t.Fatalf("expected 'capture pre-snapshot' error, got %v", err)
	}
}

func TestEvaluatePostSnapshotCaptureFailed(t *testing.T) {
	t.Parallel()
	plan := ActionPlan{ID: "plan-1", ActionCode: "deployment.rollout_restart"}
	pre := validSnapshot(map[string]any{"replicas": int32(3)}, sloSnapshot("burning_fast"))
	pending := ActionVerification{
		ID:              1,
		PlanID:          plan.ID,
		Status:          VerificationStatusPending,
		PreSnapshot:     pre,
		CooldownSeconds: MinCooldownSeconds,
	}
	provider := &fakeEvidenceProvider{postErr: errors.New("kubernetes api timeout")}
	verifier := NewVerifier(WithVerifierProvider(provider), WithVerifierNow(func() time.Time { return fixedTime }))

	got, err := verifier.Evaluate(context.Background(), plan, pending)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v (want nil — capture failure is a status, not an error)", err)
	}
	if got.Status != VerificationStatusFailed {
		t.Errorf("Status = %q, want %q", got.Status, VerificationStatusFailed)
	}
	if got.Reason != "post_snapshot_capture_failed" {
		t.Errorf("Reason = %q, want post_snapshot_capture_failed", got.Reason)
	}
	if !got.MissingEvidence {
		t.Error("MissingEvidence = false, want true")
	}
	if got.VerifiedAt == nil || !got.VerifiedAt.Equal(fixedTime) {
		t.Errorf("VerifiedAt = %v, want %v", got.VerifiedAt, fixedTime)
	}
	if provider.postCalls != 1 {
		t.Errorf("CapturePostSnapshot called %d times, want 1", provider.postCalls)
	}
}

func TestCompareEvidenceSLOImproved(t *testing.T) {
	t.Parallel()
	pre := validSnapshot(map[string]any{"replicas": int32(3)}, sloSnapshot("breached"))
	post := validSnapshot(map[string]any{"replicas": int32(3)}, sloSnapshot("healthy"))
	plan := ActionPlan{ActionCode: "deployment.rollout_restart"}
	comparison, missing := compareEvidence(pre, post, plan)
	if comparison != ComparisonImproved {
		t.Fatalf("comparison = %q, want %q", comparison, ComparisonImproved)
	}
	if missing {
		t.Error("missing = true, want false")
	}
}

func TestCompareEvidenceSLOWorse(t *testing.T) {
	t.Parallel()
	pre := validSnapshot(map[string]any{"replicas": int32(3)}, sloSnapshot("healthy"))
	post := validSnapshot(map[string]any{"replicas": int32(3)}, sloSnapshot("breached"))
	plan := ActionPlan{ActionCode: "deployment.rollout_restart"}
	comparison, missing := compareEvidence(pre, post, plan)
	if comparison != ComparisonWorse {
		t.Fatalf("comparison = %q, want %q", comparison, ComparisonWorse)
	}
	if missing {
		t.Error("missing = true, want false")
	}
}

func TestCompareEvidenceMissingPre(t *testing.T) {
	t.Parallel()
	pre := EvidenceSnapshot{} // empty ContentHash + zero CapturedAt
	post := validSnapshot(map[string]any{"replicas": int32(3)}, nil)
	plan := ActionPlan{ActionCode: "deployment.scale"}
	comparison, missing := compareEvidence(pre, post, plan)
	if comparison != ComparisonInsufficient {
		t.Fatalf("comparison = %q, want %q", comparison, ComparisonInsufficient)
	}
	if !missing {
		t.Error("missing = false, want true")
	}
}

func TestCompareEvidenceMissingPost(t *testing.T) {
	t.Parallel()
	pre := validSnapshot(map[string]any{"replicas": int32(3)}, nil)
	post := EvidenceSnapshot{} // empty ContentHash + zero CapturedAt
	plan := ActionPlan{ActionCode: "deployment.scale"}
	comparison, missing := compareEvidence(pre, post, plan)
	if comparison != ComparisonInsufficient {
		t.Fatalf("comparison = %q, want %q", comparison, ComparisonInsufficient)
	}
	if !missing {
		t.Error("missing = false, want true")
	}
}

func TestCompareEvidenceResourceScaleImproved(t *testing.T) {
	t.Parallel()
	before := int32(3)
	desired := int32(5)
	plan := ActionPlan{ActionCode: "deployment.scale", BeforeReplicas: &before, DesiredReplicas: &desired}
	// Both SLO states nil — deployment.scale is not SLO-bound, so the
	// resource comparison runs.
	pre := validSnapshot(map[string]any{"replicas": int32(before)}, nil)
	post := validSnapshot(map[string]any{"replicas": int32(desired)}, nil)
	comparison, missing := compareEvidence(pre, post, plan)
	if comparison != ComparisonImproved {
		t.Fatalf("comparison = %q, want %q", comparison, ComparisonImproved)
	}
	if missing {
		t.Error("missing = true, want false")
	}
}

func TestCompareEvidenceResourceScaleUnchanged(t *testing.T) {
	t.Parallel()
	before := int32(5)
	desired := int32(3)
	plan := ActionPlan{ActionCode: "deployment.scale", BeforeReplicas: &before, DesiredReplicas: &desired}
	// Post replicas still equal the before value → unchanged.
	pre := validSnapshot(map[string]any{"replicas": int32(before)}, nil)
	post := validSnapshot(map[string]any{"replicas": int32(before)}, nil)
	comparison, missing := compareEvidence(pre, post, plan)
	if comparison != ComparisonUnchanged {
		t.Fatalf("comparison = %q, want %q", comparison, ComparisonUnchanged)
	}
	if missing {
		t.Error("missing = true, want false")
	}
}

func TestCompareEvidenceRolloutRestartImproved(t *testing.T) {
	t.Parallel()
	// rollout_restart is SLO-bound: both SLO states must be present and
	// equal so the comparison falls through to the resource check.
	preSLO := sloSnapshot("healthy")
	postSLO := sloSnapshot("healthy")
	plan := ActionPlan{ActionCode: "deployment.rollout_restart"}
	pre := validSnapshot(map[string]any{"replicas": int32(3), "available_replicas": int32(3), "restarted_at": "2025-03-14T09:00:00Z"}, preSLO)
	post := validSnapshot(map[string]any{"replicas": int32(3), "available_replicas": int32(3), "restarted_at": "2025-03-14T09:26:53Z"}, postSLO)
	comparison, missing := compareEvidence(pre, post, plan)
	if comparison != ComparisonImproved {
		t.Fatalf("comparison = %q, want %q", comparison, ComparisonImproved)
	}
	if missing {
		t.Error("missing = true, want false")
	}
}

func TestCompareEvidenceRolloutRestartUnchangedWhenPodsNotReady(t *testing.T) {
	t.Parallel()
	preSLO := sloSnapshot("healthy")
	postSLO := sloSnapshot("healthy")
	plan := ActionPlan{ActionCode: "deployment.rollout_restart"}
	// restarted_at changed but available < replicas → not improved.
	pre := validSnapshot(map[string]any{"replicas": int32(3), "available_replicas": int32(3), "restarted_at": "t1"}, preSLO)
	post := validSnapshot(map[string]any{"replicas": int32(3), "available_replicas": int32(1), "restarted_at": "t2"}, postSLO)
	comparison, _ := compareEvidence(pre, post, plan)
	if comparison != ComparisonUnchanged {
		t.Fatalf("comparison = %q, want %q", comparison, ComparisonUnchanged)
	}
}

func TestClassifyStatus(t *testing.T) {
	t.Parallel()
	healthyPre := EvidenceSnapshot{SLOState: sloSnapshot("healthy")}
	unhealthyPre := EvidenceSnapshot{SLOState: sloSnapshot("breached")}
	noSLOPre := EvidenceSnapshot{}
	plan := ActionPlan{ActionCode: "deployment.scale"}
	cases := []struct {
		name       string
		comparison EvidenceComparison
		missing    bool
		pre        EvidenceSnapshot
		want       VerificationStatus
	}{
		{name: "improved_effective", comparison: ComparisonImproved, missing: false, pre: unhealthyPre, want: VerificationStatusEffective},
		{name: "worse_ineffective", comparison: ComparisonWorse, missing: false, pre: healthyPre, want: VerificationStatusIneffective},
		{name: "insufficient_unknown", comparison: ComparisonInsufficient, missing: false, pre: healthyPre, want: VerificationStatusUnknown},
		{name: "missing_overrides_improved_to_unknown", comparison: ComparisonImproved, missing: true, pre: healthyPre, want: VerificationStatusUnknown},
		{name: "unchanged_healthy_pre_effective", comparison: ComparisonUnchanged, missing: false, pre: healthyPre, want: VerificationStatusEffective},
		{name: "unchanged_unhealthy_pre_ineffective", comparison: ComparisonUnchanged, missing: false, pre: unhealthyPre, want: VerificationStatusIneffective},
		{name: "unchanged_no_slo_pre_ineffective", comparison: ComparisonUnchanged, missing: false, pre: noSLOPre, want: VerificationStatusIneffective},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := classifyStatus(tc.comparison, tc.missing, tc.pre, EvidenceSnapshot{}, plan)
			if got != tc.want {
				t.Fatalf("classifyStatus(%q, missing=%v) = %q, want %q", tc.comparison, tc.missing, got, tc.want)
			}
		})
	}
}

func TestHashSnapshot(t *testing.T) {
	t.Parallel()
	t.Run("zero_captured_at_returns_empty", func(t *testing.T) {
		t.Parallel()
		got := HashSnapshot(EvidenceSnapshot{ResourceState: map[string]any{"replicas": int32(3)}})
		if got != "" {
			t.Fatalf("HashSnapshot = %q, want empty when CapturedAt is zero", got)
		}
	})
	t.Run("non_zero_captured_at_returns_hex", func(t *testing.T) {
		t.Parallel()
		snap := EvidenceSnapshot{
			ResourceState: map[string]any{"replicas": int32(3)},
			CapturedAt:    fixedTime,
		}
		got := HashSnapshot(snap)
		if got == "" {
			t.Fatal("HashSnapshot returned empty for non-zero CapturedAt")
		}
		if len(got) != 64 {
			t.Fatalf("HashSnapshot length = %d, want 64 (SHA-256 hex)", len(got))
		}
		// Deterministic: same input → same hash.
		again := HashSnapshot(snap)
		if again != got {
			t.Fatalf("HashSnapshot not deterministic: %q != %q", again, got)
		}
	})
	t.Run("includes_slo_state_when_present", func(t *testing.T) {
		t.Parallel()
		withoutSLO := EvidenceSnapshot{ResourceState: map[string]any{"r": int32(1)}, CapturedAt: fixedTime}
		withSLO := EvidenceSnapshot{ResourceState: map[string]any{"r": int32(1)}, CapturedAt: fixedTime, SLOState: sloSnapshot("healthy")}
		h1 := HashSnapshot(withoutSLO)
		h2 := HashSnapshot(withSLO)
		if h1 == h2 {
			t.Fatal("expected different hashes when SLOState is added")
		}
	})
}

func TestSloStateRank(t *testing.T) {
	t.Parallel()
	cases := []struct {
		state string
		want  int
	}{
		{"healthy", 0},
		{"burning_slow", 1},
		{"burning_fast", 2},
		{"breached", 3},
		{"unavailable", 4},
		{"", 4},
		{"unknown_state", 4},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.state, func(t *testing.T) {
			t.Parallel()
			if got := sloStateRank(tc.state); got != tc.want {
				t.Fatalf("sloStateRank(%q) = %d, want %d", tc.state, got, tc.want)
			}
		})
	}
}

func TestSloBoundAction(t *testing.T) {
	t.Parallel()
	cases := []struct {
		action string
		want   bool
	}{
		{"deployment.rollback", true},
		{"deployment.image_update", true},
		{"deployment.rollout_restart", true},
		{"deployment.scale", false},
		{"cronjob.suspend", false},
		{"cronjob.resume", false},
		{"unknown.action", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.action, func(t *testing.T) {
			t.Parallel()
			if got := sloBoundAction(tc.action); got != tc.want {
				t.Fatalf("sloBoundAction(%q) = %v, want %v", tc.action, got, tc.want)
			}
		})
	}
}

// TestVerifierEvaluateEndToEnd wires a full Evaluate cycle to assert the
// missing-evidence invariant: missing post evidence yields Unknown, never
// Effective, even when the pre-snapshot looked healthy.
func TestVerifierEvaluateEndToEndMissingEvidenceIsUnknown(t *testing.T) {
	t.Parallel()
	plan := ActionPlan{ID: "plan-1", ActionCode: "deployment.rollout_restart"}
	pre := validSnapshot(map[string]any{"replicas": int32(3)}, sloSnapshot("healthy"))
	pending := ActionVerification{ID: 1, PlanID: plan.ID, Status: VerificationStatusPending, PreSnapshot: pre}
	// Post snapshot has no ContentHash → missing → Unknown.
	provider := &fakeEvidenceProvider{post: EvidenceSnapshot{}}
	verifier := NewVerifier(WithVerifierProvider(provider), WithVerifierNow(func() time.Time { return fixedTime }))

	got, err := verifier.Evaluate(context.Background(), plan, pending)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if got.Status != VerificationStatusUnknown {
		t.Fatalf("Status = %q, want %q (missing evidence must never resolve a diagnosis)", got.Status, VerificationStatusUnknown)
	}
	if !got.MissingEvidence {
		t.Error("MissingEvidence = false, want true")
	}
	if got.EvidenceComparison != ComparisonInsufficient {
		t.Errorf("EvidenceComparison = %q, want %q", got.EvidenceComparison, ComparisonInsufficient)
	}
}
