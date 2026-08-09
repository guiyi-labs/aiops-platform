package posture

import (
	"fmt"
	"sort"
	"testing"

	"k8s-aiops.local/backend/internal/finding"
)

// BenchmarkAggregateSeveritySort measures the risk-sorting loop that the
// posture evaluator applies to the flattened finding stream on a busy
// cluster. It mirrors the in-place StableSort in Evaluate.
func BenchmarkAggregateSeveritySort(b *testing.B) {
	findings := make([]PostureFinding, 600)
	for i := range findings {
		sev := finding.SeverityInfo
		switch i % 3 {
		case 0:
			sev = finding.SeverityCritical
		case 1:
			sev = finding.SeverityWarning
		}
		findings[i] = PostureFinding{
			Domain:   DomainPolicy,
			Severity: sev,
			Code:     fmt.Sprintf("POL-%d", i%40),
			Summary:  "benchmark finding",
			Resource: finding.ResourceCitation{Kind: "pod", Name: fmt.Sprintf("p-%d", i)},
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sorted := append([]PostureFinding(nil), findings...)
		sort.SliceStable(sorted, func(i, j int) bool {
			return severityRank(sorted[i].Severity) < severityRank(sorted[j].Severity)
		})
	}
}
