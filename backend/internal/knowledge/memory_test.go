package knowledge

import (
	"context"
)

// InMemoryRepository is a pure-Go test double for Repository.  It is
// intentionally minimal — enough to exercise the retriever / service
// logic without a real database.
type InMemoryRepository struct {
	entries []Entry
}

// NewInMemoryRepository builds an empty in-memory repository.
func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{}
}

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
	if len(items) > limit {
		items = items[:limit]
	}
	return ListResponse{Items: items, Total: int64(len(items)), Truncated: len(items) == limit}, nil
}

func (m *InMemoryRepository) Count(ctx context.Context) (int64, error) {
	return int64(len(m.entries)), nil
}
