package finops

import "testing"

func milliCores(n int64) int64 { return n * 1_000_000 }
func mebiBytes(n int64) int64  { return n * 1024 * 1024 }
func gibiBytes(n int64) int64  { return n * 1024 * 1024 * 1024 }

func TestRecommendCPUOverProvisioned(t *testing.T) {
	inputs := []ContainerInput{{
		ClusterID: 7, Namespace: "app", WorkloadKind: "Deployment", WorkloadName: "api", ContainerName: "main",
		Requests:  Quantity{CPURequest: milliCores(1000)},
		CPUUsageP95: milliCores(100),
		Replicas:  3,
	}}
	s := Recommend(7, inputs, DefaultCostRate())
	if len(s.Recommendations) != 1 {
		t.Fatalf("expected 1 recommendation, got %d", len(s.Recommendations))
	}
	rec := s.Recommendations[0]
	if rec.Severity != SeverityCritical {
		t.Errorf("expected critical, got %s (ratio high)", rec.Severity)
	}
	// idle 900m per replica * 3 = 2.7 cores * $30 = $81
	if got := rec.MonthlyWasteUSD; abs(got-81.0) > 0.01 {
		t.Errorf("monthly waste = %.2f, want ~81.00", got)
	}
	if s.MonthlyWasteUSD < 80 || s.MonthlyWasteUSD > 82 {
		t.Errorf("summary waste = %.2f, want ~81.00", s.MonthlyWasteUSD)
	}
	if rec.SuggestedRequests.CPURequest != milliCores(150) {
		t.Errorf("suggested CPU request = %d, want 150m", rec.SuggestedRequests.CPURequest)
	}
	if s.CPUIdleCores < 2.6 || s.CPUIdleCores > 2.8 {
		t.Errorf("cpu idle cores = %.2f, want ~2.7", s.CPUIdleCores)
	}
	if s.ContainersOverProvisioned != 1 {
		t.Errorf("over-provisioned = %d, want 1", s.ContainersOverProvisioned)
	}
}

func TestRecommendMemoryOverProvisioned(t *testing.T) {
	inputs := []ContainerInput{{
		ClusterID: 7, Namespace: "app", WorkloadKind: "StatefulSet", WorkloadName: "db", ContainerName: "pg",
		Requests:  Quantity{MemRequest: gibiBytes(1)},
		MemUsageP95: mebiBytes(256),
		Replicas:  2,
	}}
	s := Recommend(7, inputs, DefaultCostRate())
	if len(s.Recommendations) != 1 {
		t.Fatalf("expected 1 recommendation, got %d", len(s.Recommendations))
	}
	rec := s.Recommendations[0]
	if rec.Severity != SeverityCritical {
		t.Errorf("expected critical, got %s", rec.Severity)
	}
	// idle ~805Mi per replica * 2 = ~1.61 GB * $4 = ~6.44
	if got := rec.MonthlyWasteUSD; got < 6.0 || got > 7.0 {
		t.Errorf("monthly waste = %.2f, want ~6.44", got)
	}
	if rec.SuggestedRequests.MemRequest != mebiBytes(320) {
		t.Errorf("suggested mem request = %d, want 320Mi", rec.SuggestedRequests.MemRequest)
	}
}

func TestRecommendMissingRequest(t *testing.T) {
	inputs := []ContainerInput{{
		ClusterID: 7, Namespace: "app", WorkloadKind: "Deployment", WorkloadName: "api", ContainerName: "main",
		Requests:  Quantity{CPURequest: Unset},
		CPUUsageP95: milliCores(200),
		Replicas:  1,
	}}
	s := Recommend(7, inputs, DefaultCostRate())
	if len(s.Recommendations) != 1 {
		t.Fatalf("expected 1 recommendation (missing request), got %d", len(s.Recommendations))
	}
	rec := s.Recommendations[0]
	if rec.Code != "MISSING_CPU_REQUEST" {
		t.Errorf("code = %q, want MISSING_CPU_REQUEST", rec.Code)
	}
	if rec.MonthlyWasteUSD != 0 {
		t.Errorf("missing-request waste should be 0, got %.2f", rec.MonthlyWasteUSD)
	}
	if rec.SuggestedRequests.CPURequest != milliCores(250) {
		t.Errorf("suggested CPU request = %d, want 250m", rec.SuggestedRequests.CPURequest)
	}
	if s.ContainersOverProvisioned != 0 {
		t.Errorf("missing request should not count as over-provisioned")
	}
}

func TestRecommendRightSizedNotFlagged(t *testing.T) {
	inputs := []ContainerInput{{
		ClusterID: 7, Namespace: "app", WorkloadKind: "Deployment", WorkloadName: "api", ContainerName: "main",
		Requests:  Quantity{CPURequest: milliCores(120)},
		CPUUsageP95: milliCores(100),
		Replicas:  1,
	}}
	s := Recommend(7, inputs, DefaultCostRate())
	if len(s.Recommendations) != 0 {
		t.Fatalf("right-sized container should produce no recommendation, got %d", len(s.Recommendations))
	}
	if s.ContainersEvaluated != 1 {
		t.Errorf("evaluated = %d, want 1", s.ContainersEvaluated)
	}
}

func TestRecommendNoUsageSkipped(t *testing.T) {
	inputs := []ContainerInput{{
		ClusterID: 7, Namespace: "app", WorkloadKind: "Deployment", WorkloadName: "api", ContainerName: "main",
		Requests:  Quantity{CPURequest: milliCores(500)},
		Replicas:  1,
	}}
	s := Recommend(7, inputs, DefaultCostRate())
	if len(s.Recommendations) != 0 {
		t.Fatalf("no-usage container should be skipped, got %d", len(s.Recommendations))
	}
	if s.ContainersEvaluated != 1 {
		t.Errorf("evaluated = %d, want 1", s.ContainersEvaluated)
	}
}

func TestWasteSummaryToFindings(t *testing.T) {
	inputs := []ContainerInput{{
		ClusterID: 7, Namespace: "app", WorkloadKind: "Deployment", WorkloadName: "api", ContainerName: "main",
		Requests: Quantity{CPURequest: milliCores(1000)},
		CPUUsageP95: milliCores(100),
		Replicas:  1,
	}}
	s := Recommend(7, inputs, DefaultCostRate())
	findings := s.ToFindings()
	if len(findings) != len(s.Recommendations) {
		t.Fatalf("findings %d != recommendations %d", len(findings), len(s.Recommendations))
	}
	if findings[0].Severity != SeverityCritical {
		t.Errorf("finding severity = %s, want critical", findings[0].Severity)
	}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
