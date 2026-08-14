package incidentchat

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// --- prompt tests ---

func TestBuildPrompt_AuthorizedEvidenceIds(t *testing.T) {
	inc := IncidentSnapshot{ID: 7, Title: "Pod NotReady", Severity: "critical", Status: "confirmed", ClusterID: 3, Kind: "Pod", Namespace: "default", Name: "web-0", SourceType: "diagnosis"}
	evidence := []EvidenceItem{
		{SourceType: "diagnosis", SourceRef: "diagnosis:42", Title: "pod.pending"},
		{SourceType: "alert", SourceRef: "alert:9", Title: "PodNotReady alert"},
	}
	prompt := BuildPrompt(inc, evidence, nil, "为什么 Pod 无法启动？")
	if _, ok := prompt.AuthorizedEvidence["incident:7"]; !ok {
		t.Fatal("incident evidence ID must be authorized")
	}
	if _, ok := prompt.AuthorizedEvidence["evidence:diagnosis:42"]; !ok {
		t.Fatal("diagnosis evidence ID must be authorized")
	}
	if _, ok := prompt.AuthorizedEvidence["evidence:alert:9"]; !ok {
		t.Fatal("alert evidence ID must be authorized")
	}
	if len(prompt.AuthorizedEvidence) != 3 {
		t.Fatalf("authorized evidence count = %d, want 3", len(prompt.AuthorizedEvidence))
	}
}

func TestBuildPrompt_IncludesHistoryAndQuestion(t *testing.T) {
	inc := IncidentSnapshot{ID: 1, Title: "OOM", Summary: "out of memory", ClusterID: 1}
	history := []ChatMessage{{Role: "user", Content: "是什么原因？"}, {Role: "assistant", Content: "可能是 OOM"}}
	prompt := BuildPrompt(inc, nil, history, "能确认吗？")
	if prompt.System == "" || prompt.Input == "" {
		t.Fatal("system and input must be non-empty")
	}
}

// --- ValidateResult tests ---

func TestValidateResult_CitationRejectedIfNotAuthorized(t *testing.T) {
	authorized := map[string]struct{}{"incident:7": {}, "evidence:diag:42": {}}
	result := ProviderResult{
		Answer: "test",
		Citations: []Citation{{EvidenceID: "evidence:diag:42", Claim: "diag exists"}, {EvidenceID: "evidence:unknown:1", Claim: "hack"}},
	}
	if err := ValidateResult(result, authorized); err != ErrCitationRejected {
		t.Fatalf("expected citation rejected, got %v", err)
	}
}

func TestValidateResult_AnswerEmptyRejected(t *testing.T) {
	authorized := map[string]struct{}{"incident:1": {}}
	result := ProviderResult{Citations: []Citation{{EvidenceID: "incident:1", Claim: "x"}}}
	if err := ValidateResult(result, authorized); err == nil || err.Error() != "invalid output: answer is empty" {
		t.Fatalf("expected empty answer error, got %v", err)
	}
}

func TestValidateResult_AllValid(t *testing.T) {
	authorized := map[string]struct{}{"incident:1": {}, "evidence:diag:42": {}}
	result := ProviderResult{
		Answer:     "OK",
		NextChecks: []string{"check node memory"},
		Citations:  []Citation{{EvidenceID: "evidence:diag:42", Claim: "diagnosed"}},
	}
	if err := ValidateResult(result, authorized); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

func TestValidateResult_NoCitationsRejected(t *testing.T) {
	authorized := map[string]struct{}{"incident:1": {}}
	result := ProviderResult{Answer: "x", NextChecks: []string{}}
	if err := ValidateResult(result, authorized); err == nil || err.Error() != "invalid output: no citations" {
		t.Fatalf("expected no citations error, got %v", err)
	}
}

// --- NopProvider deterministic fallback ---

func TestNopProvider_CitesIncidentFromAuthorizedSet(t *testing.T) {
	prompt := Prompt{AuthorizedEvidence: map[string]struct{}{"incident:7": {}, "evidence:diag:42": {}}}
	result, err := NopProvider{}.Generate(context.Background(), prompt)
	if err != nil {
		t.Fatalf("nop error: %v", err)
	}
	if result.Provider != "nop" {
		t.Fatalf("provider = %q", result.Provider)
	}
	if len(result.Citations) != 1 {
		t.Fatalf("nop must produce exactly 1 citation, got %d", len(result.Citations))
	}
	cited := result.Citations[0].EvidenceID
	if cited != "incident:7" {
		t.Fatalf("nop should prefer incident evidence, got %q", cited)
	}
	if err := ValidateResult(result, prompt.AuthorizedEvidence); err != nil {
		t.Fatalf("nop result must pass validation, got %v", err)
	}
}

func TestNopProvider_FallbackToAnyEvidenceIfNoIncident(t *testing.T) {
	prompt := Prompt{AuthorizedEvidence: map[string]struct{}{"evidence:diag:1": {}}}
	result, err := NopProvider{}.Generate(context.Background(), prompt)
	if err != nil {
		t.Fatalf("nop error: %v", err)
	}
	if result.Citations[0].EvidenceID != "evidence:diag:1" {
		t.Fatalf("unexpected citation: %q", result.Citations[0].EvidenceID)
	}
}

// --- Service deterministic fallback ---

type testIncidentReader struct {
	snap IncidentSnapshot
	items []EvidenceItem
}

func (r *testIncidentReader) Get(_ context.Context, _ int64) (IncidentSnapshot, []EvidenceItem, error) {
	return r.snap, r.items, nil
}

func TestChat_DeterministicFallbackWhenDisabled(t *testing.T) {
	reader := &testIncidentReader{
		snap: IncidentSnapshot{ID: 1, Number: "INC-000001", Title: "OOM", Severity: "critical", Status: "open", ClusterID: 5, Kind: "Pod"},
		items: []EvidenceItem{{SourceType: "diagnosis", SourceRef: "diag:42", Title: "oom_killed"}},
	}
	svc := NewService(ServiceConfig{Enabled: false}, reader, nil)
	resp, err := svc.Chat(context.Background(), 1, []ChatMessage{{Role: "user", Content: "什么问题？"}}, time.Now().UTC())
	if err != nil {
		t.Fatalf("chat error: %v", err)
	}
	if resp.Mode != "deterministic" {
		t.Fatalf("mode = %q", resp.Mode)
	}
	if resp.Answer == "" {
		t.Fatal("answer must not be empty")
	}
	if len(resp.Citations) == 0 {
		t.Fatal("citations must not be empty")
	}
	// ResourceContext must carry the incident scope.
	if resp.ResourceContext.Scope.ClusterID != 5 || resp.ResourceContext.Scope.Kind != "Pod" {
		t.Fatalf("resource context scope mismatch: %+v", resp.ResourceContext.Scope)
	}
}

func TestChat_RejectsEmptyMessages(t *testing.T) {
	reader := &testIncidentReader{snap: IncidentSnapshot{ID: 1}}
	svc := NewService(ServiceConfig{}, reader, nil)
	_, err := svc.Chat(context.Background(), 1, nil, time.Now().UTC())
	if err != ErrNoMessages {
		t.Fatalf("expected ErrNoMessages, got %v", err)
	}
}

func TestChat_RejectsHistoryTooLong(t *testing.T) {
	reader := &testIncidentReader{snap: IncidentSnapshot{ID: 1}}
	svc := NewService(ServiceConfig{MaxMessages: 5}, reader, nil)
	msgs := make([]ChatMessage, 6)
	for i := range msgs {
		msgs[i] = ChatMessage{Role: "user", Content: "x"}
	}
	_, err := svc.Chat(context.Background(), 1, msgs, time.Now().UTC())
	if err != ErrHistoryTooLong {
		t.Fatalf("expected ErrHistoryTooLong, got %v", err)
	}
}

func TestChat_LastMessageMustBeUser(t *testing.T) {
	reader := &testIncidentReader{snap: IncidentSnapshot{ID: 1}}
	svc := NewService(ServiceConfig{}, reader, nil)
	msgs := []ChatMessage{{Role: "user", Content: "x"}, {Role: "assistant", Content: "y"}}
	_, err := svc.Chat(context.Background(), 1, msgs, time.Now().UTC())
	if err != ErrLastMessageNotUser {
		t.Fatalf("expected ErrLastMessageNotUser, got %v", err)
	}
}

// --- PromptHash stability ---

func TestPromptHash_StableAndDeterministic(t *testing.T) {
	items := []EvidenceItem{{SourceRef: "diag:1"}, {SourceRef: "alert:9"}}
	h1 := PromptHash(7, items)
	h2 := PromptHash(7, items)
	if h1 != h2 || len(h1) != 64 {
		t.Fatalf("hash mismatch: %q vs %q (len %d)", h1, h2, len(h1))
	}
}

func TestPromptHash_ChangesWhenEvidenceChanges(t *testing.T) {
	a := PromptHash(1, []EvidenceItem{{SourceRef: "a"}})
	b := PromptHash(1, []EvidenceItem{{SourceRef: "a"}, {SourceRef: "b"}})
	if a == b {
		t.Fatal("hash must change when evidence changes")
	}
}

// --- DecodeProviderJSON ---

func TestDecodeProviderJSON_Valid(t *testing.T) {
	raw := `{"answer":"ok","next_checks":["check"],"citations":[{"evidence_id":"incident:1","claim":"x"}]}`
	result, err := DecodeProviderJSON(raw)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if result.Answer != "ok" || len(result.NextChecks) != 1 || len(result.Citations) != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestDecodeProviderJSON_MissingFieldsGetDefaults(t *testing.T) {
	raw := `{"answer":"answer only"}`
	result, err := DecodeProviderJSON(raw)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(result.NextChecks) != 0 || len(result.Citations) != 0 {
		t.Fatalf("expected empty slices, got: next_checks=%v, citations=%v", result.NextChecks, result.Citations)
	}
}

func TestDecodeProviderJSON_MalformedRejected(t *testing.T) {
	_, err := DecodeProviderJSON(`not json`)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

// --- Error wrapping ---

func TestErrorTypes(t *testing.T) {
	if fmt.Sprintf("%v", ErrNoMessages) != "at least one user message is required" {
		t.Fatalf("ErrNoMessages: %v", ErrNoMessages)
	}
}