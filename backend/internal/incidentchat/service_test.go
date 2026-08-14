package incidentchat

import (
	"context"
	"fmt"
	"strings"
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
	svc := NewService(ServiceConfig{Enabled: false}, reader, nil, nil)
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
	svc := NewService(ServiceConfig{}, reader, nil, nil)
	_, err := svc.Chat(context.Background(), 1, nil, time.Now().UTC())
	if err != ErrNoMessages {
		t.Fatalf("expected ErrNoMessages, got %v", err)
	}
}

func TestChat_RejectsHistoryTooLong(t *testing.T) {
	reader := &testIncidentReader{snap: IncidentSnapshot{ID: 1}}
	svc := NewService(ServiceConfig{MaxMessages: 5}, reader, nil, nil)
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
	svc := NewService(ServiceConfig{}, reader, nil, nil)
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

// --- Summary: stage gate + deterministic fallback ---

func TestSummaryStageGate_Rules(t *testing.T) {
	cases := []struct {
		name        string
		hasEvidence bool
		aiEnabled   bool
		pass        bool
		reason      string
	}{
		{"all good", true, true, true, "ok"},
		{"no evidence blocks AI", false, true, false, "no_evidence"},
		{"ai disabled blocks AI", true, false, false, "ai_disabled"},
		{"both blockers", false, false, false, "no_evidence"},
	}
	for _, tc := range cases {
		pass, reason := SummaryStageGate(tc.hasEvidence, tc.aiEnabled)
		if pass != tc.pass || reason != tc.reason {
			t.Fatalf("%s: got (%v, %q), want (%v, %q)", tc.name, pass, reason, tc.pass, tc.reason)
		}
	}
}

func TestSummarize_DeterministicWhenGateFailsNoEvidence(t *testing.T) {
	reader := &testIncidentReader{snap: IncidentSnapshot{ID: 3, Title: "empty incident", ClusterID: 2}}
	svc := NewService(ServiceConfig{Enabled: true}, reader, nil, nil)
	resp, err := svc.Summarize(context.Background(), 3, time.Now().UTC())
	if err != nil {
		t.Fatalf("summarize error: %v", err)
	}
	if resp.StageGatePassed {
		t.Fatal("stage gate must fail without evidence")
	}
	if resp.StageGateReason != "no_evidence" {
		t.Fatalf("gate reason = %q", resp.StageGateReason)
	}
	if resp.Mode != "deterministic" {
		t.Fatalf("mode = %q, want deterministic", resp.Mode)
	}
	if resp.RootCauseCandidate == "" || resp.EvidenceSummary == "" || resp.Impact == "" {
		t.Fatal("deterministic summary must fill all fields")
	}
}

func TestSummarize_DeterministicWhenAIDisabled(t *testing.T) {
	reader := &testIncidentReader{
		snap:  IncidentSnapshot{ID: 4, Title: "OOM", ClusterID: 2},
		items: []EvidenceItem{{SourceType: "diagnosis", SourceRef: "diag:42", Title: "oom_killed"}},
	}
	svc := NewService(ServiceConfig{Enabled: false}, reader, nil, nil)
	resp, err := svc.Summarize(context.Background(), 4, time.Now().UTC())
	if err != nil {
		t.Fatalf("summarize error: %v", err)
	}
	if resp.StageGatePassed {
		t.Fatal("stage gate must fail when AI disabled")
	}
	if resp.StageGateReason != "ai_disabled" {
		t.Fatalf("gate reason = %q", resp.StageGateReason)
	}
	if len(resp.Citations) == 0 {
		t.Fatal("deterministic summary must cite evidence")
	}
}

// failingSummaryProvider simulates a provider outage.
type failingSummaryProvider struct{}

func (failingSummaryProvider) GenerateSummary(context.Context, Prompt) (summaryProviderResult, error) {
	return summaryProviderResult{}, fmt.Errorf("%w: simulated outage", ErrDisabled)
}

func TestSummarize_ProviderFailureFallsBackDeterministic(t *testing.T) {
	reader := &testIncidentReader{
		snap:  IncidentSnapshot{ID: 5, Title: "OOM", ClusterID: 2},
		items: []EvidenceItem{{SourceType: "diagnosis", SourceRef: "diag:42", Title: "oom_killed"}},
	}
	svc := NewService(ServiceConfig{Enabled: true}, reader, nil, failingSummaryProvider{})
	resp, err := svc.Summarize(context.Background(), 5, time.Now().UTC())
	if err != nil {
		t.Fatalf("summarize error: %v", err)
	}
	if resp.Mode != "deterministic" {
		t.Fatalf("mode = %q, want deterministic on provider failure", resp.Mode)
	}
	if !resp.FailClosed {
		t.Fatal("fail_closed must be true on provider failure")
	}
	if !resp.StageGatePassed {
		t.Fatal("stage gate passed but provider failed; count as gate-pass fallback")
	}
}

// nopSummaryProvider362 is a deterministic valid provider for the AI path.
type nopSummaryProvider362 struct{}

func (nopSummaryProvider362) GenerateSummary(_ context.Context, prompt Prompt) (summaryProviderResult, error) {
	var cited string
	for id := range prompt.AuthorizedEvidence {
		if strings.HasPrefix(id, "incident:") {
			cited = id
			break
		}
	}
	return summaryProviderResult{
		RootCauseCandidate: "候选：OOM kill",
		Impact:             "Pod 重启",
		EvidenceSummary:    "证据支撑",
		NextSteps:          []string{"检查 node 内存"},
		Citations:          []Citation{{EvidenceID: cited, Claim: "incident"}},
	}, nil
}

func TestSummarize_AIModeWhenProviderSucceeds(t *testing.T) {
	reader := &testIncidentReader{
		snap:  IncidentSnapshot{ID: 6, Title: "OOM", ClusterID: 2},
		items: []EvidenceItem{{SourceType: "diagnosis", SourceRef: "diag:42", Title: "oom_killed"}},
	}
	svc := NewService(ServiceConfig{Enabled: true, Model: "test-model"}, reader, nil, nopSummaryProvider362{})
	resp, err := svc.Summarize(context.Background(), 6, time.Now().UTC())
	if err != nil {
		t.Fatalf("summarize error: %v", err)
	}
	if resp.Mode != "ai" {
		t.Fatalf("mode = %q, want ai", resp.Mode)
	}
	if !resp.StageGatePassed {
		t.Fatal("stage gate must pass")
	}
	if resp.Model != "test-model" {
		t.Fatalf("model = %q, want test-model", resp.Model)
	}
	if len(resp.Citations) == 0 {
		t.Fatal("AI summary must cite evidence")
	}
}

// unauthorizedSummaryProvider returns a citation outside the authorized set.
type unauthorizedSummaryProvider struct{}

func (unauthorizedSummaryProvider) GenerateSummary(_ context.Context, _ Prompt) (summaryProviderResult, error) {
	return summaryProviderResult{
		RootCauseCandidate: "根因",
		Impact:             "影响",
		EvidenceSummary:    "摘要",
		NextSteps:          []string{"x"},
		Citations:          []Citation{{EvidenceID: "evidence:outside:1", Claim: "hack"}},
	}, nil
}

func TestSummarize_UnauthorizedCitationFailsClosed(t *testing.T) {
	reader := &testIncidentReader{
		snap:  IncidentSnapshot{ID: 7, Title: "OOM", ClusterID: 2},
		items: []EvidenceItem{{SourceType: "diagnosis", SourceRef: "diag:42", Title: "oom_killed"}},
	}
	svc := NewService(ServiceConfig{Enabled: true}, reader, nil, unauthorizedSummaryProvider{})
	resp, err := svc.Summarize(context.Background(), 7, time.Now().UTC())
	if err != nil {
		t.Fatalf("summarize error: %v", err)
	}
	if resp.Mode != "deterministic" {
		t.Fatalf("mode = %q, want deterministic on citation rejection", resp.Mode)
	}
	if !resp.FailClosed {
		t.Fatal("fail_closed must be true on citation rejection")
	}
}

func TestBuildSummaryPrompt_AuthorizedSetIncludesAllEvidence(t *testing.T) {
	inc := IncidentSnapshot{ID: 9, Title: "x", ClusterID: 1}
	items := []EvidenceItem{{SourceRef: "diag:1"}, {SourceRef: "alert:5"}}
	prompt := BuildSummaryPrompt(inc, items)
	if len(prompt.AuthorizedEvidence) != 3 {
		t.Fatalf("authorized count = %d, want 3", len(prompt.AuthorizedEvidence))
	}
	if _, ok := prompt.AuthorizedEvidence["evidence:alert:5"]; !ok {
		t.Fatal("alert evidence must be authorized")
	}
}

func TestDecodeSummaryProviderJSON_Valid(t *testing.T) {
	raw := `{"root_cause_candidate":"rc","impact":"impact","evidence_summary":"es","next_steps":["a"],"citations":[{"evidence_id":"incident:1","claim":"x"}]}`
	result, err := DecodeSummaryProviderJSON(raw)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if result.RootCauseCandidate != "rc" || len(result.NextSteps) != 1 || len(result.Citations) != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestValidateSummaryResult_EmptyRootCauseRejected(t *testing.T) {
	authorized := map[string]struct{}{"incident:1": {}}
	result := summaryProviderResult{
		RootCauseCandidate: "",
		Impact:             "impact",
		EvidenceSummary:    "es",
		Citations:          []Citation{{EvidenceID: "incident:1", Claim: "x"}},
	}
	if err := ValidateSummaryResult(result, authorized); err == nil || err.Error() != "invalid output: root_cause_candidate is empty" {
		t.Fatalf("expected root cause empty error, got %v", err)
	}
}