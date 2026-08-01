package aiinvestigator

import (
	"context"
	"strings"
	"testing"
)

// runFixture runs one golden fixture through the validator and asserts the
// expected outcome. Returns the error (or nil) so callers can inspect it.
func runFixture(t *testing.T, f GoldenFixture) error {
	t.Helper()
	prompt := Prompt{EvidenceRefs: f.AuthorizedEvidence}
	validated, err := ValidateProviderResult(f.Result, prompt, f.EligibleActionCodes)
	if f.ExpectValid {
		if err != nil {
			t.Errorf("fixture %q: expected valid, got error: %v", f.Name, err)
			return err
		}
		// Validated result must preserve the provider/providerResponseID and
		// the recommended runbook (when set).
		if validated.Provider != f.Result.Provider {
			t.Errorf("fixture %q: provider = %q, want %q", f.Name, validated.Provider, f.Result.Provider)
		}
		if validated.Model != f.Result.Model {
			t.Errorf("fixture %q: model = %q, want %q", f.Name, validated.Model, f.Result.Model)
		}
		if validated.RecommendedRunbookID != f.Result.RecommendedRunbookID {
			t.Errorf("fixture %q: runbook = %q, want %q", f.Name, validated.RecommendedRunbookID, f.Result.RecommendedRunbookID)
		}
		if len(validated.Citations) != len(f.Result.Citations) {
			t.Errorf("fixture %q: citation count = %d, want %d", f.Name, len(validated.Citations), len(f.Result.Citations))
		}
		return nil
	}
	if err == nil {
		t.Errorf("fixture %q: expected failure (%q), got valid result", f.Name, f.ExpectFailureContains)
		return nil
	}
	if f.ExpectFailureContains != "" && !strings.Contains(err.Error(), f.ExpectFailureContains) {
		t.Errorf("fixture %q: error %q does not contain %q", f.Name, err.Error(), f.ExpectFailureContains)
	}
	return err
}

func TestGoldenFixtures(t *testing.T) {
	fixtures := GoldenFixtures()
	if len(fixtures) < 8 {
		t.Fatalf("expected at least 8 golden fixtures, got %d", len(fixtures))
	}
	for _, f := range fixtures {
		f := f
		t.Run(f.Name, func(t *testing.T) {
			runFixture(t, f)
		})
	}
}

func TestGoldenFixturesCoverage(t *testing.T) {
	// The acceptance scenarios from the optimization plan must all be present.
	fixtures := GoldenFixtures()
	names := make(map[string]bool, len(fixtures))
	for _, f := range fixtures {
		names[f.Name] = true
	}
	required := []string{
		"correct_cited_investigation",
		"insufficient_evidence",
		"conflicting_evidence",
		"prompt_injection_rejected",
		"hidden_scope_citation_rejected",
		"fabricated_citation_rejected",
		"ineligible_runbook_rejected",
		"confirm_root_claim_rejected",
		"empty_summary_rejected",
		"no_citations_rejected",
	}
	for _, name := range required {
		if !names[name] {
			t.Errorf("required fixture %q missing from GoldenFixtures", name)
		}
	}
}

func TestValidateProviderResultEdgeCases(t *testing.T) {
	caseRef := EvidenceRef{Kind: EvidenceKindCorrelationCase, ID: 1}
	authorized := map[string]EvidenceRef{
		"correlation_case:1": caseRef,
	}
	base := func() ProviderResult {
		return ProviderResult{
			Provider: "test",
			Model:    "test-1.0",
			Summary:  "A valid summary.",
			Impact:   "A valid impact.",
			Hypotheses: []Hypothesis{{
				Claim:       "A claim.",
				Confidence:  HypothesisHigh,
				EvidenceIDs: []EvidenceRef{caseRef},
			}},
			Citations: []Citation{
				{EvidenceRef: caseRef, Claim: "case exists"},
			},
		}
	}
	prompt := Prompt{EvidenceRefs: authorized}

	t.Run("empty impact rejected", func(t *testing.T) {
		r := base()
		r.Impact = "  "
		if _, err := ValidateProviderResult(r, prompt, nil); err == nil {
			t.Fatalf("empty impact should be rejected")
		}
	})
	t.Run("no hypotheses rejected", func(t *testing.T) {
		r := base()
		r.Hypotheses = nil
		if _, err := ValidateProviderResult(r, prompt, nil); err == nil {
			t.Fatalf("no hypotheses should be rejected")
		}
	})
	t.Run("invalid confidence rejected", func(t *testing.T) {
		r := base()
		r.Hypotheses[0].Confidence = "very-high"
		if _, err := ValidateProviderResult(r, prompt, nil); err == nil {
			t.Fatalf("invalid confidence should be rejected")
		}
	})
	t.Run("hypothesis with no evidence rejected", func(t *testing.T) {
		r := base()
		r.Hypotheses[0].EvidenceIDs = nil
		if _, err := ValidateProviderResult(r, prompt, nil); err == nil {
			t.Fatalf("hypothesis with no evidence should be rejected")
		}
	})
	t.Run("citation with empty claim rejected", func(t *testing.T) {
		r := base()
		r.Citations[0].Claim = "  "
		if _, err := ValidateProviderResult(r, prompt, nil); err == nil {
			t.Fatalf("citation with empty claim should be rejected")
		}
	})
	t.Run("too many hypotheses rejected", func(t *testing.T) {
		r := base()
		r.Hypotheses = make([]Hypothesis, MaxHypothesesPerInvestigation+1)
		for i := range r.Hypotheses {
			r.Hypotheses[i] = Hypothesis{
				Claim:       "claim",
				Confidence:  HypothesisLow,
				EvidenceIDs: []EvidenceRef{caseRef},
			}
		}
		if _, err := ValidateProviderResult(r, prompt, nil); err == nil {
			t.Fatalf("too many hypotheses should be rejected")
		}
	})
	t.Run("disconfirming evidence unauthorized rejected", func(t *testing.T) {
		r := base()
		r.Hypotheses[0].DisconfirmingEvidence = []EvidenceRef{
			{Kind: EvidenceKindSignalOccurrence, ID: 777},
		}
		if _, err := ValidateProviderResult(r, prompt, nil); err == nil {
			t.Fatalf("unauthorized disconfirming evidence should be rejected")
		}
	})
	t.Run("disconfirming evidence authorized accepted", func(t *testing.T) {
		signalRef := EvidenceRef{Kind: EvidenceKindSignalOccurrence, ID: 5}
		auth := map[string]EvidenceRef{
			"correlation_case:1":  caseRef,
			"signal_occurrence:5": signalRef,
		}
		r := base()
		r.Hypotheses[0].DisconfirmingEvidence = []EvidenceRef{signalRef}
		if _, err := ValidateProviderResult(r, Prompt{EvidenceRefs: auth}, nil); err != nil {
			t.Fatalf("authorized disconfirming evidence should be accepted: %v", err)
		}
	})
}

func TestNopProvider(t *testing.T) {
	t.Run("produces valid cited result for case evidence", func(t *testing.T) {
		caseRef := EvidenceRef{Kind: EvidenceKindCorrelationCase, ID: 7}
		prompt := Prompt{
			System:       "system",
			Input:        "input",
			EvidenceRefs: map[string]EvidenceRef{"correlation_case:7": caseRef},
		}
		result, err := NopProvider{}.Generate(context.Background(), prompt)
		if err != nil {
			t.Fatalf("NopProvider failed: %v", err)
		}
		// The nop result must pass validation against its own evidence set.
		validated, err := ValidateProviderResult(result, prompt, nil)
		if err != nil {
			t.Fatalf("nop result failed validation: %v", err)
		}
		if validated.Provider != "nop" {
			t.Errorf("Provider = %q, want nop", validated.Provider)
		}
		if len(validated.Citations) == 0 {
			t.Errorf("nop result must have at least one citation")
		}
	})
	t.Run("fails when no case evidence in prompt", func(t *testing.T) {
		signalRef := EvidenceRef{Kind: EvidenceKindSignalOccurrence, ID: 9}
		prompt := Prompt{
			EvidenceRefs: map[string]EvidenceRef{"signal_occurrence:9": signalRef},
		}
		_, err := NopProvider{}.Generate(context.Background(), prompt)
		if err == nil {
			t.Fatalf("NopProvider should fail when no case evidence is present")
		}
	})
}

func TestDecodeProviderJSON(t *testing.T) {
	t.Run("valid json", func(t *testing.T) {
		raw := `{"summary":"s","impact":"i","hypotheses":[{"claim":"c","confidence":"high","evidence_ids":[{"kind":"correlation_case","id":1}]}],"recommended_runbook_id":"","uncertainties":[],"citations":[{"evidence_ref":{"kind":"correlation_case","id":1},"claim":"x"}]}`
		result, err := DecodeProviderJSON(raw)
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		if result.Summary != "s" {
			t.Errorf("Summary = %q, want s", result.Summary)
		}
		if len(result.Hypotheses) != 1 {
			t.Fatalf("expected 1 hypothesis, got %d", len(result.Hypotheses))
		}
		if result.Hypotheses[0].Confidence != HypothesisHigh {
			t.Errorf("confidence = %q, want high", result.Hypotheses[0].Confidence)
		}
	})
	t.Run("malformed json", func(t *testing.T) {
		_, err := DecodeProviderJSON("{not json")
		if err == nil {
			t.Fatalf("malformed json should error")
		}
		if !strings.Contains(err.Error(), "malformed JSON") {
			t.Errorf("error should mention malformed JSON, got: %v", err)
		}
	})
	t.Run("trims whitespace", func(t *testing.T) {
		raw := `{"summary":"  s  ","impact":"i","hypotheses":[],"recommended_runbook_id":"  inspect_pvc_capacity  ","uncertainties":[],"citations":[]}`
		result, err := DecodeProviderJSON(raw)
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		if result.Summary != "s" {
			t.Errorf("Summary should be trimmed, got %q", result.Summary)
		}
		if result.RecommendedRunbookID != "inspect_pvc_capacity" {
			t.Errorf("RunbookID should be trimmed, got %q", result.RecommendedRunbookID)
		}
	})
}
