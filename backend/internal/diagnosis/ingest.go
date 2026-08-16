package diagnosis

import (
	"context"
	"time"
)

// KnowledgeIngester is the narrow contract the diagnosis service uses to
// push a resolved record into the knowledge base. Defined here (not in the
// knowledge package) so diagnosis stays free of any knowledge dependency:
// the concrete implementation lives in knowledge and is wired at startup.
//
// The interface is nil-safe: a service without an ingester behaves exactly
// as before, preserving the deterministic diagnosis chain.
type KnowledgeIngester interface {
	// IngestResolved stores a distilled knowledge entry for the record.
	// Errors are logged by the caller but must never fail the transition.
	IngestResolved(ctx context.Context, input KnowledgeEntryInput) error
}

// singleton nil-guard helper used by Transition.
func isResolvableToKnowledge(record Record) bool {
	return record.Status == "resolved" &&
		(len(record.RootCauses) > 0 || len(record.Recommendations) > 0) &&
		record.ResolvedAt != nil
}

// notedAtForKnowledge returns the time to use as the knowledge "noted_at".
// The resolved timestamp is authoritative; fall back to now defensively.
func notedAtForKnowledge(record Record, now func() time.Time) time.Time {
	if record.ResolvedAt != nil {
		return *record.ResolvedAt
	}
	return now()
}

// KnowledgeEntryInput is the distilled shape handed to the ingester.
// Keeping it here (rather than leaking knowledge.Entry) means diagnosis does
// not import the knowledge package at all.
type KnowledgeEntryInput struct {
	SourceDiagnosisID int64
	RuleID            string
	Severity          string
	ResourceKind      string
	ResourceNamespace string
	ResourceName      string
	Summary           string
	RootCauses        []string
	Recommendations   []string
	NotedAt           time.Time
}

// IngestResolvedIfEligible calls the ingester only for resolved records with
// distilled content. Never fails the caller: errors are swallowed.
func IngestResolvedIfEligible(ctx context.Context, ingester KnowledgeIngester, record Record, now func() time.Time) {
	if ingester == nil || !isResolvableToKnowledge(record) {
		return
	}
	input := KnowledgeEntryInput{
		SourceDiagnosisID: record.ID,
		RuleID:            record.RuleID,
		Severity:          record.Severity,
		ResourceKind:      record.Resource.Kind,
		ResourceNamespace: record.Resource.Namespace,
		ResourceName:      record.Resource.Name,
		Summary:           truncateSummary(record.Summary),
		RootCauses:        record.RootCauses,
		Recommendations:   record.Recommendations,
		NotedAt:           notedAtForKnowledge(record, now),
	}
	_ = ingester.IngestResolved(ctx, input)
}

// truncateSummary bounds the distilled summary to a compact size.
func truncateSummary(summary string) string {
	const maxSummaryChars = 200
	runes := []rune(summary)
	if len(runes) <= maxSummaryChars {
		return summary
	}
	return string(runes[:maxSummaryChars]) + "…"
}
