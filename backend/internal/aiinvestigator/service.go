package aiinvestigator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// CaseReader reads correlation cases for the investigator. The investigator
// stays independent of the correlation package so it can be tested with
// fixtures.
type CaseReader interface {
	GetCase(ctx context.Context, caseID int64) (CaseContext, error)
	// EligibleActionCodes returns the M42 ActionCandidate codes for the case.
	// The investigator uses this to validate runbook eligibility.
	EligibleActionCodes(ctx context.Context, caseID int64) (map[string]bool, error)
}

// NopCaseReader returns empty context. Used when the investigator is in
// query-only mode.
type NopCaseReader struct{}

func (NopCaseReader) GetCase(context.Context, int64) (CaseContext, error) {
	return CaseContext{}, ErrCaseNotFound
}
func (NopCaseReader) EligibleActionCodes(context.Context, int64) (map[string]bool, error) {
	return nil, nil
}

// ErrCaseNotFound is returned when the case does not exist.
var ErrCaseNotFound = errors.New("correlation case not found")

// ErrDisabled is returned when the investigator is disabled (no provider).
var ErrDisabled = errors.New("ai investigator is disabled")

// Service is the M43 AI investigator application service. It is the only
// writer to ai_investigations. HTTP handlers translate requests into service
// calls; they never bypass the service.
type Service struct {
	enabled  bool
	provider Provider
	repo     Repository
	reader   CaseReader
	now      func() time.Time
}

// ServiceOption configures a Service at construction.
type ServiceOption func(*Service)

// WithNow overrides the clock (tests).
func WithNow(now func() time.Time) ServiceOption {
	return func(s *Service) { s.now = now }
}

// NewService constructs a Service. repository must be non-nil. The provider
// defaults to NopProvider when nil (query-only mode). The reader defaults to
// NopCaseReader when nil. The service is enabled by default; the NopProvider
// returns a provider error so callers see a clear failure rather than a
// silent disabled state.
func NewService(repo Repository, provider Provider, reader CaseReader, opts ...ServiceOption) *Service {
	if provider == nil {
		provider = NopProvider{}
	}
	if reader == nil {
		reader = NopCaseReader{}
	}
	s := &Service{
		enabled:  true,
		provider: provider,
		repo:     repo,
		reader:   reader,
		now:      time.Now,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Investigate generates a cited investigation for one correlation case.
// The investigation is a read-only advisory: it never modifies the case,
// diagnosis or alert. On provider failure, budget exhaustion, invalid output
// or citation rejection, a failed investigation is persisted with
// failure_reason set so deterministic investigation remains available.
func (s *Service) Investigate(ctx context.Context, caseID int64, actor ActorRef) (Investigation, error) {
	if !s.enabled || s.provider == nil {
		return Investigation{}, ErrDisabled
	}
	caseCtx, err := s.reader.GetCase(ctx, caseID)
	if err != nil {
		return Investigation{}, err
	}
	eligibleCodes, err := s.reader.EligibleActionCodes(ctx, caseID)
	if err != nil {
		return Investigation{}, err
	}
	if eligibleCodes == nil {
		eligibleCodes = map[string]bool{}
	}
	eligibleRunbooks := EligibleRunbooks(eligibleCodes)

	prompt, err := BuildPrompt(caseCtx, eligibleRunbooks)
	if err != nil {
		return Investigation{}, err
	}

	now := s.now().UTC()
	investigationKey := computeInvestigationKey(caseID, caseCtx)

	inv := Investigation{
		CaseID:              caseID,
		InvestigationKey:    investigationKey,
		InvestigatorVersion: InvestigatorVersion,
		Actor:               actor,
		Status:              InvestigationStatusCompleted,
		CreatedAt:           now,
	}

	result, err := s.provider.Generate(ctx, prompt)
	if err != nil {
		// Persist a failed investigation so the operator sees the failure.
		inv.Status = InvestigationStatusFailed
		inv.FailureReason = "provider_error"
		inv.Provider = "unknown"
		inv.Model = "unknown"
		inv.Summary = ""
		inv.Impact = ""
		_ = s.repo.Save(ctx, &inv)
		return inv, err
	}

	validated, err := ValidateProviderResult(result, prompt, eligibleCodes)
	if err != nil {
		inv.Status = InvestigationStatusFailed
		inv.FailureReason = "citation_rejected"
		inv.Provider = result.Provider
		inv.Model = result.Model
		inv.ProviderResponseID = result.ProviderResponseID
		inv.Summary = result.Summary
		inv.Impact = result.Impact
		inv.Hypotheses = result.Hypotheses
		inv.Citations = result.Citations
		inv.InputTokens = result.InputTokens
		inv.OutputTokens = result.OutputTokens
		_ = s.repo.Save(ctx, &inv)
		return inv, err
	}

	inv.Provider = validated.Provider
	inv.Model = validated.Model
	inv.ProviderResponseID = validated.ProviderResponseID
	inv.Summary = validated.Summary
	inv.Impact = validated.Impact
	inv.Hypotheses = validated.Hypotheses
	inv.RecommendedRunbookID = validated.RecommendedRunbookID
	inv.Uncertainties = validated.Uncertainties
	inv.Citations = validated.Citations
	inv.InputTokens = validated.InputTokens
	inv.OutputTokens = validated.OutputTokens

	if err := s.repo.Save(ctx, &inv); err != nil {
		return Investigation{}, err
	}
	return inv, nil
}

// GetInvestigation returns one investigation by ID.
func (s *Service) GetInvestigation(ctx context.Context, id int64) (Investigation, error) {
	return s.repo.Get(ctx, id)
}

// ListByCase returns investigations for a case, newest first.
func (s *Service) ListByCase(ctx context.Context, caseID int64, limit int) (InvestigationListResponse, error) {
	items, total, err := s.repo.ListByCase(ctx, caseID, limit)
	if err != nil {
		return InvestigationListResponse{}, err
	}
	resp := InvestigationListResponse{Items: items, Total: total}
	if len(items) > 0 && int64(len(items)) < total {
		resp.Truncated = true
	}
	return resp, nil
}

// ListRunbooks returns the full runbook catalog.
func (s *Service) ListRunbooks() []RunbookDescriptor {
	return AllRunbooks()
}

// computeInvestigationKey returns the SHA-256 hex over (case_id +
// investigator_version + prompt_hash). Identical evidence + prompt + version
// produce identical keys.
func computeInvestigationKey(caseID int64, caseCtx CaseContext) string {
	promptHash := PromptHash(caseCtx)
	h := sha256.New()
	fmt.Fprintf(h, "case_id=%d\n", caseID)
	fmt.Fprintf(h, "investigator_version=%s\n", InvestigatorVersion)
	fmt.Fprintf(h, "prompt_hash=%s\n", promptHash)
	return hex.EncodeToString(h.Sum(nil))
}
