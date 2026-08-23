package main

import (
	"testing"
	"time"
)

// TestCorpusLabelsMatchEngine is the CI-enforced label contract: every
// corpus scenario must reproduce its expected label through the same
// exported rule functions used in production. A failure means a rule
// changed behavior — review the rule or relabel deliberately.
func TestCorpusLabelsMatchEngine(t *testing.T) {
	corpus, err := loadCorpus()
	if err != nil {
		t.Fatal(err)
	}
	observed := corpus.ObservedAt
	if observed.IsZero() {
		observed = time.Date(2026, 8, 23, 8, 30, 0, 0, time.UTC)
	}
	for i := range corpus.Scenarios {
		s := &corpus.Scenarios[i]
		t.Run(s.ID, func(t *testing.T) {
			matched, err := evaluateTarget(s, observed)
			if err != nil {
				t.Fatal(err)
			}
			if matched != s.Expected {
				t.Fatalf("label mismatch: scenario %s (rule %s) matched=%v want %v",
					s.ID, s.TargetRule, matched, s.Expected)
			}
			if s.Kind == "Pod" {
				got, err := evaluatePipeline(s, observed)
				if err != nil {
					t.Fatal(err)
				}
				if got != s.PipelineExpect {
					t.Fatalf("pipeline mismatch: scenario %s selected %q want %q",
						s.ID, got, s.PipelineExpect)
				}
			}
		})
	}
}

// TestRetrievalBenchDeterministic guards the hermetic property of the
// retrieval benchmark: identical inputs must yield identical numbers.
func TestRetrievalBenchDeterministic(t *testing.T) {
	first := runRetrievalScale(10, 10, 3)
	second := runRetrievalScale(10, 10, 3)
	if first != second {
		t.Fatalf("retrieval bench is not deterministic:\n%+v\n%+v", first, second)
	}
	if first.Queries != len(benchmarkRules) {
		t.Fatalf("expected one query per rule (%d), got %d", len(benchmarkRules), first.Queries)
	}
}

// TestRetrievalShortlistBoundary verifies the known limitation of a
// recency-ordered structured index: when same-rule Pod-kind entries exceed
// the shortlist, the oldest (ground-truth) entry falls out of stage 1.
func TestRetrievalShortlistBoundary(t *testing.T) {
	small := runRetrievalScale(4, 10, 3) // pod-kind entries per rule = 1 <= shortlist
	if small.HitAt1 != 1 {
		t.Fatalf("within-shortlist ground truth should hit at rank 1, got Hit@1=%.3f", small.HitAt1)
	}
	large := runRetrievalScale(30, 10, 3) // pod-kind per rule = 8 > shortlist 10? no: 8 <= 10
	// At n=30 the oldest Pod entry sits at insertion index 28 -> outside the
	// newest-first shortlist of 10; Hit@1 must degrade.
	if large.HitAt1 >= small.HitAt1 {
		t.Fatalf("expected Hit@1 degradation beyond shortlist capacity, got %.3f vs %.3f",
			large.HitAt1, small.HitAt1)
	}
}
