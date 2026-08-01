package signal

import (
	"testing"
	"time"

	"k8s-aiops.local/backend/internal/alert"
	"k8s-aiops.local/backend/internal/diagnosis"
	"k8s-aiops.local/backend/internal/namespaceposture"
)

func TestDiagnosisNormalizer_FromRecord(t *testing.T) {
	n := NewDiagnosisNormalizer()
	r := diagnosis.Record{
		RuleID:     diagnosis.RulePodPending,
		Severity:   "warning",
		Status:     "open",
		ClusterID:  1,
		Resource:   diagnosis.ResourceRef{Kind: "Pod", Namespace: "default", Name: "nginx", UID: "uid-1"},
		ObservedAt: time.Now(),
	}
	req, err := n.FromRecord(r, "run-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.SignalID != "diag.pod.pending.v1" {
		t.Fatalf("expected signal_id diag.pod.pending.v1, got %s", req.SignalID)
	}
	if req.Producer != ProducerDiagnosis {
		t.Fatalf("expected producer diagnosis, got %s", req.Producer)
	}
	if req.State != StateActive {
		t.Fatalf("expected state active, got %s", req.State)
	}
}

func TestDiagnosisNormalizer_UnmappedRuleFails(t *testing.T) {
	n := NewDiagnosisNormalizer()
	r := diagnosis.Record{RuleID: "unknown.rule.v1"}
	_, err := n.FromRecord(r, "run-1")
	if err == nil {
		t.Fatal("expected error for unmapped rule")
	}
}

func TestDiagnosisNormalizer_ResolvedStatus(t *testing.T) {
	n := NewDiagnosisNormalizer()
	r := diagnosis.Record{
		RuleID:     diagnosis.RulePodPending,
		Status:     "resolved",
		ClusterID:  1,
		Resource:   diagnosis.ResourceRef{Kind: "Pod", Name: "x"},
		ObservedAt: time.Now(),
	}
	req, err := n.FromRecord(r, "run-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.State != StateResolved {
		t.Fatalf("expected state resolved, got %s", req.State)
	}
}

func TestAlertNormalizer_FromInstance(t *testing.T) {
	n := NewAlertNormalizer()
	rule := alert.Rule{ID: 1, ClusterID: 1, ResourceKind: "Node", ResourceName: "node-1", DisplayName: "CPU high"}
	inst := alert.Instance{ID: 1, State: alert.StateFiring, LastFiredAt: time.Now()}
	req, err := n.FromInstance(rule, inst, "run-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.SignalID != "alert.firing.v1" {
		t.Fatalf("expected signal_id alert.firing.v1, got %s", req.SignalID)
	}
	// Alert rules don't carry UID; the normalizer doesn't set Incomplete
	// itself, but BuildOccurrence will mark it. Here we just check the
	// signal id mapping.
}

func TestAlertNormalizer_ResolvedTransition(t *testing.T) {
	n := NewAlertNormalizer()
	rule := alert.Rule{ID: 1, ClusterID: 1, ResourceKind: "Node", ResourceName: "node-1"}
	resolved := time.Now()
	inst := alert.Instance{ID: 1, State: alert.StateResolved, ResolvedAt: &resolved}
	req, err := n.FromInstance(rule, inst, "run-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.SignalID != "alert.resolved.v1" {
		t.Fatalf("expected signal_id alert.resolved.v1, got %s", req.SignalID)
	}
	if req.State != StateResolved {
		t.Fatalf("expected state resolved, got %s", req.State)
	}
}

func TestMetricBreachNormalizer_FiringProducesSignal(t *testing.T) {
	n := NewMetricBreachNormalizer()
	in := MetricBreachInput{
		ClusterID:       1,
		Resource:        ResourceCitation{Kind: "Node", Name: "node-1", UID: "uid-1"},
		MetricName:      "cpu_usage",
		EvaluationState: "firing",
		WindowStart:     time.Now().Add(-5 * time.Minute),
		WindowEnd:       time.Now(),
		ObservedAt:      time.Now(),
	}
	req, ok, err := n.FromEvaluation(in, "run-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true for firing state")
	}
	if req.SignalID != "metric.sustained_breach.v1" {
		t.Fatalf("expected signal_id metric.sustained_breach.v1, got %s", req.SignalID)
	}
}

func TestMetricBreachNormalizer_NonFiringSkips(t *testing.T) {
	n := NewMetricBreachNormalizer()
	in := MetricBreachInput{EvaluationState: "normal"}
	_, ok, err := n.FromEvaluation(in, "run-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false for non-firing state")
	}
}

func TestPostureNormalizer_FromFinding(t *testing.T) {
	n := NewPostureNormalizer()
	f := namespaceposture.Finding{
		Code:     namespaceposture.CodeMissingQuota,
		Severity: "info",
		Summary:  "Namespace has no ResourceQuota",
		Resource: namespaceposture.ResourceCitation{Kind: "Namespace", Name: "default"},
	}
	req, ok, err := n.FromFinding(1, "default", f, "run-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true for MISSING_QUOTA")
	}
	if req.SignalID != "posture.missing_quota.v1" {
		t.Fatalf("expected signal_id posture.missing_quota.v1, got %s", req.SignalID)
	}
}

func TestPostureNormalizer_UnmappedCodeSkips(t *testing.T) {
	n := NewPostureNormalizer()
	f := namespaceposture.Finding{Code: "UNKNOWN_CODE"}
	_, ok, err := n.FromFinding(1, "default", f, "run-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false for unmapped code")
	}
}

func TestChangeNormalizer_PromotionSucceeded(t *testing.T) {
	n := NewChangeNormalizer()
	o := ChangeOutcome{
		Kind:      ChangeKindPromotion,
		ClusterID: 1,
		Namespace: "default",
		Status:    "succeeded",
		PlanID:    42,
		ActorName: "admin",
	}
	req, ok, err := n.FromOutcome(o, time.Now(), "run-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true for succeeded promotion")
	}
	if req.SignalID != "change.promotion.completed.v1" {
		t.Fatalf("expected signal_id change.promotion.completed.v1, got %s", req.SignalID)
	}
	if req.Severity != "info" {
		t.Fatalf("expected severity info for succeeded, got %s", req.Severity)
	}
}

func TestChangeNormalizer_BackupFailed(t *testing.T) {
	n := NewChangeNormalizer()
	o := ChangeOutcome{
		Kind:      ChangeKindBackup,
		ClusterID: 1,
		Namespace: "default",
		Status:    "failed",
		PlanID:    7,
	}
	req, ok, err := n.FromOutcome(o, time.Now(), "run-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true for failed backup")
	}
	if req.SignalID != "change.backup.failed.v1" {
		t.Fatalf("expected signal_id change.backup.failed.v1, got %s", req.SignalID)
	}
	if req.Severity != "warning" {
		t.Fatalf("expected severity warning for failed, got %s", req.Severity)
	}
}

func TestChangeNormalizer_PendingStatusSkips(t *testing.T) {
	n := NewChangeNormalizer()
	o := ChangeOutcome{Kind: ChangeKindPromotion, Status: "pending"}
	_, ok, err := n.FromOutcome(o, time.Now(), "run-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false for pending status")
	}
}
