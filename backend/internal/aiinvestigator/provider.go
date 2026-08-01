package aiinvestigator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Provider is the AI provider interface for the investigator. Implementations
// call the model and return raw structured output; the validator checks
// citations and runbook eligibility before the service persists.
type Provider interface {
	Generate(ctx context.Context, prompt Prompt) (ProviderResult, error)
}

// NopProvider returns a deterministic, citation-valid result for testing.
// It never calls a real model; use it in unit tests and when AI is disabled.
type NopProvider struct{}

func (NopProvider) Generate(_ context.Context, prompt Prompt) (ProviderResult, error) {
	// Build a minimal valid result: one citation to the case evidence, one
	// hypothesis citing the same, no runbook (advisory-only).
	var caseRef EvidenceRef
	for _, ref := range prompt.EvidenceRefs {
		if ref.Kind == EvidenceKindCorrelationCase {
			caseRef = ref
			break
		}
	}
	if caseRef.ID == 0 {
		return ProviderResult{}, fmt.Errorf("nop provider: no case evidence in prompt")
	}
	return ProviderResult{
		Provider: "nop",
		Model:    "nop-1.0",
		Summary:  "Deterministic nop investigation; AI provider is not configured.",
		Impact:   "No user-visible impact asserted (nop).",
		Hypotheses: []Hypothesis{{
			Claim:       "Nop hypothesis; no real AI analysis performed.",
			Confidence:  HypothesisLow,
			EvidenceIDs: []EvidenceRef{caseRef},
		}},
		Uncertainties: []string{"AI provider is not configured; configure a provider for real analysis."},
		Citations:     []Citation{{EvidenceRef: caseRef, Claim: "case exists"}},
	}, nil
}

// ValidateProviderResult checks that the provider's output is well-formed and
// that every citation references an authorized evidence ID. Returns the
// validated result, or an error describing the violation.
//
// Validation rules:
//  1. Summary and impact are non-empty.
//  2. At least one hypothesis is present (max 8).
//  3. Every hypothesis cites at least one authorized evidence ID.
//  4. Every disconfirming evidence ref is authorized.
//  5. At least one citation is present (max 64).
//  6. Every citation's evidence ref is authorized.
//  7. recommended_runbook_id (when set) is in the eligible set.
//  8. No hypothesis claims to "confirm" a root cause (the AI cannot upgrade
//     candidates — only operators can).
func ValidateProviderResult(result ProviderResult, prompt Prompt, eligibleActionCodes map[string]bool) (ProviderResult, error) {
	if containsPromptInjection(result) {
		return ProviderResult{}, fmt.Errorf("invalid output: prompt injection content rejected")
	}
	if strings.TrimSpace(result.Summary) == "" {
		return ProviderResult{}, fmt.Errorf("invalid output: summary is empty")
	}
	if strings.TrimSpace(result.Impact) == "" {
		return ProviderResult{}, fmt.Errorf("invalid output: impact is empty")
	}
	if len(result.Hypotheses) == 0 {
		return ProviderResult{}, fmt.Errorf("invalid output: no hypotheses")
	}
	if len(result.Hypotheses) > MaxHypothesesPerInvestigation {
		return ProviderResult{}, fmt.Errorf("invalid output: too many hypotheses (%d > %d)", len(result.Hypotheses), MaxHypothesesPerInvestigation)
	}
	if len(result.Citations) == 0 {
		return ProviderResult{}, fmt.Errorf("invalid output: no citations")
	}
	if len(result.Citations) > MaxCitationsPerInvestigation {
		return ProviderResult{}, fmt.Errorf("invalid output: too many citations (%d > %d)", len(result.Citations), MaxCitationsPerInvestigation)
	}
	if len(result.Uncertainties) > MaxUncertainties {
		return ProviderResult{}, fmt.Errorf("invalid output: too many uncertainties (%d > %d)", len(result.Uncertainties), MaxUncertainties)
	}

	// Check every citation references authorized evidence.
	for _, c := range result.Citations {
		if !isAuthorized(c.EvidenceRef, prompt.EvidenceRefs) {
			return ProviderResult{}, fmt.Errorf("citation rejected: evidence %s:%d not authorized", c.EvidenceRef.Kind, c.EvidenceRef.ID)
		}
		if strings.TrimSpace(c.Claim) == "" {
			return ProviderResult{}, fmt.Errorf("invalid output: citation with empty claim")
		}
	}

	// Check every hypothesis.
	for i, h := range result.Hypotheses {
		if strings.TrimSpace(h.Claim) == "" {
			return ProviderResult{}, fmt.Errorf("invalid output: hypothesis %d has empty claim", i)
		}
		if h.Confidence != HypothesisHigh && h.Confidence != HypothesisMedium && h.Confidence != HypothesisLow {
			return ProviderResult{}, fmt.Errorf("invalid output: hypothesis %d has invalid confidence %q", i, h.Confidence)
		}
		if len(h.EvidenceIDs) == 0 {
			return ProviderResult{}, fmt.Errorf("invalid output: hypothesis %d cites no evidence", i)
		}
		for _, ref := range h.EvidenceIDs {
			if !isAuthorized(ref, prompt.EvidenceRefs) {
				return ProviderResult{}, fmt.Errorf("hypothesis %d cites unauthorized evidence %s:%d", i, ref.Kind, ref.ID)
			}
		}
		for _, ref := range h.DisconfirmingEvidence {
			if !isAuthorized(ref, prompt.EvidenceRefs) {
				return ProviderResult{}, fmt.Errorf("hypothesis %d has unauthorized disconfirming evidence %s:%d", i, ref.Kind, ref.ID)
			}
		}
		if len(h.NextChecks) > MaxNextChecksPerHypothesis {
			return ProviderResult{}, fmt.Errorf("invalid output: hypothesis %d has too many next_checks", i)
		}
		// The AI cannot "confirm" a root cause — only operators can.
		// We check the claim text for the word "confirm" in a root-cause
		// context. This is a heuristic; the real invariant is that the
		// correlation case's confidence is never upgraded.
		claimLower := strings.ToLower(h.Claim)
		if strings.Contains(claimLower, "confirmed root cause") || strings.Contains(claimLower, "confirm root cause") {
			return ProviderResult{}, fmt.Errorf("invalid output: hypothesis %d claims to confirm root cause (AI cannot upgrade candidates)", i)
		}
	}

	// Check runbook eligibility.
	if result.RecommendedRunbookID != "" {
		if err := ValidateRunbookEligibility(result.RecommendedRunbookID, eligibleActionCodes); err != nil {
			return ProviderResult{}, fmt.Errorf("runbook rejected: %w", err)
		}
	}

	return result, nil
}

func containsPromptInjection(result ProviderResult) bool {
	texts := []string{result.Summary, result.Impact}
	for _, h := range result.Hypotheses {
		texts = append(texts, h.Claim)
		texts = append(texts, h.NextChecks...)
	}
	texts = append(texts, result.Uncertainties...)
	for _, citation := range result.Citations {
		texts = append(texts, citation.Claim)
	}
	for _, value := range texts {
		lower := strings.ToLower(value)
		for _, marker := range []string{
			"ignore previous instructions", "ignore all previous instructions",
			"reveal the system prompt", "system prompt has been overridden",
			"developer message", "disregard previous instructions",
		} {
			if strings.Contains(lower, marker) {
				return true
			}
		}
	}
	return false
}

// isAuthorized returns true when ref is in the authorized evidence set.
func isAuthorized(ref EvidenceRef, authorized map[string]EvidenceRef) bool {
	id := fmt.Sprintf("%s:%d", ref.Kind, ref.ID)
	_, ok := authorized[id]
	return ok
}

// DecodeProviderJSON decodes the raw JSON output from a provider into a
// ProviderResult. Used by the HTTP-style provider implementation.
func DecodeProviderJSON(raw string) (ProviderResult, error) {
	var output struct {
		Summary              string       `json:"summary"`
		Impact               string       `json:"impact"`
		Hypotheses           []Hypothesis `json:"hypotheses"`
		RecommendedRunbookID string       `json:"recommended_runbook_id"`
		Uncertainties        []string     `json:"uncertainties"`
		Citations            []Citation   `json:"citations"`
	}
	if err := json.Unmarshal([]byte(raw), &output); err != nil {
		return ProviderResult{}, fmt.Errorf("malformed JSON: %w", err)
	}
	return ProviderResult{
		Summary:              strings.TrimSpace(output.Summary),
		Impact:               strings.TrimSpace(output.Impact),
		Hypotheses:           output.Hypotheses,
		RecommendedRunbookID: strings.TrimSpace(output.RecommendedRunbookID),
		Uncertainties:        output.Uncertainties,
		Citations:            output.Citations,
	}, nil
}
