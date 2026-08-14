package capacitypreview

import (
	"testing"
	"time"
)

func TestPreview_RanksFitFirst(t *testing.T) {
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	bundle := Bundle{
		ClusterID: 1,
		Nodes: []NodeObservation{
			{
				Name:        "node-big",
				Allocatable: map[string]string{"cpu": "8", "memory": "32Gi"},
				UsageCPU:    "1", UsageMemory: "8Gi",
				Schedulable: true, StatusReady: true,
				AllocatableObservedAt: now.Add(-time.Minute).Format(time.RFC3339),
				UsageObservedAt:       now.Add(-30 * time.Second).Format(time.RFC3339),
			},
			{
				Name:        "node-small",
				Allocatable: map[string]string{"cpu": "4", "memory": "16Gi"},
				UsageCPU:    "2", UsageMemory: "10Gi",
				Schedulable: true, StatusReady: true,
				AllocatableObservedAt: now.Add(-time.Minute).Format(time.RFC3339),
				UsageObservedAt:       now.Add(-30 * time.Second).Format(time.RFC3339),
			},
		},
		ObservedAt: now,
	}
	// Request wants 1 core + 2Gi — fits both, but node-big has more headroom.
	preview, err := Evaluate(1, WorkloadRequest{CPURequestNanocores: 1_000_000_000, MemRequestBytes: 2 << 30}, bundle, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(preview.Nodes) != 2 {
		t.Fatalf("want 2 nodes, got %d", len(preview.Nodes))
	}
	if preview.Nodes[0].Name != "node-big" || !preview.Nodes[0].Fits {
		t.Fatalf("node-big should rank first and fit: %+v", preview.Nodes[0])
	}
	if !preview.Nodes[1].Fits {
		t.Fatalf("node-small should fit: %+v", preview.Nodes[1])
	}
	if preview.FitCount != 2 || preview.NodesTotal != 2 || preview.NodesSchedulable != 2 {
		t.Fatalf("rollups wrong: %+v", preview)
	}
	if launchpads := preview.Nodes[0].Score; launchpads < 6.8e9 {
		t.Fatalf("node-big CPU headroom score unexpected: %v", launchpads)
	}
}

func TestPreview_FailClosedOnUnknown(t *testing.T) {
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	bundle := Bundle{
		ClusterID: 2,
		Nodes: []NodeObservation{
			{
				Name:        "no-metrics",
				Allocatable: map[string]string{"cpu": "8", "memory": "32Gi"},
				// no usage fields -> unknown, must not fit
				Schedulable: true, StatusReady: true,
				AllocatableObservedAt: now.Add(-time.Minute).Format(time.RFC3339),
			},
			{
				Name:        "unschedulable",
				Allocatable: map[string]string{"cpu": "16", "memory": "64Gi"},
				UsageCPU:    "1", UsageMemory: "4Gi",
				Schedulable: false, StatusReady: true,
				AllocatableObservedAt: now.Add(-time.Minute).Format(time.RFC3339),
			},
		},
		ObservedAt: now,
	}
	preview, err := Evaluate(2, WorkloadRequest{CPURequestNanocores: 1_000_000_000, MemRequestBytes: 1 << 30}, bundle, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, n := range preview.Nodes {
		if n.Fits {
			t.Fatalf("node %s must not fit (missing usage / unschedulable): %+v", n.Name, n)
		}
	}
	if preview.FitCount != 0 {
		t.Fatalf("fit count must be 0, got %d", preview.FitCount)
	}
	if !preview.FailClosed {
		t.Fatalf("fail_closed must be true when any node has unknown constraints")
	}
}

func TestPreview_ViolationExplainsWhy(t *testing.T) {
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	bundle := Bundle{
		ClusterID: 3,
		Nodes: []NodeObservation{
			{
				Name:        "tight",
				Allocatable: map[string]string{"cpu": "2", "memory": "8Gi"},
				UsageCPU:    "1.8", UsageMemory: "7Gi",
				Schedulable: true, StatusReady: true,
				AllocatableObservedAt: now.Format(time.RFC3339),
				UsageObservedAt:       now.Format(time.RFC3339),
			},
		},
		ObservedAt: now,
	}
	preview, err := Evaluate(3, WorkloadRequest{CPURequestNanocores: 1_000_000_000, MemRequestBytes: 4 << 30}, bundle, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	node := preview.Nodes[0]
	if node.Fits {
		t.Fatalf("tight node must not fit: %+v", node)
	}
	var sawViolated bool
	for _, con := range node.Constraints {
		if con.Status == ConstraintViolated && con.Note != "" {
			sawViolated = true
		}
	}
	if !sawViolated {
		t.Fatalf("expected a violated constraint with a reason: %+v", node.Constraints)
	}
}

func TestPreview_GPUConstraint(t *testing.T) {
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	bundle := Bundle{
		ClusterID: 4,
		Nodes: []NodeObservation{
			{
				Name:        "gpu-node",
				Allocatable: map[string]string{"cpu": "4", "memory": "16Gi", "nvidia.com/gpu": "1"},
				UsageCPU:    "1", UsageMemory: "4Gi",
				Schedulable: true, StatusReady: true,
				AllocatableObservedAt: now.Format(time.RFC3339),
				UsageObservedAt:       now.Format(time.RFC3339),
			},
		},
		ObservedAt: now,
	}
	preview, err := Evaluate(4, WorkloadRequest{CPURequestNanocores: 500_000_000, MemRequestBytes: 1 << 30, GPURequest: 1}, bundle, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	node := preview.Nodes[0]
	if !node.Fits {
		t.Fatalf("gpu node with 1 GPU should fit request for 1: %+v", node)
	}
	// GPU allocatable is a quantity string "1" -> parseGPU Value() = 1.
	var gpuSeen bool
	for _, con := range node.Constraints {
		if con.Resource == "gpu" && con.Status == ConstraintSatisfied && con.Remaining == 1 {
			gpuSeen = true
		}
	}
	if !gpuSeen {
		t.Fatalf("expected satisfied gpu constraint: %+v", node.Constraints)
	}
}

func TestPreview_EmptyBundle(t *testing.T) {
	_, err := Evaluate(5, WorkloadRequest{}, Bundle{}, time.Now())
	if err != ErrEmpty {
		t.Fatalf("expected ErrEmpty, got %v", err)
	}
}

func TestParseCPUUnits(t *testing.T) {
	if got := parseCPU("4"); got != 4_000_000_000 {
		t.Fatalf("parseCPU(4) = %d, want 4e9", got)
	}
	if got := parseCPU("1200m"); got != 1_200_000_000 {
		t.Fatalf("parseCPU(1200m) = %d, want 1.2e9", got)
	}
	if got := parseCPU(""); got != 0 {
		t.Fatalf("parseCPU('') = %d, want 0", got)
	}
}

func TestParseMemUnits(t *testing.T) {
	if got := parseMem("16Gi"); got != 16<<30 {
		t.Fatalf("parseMem(16Gi) = %d, want %d", got, 16<<30)
	}
	if got := parseMem(""); got != 0 {
		t.Fatalf("parseMem('') = %d, want 0", got)
	}
}
