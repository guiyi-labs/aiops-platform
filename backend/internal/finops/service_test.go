package finops

import (
	"context"
	"testing"
	"time"
)

func TestQuantityFromResourceMap(t *testing.T) {
	q := QuantityFromResourceMap(
		map[string]string{"cpu": "1000m", "memory": "256Mi"},
		map[string]string{"cpu": "2", "memory": "512Mi"},
	)
	if q.CPURequest != 1_000_000_000 {
		t.Errorf("cpu request = %d, want 1e9 (1000m)", q.CPURequest)
	}
	if q.MemRequest != 256*1024*1024 {
		t.Errorf("mem request = %d, want 256Mi", q.MemRequest)
	}
	if q.CPULimit != 2_000_000_000 {
		t.Errorf("cpu limit = %d, want 2e9 (2 cores)", q.CPULimit)
	}
	if q.MemLimit != 512*1024*1024 {
		t.Errorf("mem limit = %d, want 512Mi", q.MemLimit)
	}
}

func TestQuantityFromResourceMapUnset(t *testing.T) {
	q := QuantityFromResourceMap(nil, map[string]string{"cpu": "500m"})
	if q.CPURequest != Unset {
		t.Errorf("missing cpu request should be Unset, got %d", q.CPURequest)
	}
	if q.CPULimit != 500_000_000 {
		t.Errorf("cpu limit = %d, want 500m", q.CPULimit)
	}
	if q.MemLimit != Unset {
		t.Errorf("missing mem limit should be Unset, got %d", q.MemLimit)
	}
}

func TestServiceEvaluateEndToEnd(t *testing.T) {
	repo := NewMemoryRepository()
	svc := NewService(DefaultCostRate(), repo)

	inputs := []ContainerInput{{
		ClusterID:    7,
		Namespace:    "app",
		WorkloadKind: "Deployment",
		WorkloadName: "api",
		ContainerName: "main",
		Requests:      QuantityFromResourceMap(map[string]string{"cpu": "1000m"}, nil),
		CPUUsageP95:   100_000_000, // 100m
		Replicas:      3,
	}}

	summary, err := svc.Evaluate(context.Background(), 7, inputs, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Recommendations) != 1 {
		t.Fatalf("expected 1 recommendation, got %d", len(summary.Recommendations))
	}
	stored, ok := repo.Latest(context.Background(), 7)
	if !ok {
		t.Fatal("expected stored summary")
	}
	if len(stored.Recommendations) != 1 {
		t.Errorf("stored recommendations = %d, want 1", len(stored.Recommendations))
	}
}

func TestServiceDefaultRate(t *testing.T) {
	svc := NewService(CostRate{}, nil)
	if svc.rate.PerCoreMonth == 0 || svc.rate.PerGBMonth == 0 {
		t.Error("zero rate should fall back to default")
	}
}
