package incident

import (
	"context"
	"testing"
	"time"
)

func TestServiceMetricsUsesDefaultWindowAndClusterFilter(t *testing.T) {
	service, _ := newServiceWithFake(t)
	for index := 0; index < 3; index++ {
		if _, err := service.Create(context.Background(), CreateInput{
			SourceType: SourceTypeFinding, SourceRef: "finding:7:generic:Pod:default:web-" + intToText(int64(index+1)),
			ClusterID: 7, Title: "web-" + intToText(int64(index+1)) + " pending", Severity: SeverityWarning,
			Summary: "pod pending", Resource: ResourceRef{Kind: "Pod", Namespace: "default", Name: "web-" + intToText(int64(index+1))},
		}); err != nil {
			t.Fatalf("Create #%d: %v", index, err)
		}
	}
	metrics, err := service.Metrics(context.Background(), MetricsFilter{ClusterID: 7})
	if err != nil {
		t.Fatalf("Metrics: %v", err)
	}
	if metrics.WindowDays != DefaultMetricsWindowDays || metrics.ClusterID != 7 ||
		metrics.SampleLimit != MetricsSampleLimit || metrics.Sampled != 3 || metrics.Truncated {
		t.Fatalf("unexpected metrics: %+v", metrics)
	}
}

func TestServiceMetricsClampsWindowAndRespectsClusterFilter(t *testing.T) {
	service, _ := newServiceWithFake(t)
	if _, err := service.Create(context.Background(), CreateInput{
		SourceType: SourceTypeFinding, SourceRef: "finding:7:generic:Pod:default:other",
		ClusterID: 99, Title: "other cluster", Severity: SeverityWarning,
		Summary: "other", Resource: ResourceRef{Kind: "Pod", Namespace: "default", Name: "other"},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	metrics, err := service.Metrics(context.Background(), MetricsFilter{ClusterID: 7, WindowDays: 999})
	if err != nil {
		t.Fatalf("Metrics: %v", err)
	}
	if metrics.WindowDays != MaxMetricsWindowDays || metrics.Sampled != 0 {
		t.Fatalf("window clamp expected %d, got %+v", MaxMetricsWindowDays, metrics)
	}
	metrics, err = service.Metrics(context.Background(), MetricsFilter{ClusterID: 7, WindowDays: -3})
	if err != nil {
		t.Fatalf("Metrics(default): %v", err)
	}
	if metrics.WindowDays != DefaultMetricsWindowDays {
		t.Fatalf("negative window expected default, got %d", metrics.WindowDays)
	}
}

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
