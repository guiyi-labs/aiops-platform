package correlation

import (
	"context"
	"testing"
)

// TestGoldenFixtures exercises all 9 golden replay scenarios plus the
// cold-start scenario. Each fixture is a deterministic (inputs, expected)
// pair; identical inputs + rule/correlation versions must reproduce identical
// results. This is the replayable M42 contract.
func TestGoldenFixtures(t *testing.T) {
	fixtures := append(GoldenFixtures(), coldStartFixture())
	engine := NewEngine()

	for _, fx := range fixtures {
		t.Run(fx.Name, func(t *testing.T) {
			results, err := engine.Correlate(context.Background(), fx.Inputs)
			if err != nil {
				t.Fatalf("Correlate returned error: %v", err)
			}
			if len(results) != fx.ExpectResults {
				t.Fatalf("expected %d results, got %d", fx.ExpectResults, len(results))
			}
			if len(results) == 0 {
				return
			}
			r := results[0]
			if r.Case.RuleID != fx.ExpectRuleID {
				t.Errorf("expected rule %q, got %q", fx.ExpectRuleID, r.Case.RuleID)
			}
			if r.Case.Confidence != fx.ExpectConfidence {
				t.Errorf("expected confidence %q, got %q", fx.ExpectConfidence, r.Case.Confidence)
			}
			if len(r.ChangeCandidates) != fx.ExpectCandidates {
				t.Errorf("expected %d change candidates, got %d", fx.ExpectCandidates, len(r.ChangeCandidates))
			}
			if len(r.SignalLinks) < fx.ExpectSignalLinks {
				t.Errorf("expected at least %d signal links, got %d", fx.ExpectSignalLinks, len(r.SignalLinks))
			}
			if len(r.ResourceLinks) < fx.ExpectResourceLinks {
				t.Errorf("expected at least %d resource links, got %d", fx.ExpectResourceLinks, len(r.ResourceLinks))
			}
			// Trigger signal link must always be present.
			foundTrigger := false
			for _, sl := range r.SignalLinks {
				if sl.Relation == SignalRelationTrigger {
					foundTrigger = true
					break
				}
			}
			if !foundTrigger {
				t.Error("expected a trigger signal link, found none")
			}
			// Primary resource link must always be present.
			foundPrimary := false
			for _, rl := range r.ResourceLinks {
				if rl.Relation == ResourceRelationPrimary {
					foundPrimary = true
					break
				}
			}
			if !foundPrimary {
				t.Error("expected a primary resource link, found none")
			}
			// Cold-start cases must have confidence unknown and no candidates.
			if fx.ExpectColdStart {
				if r.Case.Confidence != ConfidenceUnknown {
					t.Errorf("cold-start case must be unknown, got %q", r.Case.Confidence)
				}
				if len(r.ChangeCandidates) != 0 {
					t.Errorf("cold-start case must have 0 candidates, got %d", len(r.ChangeCandidates))
				}
				if r.Case.EvidenceCompleteness != CompletenessInsufficient {
					t.Errorf("cold-start case must be insufficient, got %q", r.Case.EvidenceCompleteness)
				}
			}
		})
	}
}

// TestGoldenFixturesDeterminism verifies that replaying the same fixtures
// produces byte-identical case_keys and confidence. This is the core M42
// deterministic invariant.
func TestGoldenFixturesDeterminism(t *testing.T) {
	fixtures := GoldenFixtures()
	engine := NewEngine()

	firstRun := make(map[string]string) // fixture name → case_key
	for _, fx := range fixtures {
		results, err := engine.Correlate(context.Background(), fx.Inputs)
		if err != nil {
			t.Fatalf("%s: Correlate error: %v", fx.Name, err)
		}
		if len(results) > 0 {
			firstRun[fx.Name] = results[0].Case.CaseKey
		}
	}

	// Replay and compare.
	for _, fx := range fixtures {
		results, err := engine.Correlate(context.Background(), fx.Inputs)
		if err != nil {
			t.Fatalf("%s: replay Correlate error: %v", fx.Name, err)
		}
		if len(results) == 0 {
			continue
		}
		if results[0].Case.CaseKey != firstRun[fx.Name] {
			t.Errorf("%s: case_key changed on replay: %q → %q", fx.Name, firstRun[fx.Name], results[0].Case.CaseKey)
		}
	}
}

// TestGoldenFixturesCaseKeyStability verifies that case_keys are stable
// across separate engine instances. The case_key depends only on
// (cluster_id, resource_uid, rule_id, correlation_version), not on the
// engine instance.
func TestGoldenFixturesCaseKeyStability(t *testing.T) {
	engine1 := NewEngine()
	engine2 := NewEngine()

	for _, fx := range GoldenFixtures() {
		r1, _ := engine1.Correlate(context.Background(), fx.Inputs)
		r2, _ := engine2.Correlate(context.Background(), fx.Inputs)
		if len(r1) != len(r2) {
			t.Fatalf("%s: result count differs between engines", fx.Name)
		}
		for i := range r1 {
			if r1[i].Case.CaseKey != r2[i].Case.CaseKey {
				t.Errorf("%s: case_key differs between engine instances", fx.Name)
			}
		}
	}
}
