package posture

import (
	"testing"

	"k8s-aiops.local/backend/internal/finding"
)

// FuzzSeverityRank exercises the risk-ordering contract shared by the posture
// sort. Every severity must map into the closed set {critical, warning, info}.
func FuzzSeverityRank(f *testing.F) {
	for _, seed := range []string{
		finding.SeverityCritical, finding.SeverityWarning, finding.SeverityInfo,
		"", "unknown", "CRITICAL", "severe",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, severity string) {
		rank := severityRank(severity)
		if rank < 0 || rank > 2 {
			t.Fatalf("severityRank(%q) = %d, want 0..2", severity, rank)
		}
		// Known severities must keep their canonical order.
		if severity == finding.SeverityCritical && rank != 0 {
			t.Fatalf("critical ranked %d, want 0", rank)
		}
		if severity == finding.SeverityWarning && rank != 1 {
			t.Fatalf("warning ranked %d, want 1", rank)
		}
		if severity == finding.SeverityInfo && rank != 2 {
			t.Fatalf("info ranked %d, want 2", rank)
		}
	})
}
