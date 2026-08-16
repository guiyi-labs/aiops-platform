package aiinvestigator

import (
	"strings"
	"testing"
)

func TestCaseContextHistoricalReferencesRender(t *testing.T) {
	ctx := CaseContext{
		CaseID:               7,
		RuleID:               "correlation.rule",
		Confidence:           "high",
		EvidenceCompleteness: "complete",
		PrimaryResourceKind:  "Deployment",
		PrimaryResourceName:  "api",
		PrimaryResourceUID:   "deploy-7",
		HistoricalCases: []HistoricalCaseContext{
			{
				RuleID:          "crash_loop",
				Severity:        "high",
				Summary:         "api crashed yesterday",
				RootCauses:      []string{"image_pull_backoff"},
				Recommendations: []string{"check registry"},
				NotedAt:         "2026-08-12",
			},
		},
	}
	prompt, err := BuildPrompt(ctx, nil)
	if err != nil {
		t.Fatalf("BuildPrompt: %v", err)
	}
	if !strings.Contains(prompt.Input, "HISTORICAL REFERENCES") {
		t.Fatalf("historical section missing: %s", prompt.Input)
	}
	if !strings.Contains(prompt.Input, "historical:1 rule=crash_loop") {
		t.Fatalf("historical entry missing: %s", prompt.Input)
	}
	if !strings.Contains(prompt.Input, "root_causes: image_pull_backoff") {
		t.Fatalf("historical root cause missing: %s", prompt.Input)
	}
	if !strings.Contains(prompt.Input, "recommendations: check registry") {
		t.Fatalf("historical recommendation missing: %s", prompt.Input)
	}
}

func TestHistoricalCasesDoNotEnterEvidenceSet(t *testing.T) {
	withHistory := CaseContext{
		CaseID:              7,
		RuleID:              "correlation.rule",
		PrimaryResourceKind: "Deployment",
		PrimaryResourceName: "api",
		PrimaryResourceUID:  "deploy-7",
		HistoricalCases: []HistoricalCaseContext{
			{RuleID: "crash_loop", Severity: "high", Summary: "old", NotedAt: "2026-08-12"},
		},
	}
	withoutHistory := withHistory
	withoutHistory.HistoricalCases = nil

	refsWith := buildAuthorizedEvidence(withHistory)
	refsWithout := buildAuthorizedEvidence(withoutHistory)
	if len(refsWith) != len(refsWithout) {
		t.Fatalf("historical cases leaked into evidence set: with=%d without=%d",
			len(refsWith), len(refsWithout))
	}
	// The historical key must not exist in either set.
	if _, ok := refsWith["historical:1"]; ok {
		t.Fatal("historical:1 must not be an authorized evidence ref")
	}

	// Prompt hash stays identical with or without history.
	hashWith := computePromptHash(withHistory, refsWith)
	hashWithout := computePromptHash(withoutHistory, refsWithout)
	if hashWith != hashWithout {
		t.Fatal("historical cases must not change the prompt hash (replay key stability)")
	}
}

func TestEvidenceKindHistoricalCaseConstant(t *testing.T) {
	if string(EvidenceKindHistoricalCase) != "historical_case" {
		t.Fatalf("EvidenceKindHistoricalCase = %q", EvidenceKindHistoricalCase)
	}
}
