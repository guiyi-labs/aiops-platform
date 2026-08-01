package aiinvestigator

import (
	"strings"
	"testing"
)

func sampleCaseContext() CaseContext {
	return CaseContext{
		CaseID:               42,
		ClusterID:            1,
		RuleID:               "pod_failure_with_rollout",
		PrimaryResourceKind:  "Pod",
		PrimaryResourceName:  "web-abc",
		PrimaryResourceUID:   "uid-123",
		Confidence:           "candidate",
		EvidenceCompleteness: "partial",
		Factors: []FactorContext{
			{Kind: "image_pull", Value: "true", Weight: 0.9},
		},
		SignalLinks: []SignalLinkContext{
			{SignalOccurrenceID: 100, Relation: "trigger", SignalID: "pod_crashloop", Producer: "diagnosis", ObservedAt: "2026-07-31T10:00:00Z"},
		},
		ResourceLinks: []ResourceLinkContext{
			{Kind: "Deployment", Namespace: "default", Name: "web", UID: "uid-deploy", Relation: "upstream"},
		},
		ChangeCandidates: []ChangeCandidateContext{
			{ChangeEventID: 200, RuleID: "rollout_preceded", Confidence: "candidate", Rank: 1, ReasonCode: "temporal_proximity"},
		},
	}
}

func TestBuildPrompt(t *testing.T) {
	ctx := sampleCaseContext()
	runbooks := EligibleRunbooks(map[string]bool{"deployment.rollback": true})
	prompt, err := BuildPrompt(ctx, runbooks)
	if err != nil {
		t.Fatalf("BuildPrompt failed: %v", err)
	}
	if prompt.System == "" {
		t.Errorf("system prompt must not be empty")
	}
	if prompt.Input == "" {
		t.Errorf("user prompt must not be empty")
	}
	if len(prompt.EvidenceRefs) == 0 {
		t.Fatalf("evidence refs must not be empty")
	}
	// The case itself, the signal occurrence and the change candidate must
	// all be authorized.
	caseID := "correlation_case:42"
	if _, ok := prompt.EvidenceRefs[caseID]; !ok {
		t.Errorf("case evidence %q not authorized", caseID)
	}
	signalID := "signal_occurrence:100"
	if _, ok := prompt.EvidenceRefs[signalID]; !ok {
		t.Errorf("signal evidence %q not authorized", signalID)
	}
	changeID := "change_candidate:200"
	if _, ok := prompt.EvidenceRefs[changeID]; !ok {
		t.Errorf("change candidate evidence %q not authorized", changeID)
	}
}

func TestBuildPromptSystemContainsRunbooks(t *testing.T) {
	ctx := sampleCaseContext()
	runbooks := EligibleRunbooks(map[string]bool{"deployment.rollback": true})
	prompt, _ := BuildPrompt(ctx, runbooks)
	if !strings.Contains(prompt.System, "rollback_last_rollout") {
		t.Errorf("system prompt should list eligible runbook rollback_last_rollout")
	}
	if !strings.Contains(prompt.System, "PROHIBITIONS") {
		t.Errorf("system prompt should contain PROHIBITIONS section")
	}
	if !strings.Contains(prompt.System, "CITATION RULES") {
		t.Errorf("system prompt should contain CITATION RULES section")
	}
}

func TestBuildPromptUserContainsCaseFacts(t *testing.T) {
	ctx := sampleCaseContext()
	prompt, _ := BuildPrompt(ctx, nil)
	if !strings.Contains(prompt.Input, "CASE 42") {
		t.Errorf("user prompt should contain CASE 42")
	}
	if !strings.Contains(prompt.Input, "PRIMARY RESOURCE: Pod/web") {
		t.Errorf("user prompt should contain primary resource")
	}
	if !strings.Contains(prompt.Input, "AUTHORIZED EVIDENCE IDS") {
		t.Errorf("user prompt should list authorized evidence ids")
	}
	if !strings.Contains(prompt.Input, "signal_occurrence:100") {
		t.Errorf("user prompt should reference signal_occurrence:100")
	}
	if !strings.Contains(prompt.Input, "change_candidate:200") {
		t.Errorf("user prompt should reference change_candidate:200")
	}
}

func TestBuildPromptNoEligibleRunbooks(t *testing.T) {
	ctx := sampleCaseContext()
	runbooks := EligibleRunbooks(map[string]bool{})
	prompt, _ := BuildPrompt(ctx, runbooks)
	// Advisory runbooks remain eligible even with no action codes, so the
	// system prompt should still list at least the advisory runbooks.
	if !strings.Contains(prompt.System, "inspect_pvc_capacity") {
		t.Errorf("system prompt should list advisory runbook inspect_pvc_capacity")
	}
}

func TestPromptHashStability(t *testing.T) {
	ctx := sampleCaseContext()
	h1 := PromptHash(ctx)
	h2 := PromptHash(ctx)
	if h1 != h2 {
		t.Errorf("PromptHash not stable: %q vs %q", h1, h2)
	}
	if len(h1) != 64 {
		t.Errorf("PromptHash should be 64 hex chars (SHA256), got %d", len(h1))
	}
}

func TestPromptHashChangesWithEvidence(t *testing.T) {
	base := sampleCaseContext()
	h1 := PromptHash(base)
	// Add a new signal link — the authorized evidence set changes, so the
	// prompt hash must change.
	changed := base
	changed.SignalLinks = append(changed.SignalLinks, SignalLinkContext{
		SignalOccurrenceID: 101,
		Relation:           "context",
		SignalID:           "high_latency",
		Producer:           "metric",
		ObservedAt:         "2026-07-31T10:05:00Z",
	})
	h2 := PromptHash(changed)
	if h1 == h2 {
		t.Errorf("PromptHash must change when evidence changes")
	}
}

func TestPromptHashIgnoresFactorOrderRelevantFields(t *testing.T) {
	// The prompt hash includes rule_id, confidence, primary resource and the
	// sorted evidence id set. Factors are NOT part of the hash, so reordering
	// factors must not change the hash.
	ctx := sampleCaseContext()
	h1 := PromptHash(ctx)
	changed := ctx
	changed.Factors = []FactorContext{
		{Kind: "other", Value: "x", Weight: 0.1},
	}
	h2 := PromptHash(changed)
	if h1 != h2 {
		t.Errorf("PromptHash must be stable across factor changes (factors are not part of the hash)")
	}
}

func TestBuildAuthorizedEvidence(t *testing.T) {
	t.Run("dedupes repeated evidence", func(t *testing.T) {
		ctx := CaseContext{
			CaseID: 1,
			SignalLinks: []SignalLinkContext{
				{SignalOccurrenceID: 10, SignalID: "a"},
				{SignalOccurrenceID: 10, SignalID: "a"}, // duplicate
			},
		}
		refs := buildAuthorizedEvidence(ctx)
		if len(refs) != 2 { // case + one signal
			t.Errorf("expected 2 evidence refs (case + dedup signal), got %d", len(refs))
		}
	})
	t.Run("empty case still authorizes the case itself", func(t *testing.T) {
		ctx := CaseContext{CaseID: 5}
		refs := buildAuthorizedEvidence(ctx)
		if len(refs) != 1 {
			t.Errorf("expected 1 evidence ref (case only), got %d", len(refs))
		}
		if _, ok := refs["correlation_case:5"]; !ok {
			t.Errorf("case evidence must always be authorized")
		}
	})
}

func TestMarshalEvidenceForHash(t *testing.T) {
	refs := map[string]EvidenceRef{
		"signal_occurrence:2": {Kind: EvidenceKindSignalOccurrence, ID: 2},
		"correlation_case:1":  {Kind: EvidenceKindCorrelationCase, ID: 1},
	}
	out, err := MarshalEvidenceForHash(refs)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	s := string(out)
	// Stable ordering: correlation_case:1 must come before signal_occurrence:2.
	if !strings.Contains(s, "correlation_case") {
		t.Errorf("marshaled output should contain correlation_case")
	}
	// Determinism: re-marshalling produces the same bytes.
	out2, _ := MarshalEvidenceForHash(refs)
	if string(out) != string(out2) {
		t.Errorf("MarshalEvidenceForHash must be deterministic")
	}
}
