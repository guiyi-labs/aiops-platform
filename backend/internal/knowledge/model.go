// Package knowledge implements the P1 RAG knowledge base: it distills
// resolved diagnosis records into searchable entries and lets the AI modules
// (aiexplain / aiinvestigator) retrieve verified historical cases before
// generating an answer.
//
// Phase 1 retrieval is a two-stage pipeline:
//  1. structured selection from PostgreSQL (rule_id + severity + resource
//     kind, newest first) — fast and deterministic;
//  2. optional LLM re-rank over the shortlist when the caller enables it.
//
// A knowledge outage never blocks diagnosis: every public entry point
// degrades to an empty result so callers fall back to the deterministic
// diagnosis chain unchanged.
package knowledge

import (
	"context"
	"time"
)

// Severity mirrors the diagnosis severity vocabulary.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

// Entry is one distilled knowledge record, sourced from a resolved diagnosis.
type Entry struct {
	// ID is the knowledge entry primary key.
	ID int64 `json:"id"`
	// SourceDiagnosisID references the resolved diagnosis record.
	SourceDiagnosisID int64  `json:"source_diagnosis_id"`
	RuleID            string `json:"rule_id"`
	Severity          string `json:"severity"`
	ResourceKind      string `json:"resource_kind"`
	ResourceNamespace string `json:"resource_namespace,omitempty"`
	ResourceName      string `json:"resource_name"`
	// Summary is a bounded description of the failure (first ~200 chars).
	Summary string `json:"summary"`
	// RootCauses are the confirmed root causes from the resolved record.
	RootCauses []string `json:"root_causes"`
	// Recommendations are the verified remediation actions.
	Recommendations []string `json:"recommendations"`
	// NotedAt is when the diagnosis was resolved (ranked newest first).
	NotedAt time.Time `json:"noted_at"`
	// Score is a retrieval score in [0,1]. Zero for the structured phase,
	// set by the re-ranker when used.
	Score float64 `json:"score,omitempty"`
}

// ListResponse is the bounded retrieval response.
type ListResponse struct {
	Items     []Entry `json:"items"`
	Total     int64   `json:"total"`
	Truncated bool    `json:"truncated,omitempty"`
}

// Filter bounds a knowledge query.
type Filter struct {
	RuleID       string
	Severity     string
	ResourceKind string
	// MinSeverity, when non-empty, retrieves entries of at least this
	// severity (info < warning < high < critical). Lower-severity noise is
	// excluded unless the caller asked for it explicitly.
	MinSeverity string
	// Limit bounds the structured shortlist; the re-ranker narrows it
	// further.
	Limit int
}

// MaxRecommendations caps the distilled recommendations per entry. Kept
// small so a prompt injection stays within the token budget.
const MaxRecommendations = 3

// SeverityRank orders severities for MinSeverity filtering.
var SeverityRank = map[string]int{
	string(SeverityInfo):     1,
	string(SeverityWarning):  2,
	string(SeverityHigh):     3,
	string(SeverityCritical): 4,
}

// Repository persists and queries knowledge entries.
type Repository interface {
	// InsertUpsert writes a distilled entry. When an identical dedup key
	// (rule_id, resource_kind, resource_name, noted_at) already exists the
	// newest entry wins (the incoming row replaces it).
	Insert(ctx context.Context, entry Entry) (Entry, error)
	// ListByFilter returns entries matching the filter, newest noted first.
	ListByFilter(ctx context.Context, filter Filter) (ListResponse, error)
	// Count returns the total entry count (used for observability and to
	// decide whether the shortlist is worth building).
	Count(ctx context.Context) (int64, error)
}
