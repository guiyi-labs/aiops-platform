package capability

import (
	"fmt"
	"testing"
	"time"
)

// BenchmarkRegistryListSnapshot measures the read-path provider listing used by
// the capability status endpoint. Registry is populated up front; only the
// read projection is timed.
func BenchmarkRegistryListSnapshot(b *testing.B) {
	reg := NewRegistry(nil, time.Second)
	for i := 0; i < 40; i++ {
		_ = reg.Register(ProviderDescriptor{
			Name:        fmt.Sprintf("provider_%d", i),
			Description: "benchmark provider",
			Version:     "1.0.0",
			Kind:        "capability",
			Configured:  i%2 == 0,
		})
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = reg.List()
	}
}
