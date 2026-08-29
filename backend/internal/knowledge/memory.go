package knowledge

import (
	"context"
)

// InMemoryRepository is a pure-Go Repository implementation backed by a
// slice. It serves three consumers: unit tests (retriever / ingest logic),
// offline benchmarking (`cmd/aiopsbench retrieval`), and any caller that
// needs the knowledge pipeline without PostgreSQL. It is intentionally
// minimal and deterministic: Insert deduplicates on
// (rule_id, resource_kind, resource_name, first root cause) exactly like the
// production Gorm repository, and ListByFilter applies the same structured
// selection semantics (equality filters plus MinSeverity rank, newest first).
type InMemoryRepository struct {
	entries []Entry
}

// NewInMemoryRepository builds an empty in-memory repository.
func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{}
}

// Insert stores one entry, deduplicating on the same natural key as the
// production repository (existing entries are updated in place).
func (m *InMemoryRepository) Insert(ctx context.Context, entry Entry) (Entry, error) {
	dedupRule := ""
	if len(entry.RootCauses) > 0 {
		dedupRule = entry.RootCauses[0]
	}
	for i := range m.entries {
		e := &m.entries[i]
		existingDedupRule := ""
		if len(e.RootCauses) > 0 {
			existingDedupRule = e.RootCauses[0]
		}
		if e.RuleID == entry.RuleID &&
			e.ResourceKind == entry.ResourceKind &&
			e.ResourceName == entry.ResourceName &&
			existingDedupRule == dedupRule {
			e.SourceDiagnosisID = entry.SourceDiagnosisID
			e.Severity = entry.Severity
			e.Summary = entry.Summary
			e.RootCauses = entry.RootCauses
			e.Recommendations = entry.Recommendations
			e.NotedAt = entry.NotedAt
			return *e, nil
		}
	}
	entry.ID = int64(len(m.entries) + 1)
	m.entries = append(m.entries, entry)
	return entry, nil
}

// ListByFilter applies the structured selection semantics: equality filters
// on RuleID / Severity / ResourceKind plus the MinSeverity rank floor,
// newest entries first, bounded by filter.Limit.
func (m *InMemoryRepository) ListByFilter(ctx context.Context, filter Filter) (ListResponse, error) {
	var items []Entry
	min := SeverityRank[filter.MinSeverity]
	for _, e := range m.entries {
		if filter.RuleID != "" && e.RuleID != filter.RuleID {
			continue
		}
		if filter.Severity != "" && e.Severity != filter.Severity {
			continue
		}
		if filter.Severity == "" && filter.MinSeverity != "" && SeverityRank[e.Severity] < min {
			continue
		}
		if filter.ResourceKind != "" && e.ResourceKind != filter.ResourceKind {
			continue
		}
		items = append(items, e)
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 10
	}
	// Newest first — mirrors the production B-tree ordering.
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if items[j].NotedAt.After(items[i].NotedAt) {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
	if len(items) > limit {
		items = items[:limit]
	}
	return ListResponse{Items: items, Total: int64(len(items)), Truncated: len(items) == limit}, nil
}

// Count returns the number of stored entries.
func (m *InMemoryRepository) Count(ctx context.Context) (int64, error) {
	return int64(len(m.entries)), nil
}

// NopRepository is a no-op Repository used by route-contract tests and as a
// safe nil replacement. Every call returns an empty result without error, so
// callers that depend on a Repository always succeed regardless of whether the
// knowledge layer is wired up.
type NopRepository struct{}

// Insert is a no-op that returns the input unchanged.
func (NopRepository) Insert(ctx context.Context, entry Entry) (Entry, error) { return entry, nil }

// ListByFilter returns an empty response.
func (NopRepository) ListByFilter(ctx context.Context, filter Filter) (ListResponse, error) {
	return ListResponse{}, nil
}

// Count returns zero.
func (NopRepository) Count(ctx context.Context) (int64, error) { return 0, nil }

// Compile-time assertion that both repositories satisfy the interface.
var (
	_ Repository = (*InMemoryRepository)(nil)
	_ Repository = NopRepository{}
)
