package metricshistory

import (
	"testing"
	"time"
)

// These pure helpers gate the 75% core-package coverage line; the branches
// here are cheap to exercise directly without a database.

func TestMetricTime(t *testing.T) {
	// Valid input.
	ts, window, ok := metricTime("2026-08-16T08:00:00Z", "5m")
	if !ok {
		t.Fatal("valid input rejected")
	}
	if !ts.Equal(time.Date(2026, 8, 16, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("ts = %v", ts)
	}
	if window != 5*time.Minute {
		t.Fatalf("window = %v", window)
	}
	// Invalid timestamp.
	if _, _, ok := metricTime("not-a-time", "5m"); ok {
		t.Fatal("invalid timestamp accepted")
	}
	// Zero timestamp.
	if _, _, ok := metricTime("0001-01-01T00:00:00Z", "5m"); ok {
		t.Fatal("zero timestamp accepted")
	}
	// Invalid window.
	if _, _, ok := metricTime("2026-08-16T08:00:00Z", "bogus"); ok {
		t.Fatal("invalid window accepted")
	}
	// Out-of-range window (too short).
	if _, _, ok := metricTime("2026-08-16T08:00:00Z", "10ms"); ok {
		t.Fatal("too-short window accepted")
	}
	// Out-of-range window (too long).
	if _, _, ok := metricTime("2026-08-16T08:00:00Z", "2h"); ok {
		t.Fatal("too-long window accepted")
	}
}

func TestValidSeriesShape(t *testing.T) {
	cases := []struct {
		kind, namespace, name, container string
		want                             bool
	}{
		{ResourceNode, "", "node-1", "", true},
		{ResourceNode, "default", "node-1", "", false}, // node must be cluster-scoped
		{ResourceNode, "", "node-1", "c1", false},
		{ResourcePod, "default", "pod-1", "c1", true},
		{ResourcePod, "", "pod-1", "c1", false}, // pod must be namespaced
		{ResourceDeployment, "default", "deploy-1", "", true},
		{ResourceDeployment, "default", "deploy-1", "c1", false}, // deployment has no container
		{"UnknownKind", "default", "x", "", false},
		{ResourcePod, "default", "", "c1", false}, // name required
	}
	for _, c := range cases {
		if got := validSeriesShape(c.kind, c.namespace, c.name, c.container); got != c.want {
			t.Errorf("validSeriesShape(%q,%q,%q,%q) = %v, want %v", c.kind, c.namespace, c.name, c.container, got, c.want)
		}
	}
	// Length bounds.
	if validSeriesShape(ResourcePod, "default", string(make([]byte, 254)), "c1") {
		t.Fatal("name > 253 accepted")
	}
	if validSeriesShape(ResourcePod, string(make([]byte, 64)), "pod-1", "c1") {
		t.Fatal("namespace > 63 accepted")
	}
	if validSeriesShape(ResourcePod, "default", "pod-1", string(make([]byte, 254))) {
		t.Fatal("container > 253 accepted")
	}
}

func TestMetricUnit(t *testing.T) {
	if unit, ok := metricUnit(MetricCPU); !ok || unit != UnitNanocores {
		t.Fatalf("CPU unit = %q, %v", unit, ok)
	}
	if unit, ok := metricUnit(MetricMemory); !ok || unit != UnitBytes {
		t.Fatalf("memory unit = %q, %v", unit, ok)
	}
	if unit, ok := metricUnit(MetricReadinessReady); !ok || unit != UnitCount {
		t.Fatalf("readiness unit = %q, %v", unit, ok)
	}
	if _, ok := metricUnit("unknown.metric"); ok {
		t.Fatal("unknown metric accepted")
	}
}

func TestValidCoverage(t *testing.T) {
	// Succeeded: sampled/total consistent, complete flag matches equality.
	if !validCoverage(TargetCoverage{Status: SourceSucceeded, Sampled: 3, Total: 3, Complete: true}) {
		t.Fatal("complete succeeded rejected")
	}
	if validCoverage(TargetCoverage{Status: SourceSucceeded, Sampled: 3, Total: 3, Complete: false}) {
		t.Fatal("complete mismatch accepted")
	}
	// Failed/unavailable: zero counts, incomplete.
	if !validCoverage(TargetCoverage{Status: SourceUnavailable, Sampled: 0, Total: 0, Complete: false}) {
		t.Fatal("unavailable rejected")
	}
	if validCoverage(TargetCoverage{Status: SourceFailed, Sampled: 1, Total: 0, Complete: false}) {
		t.Fatal("failed with nonzero samples accepted")
	}
	// Unknown status.
	if validCoverage(TargetCoverage{Status: "bogus"}) {
		t.Fatal("unknown status accepted")
	}
}

func TestCleanupRejectsZeroTime(t *testing.T) {
	s := &Service{}
	if _, err := s.Cleanup(t.Context(), time.Time{}); err == nil {
		t.Fatal("Cleanup with zero now must error")
	}
}
