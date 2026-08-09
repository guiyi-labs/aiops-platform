package optimization

import (
	"testing"
)

// FuzzParseCPU exercises the collector CPU quantity parser. It must never
// panic, and an empty input must stay at 0 (unset) exactly
// as production treats a missing quantity.
func FuzzParseCPU(f *testing.F) {
	for _, seed := range []string{"", "500m", "1.5", "2", "-11", "not-a-cpu", "999999999999999999999999999m", "1e5", "0"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		if raw == "" && parseCPU(raw) != 0 {
			t.Fatalf("empty raw should yield 0, got %d", parseCPU(raw))
		}
	})
}

// FuzzParseMem exercises the collector memory quantity parser. It must
// never panic and an empty input must stay at 0 bytes.
func FuzzParseMem(f *testing.F) {
	for _, seed := range []string{"", "128Mi", "4Gi", "1", "-1", "not-a", "1e6", "999999999999999999999999999"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		if raw == "" && parseMem(raw) != 0 {
			t.Fatalf("empty raw should yield 0, got %d", parseMem(raw))
		}
	})
}
