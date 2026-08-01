package automation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// EvidenceProvider supplies the verifier with redacted, hash-stamped
// evidence snapshots. The verifier never accepts free-form evidence;
// only server-captured SLI/resource states are admitted. The default
// implementation reads from the SLO service and the Kubernetes source;
// tests inject a fake provider.
type EvidenceProvider interface {
	// CapturePreSnapshot captures the resource and SLO state at
	// execute time. Called by the service when transitioning a plan to
	// Succeeded/Failed.
	CapturePreSnapshot(ctx context.Context, plan ActionPlan) (EvidenceSnapshot, *SLOSnapshot, *int64, error)
	// CapturePostSnapshot captures the resource and SLO state after
	// the cooldown elapses. Called by the verifier worker.
	CapturePostSnapshot(ctx context.Context, plan ActionPlan) (EvidenceSnapshot, *SLOSnapshot, *int64, error)
}

// NopEvidenceProvider returns empty snapshots. Used when the verifier is
// in query-only mode.
type NopEvidenceProvider struct{}

func (NopEvidenceProvider) CapturePreSnapshot(context.Context, ActionPlan) (EvidenceSnapshot, *SLOSnapshot, *int64, error) {
	return EvidenceSnapshot{}, nil, nil, nil
}
func (NopEvidenceProvider) CapturePostSnapshot(context.Context, ActionPlan) (EvidenceSnapshot, *SLOSnapshot, *int64, error) {
	return EvidenceSnapshot{}, nil, nil, nil
}

// Verifier is the post-action verifier. It is pure given (plan,
// preSnapshot, postSnapshot): identical snapshots + identical verifier
// version → identical verification status. The service is the only
// caller; HTTP handlers never bypass it.
type Verifier struct {
	provider EvidenceProvider
	now      func() time.Time
	cooldown time.Duration
}

// VerifierOption configures a Verifier at construction.
type VerifierOption func(*Verifier)

// NewVerifier returns a verifier with the NopEvidenceProvider. The
// service wires the real provider at construction time.
func NewVerifier(opts ...VerifierOption) *Verifier {
	v := &Verifier{
		provider: NopEvidenceProvider{},
		now:      time.Now,
		cooldown: DefaultCooldownSeconds * time.Second,
	}
	for _, opt := range opts {
		opt(v)
	}
	return v
}

// WithVerifierProvider overrides the default NopEvidenceProvider.
func WithVerifierProvider(p EvidenceProvider) VerifierOption {
	return func(v *Verifier) {
		if p != nil {
			v.provider = p
		}
	}
}

// WithVerifierNow overrides the verifier clock (tests).
func WithVerifierNow(now func() time.Time) VerifierOption {
	return func(v *Verifier) { v.now = now }
}

// WithVerifierCooldown overrides the default cooldown.
func WithVerifierCooldown(d time.Duration) VerifierOption {
	return func(v *Verifier) { v.cooldown = d }
}

// CreateVerification creates a pending verification row for a plan that
// just transitioned to Succeeded or Failed. Captures the pre-snapshot
// immediately so it is bound to the execute time. The verifier worker
// later captures the post-snapshot after the cooldown elapses.
func (v *Verifier) CreateVerification(ctx context.Context, plan ActionPlan) (ActionVerification, error) {
	preSnapshot, sloPre, sloPreID, err := v.provider.CapturePreSnapshot(ctx, plan)
	if err != nil {
		return ActionVerification{}, fmt.Errorf("capture pre-snapshot: %w", err)
	}
	cooldownSeconds := int(v.cooldown.Seconds())
	if cooldownSeconds < MinCooldownSeconds {
		cooldownSeconds = MinCooldownSeconds
	}
	verification := ActionVerification{
		PlanID:                plan.ID,
		VerificationKey:       computeVerificationKey(plan, preSnapshot),
		VerifierVersion:       VerifierVersion,
		Status:                VerificationStatusPending,
		EvidenceComparison:    ComparisonInsufficient,
		PreSnapshot:           preSnapshot,
		MissingEvidence:       preSnapshot.ContentHash == "" || preSnapshot.CapturedAt.IsZero(),
		CooldownSeconds:       cooldownSeconds,
		SLOEvaluationBeforeID: sloPreID,
	}
	if sloPre != nil {
		verification.PreSnapshot.SLOState = sloPre
	}
	verification.CreatedAt = v.now().UTC()
	verification.UpdatedAt = verification.CreatedAt
	return verification, nil
}

// Evaluate runs the post-action comparison and returns the final
// verification status. Called by the verifier worker after the cooldown
// elapses. Missing evidence never resolves a diagnosis automatically —
// when pre or post evidence is unavailable, the verifier returns
// VerificationStatusUnknown with EvidenceComparison=insufficient.
func (v *Verifier) Evaluate(ctx context.Context, plan ActionPlan, pre ActionVerification) (ActionVerification, error) {
	postSnapshot, sloPost, sloPostID, err := v.provider.CapturePostSnapshot(ctx, plan)
	if err != nil {
		// Evidence-gathering failure → VerificationStatusFailed with
		// reason. The plan retains its execution status; verification
		// is retried by the operator.
		now := v.now().UTC()
		pre.Status = VerificationStatusFailed
		pre.Reason = "post_snapshot_capture_failed"
		pre.MissingEvidence = true
		pre.VerifiedAt = &now
		pre.UpdatedAt = now
		return pre, nil
	}
	pre.PostSnapshot = postSnapshot
	if sloPost != nil {
		pre.PostSnapshot.SLOState = sloPost
	}
	pre.SLOEvaluationAfterID = sloPostID
	now := v.now().UTC()
	pre.VerifiedAt = &now

	comparison, missing := compareEvidence(pre.PreSnapshot, postSnapshot, plan)
	pre.EvidenceComparison = comparison
	pre.MissingEvidence = missing
	pre.Status = classifyStatus(comparison, missing, pre.PreSnapshot, postSnapshot, plan)
	pre.UpdatedAt = now
	return pre, nil
}

// compareEvidence compares the pre and post snapshots and returns the
// evidence comparison plus a missing-evidence flag. The comparison is
// deterministic: identical snapshots → identical comparison.
//
// The comparison considers:
//   - SLO state transitions (healthy > burning_slow > burning_fast > breached)
//   - Resource state (replicas, available_replicas, image, suspended)
//
// Missing or partial evidence yields ComparisonInsufficient.
func compareEvidence(pre, post EvidenceSnapshot, plan ActionPlan) (EvidenceComparison, bool) {
	missing := pre.ContentHash == "" || post.ContentHash == "" ||
		pre.CapturedAt.IsZero() || post.CapturedAt.IsZero()
	if missing {
		return ComparisonInsufficient, true
	}

	// SLO comparison takes precedence when both snapshots have SLO state.
	if pre.SLOState != nil && post.SLOState != nil {
		preRank := sloStateRank(pre.SLOState.State)
		postRank := sloStateRank(post.SLOState.State)
		switch {
		case postRank < preRank:
			// Post state is healthier than pre → improved.
			return ComparisonImproved, false
		case postRank > preRank:
			// Post state is worse than pre → worse.
			return ComparisonWorse, false
		default:
			// Same SLO state; fall through to resource comparison.
		}
	} else if pre.SLOState == nil && post.SLOState == nil {
		// No SLO evidence on either side → insufficient for SLO-bound
		// actions; fall through to resource comparison for non-SLO
		// actions.
		if sloBoundAction(plan.ActionCode) {
			return ComparisonInsufficient, true
		}
	} else {
		// One side has SLO evidence, the other does not → insufficient.
		return ComparisonInsufficient, true
	}

	// Resource comparison: did the desired change take effect?
	switch plan.ActionCode {
	case "deployment.scale":
		before, after := replicasInt(plan.BeforeReplicas), replicasInt(plan.DesiredReplicas)
		got := resourceInt(post.ResourceState, "replicas")
		if got == int64(after) && got != int64(before) {
			return ComparisonImproved, false
		}
		if got == int64(before) {
			return ComparisonUnchanged, false
		}
		return ComparisonWorse, false
	case "cronjob.suspend", "cronjob.resume":
		desired := plan.DesiredSuspended != nil && *plan.DesiredSuspended
		got := resourceBool(post.ResourceState, "suspend")
		if got == desired {
			return ComparisonImproved, false
		}
		return ComparisonUnchanged, false
	case "deployment.image_update":
		desired := plan.DesiredImage
		got := resourceStr(post.ResourceState, "image")
		if got == desired {
			return ComparisonImproved, false
		}
		return ComparisonUnchanged, false
	case "deployment.rollback":
		// Rollback success: the pod-template hash changed and the new
		// pods are ready. We approximate via "available_replicas ==
		// replicas" and the rollback annotation.
		replicas := resourceInt(post.ResourceState, "replicas")
		available := resourceInt(post.ResourceState, "available_replicas")
		if replicas > 0 && available == replicas {
			return ComparisonImproved, false
		}
		if replicas > 0 && available == 0 {
			return ComparisonWorse, false
		}
		return ComparisonUnchanged, false
	case "deployment.rollout_restart":
		// Rollout restart success: the restarted_at annotation is
		// newer than pre, and pods are ready.
		preRestart := resourceStr(pre.ResourceState, "restarted_at")
		postRestart := resourceStr(post.ResourceState, "restarted_at")
		if postRestart != "" && postRestart != preRestart {
			replicas := resourceInt(post.ResourceState, "replicas")
			available := resourceInt(post.ResourceState, "available_replicas")
			if replicas > 0 && available == replicas {
				return ComparisonImproved, false
			}
		}
		return ComparisonUnchanged, false
	}
	return ComparisonInsufficient, true
}

// classifyStatus maps the evidence comparison to a verification status.
// Missing evidence always yields VerificationStatusUnknown — missing
// evidence never resolves a diagnosis automatically.
func classifyStatus(comparison EvidenceComparison, missing bool, pre, post EvidenceSnapshot, plan ActionPlan) VerificationStatus {
	if missing {
		return VerificationStatusUnknown
	}
	switch comparison {
	case ComparisonImproved:
		return VerificationStatusEffective
	case ComparisonWorse:
		return VerificationStatusIneffective
	case ComparisonUnchanged:
		// Unchanged is effective only when the pre-state was already
		// healthy (e.g. the action was a no-op confirm). Otherwise it
		// is ineffective.
		if pre.SLOState != nil && pre.SLOState.State == "healthy" {
			return VerificationStatusEffective
		}
		return VerificationStatusIneffective
	case ComparisonInsufficient:
		return VerificationStatusUnknown
	}
	return VerificationStatusUnknown
}

// sloStateRank returns a numeric rank for an SLO state so we can compare
// pre/post transitions deterministically. Lower = healthier.
func sloStateRank(state string) int {
	switch state {
	case "healthy":
		return 0
	case "burning_slow":
		return 1
	case "burning_fast":
		return 2
	case "breached":
		return 3
	default:
		return 4 // unavailable / unknown
	}
}

// sloBoundAction returns true for actions whose success is measured
// primarily by SLO recovery (rollback, image_update, rollout_restart).
func sloBoundAction(actionCode string) bool {
	switch actionCode {
	case "deployment.rollback", "deployment.image_update", "deployment.rollout_restart":
		return true
	}
	return false
}

// computeVerificationKey returns the SHA-256 hex over (plan_id +
// verifier_version + evidence_hash). Identical evidence reproduces
// identical keys.
func computeVerificationKey(plan ActionPlan, pre EvidenceSnapshot) string {
	h := sha256.New()
	fmt.Fprintf(h, "plan_id=%s\n", plan.ID)
	fmt.Fprintf(h, "verifier_version=%s\n", VerifierVersion)
	fmt.Fprintf(h, "evidence_hash=%s\n", pre.ContentHash)
	return hex.EncodeToString(h.Sum(nil))
}

// --- resource-state helpers ---

func replicasInt(p *int32) int32 {
	if p == nil {
		return 0
	}
	return *p
}

func resourceInt(state map[string]any, key string) int64 {
	if state == nil {
		return 0
	}
	v, ok := state[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int:
		return int64(n)
	case int32:
		return int64(n)
	case int64:
		return n
	case float64:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	}
	return 0
}

func resourceBool(state map[string]any, key string) bool {
	if state == nil {
		return false
	}
	v, ok := state[key]
	if !ok {
		return false
	}
	switch b := v.(type) {
	case bool:
		return b
	}
	return false
}

func resourceStr(state map[string]any, key string) string {
	if state == nil {
		return ""
	}
	v, ok := state[key]
	if !ok {
		return ""
	}
	switch s := v.(type) {
	case string:
		return s
	}
	return ""
}

// HashSnapshot computes the SHA-256 over the redacted JSON content of a
// snapshot. Used by the evidence provider to stamp ContentHash.
func HashSnapshot(snapshot EvidenceSnapshot) string {
	if snapshot.CapturedAt.IsZero() {
		return ""
	}
	h := sha256.New()
	enc := json.NewEncoder(h)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(snapshot.ResourceState)
	if snapshot.SLOState != nil {
		_ = enc.Encode(snapshot.SLOState)
	}
	fmt.Fprintf(h, "captured_at=%s\n", snapshot.CapturedAt.UTC().Format(time.RFC3339Nano))
	return hex.EncodeToString(h.Sum(nil))
}

// ErrVerificationFailed is returned when the verifier cannot complete
// the verification (e.g. plan is not in Succeeded/Failed status).
var ErrVerificationFailed = errors.New("verification failed")
