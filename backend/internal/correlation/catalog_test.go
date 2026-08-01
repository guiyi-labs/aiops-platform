package correlation

import (
	"testing"
)

// TestCatalogAllRules returns the full rule catalog with stable rule IDs.
func TestCatalogAllRules(t *testing.T) {
	rules := AllRules()
	if len(rules) == 0 {
		t.Fatal("expected at least one rule in catalog")
	}
	seen := make(map[string]bool)
	for _, r := range rules {
		if r.RuleID == "" {
			t.Error("rule with empty RuleID")
		}
		if seen[r.RuleID] {
			t.Errorf("duplicate rule ID: %s", r.RuleID)
		}
		seen[r.RuleID] = true
	}
}

// TestCatalogLookupRule verifies known and unknown rule lookups.
func TestCatalogLookupRule(t *testing.T) {
	r, ok := LookupRule("correlation.rollout_causes_pod_failure.v1")
	if !ok {
		t.Fatal("expected rollout_causes_pod_failure rule to exist")
	}
	if r.PrimaryKind != "Pod" {
		t.Errorf("expected PrimaryKind Pod, got %s", r.PrimaryKind)
	}
	if r.TimeWindowSecs != 3600 {
		t.Errorf("expected TimeWindowSecs 3600, got %d", r.TimeWindowSecs)
	}

	if _, ok := LookupRule("nonexistent.rule.v1"); ok {
		t.Error("expected lookup of nonexistent rule to fail")
	}
}

// TestCatalogRulesForTriggerSignal verifies trigger signal → rules mapping.
func TestCatalogRulesForTriggerSignal(t *testing.T) {
	rules := RulesForTriggerSignal("diag.pod.image_pull_backoff.v1")
	if len(rules) == 0 {
		t.Fatal("expected at least one rule for image_pull_backoff signal")
	}
	for _, r := range rules {
		found := false
		for _, sig := range r.TriggerSignals {
			if sig == "diag.pod.image_pull_backoff.v1" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("rule %s returned but does not list the trigger signal", r.RuleID)
		}
	}

	// Unknown signal returns no rules.
	rules = RulesForTriggerSignal("nonexistent.signal.v1")
	if len(rules) != 0 {
		t.Errorf("expected 0 rules for unknown signal, got %d", len(rules))
	}
}

// TestCatalogCorrelationVersion is stable.
func TestCatalogCorrelationVersion(t *testing.T) {
	if CorrelationVersion == "" {
		t.Error("CorrelationVersion must not be empty")
	}
}

// TestCatalogRequiredFactors verifies each rule has at least one required factor.
func TestCatalogRequiredFactors(t *testing.T) {
	for _, r := range AllRules() {
		if len(r.RequiredFactors) == 0 {
			t.Errorf("rule %s has no required factors", r.RuleID)
		}
	}
}
