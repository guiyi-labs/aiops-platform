package topology

import (
	"fmt"
	"testing"
	"time"
)

// BenchmarkSortEdges measures the deterministic edge sorter used when
// persisting/discovering topology edges on large fleets (500 entities).
func BenchmarkSortEdges(b *testing.B) {
	edges := make([]Edge, 500)
	base := time.Now()
	for i := range edges {
		edges[i] = Edge{
			ClusterID:       1,
			Kind:            EdgeOwns,
			Source:          ResourceCitation{Kind: "deployment", Namespace: "app", Name: fmt.Sprintf("dep-%d", i%25), UID: fmt.Sprintf("uid-%d", i)},
			Target:          ResourceCitation{Kind: "replicaset", Namespace: "app", Name: fmt.Sprintf("rs-%d", i%97), UID: fmt.Sprintf("rs-uid-%d", i)},
			Derivation:      DerivationOwnerReference,
			FirstObservedAt: base,
			LastObservedAt:  base.Add(time.Minute),
			ValidFrom:       base,
			SourceHash:      fmt.Sprintf("hash-%d", i%7),
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = SortEdges(edges)
	}
}
