package aiexplain

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"k8s-aiops.local/backend/internal/diagnosis"
	"k8s-aiops.local/backend/internal/knowledge"
)

type diagnosisReader interface {
	Get(context.Context, int64) (diagnosis.Record, error)
}

// knowledgeRetriever is the narrow retrieval contract used by Generate.
// It keeps aiexplain independent of the concrete retriever implementation.
type knowledgeRetriever interface {
	Retrieve(ctx context.Context, query knowledge.Query, reranker knowledge.Reranker) ([]knowledge.Entry, error)
}

type Service struct {
	enabled          bool
	diagnoses        diagnosisReader
	provider         Provider
	repository       Repository
	knowledge        knowledgeRetriever
	providerName     string
	model            string
	dailyTokenBudget int
	maxOutputTokens  int
	reservationTTL   time.Duration
	semaphore        chan struct{}
}

type ServiceConfig struct {
	Enabled               bool
	ProviderName          string
	Model                 string
	DailyTokenBudget      int
	MaxConcurrentRequests int
	MaxOutputTokens       int
	ReservationTTL        time.Duration
}

func NewService(config ServiceConfig, diagnoses diagnosisReader, provider Provider, repository Repository) *Service {
	concurrency := config.MaxConcurrentRequests
	if concurrency < 1 {
		concurrency = 1
	}
	return &Service{enabled: config.Enabled, diagnoses: diagnoses, provider: provider, repository: repository, providerName: config.ProviderName, model: config.Model, dailyTokenBudget: config.DailyTokenBudget, maxOutputTokens: config.MaxOutputTokens, reservationTTL: config.ReservationTTL, semaphore: make(chan struct{}, concurrency)}
}

func (s *Service) Generate(ctx context.Context, diagnosisID int64, actor ActorRef) (Explanation, error) {
	if !s.enabled || s.provider == nil {
		return Explanation{}, ErrDisabled
	}
	select {
	case s.semaphore <- struct{}{}:
		defer func() { <-s.semaphore }()
	default:
		return Explanation{}, ErrBusy
	}
	record, err := s.diagnoses.Get(ctx, diagnosisID)
	if err != nil {
		return Explanation{}, err
	}
	prompt := BuildPrompt(record)
	// RAG: retrieve verified historical cases for this diagnosis context.
	// A failed knowledge lookup must never block explanation — degrade to
	// the legacy prompt silently.
	if s.knowledge != nil {
		var history []knowledge.Entry
		if entries, retrErr := s.knowledge.Retrieve(ctx, knowledge.Query{
			RuleID:       record.RuleID,
			Severity:     record.Severity,
			ResourceKind: record.Resource.Kind,
			SummaryHint:  record.Summary,
		}, nil); retrErr == nil {
			history = entries
		}
		prompt = BuildPromptWithHistory(record, history)
	}
	if len(prompt.EvidenceIDs) == 0 {
		return Explanation{}, ErrNoEvidence
	}
	reservationID, err := newReservationID()
	if err != nil {
		return Explanation{}, err
	}
	reservedTokens := estimateInputTokens(prompt) + s.maxOutputTokens
	if err := s.repository.Reserve(ctx, Reservation{ID: reservationID, DiagnosisID: diagnosisID, ReservedTokens: reservedTokens, ExpiresAt: time.Now().UTC().Add(s.reservationTTL)}, s.dailyTokenBudget); err != nil {
		return Explanation{}, err
	}
	defer s.releaseReservation(reservationID)
	result, err := s.provider.Generate(ctx, prompt)
	if err != nil {
		return Explanation{}, err
	}
	item := Explanation{DiagnosisID: diagnosisID, Actor: actor, Provider: result.Provider, Model: result.Model, ProviderResponseID: result.ProviderResponseID, Summary: result.Summary, Analysis: result.Analysis, RecommendedActions: result.RecommendedActions, Citations: result.Citations, InputTokens: result.InputTokens, OutputTokens: result.OutputTokens}
	if err := s.repository.Save(ctx, &item); err != nil {
		return Explanation{}, err
	}
	return item, nil
}

// WithKnowledgeRetriever enables RAG: resolved historical cases are injected
// into the explanation prompt. Removing/omitting it keeps the legacy
// explanation path (nil-safe).
func (s *Service) WithKnowledgeRetriever(retriever knowledgeRetriever) *Service {
	s.knowledge = retriever
	return s
}

func (s *Service) Status(ctx context.Context) (RuntimeStatus, error) {
	usage, err := s.repository.Usage(ctx)
	if err != nil {
		return RuntimeStatus{}, err
	}
	status := RuntimeStatus{Enabled: s.enabled, Provider: s.providerName, Model: s.model, MaxConcurrentRequests: cap(s.semaphore), ActiveRequests: len(s.semaphore), MaxOutputTokens: s.maxOutputTokens, DailyTokenBudget: s.dailyTokenBudget, UsedTokensToday: usage.UsedTokensToday, ReservedTokens: usage.ReservedTokens, ExplanationCount: usage.ExplanationCount, LastSuccessAt: usage.LastSuccessAt}
	if s.dailyTokenBudget > 0 {
		remaining := max(0, s.dailyTokenBudget-usage.UsedTokensToday-usage.ReservedTokens)
		status.RemainingTokens = &remaining
		status.Available = s.enabled && remaining > 0 && len(s.semaphore) < cap(s.semaphore)
	} else {
		status.Available = s.enabled && len(s.semaphore) < cap(s.semaphore)
	}
	return status, nil
}

func estimateInputTokens(prompt Prompt) int {
	bytes := len(prompt.System) + len(prompt.Input)
	return max(1, (bytes+3)/4)
}

func newReservationID() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

func (s *Service) releaseReservation(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = s.repository.Release(ctx, id)
}

func (s *Service) List(ctx context.Context, diagnosisID, actorID int64) ([]Explanation, error) {
	if _, err := s.diagnoses.Get(ctx, diagnosisID); err != nil {
		return nil, err
	}
	return s.repository.List(ctx, diagnosisID, actorID)
}

func (s *Service) AddFeedback(ctx context.Context, explanationID int64, actor ActorRef, verdict, comment string) (FeedbackResult, error) {
	if !validFeedbackVerdict(verdict) || len([]rune(comment)) > 1000 {
		return FeedbackResult{}, ErrInvalidFeedback
	}
	return s.repository.AddFeedback(ctx, explanationID, actor, verdict, comment)
}

func (s *Service) Quality(ctx context.Context) (QualitySummary, error) {
	return s.repository.Quality(ctx)
}

// Coverage returns the M112-4 explanation coverage dashboard snapshot.
func (s *Service) Coverage(ctx context.Context) (CoverageSummary, error) {
	return s.repository.Coverage(ctx)
}

func validFeedbackVerdict(verdict string) bool {
	return verdict == "helpful" || verdict == "partially_helpful" || verdict == "not_helpful"
}
