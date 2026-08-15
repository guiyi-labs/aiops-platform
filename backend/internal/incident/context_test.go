package incident

import (
	"testing"
	"time"
)

func TestBuildContextCockpit_ResourceContextContract(t *testing.T) {
	observedAt := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	inc := Incident{
		ID:         1,
		Number:     "INC-000001",
		Title:      "Node NotReady",
		Severity:   SeverityCritical,
		Status:     StatusConfirmed,
		Summary:    "k8s-worker-1 not ready",
		SourceType: SourceTypeDiagnosis,
		ClusterID:  7,
		Resource:   ResourceRef{Kind: "Node", Namespace: "", Name: "k8s-worker-1"},
		SLADueAt:   observedAt.Add(time.Hour),
		Overdue:    false,
		Version:    3,
		CreatedAt:  observedAt.Add(-2 * time.Hour),
		UpdatedAt:  observedAt.Add(-30 * time.Minute),
		Timeline: []TimelineEvent{
			{ID: 1, EventType: EventTypeSystem, Content: "incident opened", CreatedAt: observedAt.Add(-2 * time.Hour)},
			{ID: 2, EventType: EventTypeNote, Content: "assignee confirmed", CreatedAt: observedAt.Add(-1 * time.Hour)},
			{ID: 3, EventType: EventTypeSystem, Content: "escalation level 1", CreatedAt: observedAt.Add(-10 * time.Minute)},
		},
	}
	evidence := []EvidenceItem{
		{SourceType: SourceTypeDiagnosis, SourceRef: "diagnosis:42", Title: "node.not_ready", ObservedAt: observedAt.Add(-45 * time.Minute).Format(time.RFC3339), DeepLink: "/diagnoses"},
		{SourceType: SourceTypeAlert, SourceRef: "alert:9", Title: "NodeNotReady Alert", ObservedAt: observedAt.Add(-50 * time.Minute).Format(time.RFC3339), DeepLink: "/alerts"},
	}
	cockpit := BuildContextCockpit(ContextCockpitInput{
		Incident:     inc,
		Evidence:     evidence,
		RunbookBrief: BuildRunbookBriefFromResolved("node", "node.not_ready.v1", 2, 1, 1),
		RecommendedActions: []RecommendedAction{
			{Action: "cordon", TargetKind: "Node", DryRunFirst: true, Summary: "Preview cordoning the node"},
		},
	}, observedAt)

	rc := cockpit.ResourceContext
	if rc.Scope.ClusterID != 7 || rc.Scope.Kind != "Node" || rc.Scope.Name != "k8s-worker-1" {
		t.Fatalf("scope mismatch: %+v", rc.Scope)
	}
	if rc.Scope.SourceType != SourceTypeDiagnosis {
		t.Fatalf("source scope mismatch: %q", rc.Scope.SourceType)
	}
	if !rc.ObservedAt.Equal(observedAt) {
		t.Fatalf("observed_at mismatch: %v", rc.ObservedAt)
	}
	if rc.Source != "incident_snapshot" {
		t.Fatalf("source mismatch: %q", rc.Source)
	}
	// Freshness must reflect oldest evidence (50 minutes ago = 3000s).
	if rc.Freshness.AgeSeconds != 3000 {
		t.Fatalf("freshness.age_seconds = %d, want 3000", rc.Freshness.AgeSeconds)
	}
	if rc.Freshness.AsOf != observedAt.Format(time.RFC3339) {
		t.Fatalf("freshness.as_of mismatch: %q", rc.Freshness.AsOf)
	}
	if rc.EmptySample.Count != 0 || !rc.EmptySample.Bounded || rc.EmptySample.Semantic != "fail_closed" {
		t.Fatalf("empty_sample mismatch: %+v", rc.EmptySample)
	}
}

func TestBuildContextCockpit_Aggregates(t *testing.T) {
	observedAt := time.Now().UTC()
	inc := Incident{
		ID:        2,
		Number:    "INC-000002",
		Severity:  SeverityHigh,
		Status:    StatusOpen,
		Overdue:   true,
		ClusterID: 1,
		SLADueAt:  observedAt.Add(-30 * time.Minute),
		Timeline:  []TimelineEvent{{ID: 1, EventType: EventTypeNote, Content: "note"}, {ID: 2, EventType: EventTypeSystem, Content: "sys"}},
	}
	evidence := []EvidenceItem{
		{SourceType: SourceTypeInspection, DeepLink: "/inspection", ObservedAt: observedAt.Format(time.RFC3339)},
		{SourceType: SourceTypeInspection, DeepLink: "/inspection", ObservedAt: observedAt.Format(time.RFC3339)},
		{SourceType: SourceTypeCorrelation, DeepLink: "/aiops/correlation", ObservedAt: observedAt.Format(time.RFC3339)},
	}
	cockpit := BuildContextCockpit(ContextCockpitInput{Incident: inc, Evidence: evidence}, observedAt)

	if cockpit.SLA.Overdue != true {
		t.Fatalf("sla.overdue = %v, want true", cockpit.SLA.Overdue)
	}
	if cockpit.SLA.DeadlineText != "已逾期" {
		t.Fatalf("sla.deadline_text = %q, want 已逾期", cockpit.SLA.DeadlineText)
	}
	if cockpit.Health.Status != StatusOpen || !cockpit.Health.Overdue || !cockpit.Health.EvidenceAvailable || cockpit.Health.RunbookAvailable {
		t.Fatalf("health mismatch: %+v", cockpit.Health)
	}
	if cockpit.Health.NoteCount != 1 || cockpit.Health.SystemEventCount != 1 {
		t.Fatalf("timeline counts mismatch: %+v", cockpit.Health)
	}
	if len(cockpit.EvidenceSources) != 2 {
		t.Fatalf("evidence sources = %d, want 2", len(cockpit.EvidenceSources))
	}
	for _, src := range cockpit.EvidenceSources {
		if src.SourceType == SourceTypeInspection && src.Count != 2 {
			t.Fatalf("inspection count = %d, want 2", src.Count)
		}
		if src.SourceType == SourceTypeCorrelation && src.DeepLink != "/aiops/correlation" {
			t.Fatalf("correlation deep link mismatch: %q", src.DeepLink)
		}
	}
	if len(cockpit.RecommendedActions) != 0 {
		t.Fatalf("recommended actions should be empty when no runbook, got %d", len(cockpit.RecommendedActions))
	}
}

func TestBuildContextCockpit_RecentTimelineNewestFirst(t *testing.T) {
	observedAt := time.Now().UTC()
	events := make([]TimelineEvent, 0, 15)
	for i := 1; i <= 15; i++ {
		events = append(events, TimelineEvent{ID: int64(i), EventType: EventTypeSystem, Content: "e"})
	}
	inc := Incident{Timeline: events}
	cockpit := BuildContextCockpit(ContextCockpitInput{Incident: inc}, observedAt)
	if len(cockpit.RecentEvents) != maxRecentEvents {
		t.Fatalf("recent events = %d, want %d", len(cockpit.RecentEvents), maxRecentEvents)
	}
	// Newest first: the last timeline item (ID 15) must come first.
	if cockpit.RecentEvents[0].ID != 15 {
		t.Fatalf("recent events not newest-first: first id = %d, want 15", cockpit.RecentEvents[0].ID)
	}
	if cockpit.RecentEvents[len(cockpit.RecentEvents)-1].ID != 6 {
		t.Fatalf("recent events tail id = %d, want 6", cockpit.RecentEvents[len(cockpit.RecentEvents)-1].ID)
	}
}

func TestBuildRunbookBriefFromResolved_EmptyDomainNil(t *testing.T) {
	if brief := BuildRunbookBriefFromResolved("", "code", 1, 1, 1); brief != nil {
		t.Fatalf("empty domain should yield nil brief, got %+v", brief)
	}
	brief := BuildRunbookBriefFromResolved("node", "node.not_ready.v1", 2, 1, 1)
	if brief == nil {
		t.Fatal("expected brief for node domain")
	}
	if brief.Domain != "node" || brief.DiagnosisRoutes != 2 || brief.InspectionRules != 1 || brief.OperationCount != 1 {
		t.Fatalf("brief mismatch: %+v", brief)
	}
}
