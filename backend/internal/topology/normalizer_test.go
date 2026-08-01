package topology

import (
	"context"
	"testing"
	"time"
)

func TestChangeNormalizer_FromPlan_Succeeded(t *testing.T) {
	n := NewChangeNormalizer()
	created := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)
	executed := time.Date(2026, 7, 31, 10, 5, 0, 0, time.UTC)
	auditID := int64(42)

	event, err := n.FromPlan(ChangePlanInput{
		Kind:       "promotion",
		ClusterID:  1,
		Namespace:  "default",
		PlanID:     "plan-abc",
		Target:     ResourceCitation{Kind: "Deployment", Name: "web", UID: "dep-uid-1"},
		Action:     "promote",
		PlanStatus: "succeeded",
		CreatedAt:  created,
		ExecutedAt: executed,
		AuditID:    &auditID,
		RequestID:  "req-123",
	})
	if err != nil {
		t.Fatalf("FromPlan failed: %v", err)
	}

	if event.Kind != "promotion" {
		t.Errorf("expected kind promotion, got %s", event.Kind)
	}
	if event.Result != "succeeded" {
		t.Errorf("expected result succeeded, got %s", event.Result)
	}
	if event.Confidence != "high" {
		t.Errorf("expected confidence high (platform + audit_id), got %s", event.Confidence)
	}
	if event.Source != "platform" {
		t.Errorf("expected source platform, got %s", event.Source)
	}
	if event.StartedAt != created {
		t.Errorf("expected started_at %v, got %v", created, event.StartedAt)
	}
	if event.FinishedAt == nil || !event.FinishedAt.Equal(executed) {
		t.Errorf("expected finished_at %v, got %v", executed, event.FinishedAt)
	}
	if event.AuditID == nil || *event.AuditID != auditID {
		t.Errorf("expected audit_id %d", auditID)
	}
	if event.RequestID != "req-123" {
		t.Errorf("expected request_id req-123, got %s", event.RequestID)
	}
}

func TestChangeNormalizer_FromPlan_Pending(t *testing.T) {
	n := NewChangeNormalizer()
	event, err := n.FromPlan(ChangePlanInput{
		Kind:       "backup",
		ClusterID:  1,
		Namespace:  "default",
		PlanID:     "plan-def",
		Target:     ResourceCitation{Kind: "Namespace", Name: "default", UID: "ns-uid"},
		PlanStatus: "awaiting_confirmation",
		CreatedAt:  time.Now(),
	})
	if err != nil {
		t.Fatalf("FromPlan failed: %v", err)
	}
	if event.Result != "pending" {
		t.Errorf("expected result pending, got %s", event.Result)
	}
	if event.FinishedAt != nil {
		t.Error("expected nil finished_at for pending plan")
	}
	if event.Confidence != "low" {
		t.Errorf("expected confidence low (no audit_id), got %s", event.Confidence)
	}
}

func TestChangeNormalizer_FromPlan_ExpiredMapsToFailed(t *testing.T) {
	n := NewChangeNormalizer()
	event, err := n.FromPlan(ChangePlanInput{
		Kind:       "maintenance",
		ClusterID:  1,
		PlanID:     "plan-ghi",
		Target:     ResourceCitation{Kind: "Node", Name: "node-1", Incomplete: true},
		PlanStatus: "expired",
		CreatedAt:  time.Now(),
	})
	if err != nil {
		t.Fatalf("FromPlan failed: %v", err)
	}
	if event.Result != "failed" {
		t.Errorf("expected result failed for expired, got %s", event.Result)
	}
}

func TestChangeNormalizer_FromPlan_Partial(t *testing.T) {
	n := NewChangeNormalizer()
	event, err := n.FromPlan(ChangePlanInput{
		Kind:       "promotion",
		ClusterID:  1,
		PlanID:     "plan-jkl",
		Target:     ResourceCitation{Kind: "Deployment", Name: "web"},
		PlanStatus: "partial",
		CreatedAt:  time.Now(),
	})
	if err != nil {
		t.Fatalf("FromPlan failed: %v", err)
	}
	if event.Result != "partial" {
		t.Errorf("expected result partial, got %s", event.Result)
	}
}

func TestChangeNormalizer_FromPlan_DefaultAction(t *testing.T) {
	n := NewChangeNormalizer()
	event, err := n.FromPlan(ChangePlanInput{
		Kind:       "restore",
		ClusterID:  1,
		PlanID:     "plan-mno",
		Target:     ResourceCitation{Kind: "Namespace", Name: "quarantine"},
		PlanStatus: "succeeded",
		CreatedAt:  time.Now(),
	})
	if err != nil {
		t.Fatalf("FromPlan failed: %v", err)
	}
	if event.Action != "restore.rehearse" {
		t.Errorf("expected default action restore.rehearse, got %s", event.Action)
	}
}

func TestChangeNormalizer_FromPlan_ValidationErrors(t *testing.T) {
	n := NewChangeNormalizer()
	tests := []struct {
		name  string
		input ChangePlanInput
	}{
		{"empty kind", ChangePlanInput{PlanID: "p1"}},
		{"invalid kind", ChangePlanInput{Kind: "invalid", PlanID: "p1"}},
		{"empty plan_id", ChangePlanInput{Kind: "promotion"}},
		{"invalid source", ChangePlanInput{Kind: "promotion", PlanID: "p1", Source: "invalid"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := n.FromPlan(tt.input)
			if err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestChangeNormalizer_FromAudit(t *testing.T) {
	n := NewChangeNormalizer()
	observed := time.Now().UTC()
	event, err := n.FromAudit(AuditChangeInput{
		ClusterID:  1,
		Namespace:  "default",
		Kind:       "rollout",
		PlanID:     "plan-pqr",
		Target:     ResourceCitation{Kind: "Deployment", Name: "web", UID: "dep-uid"},
		Action:     "deployment.rollout_restart",
		Actor:      "alice",
		Result:     "succeeded",
		ObservedAt: observed,
		AuditID:    99,
		RequestID:  "req-456",
	})
	if err != nil {
		t.Fatalf("FromAudit failed: %v", err)
	}
	if event.Kind != "rollout" {
		t.Errorf("expected kind rollout, got %s", event.Kind)
	}
	if event.Result != "succeeded" {
		t.Errorf("expected result succeeded, got %s", event.Result)
	}
	if event.Confidence != "high" {
		t.Errorf("expected confidence high (has request_id), got %s", event.Confidence)
	}
	if event.AuditID == nil || *event.AuditID != 99 {
		t.Errorf("expected audit_id 99")
	}
}

func TestChangeNormalizer_FromAudit_DeniedMapsToFailed(t *testing.T) {
	n := NewChangeNormalizer()
	event, err := n.FromAudit(AuditChangeInput{
		ClusterID:  1,
		Kind:       "audit",
		AuditID:    1,
		Result:     "denied",
		ObservedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("FromAudit failed: %v", err)
	}
	if event.Result != "failed" {
		t.Errorf("expected result failed for denied, got %s", event.Result)
	}
}

func TestNormalizePlanStatus(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"succeeded", "succeeded"},
		{"failed", "failed"},
		{"expired", "failed"},
		{"partial", "partial"},
		{"awaiting_confirmation", "pending"},
		{"executing", "pending"},
		{"", "pending"},
		{"unknown", "pending"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := normalizePlanStatus(tt.input); got != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, got)
			}
		})
	}
}

func TestHashSafeDiff(t *testing.T) {
	h := HashSafeDiff([]byte("some diff content"))
	if h == "" {
		t.Error("expected non-empty hash")
	}
	if len(h) != 64 {
		t.Errorf("expected 64-char hex hash, got %d chars", len(h))
	}

	h2 := HashSafeDiff([]byte("some diff content"))
	if h != h2 {
		t.Error("hash should be deterministic")
	}

	h3 := HashSafeDiff([]byte("different content"))
	if h == h3 {
		t.Error("different content should produce different hash")
	}

	if HashSafeDiff(nil) != "" {
		t.Error("nil input should produce empty hash")
	}
}

func TestServiceIngestChangeEvent_Validation(t *testing.T) {
	svc := NewService(nil, NopRepository{}, nil)
	ctx := context.Background()

	// Valid event
	valid := &ChangeEvent{
		ClusterID:  1,
		Kind:       "promotion",
		PlanID:     "plan-1",
		Target:     ResourceCitation{Kind: "Deployment", Name: "web"},
		StartedAt:  time.Now(),
		Result:     "succeeded",
		Confidence: "high",
		Source:     "platform",
	}
	if err := svc.IngestChangeEvent(ctx, valid); err != nil {
		t.Fatalf("valid event should persist, got: %v", err)
	}

	// Missing kind
	if err := svc.IngestChangeEvent(ctx, &ChangeEvent{PlanID: "p1"}); err == nil {
		t.Error("expected error for missing kind")
	}

	// Invalid kind
	if err := svc.IngestChangeEvent(ctx, &ChangeEvent{Kind: "invalid", PlanID: "p1"}); err == nil {
		t.Error("expected error for invalid kind")
	}

	// Missing plan_id and audit_id
	if err := svc.IngestChangeEvent(ctx, &ChangeEvent{Kind: "promotion"}); err == nil {
		t.Error("expected error for missing plan_id and audit_id")
	}

	// Nil event
	if err := svc.IngestChangeEvent(ctx, nil); err == nil {
		t.Error("expected error for nil event")
	}
}

func TestServiceIngestChangeEvent_FillsDefaults(t *testing.T) {
	svc := NewService(nil, NopRepository{}, nil)
	ctx := context.Background()

	event := &ChangeEvent{
		ClusterID: 1,
		Kind:      "backup",
		PlanID:    "plan-2",
		Target:    ResourceCitation{Kind: "Namespace", Name: "default"},
		StartedAt: time.Now(),
	}
	if err := svc.IngestChangeEvent(ctx, event); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if event.Result != "pending" {
		t.Errorf("expected default result pending, got %s", event.Result)
	}
	if event.Confidence != "low" {
		t.Errorf("expected default confidence low, got %s", event.Confidence)
	}
	if event.Source != "platform" {
		t.Errorf("expected default source platform, got %s", event.Source)
	}
}
