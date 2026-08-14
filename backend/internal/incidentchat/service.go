package incidentchat

import (
	"context"
	"time"
)

// IncidentReader is the incident service interface needed by the chat
// service. The real incident.Service implements both methods.
type IncidentReader interface {
	Get(ctx context.Context, id int64) (IncidentSnapshot, []EvidenceItem, error)
}

// Service is the M112-2 incident AI chat application service. It is
// stateless: each request is independent, with the client holding
// conversation history.
type Service struct {
	enabled         bool
	provider        Provider
	summaryProvider SummaryProvider
	model           string
	reader          IncidentReader
	maxMessages     int
	semaphore       chan struct{}
}

// ServiceConfig configures the incident AI chat service.
type ServiceConfig struct {
	Enabled               bool
	Model                 string
	MaxConcurrentRequests int
	MaxMessages           int
}

// NewService constructs a Chat + Summary service. When provider or
// summaryProvider is nil the NopProvider deterministic fallback is used.
func NewService(cfg ServiceConfig, reader IncidentReader, provider Provider, summaryProvider SummaryProvider) *Service {
	concurrency := cfg.MaxConcurrentRequests
	if concurrency < 1 {
		concurrency = 1
	}
	maxMsgs := cfg.MaxMessages
	if maxMsgs < 1 {
		maxMsgs = 20
	}
	model := cfg.Model
	if model == "" {
		model = "responses-compatible"
	}
	if provider == nil {
		provider = NopProvider{}
	}
	if summaryProvider == nil {
		summaryProvider = NopSummaryProvider{}
	}
	return &Service{
		enabled:         cfg.Enabled,
		provider:        provider,
		summaryProvider: summaryProvider,
		model:           model,
		reader:          reader,
		maxMessages:     maxMsgs,
		semaphore:       make(chan struct{}, concurrency),
	}
}

// Chat processes one conversational turn in incident context. It is
// idempotent (no side effects, no database writes). The response carries
// the resource-context contract block for M112-M114 unified shape.
func (s *Service) Chat(ctx context.Context, incidentID int64, messages []ChatMessage, observedAt time.Time) (ChatResponse, error) {
	if len(messages) == 0 {
		return ChatResponse{}, ErrNoMessages
	}
	if len(messages) > s.maxMessages {
		return ChatResponse{}, ErrHistoryTooLong
	}
	last := messages[len(messages)-1]
	if last.Role != "user" {
		return ChatResponse{}, ErrLastMessageNotUser
	}

	// Resolve incident snapshot + evidence from the reader.
	incSnap, evidenceItems, err := s.reader.Get(ctx, incidentID)
	if err != nil {
		return ChatResponse{}, err
	}

	// Build authorized evidence set + prompt.
	question := last.Content
	history := messages[:len(messages)-1]
	prompt := BuildPrompt(incSnap, evidenceItems, history, question)

	// Determine mode and get answer.
	var result ProviderResult
	var mode string
	var failClosed bool

	if !s.enabled || s.provider == nil {
		// Deterministic fallback: use NopProvider.
		result, err = NopProvider{}.Generate(ctx, prompt)
		mode = "deterministic"
		failClosed = false
	} else {
		select {
		case s.semaphore <- struct{}{}:
			defer func() { <-s.semaphore }()
		default:
			return ChatResponse{}, ErrBusy
		}
		result, err = s.provider.Generate(ctx, prompt)
		mode = "ai"
	}

	if err != nil {
		// Provider failure → deterministic fallback, not 500.
		detResult, fallbackErr := NopProvider{}.Generate(ctx, prompt)
		if fallbackErr != nil {
			return ChatResponse{}, fallbackErr
		}
		result = detResult
		mode = "deterministic"
		failClosed = true
	} else {
		// Validate citations against authorized set (fail-closed).
		if validateErr := ValidateResult(result, prompt.AuthorizedEvidence); validateErr != nil {
			detResult, _ := NopProvider{}.Generate(ctx, prompt)
			detResult.Answer = "引用校验失败，已降级为确定性摘要。原因：" + validateErr.Error()
			result = detResult
			mode = "deterministic"
			failClosed = true
		}
	}

	// Build the resource-context contract for this response.
	rc := ResourceContext{
		Scope: ScopeInfo{
			ClusterID:  incSnap.ClusterID,
			Namespace:  incSnap.Namespace,
			Kind:       incSnap.Kind,
			Name:       incSnap.Name,
			SourceType: incSnap.SourceType,
		},
		ObservedAt: observedAt,
		Source:     "incident_ai_chat",
		Freshness: FreshnessInfo{
			AgeSeconds: 0,
			AsOf:       observedAt.UTC().Format(time.RFC3339),
		},
		EmptySample: EmptySampleInfo{
			Count:    0,
			Bounded:  true,
			Semantic: "fail_closed",
		},
	}

	return ChatResponse{
		IncidentID:      incSnap.ID,
		ResourceContext: rc,
		Mode:            mode,
		Answer:          result.Answer,
		NextChecks:      result.NextChecks,
		Citations:       result.Citations,
		Provider:        result.Provider,
		Model:           result.Model,
		InputTokens:     result.InputTokens,
		OutputTokens:    result.OutputTokens,
		FailClosed:      failClosed,
	}, nil
}

// Summarize generates a cited incident summary with a deterministic stage
// gate. The stage gate decides whether the AI provider is eligible. When
// AI is not eligible or fails, a deterministic summary is returned with
// StageGatePassed=false. The response always carries the resource-context
// contract block.
func (s *Service) Summarize(ctx context.Context, incidentID int64, observedAt time.Time) (SummaryResponse, error) {
	incSnap, evidenceItems, err := s.reader.Get(ctx, incidentID)
	if err != nil {
		return SummaryResponse{}, err
	}

	// Deterministic stage gate.
	hasEvidence := len(evidenceItems) > 0
	gatePass, gateReason := SummaryStageGate(hasEvidence, s.enabled)

	prompt := BuildSummaryPrompt(incSnap, evidenceItems)

	// Build the resource-context contract (always present in the response).
	rc := ResourceContext{
		Scope: ScopeInfo{
			ClusterID:  incSnap.ClusterID,
			Namespace:  incSnap.Namespace,
			Kind:       incSnap.Kind,
			Name:       incSnap.Name,
			SourceType: incSnap.SourceType,
		},
		ObservedAt: observedAt,
		Source:     "incident_summary",
		Freshness: FreshnessInfo{
			AgeSeconds: 0,
			AsOf:       observedAt.UTC().Format(time.RFC3339),
		},
		EmptySample: EmptySampleInfo{
			Count:    0,
			Bounded:  true,
			Semantic: "fail_closed",
		},
	}

	if !gatePass {
		// Stage gate failed → deterministic summary.
		result, err := NopSummaryProvider{}.GenerateSummary(ctx, prompt)
		if err != nil {
			return SummaryResponse{}, err
		}
		return SummaryResponse{
			IncidentID:         incSnap.ID,
			ResourceContext:    rc,
			Mode:               "deterministic",
			RootCauseCandidate: result.RootCauseCandidate,
			Impact:             result.Impact,
			EvidenceSummary:    result.EvidenceSummary,
			NextSteps:          result.NextSteps,
			Citations:          result.Citations,
			Provider:           "nop",
			Model:              "nop-1.0",
			FailClosed:         false,
			StageGatePassed:    false,
			StageGateReason:    gateReason,
		}, nil
	}

	// Gate passed → try AI provider.
	select {
	case s.semaphore <- struct{}{}:
		defer func() { <-s.semaphore }()
	default:
		return SummaryResponse{}, ErrBusy
	}

	result, err := s.summaryProvider.GenerateSummary(ctx, prompt)
	if err != nil {
		// Provider failure → deterministic fallback, not 500.
		detResult, _ := NopSummaryProvider{}.GenerateSummary(ctx, prompt)
		return SummaryResponse{
			IncidentID:         incSnap.ID,
			ResourceContext:    rc,
			Mode:               "deterministic",
			RootCauseCandidate: detResult.RootCauseCandidate,
			Impact:             detResult.Impact,
			EvidenceSummary:    detResult.EvidenceSummary,
			NextSteps:          detResult.NextSteps,
			Citations:          detResult.Citations,
			Provider:           "nop",
			Model:              "nop-1.0",
			FailClosed:         true,
			StageGatePassed:    true,
			StageGateReason:    gateReason,
		}, nil
	}

	// Validate citations against authorized set (fail-closed).
	if validateErr := ValidateSummaryResult(result, prompt.AuthorizedEvidence); validateErr != nil {
		detResult, _ := NopSummaryProvider{}.GenerateSummary(ctx, prompt)
		detResult.Impact = "引用校验失败，已降级为确定性摘要。原因：" + validateErr.Error()
		return SummaryResponse{
			IncidentID:         incSnap.ID,
			ResourceContext:    rc,
			Mode:               "deterministic",
			RootCauseCandidate: detResult.RootCauseCandidate,
			Impact:             detResult.Impact,
			EvidenceSummary:    detResult.EvidenceSummary,
			NextSteps:          detResult.NextSteps,
			Citations:          detResult.Citations,
			Provider:           "nop",
			Model:              "nop-1.0",
			FailClosed:         true,
			StageGatePassed:    true,
			StageGateReason:    gateReason,
		}, nil
	}

	return SummaryResponse{
		IncidentID:         incSnap.ID,
		ResourceContext:    rc,
		Mode:               "ai",
		RootCauseCandidate: result.RootCauseCandidate,
		Impact:             result.Impact,
		EvidenceSummary:    result.EvidenceSummary,
		NextSteps:          result.NextSteps,
		Citations:          result.Citations,
		Provider:           "ai",
		Model:              s.model,
		InputTokens:        0,
		OutputTokens:       0,
		FailClosed:         false,
		StageGatePassed:    true,
		StageGateReason:    gateReason,
	}, nil
}