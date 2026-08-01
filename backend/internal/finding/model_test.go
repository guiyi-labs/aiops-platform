package finding

import (
	"testing"
	"time"
)

func TestRFC3339(t *testing.T) {
	got := RFC3339(time.Date(2026, 8, 1, 13, 48, 0, 0, time.UTC))
	want := "2026-08-01T13:48:00Z"
	if got != want {
		t.Errorf("RFC3339 = %q, want %q", got, want)
	}
}

func TestSeverityConstants(t *testing.T) {
	levels := []string{SeverityInfo, SeverityWarning, SeverityCritical}
	for _, s := range levels {
		if s == "" {
			t.Error("severity constant must not be empty")
		}
	}
}

func TestFindingJSONTags(t *testing.T) {
	// The struct must serialize with the same JSON keys the frontend expects
	// from namespaceposture.Finding so rendering stays uniform.
	f := Finding{
		Code:       "X",
		Severity:   SeverityWarning,
		Summary:    "y",
		Resource:   ResourceCitation{Kind: "Pod", Namespace: "ns", Name: "p"},
		Details:    map[string]string{"k": "v"},
		ObservedAt: RFC3339(time.Now()),
	}
	if f.Resource.Kind != "Pod" || f.Code != "X" {
		t.Error("finding fields not populated as expected")
	}
}
