package incident

import (
	"testing"
	"time"
)

func TestDeriveMetricsUsesFirstLifecycleEvents(t *testing.T) {
	base := time.Date(2026, time.August, 14, 8, 0, 0, 0, time.UTC)
	resolvedFirst := base.Add(45 * time.Minute)
	resolvedSecond := base.Add(2 * time.Hour)
	items := []Incident{
		{
			CreatedAt:  base,
			SLADueAt:   base.Add(time.Hour),
			Assignee:   &ActorRef{ID: 2, Name: "ops"},
			ResolvedAt: &resolvedFirst,
			Timeline: []TimelineEvent{
				{Content: "handoff from unassigned to ops", CreatedAt: base.Add(2 * time.Minute)},
				{Content: "status changed from open to confirmed", CreatedAt: base.Add(5 * time.Minute)},
				{Content: "status changed from confirmed to resolved", CreatedAt: base.Add(45 * time.Minute)},
				{Content: "status changed from resolved to open", CreatedAt: base.Add(3 * time.Hour)},
			},
		},
		{
			CreatedAt:  base,
			SLADueAt:   base.Add(time.Hour),
			ResolvedAt: &resolvedSecond,
			Timeline: []TimelineEvent{
				{Content: "status changed from open to confirmed", CreatedAt: base.Add(10 * time.Minute)},
			},
		},
		{CreatedAt: base, Overdue: true},
	}

	metrics := DeriveMetrics(items)
	if metrics.Assigned != 1 || metrics.Acknowledged != 2 || metrics.Resolved != 2 || metrics.Overdue != 1 {
		t.Fatalf("unexpected counts: %+v", metrics)
	}
	if metrics.FirstAssignedSecs == nil || *metrics.FirstAssignedSecs != 120 {
		t.Fatalf("first assigned average = %v, want 120 seconds", metrics.FirstAssignedSecs)
	}
	if metrics.MTTASeconds == nil || *metrics.MTTASeconds != 450 {
		t.Fatalf("MTTA average = %v, want 450 seconds", metrics.MTTASeconds)
	}
	if metrics.MTTRSeconds == nil || *metrics.MTTRSeconds != 4950 {
		t.Fatalf("MTTR average = %v, want 4950 seconds", metrics.MTTRSeconds)
	}
	if metrics.SLAEvaluated != 2 || metrics.SLACompliant != 1 || metrics.SLAComplianceRate == nil || *metrics.SLAComplianceRate != 0.5 {
		t.Fatalf("unexpected SLA metrics: %+v", metrics)
	}
}

func TestDeriveMetricsReturnsNullAveragesWithoutSamples(t *testing.T) {
	metrics := DeriveMetrics([]Incident{{CreatedAt: time.Now().UTC()}})
	if metrics.FirstAssignedSecs != nil || metrics.MTTASeconds != nil || metrics.MTTRSeconds != nil || metrics.SLAComplianceRate != nil {
		t.Fatalf("expected null metrics without lifecycle samples: %+v", metrics)
	}
}
