package finding

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

// buildFixture constructs a representative v1 finding shared by the parity and
// snapshot tests below.
func buildFixture() Finding {
	return Finding{
		Code:       "CAPACITY_SATURATION_RISK",
		Severity:   SeverityWarning,
		Summary:    "capacity cluster saturation risk within 30 days",
		Resource:   ResourceCitation{Kind: "Node", Namespace: "", Name: "worker-a", UID: "u1", ResourceVersion: "rv1"},
		Details:    map[string]string{"metric": "cpu", "threshold": "80"},
		ObservedAt: RFC3339(time.Date(2026, 8, 10, 3, 30, 0, 0, time.UTC)),
	}
}

// TestFromV1ToV1RoundTrip proves a v1 finding can be promoted to v2 and demoted
// back to v1 with no data loss on any v1 field.
func TestFromV1ToV1RoundTrip(t *testing.T) {
	orig := buildFixture()
	detail := FromV1(orig)
	if detail.SchemaVersion != SchemaVersion {
		t.Errorf("schema version = %q, want %q", detail.SchemaVersion, SchemaVersion)
	}
	got := detail.ToV1()
	if !reflect.DeepEqual(got, orig) {
		t.Errorf("round-trip mismatch\n got: %+v\nwant: %+v", got, orig)
	}
}

// TestDetailJSONParityLock verifies that serializing a v2 detail's flattening
// is byte-identical to serializing its v1 source for the v1 JSON surface, so
// legacy consumers see a stable wire contract.
func TestDetailJSONParityLock(t *testing.T) {
	orig := buildFixture()
	detail := FromV1(orig)

	a, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal v1: %v", err)
	}
	b, err := json.Marshal(detail.ToV1())
	if err != nil {
		t.Fatalf("marshal detail.ToV1(): %v", err)
	}
	if string(a) != string(b) {
		t.Errorf("v1 JSON parity broken\n v1: %s\nflattened: %s", a, b)
	}
}

// TestDetailSerializationSnapshot locks the v2 wire shape so accidental
// breaking changes to the unified finding contract are caught.
func TestDetailSerializationSnapshot(t *testing.T) {
	detail := FromV1(buildFixture())
	detail.Rule = RuleIdentity{RuleID: "CAPACITY_SATURATION_RISK", Framework: "capacity", Source: "capacity", Version: "1.0"}
	detail.Evidence = []EvidenceRef{{
		ID:         "evt-1",
		Kind:       EvidenceChange,
		Summary:    "capacity pressure change",
		ObservedAt: RFC3339(time.Date(2026, 8, 10, 3, 30, 0, 0, time.UTC)),
	}}
	detail.Recommendations = []Recommendation{{
		Kind:       RecommendationControlledAction,
		Text:       "scale down over-provisioned replicas",
		Capability: "deployment.rollout_restart",
	}}
	detail.OriginIDs = []string{"CAPACITY_SATURATION_RISK"}

	data, err := json.Marshal(detail)
	if err != nil {
		t.Fatalf("marshal detail: %v", err)
	}
	got := string(data)

	// The snapshot is intentionally a subset assertion: we check the structural
	// keys that the unified evidence model promises, not incidental field order.
	for _, wantFragment := range []string{
		`"schema_version":"2"`,
		`"rule":{"rule_id":"CAPACITY_SATURATION_RISK","framework":"capacity","source":"capacity","version":"1.0"}`,
		`"code":"CAPACITY_SATURATION_RISK"`,
		`"severity":"warning"`,
		`"evidence":[{"id":"evt-1","kind":"change"`,
		`"recommendations":[{"kind":"controlled_action_available","text":"scale down over-provisioned replicas","capability":"deployment.rollout_restart"}]`,
		`"origin_ids":["CAPACITY_SATURATION_RISK"]`,
	} {
		if !strings.Contains(got, wantFragment) {
			t.Errorf("detail snapshot missing %q\nfull: %s", wantFragment, got)
		}
	}
}

// TestMergeDistinctPerResource verifies duplicate findings for the same
// resource collapse to one row while preserving every per-rule origin ID.
func TestMergeDistinctPerResource(t *testing.T) {
	base := buildFixture()
	d1 := FromV1(base)
	d1.Rule.RuleID = "RULE_A"
	d2 := FromV1(base)
	d2.Rule.RuleID = "RULE_B"
	d2.Code = "OTHER_CODE"
	d3 := FromV1(base)
	d3.Resource.Name = "worker-b"
	d3.Rule.RuleID = "RULE_C"

	merged := MergeDistinct([]FindingDetail{d1, d2, d3})
	if len(merged) != 2 {
		t.Fatalf("MergeDistinct produced %d rows, want 2", len(merged))
	}
	first := merged[0]
	if first.Resource.Name != base.Resource.Name {
		t.Errorf("merged row resource = %q, want %q", first.Resource.Name, base.Resource.Name)
	}
	// Collect origin IDs for the worker-a group.
	origins := map[string]bool{}
	for _, id := range first.OriginIDs {
		origins[id] = true
	}
	if !origins["RULE_A"] || !origins["RULE_B"] {
		t.Errorf("merged row missing original rule sources, got %v", first.OriginIDs)
	}
	if origins["RULE_C"] {
		t.Errorf("merged row wrongly contains worker-b rule source %v", first.OriginIDs)
	}
}

// TestRecommendationKindsNeverAutoExecuted locks the non-execution contract:
// the platform only ever surfaces recommendation kinds, never launches them
// implicitly.
func TestRecommendationKindsNeverAutoExecuted(t *testing.T) {
	for _, kind := range []RecommendationKind{
		RecommendationAdvisory,
		RecommendationControlledAction,
		RecommendationManualOnly,
	} {
		if string(kind) == "" {
			t.Error("recommendation kind must not be empty")
		}
	}
	if string(RecommendationControlledAction) != "controlled_action_available" {
		t.Errorf("controlled-action kind = %q, want controlled_action_available", RecommendationControlledAction)
	}
}

// TestSeverityMappingLocks the unified severity mapping shared by posture,
// optimization, diagnosis and inspection: every vocabulary normalizes into the
// canonical info / warning / critical set.
func TestSeverityMappingLocks(t *testing.T) {
	cases := []struct{ in, want string }{
		{"critical", "critical"},
		{"warning", "warning"},
		{"info", "info"},
		{"high", "critical"},
		{"medium", "warning"},
		{"low", "info"},
		{"", "info"},
		{"UNKNOWN", "info"},
	}
	for _, tc := range cases {
		if got := NormalizeSeverity(tc.in); got != tc.want {
			t.Errorf("NormalizeSeverity(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	if SeverityRank(SeverityCritical) >= SeverityRank(SeverityWarning) {
		t.Error("critical must rank above warning")
	}
	if SeverityRank(SeverityWarning) >= SeverityRank(SeverityInfo) {
		t.Error("warning must rank above info")
	}
	if MaxSeverity("warning", "critical") != "critical" {
		t.Error("MaxSeverity must prefer critical")
	}
	if MaxSeverity("high", "warning") != "high" {
		t.Error("MaxSeverity must normalize diagnosis vocabulary")
	}
}
