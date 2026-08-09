package metricshistory

import (
	"errors"
	"strings"
	"testing"
)

// FuzzCPUQuantity exercises the CPU quantity parser against arbitrary input.
// The parser must never panic, and every accepted value must parse into a
// finite, non-negative nanocore count.
func FuzzCPUQuantity(f *testing.F) {
	for _, seed := range []string{"", "500m", "2", "1.5", "123456789n", "-1m", "1e9", "1E6", "\t 4 \n", "999999999999999999999999999"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		value, err := cpuNanocores(raw)
		if err != nil {
			if !errors.Is(err, errInvalidQuantity) {
				t.Fatalf("unexpected error type: %v", err)
			}
			return
		}
		if value < 0 {
			t.Fatalf("negative nanocores for %q: %d", raw, value)
		}
		if strings.TrimSpace(raw) != "" && strings.Contains(raw, "inf") {
			t.Fatalf("infinity accepted: %q", raw)
		}
	})
}

// FuzzMemoryQuantity exercises the memory Quantity parser. Accepted input
// must always yield a non-negative byte count.
func FuzzMemoryQuantity(f *testing.F) {
	for _, seed := range []string{"", "128Mi", "1Ki", "2.5G", "0", "-4Gi", "abc", "1e3"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		value, err := memoryBytes(raw)
		if err != nil {
			if !errors.Is(err, errInvalidQuantity) {
				t.Fatalf("unexpected error type: %v", err)
			}
			return
		}
		if value < 0 {
			t.Fatalf("negative bytes for %q: %d", raw, value)
		}
	})
}
