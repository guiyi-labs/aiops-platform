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
	enabled       bool
	provider      Provider
	reader        IncidentReader
	maxMessages   int
	semaphore     chan struct{}
}

// ServiceConfig configures the incident AI chat service.
type ServiceConfig struct {
	Enabled               bool
	MaxConcurrentRequests int
	MaxMessages           int
}

// NewService constructs a chat Service. provider defaults to NopProvider
// when nil (deterministic fallback mode).
func NewService(cfg ServiceConfig, reader IncidentReader, provider Provider) *Service {
	concurrency := cfg.MaxConcurrentRequests
	if concurrency < 1 {
		concurrency = 1
	}
	maxMsgs := cfg.MaxMessages
	if maxMsgs < 1 {
		maxMsgs = 20
	}
	if provider == nil {
		provider = NopProvider{}
	}
	return &Service{
		enabled:     cfg.Enabled,
		provider:    provider,
		reader:      reader,
		maxMessages: maxMsgs,
		semaphore:   make(chan struct{}, concurrency),
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