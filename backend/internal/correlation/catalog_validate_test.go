package correlation

// ValidateRule covers the catalog validation contract used at compile time:
// every invariant must fail closed with a descriptive error and the
// compiled-in catalog must be internally consistent.

import "testing"

func TestValidateRuleErrors(t *testing.T) {
	baseValid := RuleDescriptor{
		RuleID:          "correlation.test.v1",
		TriggerSignals:  []string{"sig.one"},
		ChangeKinds:     []string{"rollout"},
		PrimaryKind:     "Pod",
		TimeWindowSecs:  60,
		RequiredFactors: []string{"time_distance"},
		ReasonCode:      "test.reason",
	}
	if err := ValidateRule(baseValid); err != nil {
		t.Fatalf("ValidateRule(baseValid) = %v, want nil", err)
	}

	cases := []struct {
		name string
		mut  func(*RuleDescriptor)
	}{
		{"empty_rule_id", func(d *RuleDescriptor) { d.RuleID = "" }},
		{"no_triggers", func(d *RuleDescriptor) { d.TriggerSignals = nil }},
		{"no_change_kinds", func(d *RuleDescriptor) { d.ChangeKinds = nil }},
		{"no_primary_kind", func(d *RuleDescriptor) { d.PrimaryKind = "" }},
		{"time_window_zero", func(d *RuleDescriptor) { d.TimeWindowSecs = 0 }},
		{"time_window_too_large", func(d *RuleDescriptor) { d.TimeWindowSecs = MaxTimeDistanceSecs + 1 }},
		{"no_required_factors", func(d *RuleDescriptor) { d.RequiredFactors = nil }},
		{"no_reason_code", func(d *RuleDescriptor) { d.ReasonCode = "" }},
	}
	for _, tc := range cases {
		d := baseValid
		tc.mut(&d)
		if err := ValidateRule(d); err == nil {
			t.Errorf("%s: ValidateRule should fail", tc.name)
		}
	}
}

func TestAllCatalogRulesValid(t *testing.T) {
	for _, r := range AllRules() {
		if err := ValidateRule(r); err != nil {
			t.Errorf("catalog rule %s invalid: %v", r.RuleID, err)
		}
	}
}
