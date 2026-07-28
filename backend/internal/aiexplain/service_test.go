package aiexplain

import (
	"context"
	"errors"
	"testing"
	"time"

	"k8s-aiops.local/backend/internal/diagnosis"
)

type diagnosisStub struct {
	record diagnosis.Record
	err    error
}

func (s diagnosisStub) Get(context.Context, int64) (diagnosis.Record, error) { return s.record, s.err }

type providerStub struct {
	result  ProviderResult
	err     error
	calls   int
	wait    <-chan struct{}
	started chan struct{}
}

func (s *providerStub) Generate(context.Context, Prompt) (ProviderResult, error) {
	s.calls++
	if s.started != nil {
		close(s.started)
	}
	if s.wait != nil {
		<-s.wait
	}
	return s.result, s.err
}

type repositoryStub struct {
	saved          Explanation
	items          []Explanation
	usage          Usage
	reserveErr     error
	reservations   int
	releases       int
	feedbackResult FeedbackResult
	feedbackErr    error
	quality        QualitySummary
	listActorID    int64
}

func (s *repositoryStub) Save(_ context.Context, item *Explanation) error {
	item.ID = 9
	s.saved = *item
	return nil
}
func (s *repositoryStub) List(_ context.Context, _ int64, actorID int64) ([]Explanation, error) {
	s.listActorID = actorID
	return s.items, nil
}
func (s *repositoryStub) AddFeedback(context.Context, int64, ActorRef, string, string) (FeedbackResult, error) {
	return s.feedbackResult, s.feedbackErr
}
func (s *repositoryStub) Quality(context.Context) (QualitySummary, error) { return s.quality, nil }
func (s *repositoryStub) Usage(context.Context) (Usage, error)            { return s.usage, nil }
func (s *repositoryStub) Reserve(context.Context, Reservation, int) error {
	s.reservations++
	return s.reserveErr
}
func (s *repositoryStub) Release(context.Context, string) error { s.releases++; return nil }

func testServiceConfig() ServiceConfig {
	return ServiceConfig{Enabled: true, ProviderName: "test", Model: "model", DailyTokenBudget: 10000, MaxConcurrentRequests: 1, MaxOutputTokens: 800, ReservationTTL: time.Minute}
}

func testDiagnosis() diagnosis.Record {
	return diagnosis.Record{Evidence: []diagnosis.Evidence{{Type: "event", Source: "event/test", Content: map[string]any{"reason": "Failed"}}}}
}

func TestServiceGeneratesAndPersistsExplanation(t *testing.T) {
	provider := &providerStub{result: ProviderResult{Provider: "test", Model: "model", Summary: "summary", Analysis: "analysis", Citations: []Citation{{EvidenceID: "E1", Claim: "claim"}}}}
	repository := &repositoryStub{}
	service := NewService(testServiceConfig(), diagnosisStub{record: testDiagnosis()}, provider, repository)
	item, err := service.Generate(context.Background(), 7, ActorRef{ID: 3, Name: "Operator"})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if item.ID != 9 || repository.saved.DiagnosisID != 7 || repository.saved.Actor.Name != "Operator" || provider.calls != 1 || repository.reservations != 1 || repository.releases != 1 {
		t.Fatalf("unexpected state %#v", repository)
	}
}

func TestServiceDisabledDoesNotReadDiagnosis(t *testing.T) {
	config := testServiceConfig()
	config.Enabled = false
	service := NewService(config, diagnosisStub{err: errors.New("should not be called")}, nil, &repositoryStub{})
	_, err := service.Generate(context.Background(), 1, ActorRef{})
	if !errors.Is(err, ErrDisabled) {
		t.Fatalf("Generate() error = %v, want ErrDisabled", err)
	}
}

func TestServiceRejectsBudgetBeforeProviderCall(t *testing.T) {
	provider := &providerStub{}
	repository := &repositoryStub{reserveErr: ErrBudgetExceeded}
	service := NewService(testServiceConfig(), diagnosisStub{record: testDiagnosis()}, provider, repository)
	_, err := service.Generate(context.Background(), 1, ActorRef{})
	if !errors.Is(err, ErrBudgetExceeded) || provider.calls != 0 {
		t.Fatalf("Generate() error=%v calls=%d", err, provider.calls)
	}
}

func TestServiceRejectsConcurrentGeneration(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	provider := &providerStub{wait: release, started: started, result: ProviderResult{Summary: "summary", Analysis: "analysis"}}
	service := NewService(testServiceConfig(), diagnosisStub{record: testDiagnosis()}, provider, &repositoryStub{})
	done := make(chan error, 1)
	go func() { _, err := service.Generate(context.Background(), 1, ActorRef{}); done <- err }()
	<-started
	_, err := service.Generate(context.Background(), 1, ActorRef{})
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("Generate() error=%v, want ErrBusy", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("first Generate() error=%v", err)
	}
}

func TestServiceStatusReportsBudget(t *testing.T) {
	repository := &repositoryStub{usage: Usage{UsedTokensToday: 1200, ReservedTokens: 300, ExplanationCount: 2}}
	service := NewService(testServiceConfig(), diagnosisStub{}, &providerStub{}, repository)
	status, err := service.Status(context.Background())
	if err != nil || status.RemainingTokens == nil || *status.RemainingTokens != 8500 || !status.Available || status.ExplanationCount != 2 {
		t.Fatalf("Status()=%#v err=%v", status, err)
	}
}

func TestServiceAddsValidatedFeedback(t *testing.T) {
	repository := &repositoryStub{feedbackResult: FeedbackResult{Feedback: Feedback{ID: 4, Verdict: "helpful"}, Summary: FeedbackSummary{Total: 1, Helpful: 1, HelpfulRate: 1}}}
	service := NewService(testServiceConfig(), diagnosisStub{}, &providerStub{}, repository)
	result, err := service.AddFeedback(context.Background(), 8, ActorRef{ID: 3, Name: "Viewer"}, "helpful", "clear evidence")
	if err != nil || result.Feedback.ID != 4 || result.Summary.HelpfulRate != 1 {
		t.Fatalf("AddFeedback()=%#v err=%v", result, err)
	}
	if _, err := service.AddFeedback(context.Background(), 8, ActorRef{ID: 3}, "accurate", ""); !errors.Is(err, ErrInvalidFeedback) {
		t.Fatalf("AddFeedback() error=%v, want ErrInvalidFeedback", err)
	}
}

func TestServiceListsPersonalFeedbackAndQuality(t *testing.T) {
	repository := &repositoryStub{items: []Explanation{{ID: 2}}, quality: QualitySummary{TotalFeedback: 3, Helpful: 2, HelpfulRate: 2.0 / 3.0}}
	service := NewService(testServiceConfig(), diagnosisStub{record: testDiagnosis()}, &providerStub{}, repository)
	items, err := service.List(context.Background(), 7, 42)
	if err != nil || len(items) != 1 || repository.listActorID != 42 {
		t.Fatalf("List()=%#v actor=%d err=%v", items, repository.listActorID, err)
	}
	quality, err := service.Quality(context.Background())
	if err != nil || quality.TotalFeedback != 3 || quality.HelpfulRate <= 0 {
		t.Fatalf("Quality()=%#v err=%v", quality, err)
	}
}
