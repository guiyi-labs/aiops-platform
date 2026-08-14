// Package incidentchat implements M112-2: conversational investigation in
// incident context. The conversation is stateless (client holds history);
// each request carries the bounded message history for context.
//
// Every factual claim cites an authorized evidence ID built from the
// incident snapshot and its evidence timeline. Citations not in the
// authorized set are rejected (fail-closed). When AI is disabled or the
// provider fails, a deterministic fallback produces an answer that cites
// only existing evidence and never fabricates root causes.
package incidentchat

import (
	"context"
	"errors"
	"time"
)

var (
	ErrDisabled = errors.New("incident AI chat is disabled")
	ErrBusy     = errors.New("incident AI chat concurrency limit reached")
	ErrNoMessages = errors.New("at least one user message is required")
	ErrHistoryTooLong = errors.New("message history exceeds maximum length")
	ErrLastMessageNotUser = errors.New("the last message must be from the user")
	ErrCitationRejected = errors.New("citation references unauthorized evidence")
)

// ChatMessage is one turn in the conversation history.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is the inbound request body for the incident AI chat.
type ChatRequest struct {
	Messages []ChatMessage `json:"messages"`
}

// Citation ties one factual claim to an authorized evidence ID.
type Citation struct {
	EvidenceID string `json:"evidence_id"`
	Claim      string `json:"claim"`
}

// ProviderResult is the raw provider output before validation.
type ProviderResult struct {
	Provider      string
	Model         string
	Answer        string
	NextChecks    []string
	Citations     []Citation
	InputTokens   int
	OutputTokens  int
}

// ChatResponse is the M112-2 chat response. It always carries the
// resource-context contract block (scope/observed_at/source/freshness/
// empty_sample) so the client knows what was observed.
type ChatResponse struct {
	IncidentID      int64                 `json:"incident_id"`
	ResourceContext ResourceContext       `json:"resource_context"`
	Mode            string                `json:"mode"` // "ai" or "deterministic"
	Answer          string                `json:"answer"`
	NextChecks      []string              `json:"next_checks,omitempty"`
	Citations       []Citation            `json:"citations"`
	Provider        string                `json:"provider"`
	Model           string                `json:"model"`
	InputTokens     int                   `json:"input_tokens"`
	OutputTokens    int                   `json:"output_tokens"`
	FailClosed      bool                  `json:"fail_closed"`
}

// ResourceContext is a local copy of the incident.ResourceContext contract
// to avoid importing the incident package from here. The handler populates
// this from the incident context cockpit builder in incident.ResourceContext.
type ResourceContext struct {
	Scope       ScopeInfo  `json:"scope"`
	ObservedAt  time.Time  `json:"observed_at"`
	Source      string     `json:"source"`
	Freshness   FreshnessInfo `json:"freshness"`
	EmptySample EmptySampleInfo `json:"empty_sample"`
}

type ScopeInfo struct {
	ClusterID  int64  `json:"cluster_id"`
	Namespace  string `json:"namespace,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Name       string `json:"name,omitempty"`
	SourceType string `json:"source_type,omitempty"`
}

type FreshnessInfo struct {
	AgeSeconds int64  `json:"age_seconds"`
	AsOf       string `json:"as_of"`
}

type EmptySampleInfo struct {
	Count    int    `json:"count"`
	Bounded  bool   `json:"bounded"`
	Semantic string `json:"semantic"`
}

// Provider is the AI provider for incident chat. Implementations send the
// prompt to the model and return structured JSON output.
type Provider interface {
	Generate(ctx context.Context, prompt Prompt) (ProviderResult, error)
}

// Prompt is the typed prompt sent to the provider.
type Prompt struct {
	System string
	Input  string
	// AuthorizedEvidence is the set of evidence IDs the model may cite.
	AuthorizedEvidence map[string]struct{}
}
