package finding_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"k8s-aiops.local/backend/internal/capacity"
	"k8s-aiops.local/backend/internal/cis"
	"k8s-aiops.local/backend/internal/deprecatedapi"
	"k8s-aiops.local/backend/internal/finding"
	"k8s-aiops.local/backend/internal/finops"
	"k8s-aiops.local/backend/internal/gitopsdrift"
	"k8s-aiops.local/backend/internal/hpa"
	"k8s-aiops.local/backend/internal/imagepolicy"
	"k8s-aiops.local/backend/internal/ingressposture"
	"k8s-aiops.local/backend/internal/netpolicy"
	"k8s-aiops.local/backend/internal/pdb"
	"k8s-aiops.local/backend/internal/policy"
)

// assertCanonicalSlice verifies the given value is a slice whose element type
// is the canonical finding.Finding struct.
func assertCanonicalSlice(t *testing.T, named string, value interface{}) {
	t.Helper()
	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Slice {
		t.Fatalf("%s: expected a slice, got %v", named, rv.Kind())
	}
	want := reflect.TypeOf(finding.Finding{})
	if got := rv.Type().Elem(); got != want {
		t.Fatalf("%s element type = %v, want canonical %v", named, got, want)
	}
}

// assertStatusFindings verifies a Status-like type exposes a []finding.Finding
// Findings field.
func assertStatusFindings(t *testing.T, named string, value interface{}) {
	t.Helper()
	rv := reflect.ValueOf(value)
	for rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	field := rv.FieldByName("Findings")
	if !field.IsValid() {
		t.Fatalf("%s: no Findings field", named)
	}
	assertCanonicalSlice(t, named, field.Interface())
}

// TestAnalyzerSchemaParity locks the M95 acceptance: every posture analyzer
// emits the canonical finding.Finding type (documented alias), so they share
// one severity mapping and one evidence model. If an analyzer drifts to a
// private finding struct, the uniform frontend renderer would break silently.
func TestAnalyzerSchemaParity(t *testing.T) {
	t.Run("findings-field analyzers", func(t *testing.T) {
		cases := []struct {
			name  string
			value interface{}
		}{
			{"capacity", &capacity.Status{}},
			{"cis", &cis.Status{}},
			{"deprecatedapi", &deprecatedapi.Status{}},
			{"gitopsdrift", &gitopsdrift.Status{}},
			{"hpa", &hpa.Status{}},
			{"imagepolicy", &imagepolicy.Status{}},
			{"ingressposture", &ingressposture.Status{}},
			{"netpolicy", &netpolicy.Status{}},
			{"pdb", &pdb.Status{}},
			{"policy", &policy.Status{}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				assertStatusFindings(t, tc.name, tc.value)
			})
		}
	})

	t.Run("finops ToFindings", func(t *testing.T) {
		assertCanonicalSlice(t, "finops.ToFindings", finops.WasteSummary{}.ToFindings())
	})
}

// TestV1ToV2ParityLock proves the unified v2 model never changes the legacy v1
// JSON wire contract for consumers.
func TestV1ToV2ParityLock(t *testing.T) {
	sample := finding.Finding{
		Code:       "SAMPLE_RULE",
		Severity:   finding.SeverityWarning,
		Summary:    "sample unified evidence finding",
		Resource:   finding.ResourceCitation{Kind: "Node", Namespace: "", Name: "node-a", UID: "uid-1"},
		Details:    map[string]string{"key": "value"},
		ObservedAt: "2026-08-10T10:00:00Z",
	}
	v1, err := json.Marshal(sample)
	if err != nil {
		t.Fatalf("marshal v1: %v", err)
	}
	detail := finding.FromV1(sample)
	flat, err := json.Marshal(detail.ToV1())
	if err != nil {
		t.Fatalf("marshal ToV1(): %v", err)
	}
	if string(v1) != string(flat) {
		t.Errorf("v1 wire contract changed\n v1: %s\nflattened: %s", v1, flat)
	}
}
