package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// GormRepository persists knowledge entries via GORM with raw SQL statements
// (matching the repository style used by diagnosis / aiexplain).
type GormRepository struct{ db *gorm.DB }

// NewGormRepository builds a GORM-backed knowledge repository.
func NewGormRepository(db *gorm.DB) *GormRepository {
	return &GormRepository{db: db}
}

// Insert writes a distilled entry. Dedup: when the same recurring defect
// (rule_id + resource_kind + resource_name + first root cause) already has
// an entry, the incoming resolved record replaces it — the newest verified
// outcome wins while a genuinely different failure still lands as its own
// entry.
func (r *GormRepository) Insert(ctx context.Context, entry Entry) (Entry, error) {
	rootCauses, err := json.Marshal(entry.RootCauses)
	if err != nil {
		return Entry{}, fmt.Errorf("marshal root_causes: %w", err)
	}
	recommendations, err := json.Marshal(entry.Recommendations)
	if err != nil {
		return Entry{}, fmt.Errorf("marshal recommendations: %w", err)
	}

	var existingID int64
	dedupRule := ""
	if len(entry.RootCauses) > 0 {
		dedupRule = entry.RootCauses[0]
	}
	if err := r.db.WithContext(ctx).Raw(`
		SELECT id FROM knowledge_entries
		WHERE rule_id = ? AND resource_kind = ? AND resource_name = ? AND root_causes->>0 = ?
		ORDER BY noted_at DESC LIMIT 1`,
		entry.RuleID, entry.ResourceKind, entry.ResourceName, dedupRule).Scan(&existingID).Error; err != nil {
		return Entry{}, fmt.Errorf("lookup knowledge dedup: %w", err)
	}

	var out idOnly
	if existingID > 0 {
		err = r.db.WithContext(ctx).Raw(`
			UPDATE knowledge_entries
			SET source_diagnosis_id = ?, severity = ?, summary = ?, root_causes = ?,
			    recommendations = ?, noted_at = ?, created_at = NOW()
			WHERE id = ? RETURNING id`,
			entry.SourceDiagnosisID, entry.Severity, entry.Summary, rootCauses,
			recommendations, entry.NotedAt, existingID).Scan(&out).Error
	} else {
		err = r.db.WithContext(ctx).Raw(`
			INSERT INTO knowledge_entries
				(source_diagnosis_id, rule_id, severity, resource_kind, resource_namespace,
				 resource_name, summary, root_causes, recommendations, noted_at, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW())
			RETURNING id`,
			entry.SourceDiagnosisID, entry.RuleID, entry.Severity, entry.ResourceKind,
			entry.ResourceNamespace, entry.ResourceName, entry.Summary, rootCauses,
			recommendations, entry.NotedAt).Scan(&out).Error
	}
	if err != nil {
		return Entry{}, fmt.Errorf("write knowledge entry: %w", err)
	}
	entry.ID = out.ID
	return entry, nil
}

// ListByFilter returns entries matching the filter, newest noted first.
// MinSeverity (when Severity is empty) keeps entries at or above the given
// severity: lower-severity noise is excluded unless explicitly requested.
func (r *GormRepository) ListByFilter(ctx context.Context, filter Filter) (ListResponse, error) {
	var rows []entryRow
	query := `SELECT id, source_diagnosis_id, rule_id, severity, resource_kind,
	                 resource_namespace, resource_name, summary, root_causes,
	                 recommendations, noted_at
	          FROM knowledge_entries WHERE 1 = 1`
	var args []any
	if filter.RuleID != "" {
		query += ` AND rule_id = ?`
		args = append(args, filter.RuleID)
	}
	if filter.Severity != "" {
		query += ` AND severity = ?`
		args = append(args, filter.Severity)
	} else if filter.MinSeverity != "" {
		min := SeverityRank[filter.MinSeverity]
		// Deterministic IN-list regardless of map iteration order, so the
		// generated SQL is stable and test assertions are not flaky.
		var allowed []string
		for _, severity := range []string{string(SeverityInfo), string(SeverityWarning), string(SeverityHigh), string(SeverityCritical)} {
			if SeverityRank[severity] >= min {
				allowed = append(allowed, "'"+severity+"'")
			}
		}
		query += ` AND severity IN (` + strings.Join(allowed, ", ") + `)`
	}
	if filter.ResourceKind != "" {
		query += ` AND resource_kind = ?`
		args = append(args, filter.ResourceKind)
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 10
	}
	query += ` ORDER BY noted_at DESC LIMIT ?`
	args = append(args, limit)
	if err := r.db.WithContext(ctx).Raw(query, args...).Scan(&rows).Error; err != nil {
		return ListResponse{}, fmt.Errorf("list knowledge entries: %w", err)
	}
	items := make([]Entry, 0, len(rows))
	for _, row := range rows {
		items = append(items, row.entry())
	}
	return ListResponse{Items: items, Total: int64(len(items)), Truncated: len(items) == limit}, nil
}

// Count returns the total entry count.
func (r *GormRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Raw(`SELECT COUNT(*) FROM knowledge_entries`).Scan(&count).Error; err != nil {
		return 0, fmt.Errorf("count knowledge entries: %w", err)
	}
	return count, nil
}

type idOnly struct{ ID int64 }

type entryRow struct {
	ID                int64
	SourceDiagnosisID int64
	RuleID            string
	Severity          string
	ResourceKind      string
	ResourceNamespace string
	ResourceName      string
	Summary           string
	RootCauses        []byte
	Recommendations   []byte
	NotedAt           time.Time
}

func (r entryRow) entry() Entry {
	var rootCauses []string
	var recommendations []string
	_ = json.Unmarshal(r.RootCauses, &rootCauses)
	_ = json.Unmarshal(r.Recommendations, &recommendations)
	return Entry{
		ID: r.ID, SourceDiagnosisID: r.SourceDiagnosisID, RuleID: r.RuleID,
		Severity: r.Severity, ResourceKind: r.ResourceKind,
		ResourceNamespace: r.ResourceNamespace, ResourceName: r.ResourceName,
		Summary: r.Summary, RootCauses: rootCauses,
		Recommendations: recommendations, NotedAt: r.NotedAt,
	}
}
