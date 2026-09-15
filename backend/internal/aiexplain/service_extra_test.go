package aiexplain

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"k8s-aiops.local/backend/internal/diagnosis"
	"k8s-aiops.local/backend/internal/knowledge"
)

// capturingProvider records the prompt handed to the provider so the RAG
// prompt assembly can be asserted end to end.
type capturingProvider struct {
	result  ProviderResult
	err     error
	prompts []Prompt
}

func (p *capturingProvider) Generate(_ context.Context, prompt Prompt) (ProviderResult, error) {
	p.prompts = append(p.prompts, prompt)
	return p.result, p.err
}

// repositoryOverride embeds the shared stub and lets a single method be
// steered into its failure branch.
type repositoryOverride struct {
	repositoryStub
	saveErr  error
	usageErr error
}

func (r *repositoryOverride) Save(_ context.Context, item *Explanation) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	item.ID = 9
	r.saved = *item
	return nil
}

func (r *repositoryOverride) Usage(context.Context) (Usage, error) {
	if r.usageErr != nil {
		return Usage{}, r.usageErr
	}
	return r.usage, nil
}

// knowledgeStub implements the narrow knowledgeRetriever contract.
type knowledgeStub struct {
	entries []knowledge.Entry
	err     error
	queries []knowledge.Query
}

func (s *knowledgeStub) Retrieve(_ context.Context, query knowledge.Query, _ knowledge.Reranker) ([]knowledge.Entry, error) {
	s.queries = append(s.queries, query)
	return s.entries, s.err
}

func historyEntry() knowledge.Entry {
	return knowledge.Entry{
		ID: 1, RuleID: "crash_loop", Severity: "high",
		ResourceKind: "Deployment", ResourceName: "api",
		Summary: "past api crash", RootCauses: []string{"image_pull_backoff"},
		Recommendations: []string{"check registry"},
		NotedAt:         time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC),
	}
}

func ragDiagnosis() diagnosis.Record {
	return diagnosis.Record{
		RuleID:   "crash_loop",
		Severity: "high",
		Resource: diagnosis.ResourceRef{Kind: "Deployment", Namespace: "payments", Name: "api"},
		Summary:  "api is crash looping",
		Evidence: []diagnosis.Evidence{{Type: "event", Source: "event/test", Content: map[string]any{"reason": "Failed"}}},
	}
}

func TestServiceWithKnowledgeRetrieverEnablesRAG(t *testing.T) {
	retriever := &knowledgeStub{entries: []knowledge.Entry{historyEntry()}}
	provider := &capturingProvider{result: ProviderResult{Provider: "test", Model: "model", Summary: "summary", Analysis: "analysis", Citations: []Citation{{EvidenceID: "historical:1", Claim: "matches past incident"}}}}
	repository := &repositoryStub{}
	service := NewService(testServiceConfig(), diagnosisStub{record: ragDiagnosis()}, provider, repository)

	if got := service.WithKnowledgeRetriever(retriever); got != service {
		t.Fatal("WithKnowledgeRetriever() must return the receiver for chaining")
	}
	if service.knowledge != retriever {
		t.Fatalf("knowledge retriever = %#v, want the injected stub", service.knowledge)
	}

	item, err := service.Generate(context.Background(), 7, ActorRef{ID: 3, Name: "Operator"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if item.ID != 9 || repository.saved.DiagnosisID != 7 {
		t.Fatalf("unexpected persisted item %#v", item)
	}
	if len(provider.prompts) != 1 {
		t.Fatalf("provider prompts = %d, want 1", len(provider.prompts))
	}
	prompt := provider.prompts[0]
	if !strings.Contains(prompt.Input, "历史相似案例") || !strings.Contains(prompt.Input, "image_pull_backoff") {
		t.Fatalf("RAG context missing from prompt: %s", prompt.Input)
	}
	for _, id := range []string{"E1", "historical:1"} {
		if _, ok := prompt.EvidenceIDs[id]; !ok {
			t.Fatalf("evidence %q not registered: %#v", id, prompt.EvidenceIDs)
		}
	}
	if len(retriever.queries) != 1 {
		t.Fatalf("retriever calls = %d, want 1", len(retriever.queries))
	}
	query := retriever.queries[0]
	if query.RuleID != "crash_loop" || query.Severity != "high" || query.ResourceKind != "Deployment" || query.SummaryHint != "api is crash looping" {
		t.Fatalf("retrieval query = %#v", query)
	}
	if repository.reservations != 1 || repository.releases != 1 {
		t.Fatalf("reservations = %d releases = %d, want 1/1", repository.reservations, repository.releases)
	}
}

func TestServiceGenerateDegradesWhenRetrieverFails(t *testing.T) {
	retriever := &knowledgeStub{err: errors.New("vector store unavailable")}
	provider := &capturingProvider{result: ProviderResult{Provider: "test", Model: "model", Summary: "summary", Analysis: "analysis", Citations: []Citation{{EvidenceID: "E1", Claim: "claim"}}}}
	repository := &repositoryStub{}
	service := NewService(testServiceConfig(), diagnosisStub{record: ragDiagnosis()}, provider, repository).WithKnowledgeRetriever(retriever)

	item, err := service.Generate(context.Background(), 7, ActorRef{})
	if err != nil {
		t.Fatalf("Generate must degrade silently on retrieval failure, got %v", err)
	}
	if item.ID != 9 {
		t.Fatalf("item = %#v", item)
	}
	prompt := provider.prompts[0]
	if strings.Contains(prompt.Input, "历史相似案例") {
		t.Fatalf("failed retrieval must not inject history: %s", prompt.Input)
	}
	if _, ok := prompt.EvidenceIDs["historical:1"]; ok {
		t.Fatalf("failed retrieval must not register historical evidence: %#v", prompt.EvidenceIDs)
	}
	if _, ok := prompt.EvidenceIDs["E1"]; !ok {
		t.Fatalf("base evidence must survive: %#v", prompt.EvidenceIDs)
	}
}

func TestServiceGenerateRejectsDiagnosisWithoutEvidence(t *testing.T) {
	provider := &capturingProvider{}
	repository := &repositoryStub{}
	service := NewService(testServiceConfig(), diagnosisStub{record: diagnosis.Record{RuleID: "empty.rule"}}, provider, repository)

	item, err := service.Generate(context.Background(), 7, ActorRef{})
	if !errors.Is(err, ErrNoEvidence) {
		t.Fatalf("Generate() error = %v, want ErrNoEvidence", err)
	}
	if item.ID != 0 || len(provider.prompts) != 0 || repository.reservations != 0 {
		t.Fatalf("no provider call or reservation expected: item=%#v prompts=%d reservations=%d", item, len(provider.prompts), repository.reservations)
	}
}

func TestServiceGeneratePropagatesDiagnosisFailure(t *testing.T) {
	failure := errors.New("diagnosis unavailable")
	provider := &capturingProvider{}
	repository := &repositoryStub{}
	service := NewService(testServiceConfig(), diagnosisStub{err: failure}, provider, repository)

	if _, err := service.Generate(context.Background(), 7, ActorRef{}); !errors.Is(err, failure) {
		t.Fatalf("Generate() error = %v, want %v", err, failure)
	}
	if len(provider.prompts) != 0 || repository.reservations != 0 {
		t.Fatalf("provider prompts = %d reservations = %d, want 0/0", len(provider.prompts), repository.reservations)
	}
}

func TestServiceGenerateReleasesReservationWhenProviderFails(t *testing.T) {
	failure := errors.New("provider exploded")
	provider := &capturingProvider{err: failure}
	repository := &repositoryStub{}
	service := NewService(testServiceConfig(), diagnosisStub{record: testDiagnosis()}, provider, repository)

	item, err := service.Generate(context.Background(), 7, ActorRef{})
	if !errors.Is(err, failure) {
		t.Fatalf("Generate() error = %v, want %v", err, failure)
	}
	if item.ID != 0 {
		t.Fatalf("item = %#v, want zero value", item)
	}
	if repository.reservations != 1 || repository.releases != 1 {
		t.Fatalf("reservations = %d releases = %d, want 1/1", repository.reservations, repository.releases)
	}
}

func TestServiceGenerateReleasesReservationWhenSaveFails(t *testing.T) {
	failure := errors.New("persist failed")
	provider := &capturingProvider{result: ProviderResult{Provider: "test", Model: "model", Summary: "summary", Analysis: "analysis", Citations: []Citation{{EvidenceID: "E1", Claim: "claim"}}}}
	repository := &repositoryOverride{saveErr: failure}
	service := NewService(testServiceConfig(), diagnosisStub{record: testDiagnosis()}, provider, repository)

	if _, err := service.Generate(context.Background(), 7, ActorRef{}); !errors.Is(err, failure) {
		t.Fatalf("Generate() error = %v, want %v", err, failure)
	}
	if repository.reservations != 1 || repository.releases != 1 {
		t.Fatalf("reservations = %d releases = %d, want 1/1", repository.reservations, repository.releases)
	}
}

func TestServiceGeneratePropagatesReserveFailure(t *testing.T) {
	failure := errors.New("reserve unavailable")
	provider := &capturingProvider{}
	repository := &repositoryStub{reserveErr: failure}
	service := NewService(testServiceConfig(), diagnosisStub{record: testDiagnosis()}, provider, repository)

	if _, err := service.Generate(context.Background(), 7, ActorRef{}); !errors.Is(err, failure) {
		t.Fatalf("Generate() error = %v, want %v", err, failure)
	}
	if len(provider.prompts) != 0 {
		t.Fatalf("provider prompts = %d, want 0", len(provider.prompts))
	}
	if repository.releases != 0 {
		t.Fatalf("releases = %d, want 0 (nothing was reserved)", repository.releases)
	}
}

func TestNewServiceFloorsConcurrencyAtOne(t *testing.T) {
	config := testServiceConfig()
	config.MaxConcurrentRequests = 0
	service := NewService(config, diagnosisStub{}, &capturingProvider{}, &repositoryStub{})
	if cap(service.semaphore) != 1 {
		t.Fatalf("semaphore capacity = %d, want 1", cap(service.semaphore))
	}

	config.MaxConcurrentRequests = 4
	service = NewService(config, diagnosisStub{}, &capturingProvider{}, &repositoryStub{})
	if cap(service.semaphore) != 4 {
		t.Fatalf("semaphore capacity = %d, want 4", cap(service.semaphore))
	}
}

func TestServiceListPropagatesDiagnosisFailure(t *testing.T) {
	failure := errors.New("diagnosis missing")
	repository := &repositoryStub{}
	service := NewService(testServiceConfig(), diagnosisStub{err: failure}, &capturingProvider{}, repository)

	items, err := service.List(context.Background(), 7, 42)
	if !errors.Is(err, failure) || items != nil {
		t.Fatalf("List() items=%#v error=%v, want nil/%v", items, err, failure)
	}
	if repository.listActorID != 0 {
		t.Fatalf("repository must not be queried when the diagnosis lookup fails")
	}
}

func TestServiceStatusPropagatesUsageFailure(t *testing.T) {
	failure := errors.New("usage unavailable")
	repository := &repositoryOverride{usageErr: failure}
	service := NewService(testServiceConfig(), diagnosisStub{}, &capturingProvider{}, repository)

	if _, err := service.Status(context.Background()); !errors.Is(err, failure) {
		t.Fatalf("Status() error = %v, want %v", err, failure)
	}
}

func TestServiceStatusWithoutBudgetOmitsRemainingTokens(t *testing.T) {
	config := testServiceConfig()
	config.DailyTokenBudget = 0
	repository := &repositoryStub{usage: Usage{UsedTokensToday: 9999, ExplanationCount: 1}}
	service := NewService(config, diagnosisStub{}, &capturingProvider{}, repository)

	status, err := service.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.RemainingTokens != nil {
		t.Fatalf("remaining tokens = %v, want nil when no budget is configured", *status.RemainingTokens)
	}
	if !status.Available || !status.Enabled {
		t.Fatalf("status = %#v", status)
	}
	if status.MaxConcurrentRequests != 1 || status.ActiveRequests != 0 {
		t.Fatalf("concurrency snapshot = %#v", status)
	}
}

func TestServiceStatusMarksUnavailableWhenBudgetExhausted(t *testing.T) {
	repository := &repositoryStub{usage: Usage{UsedTokensToday: 9000, ReservedTokens: 1000}}
	service := NewService(testServiceConfig(), diagnosisStub{}, &capturingProvider{}, repository)

	status, err := service.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.RemainingTokens == nil || *status.RemainingTokens != 0 {
		t.Fatalf("remaining tokens = %v, want 0", status.RemainingTokens)
	}
	if status.Available {
		t.Fatalf("status must be unavailable once the budget is exhausted: %#v", status)
	}
}

func TestServiceStatusMarksUnavailableWhenDisabled(t *testing.T) {
	config := testServiceConfig()
	config.Enabled = false
	repository := &repositoryStub{}
	service := NewService(config, diagnosisStub{}, &capturingProvider{}, repository)

	status, err := service.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.Enabled || status.Available {
		t.Fatalf("disabled service must report enabled=false available=false: %#v", status)
	}
}
