package aiexplain

import (
	"errors"
	"time"
)

var (
	ErrDisabled            = errors.New("AI explanation is disabled")
	ErrNoEvidence          = errors.New("diagnosis has no evidence for AI explanation")
	ErrBudgetExceeded      = errors.New("AI daily token budget exceeded")
	ErrBusy                = errors.New("AI explanation concurrency limit reached")
	ErrProviderFailure     = errors.New("AI provider request failed")
	ErrInvalidOutput       = errors.New("AI provider returned invalid structured output")
	ErrInvalidFeedback     = errors.New("invalid AI explanation feedback")
	ErrFeedbackExists      = errors.New("AI explanation feedback already exists for this user")
	ErrExplanationNotFound = errors.New("AI explanation does not exist")
)

type ActorRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type Citation struct {
	EvidenceID string `json:"evidence_id"`
	Claim      string `json:"claim"`
}

type RecommendedAction struct {
	Action      string   `json:"action"`
	Priority    string   `json:"priority"`
	EvidenceIDs []string `json:"evidence_ids"`
}

type Explanation struct {
	ID                 int64               `json:"id"`
	DiagnosisID        int64               `json:"diagnosis_id"`
	Actor              ActorRef            `json:"actor"`
	Provider           string              `json:"provider"`
	Model              string              `json:"model"`
	ProviderResponseID string              `json:"provider_response_id,omitempty"`
	Summary            string              `json:"summary"`
	Analysis           string              `json:"analysis"`
	RecommendedActions []RecommendedAction `json:"recommended_actions"`
	Citations          []Citation          `json:"citations"`
	InputTokens        int                 `json:"input_tokens"`
	OutputTokens       int                 `json:"output_tokens"`
	FeedbackSummary    FeedbackSummary     `json:"feedback_summary"`
	MyFeedback         *Feedback           `json:"my_feedback,omitempty"`
	CreatedAt          time.Time           `json:"created_at"`
}

type Feedback struct {
	ID            int64     `json:"id"`
	ExplanationID int64     `json:"explanation_id"`
	Actor         ActorRef  `json:"actor"`
	Verdict       string    `json:"verdict"`
	Comment       string    `json:"comment"`
	CreatedAt     time.Time `json:"created_at"`
}

type FeedbackSummary struct {
	Total            int     `json:"total"`
	Helpful          int     `json:"helpful"`
	PartiallyHelpful int     `json:"partially_helpful"`
	NotHelpful       int     `json:"not_helpful"`
	HelpfulRate      float64 `json:"helpful_rate"`
}

type FeedbackResult struct {
	Feedback Feedback        `json:"feedback"`
	Summary  FeedbackSummary `json:"summary"`
}

type ModelQualitySummary struct {
	Model            string  `json:"model"`
	TotalFeedback    int     `json:"total_feedback"`
	Helpful          int     `json:"helpful"`
	PartiallyHelpful int     `json:"partially_helpful"`
	NotHelpful       int     `json:"not_helpful"`
	HelpfulRate      float64 `json:"helpful_rate"`
}

type QualitySummary struct {
	TotalFeedback            int                   `json:"total_feedback"`
	Helpful                  int                   `json:"helpful"`
	PartiallyHelpful         int                   `json:"partially_helpful"`
	NotHelpful               int                   `json:"not_helpful"`
	HelpfulRate              float64               `json:"helpful_rate"`
	ExplanationsWithFeedback int                   `json:"explanations_with_feedback"`
	Contributors             int                   `json:"contributors"`
	ByModel                  []ModelQualitySummary `json:"by_model"`
}

// CoverageSummary is the M112-4 explanation coverage dashboard snapshot.
// All fields are derived from the ai_explanations table (no join with
// external tables required). The endpoint is read-only and idempotent.
type CoverageSummary struct {
	TotalExplanations       int            `json:"total_explanations"`
	ExplainedDiagnoses      int            `json:"explained_diagnoses"`
	WithCitations           int            `json:"with_citations"`
	CitationRate            float64        `json:"citation_rate"`
	DeterministicCount      int            `json:"deterministic_count"`
	DeterministicRate       float64        `json:"deterministic_rate"`
	Quality                 QualitySummary `json:"quality"`
	WindowNote              string         `json:"window_note"`
}

type Prompt struct {
	System      string
	Input       string
	EvidenceIDs map[string]struct{}
}

type ProviderResult struct {
	Provider           string
	Model              string
	ProviderResponseID string
	Summary            string
	Analysis           string
	RecommendedActions []RecommendedAction
	Citations          []Citation
	InputTokens        int
	OutputTokens       int
}

type Usage struct {
	UsedTokensToday  int
	ReservedTokens   int
	ExplanationCount int
	LastSuccessAt    *time.Time
}

type Reservation struct {
	ID             string
	DiagnosisID    int64
	ReservedTokens int
	ExpiresAt      time.Time
}

type RuntimeStatus struct {
	Enabled               bool       `json:"enabled"`
	Available             bool       `json:"available"`
	Provider              string     `json:"provider"`
	Model                 string     `json:"model"`
	MaxConcurrentRequests int        `json:"max_concurrent_requests"`
	ActiveRequests        int        `json:"active_requests"`
	MaxOutputTokens       int        `json:"max_output_tokens"`
	DailyTokenBudget      int        `json:"daily_token_budget"`
	UsedTokensToday       int        `json:"used_tokens_today"`
	ReservedTokens        int        `json:"reserved_tokens"`
	RemainingTokens       *int       `json:"remaining_tokens,omitempty"`
	ExplanationCount      int        `json:"explanation_count_today"`
	LastSuccessAt         *time.Time `json:"last_success_at,omitempty"`
}
