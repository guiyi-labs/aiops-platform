package hpa

import (
	"testing"
	"time"
)

func i32(v int32) *int32 { return &v }

func testTime() time.Time {
	return time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
}

func healthyHPA() HPAInput {
	return HPAInput{
		Namespace:   "prod",
		Name:        "web",
		UID:         "u-web",
		MinReplicas: i32(2),
		MaxReplicas: 10,
		// healthy: current below max, target declared, utilization in band
		CurrentReplicas:       3,
		TargetMetric:          "cpu",
		TargetValue:           80,
		CurrentUtilizationPct: i32(60),
	}
}

// TestEvaluate_HealthyHPA_NoFindings: an HPA with a declared target, headroom
// and in-band utilization produces zero findings.
func TestEvaluate_HealthyHPA_NoFindings(t *testing.T) {
	in := Inputs{HPAs: []HPAInput{healthyHPA()}}
	s := Evaluate(7, in, testTime())

	if s.Failed != 0 {
		t.Fatalf("failed = %d, want 0; findings=%+v", s.Failed, s.Findings)
	}
	if s.Total != 3 { // target + max headroom + utilization
		t.Fatalf("total = %d, want 3", s.Total)
	}
	if s.Passed != s.Total {
		t.Fatalf("passed = %d, want %d", s.Passed, s.Total)
	}
	if s.HPAsTotal != 1 {
		t.Fatalf("hpas_total = %d, want 1", s.HPAsTotal)
	}
}

// TestEvaluate_NoTargetAndAtMax: missing target and pinned at max both warn.
func TestEvaluate_NoTargetAndAtMax(t *testing.T) {
	in := Inputs{HPAs: []HPAInput{{
		Namespace:             "prod",
		Name:                  "api",
		UID:                   "u-api",
		MaxReplicas:           5,
		CurrentReplicas:       5, // at max
		CurrentUtilizationPct: i32(90),
		// no TargetMetric
	}}}
	s := Evaluate(7, in, testTime())

	if s.AtMaxReplicasCount != 1 {
		t.Fatalf("at_max_replicas_count = %d, want 1", s.AtMaxReplicasCount)
	}
	if s.OverTargetCount != 1 {
		t.Fatalf("over_target_count = %d, want 1 (90 > default 80)", s.OverTargetCount)
	}
	// findings: HPA_NO_TARGET_METRIC, HPA_AT_MAX_REPLICAS, HPA_UTILIZATION_OVER_TARGET
	if s.Failed != 3 {
		t.Fatalf("failed = %d, want 3; findings=%+v", s.Failed, s.Findings)
	}
	if s.Total != 3 { // target + max headroom + utilization (default 80 target)
		t.Fatalf("total = %d, want 3", s.Total)
	}
}

// TestEvaluate_UtilizationUnderTarget: utilization far below target is info.
func TestEvaluate_UtilizationUnderTarget(t *testing.T) {
	in := Inputs{HPAs: []HPAInput{{
		Namespace:             "prod",
		Name:                  "worker",
		MaxReplicas:           10,
		CurrentReplicas:       5,
		TargetMetric:          "cpu",
		TargetValue:           80,
		CurrentUtilizationPct: i32(20), // 20*2 < 80
	}}}
	s := Evaluate(7, in, testTime())

	if s.Failed != 1 {
		t.Fatalf("failed = %d, want 1", s.Failed)
	}
	if s.Findings[0].Code != CodeUnderTarget {
		t.Fatalf("finding code = %s, want %s", s.Findings[0].Code, CodeUnderTarget)
	}
}

// TestEvaluate_MaxReplicasLow: maxReplicas <= 2 adds an info finding.
func TestEvaluate_MaxReplicasLow(t *testing.T) {
	in := Inputs{HPAs: []HPAInput{{
		Namespace:       "prod",
		Name:            "tiny",
		MaxReplicas:     2,
		CurrentReplicas: 1,
		TargetMetric:    "cpu",
		TargetValue:     80,
	}}}
	s := Evaluate(7, in, testTime())

	if s.Failed != 1 {
		t.Fatalf("failed = %d, want 1", s.Failed)
	}
	if s.Findings[0].Code != CodeMaxReplicasLow {
		t.Fatalf("finding code = %s, want %s", s.Findings[0].Code, CodeMaxReplicasLow)
	}
}

// TestEvaluate_PodsMetricSkipsUtilization: utilization percentage only applies
// to resource targets; a pods target must not emit over/under findings.
func TestEvaluate_PodsMetricSkipsUtilization(t *testing.T) {
	in := Inputs{HPAs: []HPAInput{{
		Namespace:             "prod",
		Name:                  "queue",
		MaxReplicas:           10,
		CurrentReplicas:       3,
		TargetMetric:          "pods",
		TargetValue:           4,
		CurrentUtilizationPct: i32(95), // irrelevant for pods metric
	}}}
	s := Evaluate(7, in, testTime())

	// findings: none from utilization; target is declared, max headroom ok.
	if s.Failed != 0 {
		t.Fatalf("failed = %d, want 0; findings=%+v", s.Failed, s.Findings)
	}
	if s.Total != 2 { // target + max headroom; utilization check skipped
		t.Fatalf("total = %d, want 2", s.Total)
	}
}

// TestEvaluate_EmptyInputs_EmptyResult: nothing to analyze means zero checks.
func TestEvaluate_EmptyInputs_EmptyResult(t *testing.T) {
	s := Evaluate(7, Inputs{}, testTime())

	if s.Total != 0 || s.Failed != 0 || s.Passed != 0 {
		t.Fatalf("empty input counters = %+v, want all zero", s)
	}
	if s.Findings == nil {
		t.Fatal("findings must serialize as [] rather than null")
	}
	if s.BySeverity == nil || s.ByFamily == nil {
		t.Fatal("maps must be initialized")
	}
}

// TestEvaluate_SortDeterministic: same input yields the same finding order.
func TestEvaluate_SortDeterministic(t *testing.T) {
	in := Inputs{HPAs: []HPAInput{
		{Namespace: "b", Name: "z", MaxReplicas: 5, CurrentReplicas: 5},
		{Namespace: "a", Name: "y", MaxReplicas: 5, CurrentReplicas: 5},
	}}
	a := Evaluate(7, in, testTime())
	b := Evaluate(7, in, testTime())

	if len(a.Findings) != len(b.Findings) {
		t.Fatalf("finding counts differ: %d vs %d", len(a.Findings), len(b.Findings))
	}
	for i := range a.Findings {
		if a.Findings[i].Code != b.Findings[i].Code || a.Findings[i].Resource.Name != b.Findings[i].Resource.Name {
			t.Fatalf("order mismatch at %d: %+v vs %+v", i, a.Findings[i], b.Findings[i])
		}
	}
}
