package diagnosis

import (
	"testing"
	"time"

	"k8s-aiops.local/backend/internal/metricshistory"
)

func TestEvaluateSustainedMetricBreachFiring(t *testing.T) {
	start := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	eval := metricshistory.EvaluationResponse{
		Series: metricshistory.Series{
			ClusterID: 7, ResourceKind: metricshistory.ResourceNode,
			ResourceName: "worker-a", MetricName: metricshistory.MetricCPU,
			Unit: metricshistory.UnitNanocores,
		},
		From: start, To: start.Add(1 * time.Hour),
		Coverage:  metricshistory.QueryCoverage{Collections: 4, Succeeded: 4, Points: 4},
		State:     metricshistory.EvaluationStateFiring,
		Operator:  metricshistory.OperatorGreaterThanOrEqual,
		Threshold: 50_000_000, ForSeconds: 60, MinimumPoints: 2,
		PointsEvaluated: 4, BreachingPoints: 3,
		SustainedWindows: []metricshistory.SustainedWindow{
			{StartCollectedAt: start, EndCollectedAt: start.Add(2 * time.Minute), BreachingPoints: 3, SpanSeconds: 120},
		},
		LatestFiringWindow: &metricshistory.SustainedWindow{
			StartCollectedAt: start, EndCollectedAt: start.Add(2 * time.Minute), BreachingPoints: 3, SpanSeconds: 120,
		},
	}
	record, matched := EvaluateSustainedMetricBreach(7, eval, start.Add(1*time.Hour))
	if !matched {
		t.Fatal("rule should match firing state")
	}
	if record.RuleID != RuleNodeSustainedMetricBreach {
		t.Fatalf("expected rule %s, got %s", RuleNodeSustainedMetricBreach, record.RuleID)
	}
	if record.Severity != "high" {
		t.Fatalf("expected high severity for CPU, got %s", record.Severity)
	}
	if record.Resource.Name != "worker-a" {
		t.Fatalf("expected resource worker-a, got %s", record.Resource.Name)
	}
	if record.Resource.Kind != metricshistory.ResourceNode {
		t.Fatalf("expected resource kind Node, got %s", record.Resource.Kind)
	}
	if len(record.Evidence) != 2 {
		t.Fatalf("expected 2 evidence entries (1 window + 1 summary), got %d", len(record.Evidence))
	}
	if record.Evidence[0].Type != "metric_sustained_breach" {
		t.Fatalf("expected first evidence type metric_sustained_breach, got %s", record.Evidence[0].Type)
	}
	if record.Evidence[1].Type != "metric_evaluation_summary" {
		t.Fatalf("expected second evidence type metric_evaluation_summary, got %s", record.Evidence[1].Type)
	}
	if record.Evidence[0].Content["threshold"] != int64(50_000_000) {
		t.Fatalf("expected threshold 50000000, got %v", record.Evidence[0].Content["threshold"])
	}
	if record.Evidence[0].Content["breaching_points"] != 3 {
		t.Fatalf("expected 3 breaching points, got %v", record.Evidence[0].Content["breaching_points"])
	}
	if record.Evidence[0].Content["span_seconds"] != int64(120) {
		t.Fatalf("expected span 120s, got %v", record.Evidence[0].Content["span_seconds"])
	}
	if record.Evidence[1].Content["state"] != metricshistory.EvaluationStateFiring {
		t.Fatalf("expected summary state firing, got %v", record.Evidence[1].Content["state"])
	}
	if record.Summary == "" {
		t.Fatal("expected non-empty summary")
	}
	if len(record.RootCauses) == 0 {
		t.Fatal("expected root causes for CPU breach")
	}
	if len(record.Recommendations) == 0 {
		t.Fatal("expected recommendations for CPU breach")
	}
}

func TestEvaluateSustainedMetricBreachMemorySeverity(t *testing.T) {
	start := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	eval := metricshistory.EvaluationResponse{
		Series: metricshistory.Series{
			ClusterID: 7, ResourceKind: metricshistory.ResourceNode,
			ResourceName: "worker-b", MetricName: metricshistory.MetricMemory,
			Unit: metricshistory.UnitBytes,
		},
		From: start, To: start.Add(1 * time.Hour),
		Coverage:  metricshistory.QueryCoverage{Collections: 4, Succeeded: 4, Points: 4},
		State:     metricshistory.EvaluationStateFiring,
		Operator:  metricshistory.OperatorGreaterThanOrEqual,
		Threshold: 8_000_000_000, ForSeconds: 120, MinimumPoints: 2,
		PointsEvaluated: 4, BreachingPoints: 2,
		SustainedWindows: []metricshistory.SustainedWindow{
			{StartCollectedAt: start, EndCollectedAt: start.Add(5 * time.Minute), BreachingPoints: 2, SpanSeconds: 300},
		},
		LatestFiringWindow: &metricshistory.SustainedWindow{
			StartCollectedAt: start, EndCollectedAt: start.Add(5 * time.Minute), BreachingPoints: 2, SpanSeconds: 300,
		},
	}
	record, matched := EvaluateSustainedMetricBreach(7, eval, start.Add(1*time.Hour))
	if !matched {
		t.Fatal("rule should match memory firing")
	}
	if record.Severity != "medium" {
		t.Fatalf("expected medium severity for memory, got %s", record.Severity)
	}
}

func TestEvaluateSustainedMetricBreachNoMatchNormal(t *testing.T) {
	start := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	eval := metricshistory.EvaluationResponse{
		Series: metricshistory.Series{
			ClusterID: 7, ResourceKind: metricshistory.ResourceNode,
			ResourceName: "worker-a", MetricName: metricshistory.MetricCPU,
			Unit: metricshistory.UnitNanocores,
		},
		From: start, To: start.Add(1 * time.Hour),
		Coverage:  metricshistory.QueryCoverage{Collections: 4, Succeeded: 4, Points: 4},
		State:     metricshistory.EvaluationStateNormal,
		Operator:  metricshistory.OperatorGreaterThanOrEqual,
		Threshold: 50_000_000, ForSeconds: 60, MinimumPoints: 2,
		PointsEvaluated: 4,
	}
	record, matched := EvaluateSustainedMetricBreach(7, eval, start.Add(1*time.Hour))
	if matched {
		t.Fatal("rule should not match normal state")
	}
	if record.RuleID != "" {
		t.Fatal("expected empty record on no match")
	}
}

func TestEvaluateSustainedMetricBreachNoMatchInsufficientData(t *testing.T) {
	start := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	eval := metricshistory.EvaluationResponse{
		Series: metricshistory.Series{
			ClusterID: 7, ResourceKind: metricshistory.ResourceNode,
			ResourceName: "worker-a", MetricName: metricshistory.MetricCPU,
			Unit: metricshistory.UnitNanocores,
		},
		From: start, To: start.Add(1 * time.Hour),
		Coverage:  metricshistory.QueryCoverage{Collections: 4, Succeeded: 2, Missing: 2, Points: 2},
		State:     metricshistory.EvaluationStateInsufficientData,
		Operator:  metricshistory.OperatorGreaterThanOrEqual,
		Threshold: 50_000_000, ForSeconds: 60, MinimumPoints: 2,
		PointsEvaluated: 2,
	}
	record, matched := EvaluateSustainedMetricBreach(7, eval, start.Add(1*time.Hour))
	if matched {
		t.Fatal("rule should not match insufficient_data state")
	}
	if record.RuleID != "" {
		t.Fatal("expected empty record on no match")
	}
}

func TestEvaluateSustainedMetricBreachMultipleWindows(t *testing.T) {
	start := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	eval := metricshistory.EvaluationResponse{
		Series: metricshistory.Series{
			ClusterID: 7, ResourceKind: metricshistory.ResourceNode,
			ResourceName: "worker-a", MetricName: metricshistory.MetricCPU,
			Unit: metricshistory.UnitNanocores,
		},
		From: start, To: start.Add(1 * time.Hour),
		Coverage:  metricshistory.QueryCoverage{Collections: 6, Succeeded: 6, Points: 6},
		State:     metricshistory.EvaluationStateFiring,
		Operator:  metricshistory.OperatorGreaterThanOrEqual,
		Threshold: 50_000_000, ForSeconds: 60, MinimumPoints: 2,
		PointsEvaluated: 6, BreachingPoints: 4,
		SustainedWindows: []metricshistory.SustainedWindow{
			{StartCollectedAt: start, EndCollectedAt: start.Add(2 * time.Minute), BreachingPoints: 2, SpanSeconds: 120},
			{StartCollectedAt: start.Add(10 * time.Minute), EndCollectedAt: start.Add(13 * time.Minute), BreachingPoints: 2, SpanSeconds: 180},
		},
		LatestFiringWindow: &metricshistory.SustainedWindow{
			StartCollectedAt: start.Add(10 * time.Minute), EndCollectedAt: start.Add(13 * time.Minute), BreachingPoints: 2, SpanSeconds: 180,
		},
	}
	record, matched := EvaluateSustainedMetricBreach(7, eval, start.Add(1*time.Hour))
	if !matched {
		t.Fatal("rule should match with multiple windows")
	}
	expectedEvidence := len(eval.SustainedWindows) + 1
	if len(record.Evidence) != expectedEvidence {
		t.Fatalf("expected %d evidence entries, got %d", expectedEvidence, len(record.Evidence))
	}
	if record.Evidence[0].Content["window_index"] != 1 {
		t.Fatalf("expected first window_index 1, got %v", record.Evidence[0].Content["window_index"])
	}
	if record.Evidence[1].Content["window_index"] != 2 {
		t.Fatalf("expected second window_index 2, got %v", record.Evidence[1].Content["window_index"])
	}
	if record.Evidence[2].Content["sustained_windows"] != 2 {
		t.Fatalf("expected summary sustained_windows 2, got %v", record.Evidence[2].Content["sustained_windows"])
	}
}

func TestEvaluateSustainedMetricBreachSummaryIncludesDetails(t *testing.T) {
	start := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	eval := metricshistory.EvaluationResponse{
		Series: metricshistory.Series{
			ClusterID: 7, ResourceKind: metricshistory.ResourceNode,
			ResourceName: "node-xyz", MetricName: metricshistory.MetricCPU,
			Unit: metricshistory.UnitNanocores,
		},
		From: start, To: start.Add(1 * time.Hour),
		Coverage:  metricshistory.QueryCoverage{Collections: 4, Succeeded: 4, Points: 4},
		State:     metricshistory.EvaluationStateFiring,
		Operator:  metricshistory.OperatorGreaterThanOrEqual,
		Threshold: 80_000_000, ForSeconds: 120, MinimumPoints: 3,
		PointsEvaluated: 4, BreachingPoints: 3, ObservedSpanSeconds: 180,
		SustainedWindows: []metricshistory.SustainedWindow{
			{StartCollectedAt: start, EndCollectedAt: start.Add(3 * time.Minute), BreachingPoints: 3, SpanSeconds: 180},
		},
		LatestFiringWindow: &metricshistory.SustainedWindow{
			StartCollectedAt: start, EndCollectedAt: start.Add(3 * time.Minute), BreachingPoints: 3, SpanSeconds: 180,
		},
	}
	record, matched := EvaluateSustainedMetricBreach(7, eval, start.Add(1*time.Hour))
	if !matched {
		t.Fatal("rule should match")
	}
	if record.Severity != "high" {
		t.Fatalf("expected high severity, got %s", record.Severity)
	}
	summary := record.Evidence[len(record.Evidence)-1]
	if summary.Content["coverage_collections"] != 4 {
		t.Fatalf("expected coverage_collections 4, got %v", summary.Content["coverage_collections"])
	}
	if summary.Content["coverage_points"] != 4 {
		t.Fatalf("expected coverage_points 4, got %v", summary.Content["coverage_points"])
	}
	if summary.Content["breaching_points"] != 3 {
		t.Fatalf("expected breaching_points 3, got %v", summary.Content["breaching_points"])
	}
}
