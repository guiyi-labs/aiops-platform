package correlation

// Covers the pure confidence/completeness/sorting helpers and the JSONB
// repository serialization helpers. These run without a store or cluster.

import (
	"testing"
	"time"
)

func TestClassifyConfidence(t *testing.T) {
	rule := RuleDescriptor{
		RequiredFactors: []string{"time_distance", "change_symptom_rule"},
	}
	full := []Factor{
		{Kind: "time_distance", Weight: 1.0},
		{Kind: "change_symptom_rule", Weight: 1.0},
	}
	partial := []Factor{{Kind: "time_distance", Weight: 1.0}}
	contradicting := []EvidenceRef{{Kind: "contradicting_signal", ID: 1}}

	if got := classifyConfidence(rule, full, nil); got != ConfidenceConfirmed {
		t.Errorf("full factors = %s, want confirmed", got)
	}
	if got := classifyConfidence(rule, partial, nil); got != ConfidenceCandidate {
		t.Errorf("partial factors = %s, want candidate", got)
	}
	if got := classifyConfidence(rule, nil, nil); got != ConfidenceUnknown {
		t.Errorf("no factors = %s, want unknown", got)
	}
	if got := classifyConfidence(rule, full, contradicting); got != ConfidenceCandidate {
		t.Errorf("contradicting + full = %s, want candidate", got)
	}
	if got := classifyConfidence(rule, partial, contradicting); got != ConfidenceContradicted {
		t.Errorf("contradicting + partial = %s, want contradicted", got)
	}
	if got := countRequiredFactors(rule, nil); got != 0 {
		t.Errorf("countRequiredFactors(nil) = %d, want 0", got)
	}
	if got := countRequiredFactors(rule, partial); got != 1 {
		t.Errorf("countRequiredFactors(partial) = %d, want 1", got)
	}
}

func TestSortAndMergeHelpers(t *testing.T) {
	now := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	results := []CorrelationResult{
		{Case: Case{FirstObservedAt: now.Add(2 * time.Hour)}},
		{Case: Case{FirstObservedAt: now}},
	}
	sortResultsByFirstObserved(results)
	if !results[0].Case.FirstObservedAt.Equal(now) {
		t.Error("sortResultsByFirstObserved did not order by time")
	}
	cases := []Case{
		{FirstObservedAt: now.Add(2 * time.Hour)},
		{FirstObservedAt: now.Add(3 * time.Minute)},
	}
	sortCasesByFirstObserved(cases)
	if !cases[0].FirstObservedAt.Equal(now.Add(3 * time.Minute)) {
		t.Error("sortCasesByFirstObserved did not order by time")
	}
	merged := mergeFactors(
		[]Factor{{Kind: "a", Weight: 0.5}, {Kind: "b", Weight: 1.0}},
		[]Factor{{Kind: "a", Weight: 0.9}, {Kind: "c", Weight: 0.2}},
	)
	if len(merged) != 3 {
		t.Fatalf("mergeFactors = %d, want 3", len(merged))
	}
	for _, f := range merged {
		if f.Kind == "a" && f.Weight != 0.9 {
			t.Errorf("factor a weight = %v, want 0.9", f.Weight)
		}
	}
}

func TestJSONBHelpers(t *testing.T) {
	if got := mustMarshalJSONB([]int64{1, 2}); string(got) != "[1,2]" {
		t.Errorf("mustMarshalJSONB = %s", got)
	}
	if got := unmarshalFactors(JSONB(`[{"kind":"time","weight":1}]`)); len(got) != 1 || got[0].Kind != "time" {
		t.Errorf("unmarshalFactors = %+v", got)
	}
	if got := unmarshalFactors(nil); got != nil {
		t.Errorf("unmarshalFactors(nil) = %v, want nil", got)
	}
	if got := unmarshalFactors(JSONB(`not-json`)); got != nil {
		t.Errorf("unmarshalFactors(bad) = %v, want nil", got)
	}
	if got := unmarshalEvidenceRefs(JSONB(`[{"kind":"signal","id":1,"content_hash":"abc"}]`)); len(got) != 1 || got[0].ID != 1 || got[0].ContentHash != "abc" {
		t.Errorf("unmarshalEvidenceRefs = %+v", got)
	}
	if got := unmarshalInt64s(JSONB(`[1,2]`)); len(got) != 2 || got[0] != 1 {
		t.Errorf("unmarshalInt64s = %v", got)
	}
	if got := unmarshalStrings(JSONB(`["a"]`)); len(got) != 1 || got[0] != "a" {
		t.Errorf("unmarshalStrings = %v", got)
	}
	merged := mergeFactorsJSON(JSONB(`[{"kind":"a","weight":0.5}]`), JSONB(`[{"kind":"a","weight":0.9},{"kind":"b","weight":0.3}]`))
	if !containsFactor(merged, "a", 0.9) || !containsFactor(merged, "b", 0.3) {
		t.Errorf("mergeFactorsJSON = %s", string(merged))
	}
}

func containsFactor(j JSONB, kind string, weight float64) bool {
	factors := unmarshalFactors(j)
	for _, f := range factors {
		if f.Kind == kind && f.Weight == weight {
			return true
		}
	}
	return false
}

func TestRankCompleteness(t *testing.T) {
	if rankCompleteness(CompletenessComplete) != 2 {
		t.Error("complete rank should be 2")
	}
	if rankCompleteness(CompletenessPartial) != 1 {
		t.Error("partial rank should be 1")
	}
	if rankCompleteness(CompletenessInsufficient) != 0 {
		t.Error("insufficient rank should be 0")
	}
}
