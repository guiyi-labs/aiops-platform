package topology

import (
	"testing"
)

// FuzzHashSafeDiff proves the deterministic evidence-hash contract. The same
// input must always hash to the same value, and empty input must yield an
// empty hash.
func FuzzHashSafeDiff(f *testing.F) {
	for _, seed := range []string{"", "abc", "VerdictA==", "\u0000\u0001"} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		first := HashSafeDiff(input)
		second := HashSafeDiff(input)
		if first != second {
			t.Fatalf("non-deterministic hash for %v", input)
		}
		if len(input) == 0 && first != "" {
			t.Fatalf("empty input should yield empty hash, got %q", first)
		}
	})
}

// FuzzFormatPlanIDHash exercises the short plan-ID hash used in evidence
// references. Output must always be the fixed 16-char shape.
func FuzzFormatPlanIDHash(f *testing.F) {
	for _, seed := range []string{"", "plan-123", "a longer plan ID spaces !@#$%^&*()"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, planID string) {
		value := FormatPlanIDHash(planID)
		if planID == "" {
			if value != "" {
				t.Fatalf("empty plan ID should yield empty hash, got %q", value)
			}
			return
		}
		if len(value) != 16 {
			t.Fatalf("hash length = %d, want 16 for %q", len(value), planID)
		}
	})
}
