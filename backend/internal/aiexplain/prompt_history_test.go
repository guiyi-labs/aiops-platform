package aiexplain

import (
	"strings"
	"testing"
	"time"

	"k8s-aiops.local/backend/internal/diagnosis"
	"k8s-aiops.local/backend/internal/knowledge"
)

func TestBuildPromptWithHistoryInjectsAndRegistersEvidence(t *testing.T) {
	record := diagnosis.Record{
		RuleID:     "crash_loop",
		Severity:   "high",
		Summary:    "api crashing",
		Resource:   diagnosis.ResourceRef{Kind: "Deployment", Name: "api"},
		Evidence:   []diagnosis.Evidence{{Type: "event", Source: "event/test", Content: map[string]any{"message": "pull failed"}}},
		RootCauses: []string{"hypothesis"},
	}
	history := []knowledge.Entry{
		{
			RuleID:          "crash_loop",
			Severity:        "high",
			Summary:         "api crash one",
			RootCauses:      []string{"image_pull_backoff"},
			Recommendations: []string{"check registry"},
			NotedAt:         time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC),
		},
		{
			RuleID:          "crash_loop",
			Severity:        "critical",
			Summary:         "api crash two",
			RootCauses:      []string{"OOMKilled"},
			Recommendations: []string{"raise limits"},
			NotedAt:         time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC),
		},
	}

	prompt := BuildPromptWithHistory(record, history)
	if !strings.Contains(prompt.Input, "历史相似案例") {
		t.Fatalf("history lead-in missing in prompt: %s", prompt.Input)
	}
	if !strings.Contains(prompt.Input, "image_pull_backoff") || !strings.Contains(prompt.Input, "check registry") {
		t.Fatalf("history content missing: %s", prompt.Input)
	}
	if _, ok := prompt.EvidenceIDs["historical:1"]; !ok {
		t.Fatalf("historical:1 not registered as evidence: %#v", prompt.EvidenceIDs)
	}
	if _, ok := prompt.EvidenceIDs["historical:2"]; !ok {
		t.Fatalf("historical:2 not registered: %#v", prompt.EvidenceIDs)
	}
	// Base evidence remains citable too.
	if _, ok := prompt.EvidenceIDs["E1"]; !ok {
		t.Fatalf("base E1 not registered: %#v", prompt.EvidenceIDs)
	}
}

func TestBuildPromptWithHistoryEmpty(t *testing.T) {
	record := diagnosis.Record{
		RuleID:   "crash_loop",
		Severity: "high",
		Evidence: []diagnosis.Evidence{{Type: "event", Source: "event/test", Content: map[string]any{"message": "x"}}},
	}
	base := BuildPrompt(record)
	prompt := BuildPromptWithHistory(record, nil)
	if prompt.Input != base.Input {
		t.Fatalf("empty history must equal base prompt")
	}
	if len(prompt.EvidenceIDs) != len(base.EvidenceIDs) {
		t.Fatalf("empty history must not add evidence")
	}
}

func TestDecodeStructuredOutputAcceptsHistoricalCitation(t *testing.T) {
	// The citation validator rejects unknown evidence IDs; historical:1 must
	// be accepted when registered (the RAG prompt registers it).
	evidenceIDs := map[string]struct{}{"E1": {}, "historical:1": {}}
	raw := `{"summary":"s","analysis":"a","recommended_actions":[],"citations":[{"evidence_id":"historical:1","claim":"matches past image_pull_backoff"}]}`
	result, err := decodeStructuredOutput(raw, evidenceIDs)
	if err != nil {
		t.Fatalf("decode with historical citation: %v", err)
	}
	if len(result.Citations) != 1 || result.Citations[0].EvidenceID != "historical:1" {
		t.Fatalf("citations = %#v", result.Citations)
	}
}

func TestDecodeStructuredOutputRejectsUnknownHistory(t *testing.T) {
	evidenceIDs := map[string]struct{}{"E1": {}}
	raw := `{"summary":"s","analysis":"a","recommended_actions":[],"citations":[{"evidence_id":"historical:9","claim":"fabricated"}]}`
	if _, err := decodeStructuredOutput(raw, evidenceIDs); err == nil {
		t.Fatal("unregistered historical citation must be rejected")
	}
}
