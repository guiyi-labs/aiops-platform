package metricshistory

import (
	"errors"
	"testing"
)

func TestCPUQuantityConvertsToNanocores(t *testing.T) {
	tests := map[string]int64{
		"250m":       250000000,
		"123456789n": 123456789,
		"2":          2000000000,
	}
	for raw, expected := range tests {
		value, err := cpuNanocores(raw)
		if err != nil || value != expected {
			t.Fatalf("cpuNanocores(%q) = %d, %v; want %d", raw, value, err, expected)
		}
	}
}

func TestMemoryQuantityConvertsToBytes(t *testing.T) {
	tests := map[string]int64{
		"1Ki": 1024,
		"5Mi": 5 * 1024 * 1024,
		"1G":  1000000000,
	}
	for raw, expected := range tests {
		value, err := memoryBytes(raw)
		if err != nil || value != expected {
			t.Fatalf("memoryBytes(%q) = %d, %v; want %d", raw, value, err, expected)
		}
	}
}

func TestQuantityConversionRejectsInvalidNegativeAndOverflowedValues(t *testing.T) {
	for _, raw := range []string{"", "not-a-quantity", "-1m", "999999999999999999999999999999999999999999"} {
		if _, err := cpuNanocores(raw); !errors.Is(err, errInvalidQuantity) {
			t.Fatalf("cpuNanocores(%q) error = %v", raw, err)
		}
		if _, err := memoryBytes(raw); !errors.Is(err, errInvalidQuantity) {
			t.Fatalf("memoryBytes(%q) error = %v", raw, err)
		}
	}
}
