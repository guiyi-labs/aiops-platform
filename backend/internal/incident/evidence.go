package incident

import (
	"context"
	"strings"
)

// EvidenceField is one labeled source-specific detail on an evidence block
// (for example "规则" / "Rule", "证据条数", "首次触发").
type EvidenceField struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// EvidenceItem is one structured evidence block on the incident detail
// "evidence timeline". It aggregates the source (diagnosis / finding /
// alert / inspection / signal) behind an incident so operators can reach the
// original evidence in at most a few interactions.
type EvidenceItem struct {
	SourceType string          `json:"source_type"`
	SourceRef  string          `json:"source_ref"`
	Title      string          `json:"title"`
	Summary    string          `json:"summary"`
	Severity   string          `json:"severity"`
	Resource   ResourceRef     `json:"resource"`
	ObservedAt string          `json:"observed_at"`
	Fields     []EvidenceField `json:"fields,omitempty"`
	DeepLink   string          `json:"deep_link"`
}

// EvidenceResolver resolves an incident source into the structured evidence
// block shown on the incident detail evidence timeline. A nil resolver or one
// returning ErrInvalidSource falls back to the incident snapshot fields.
type EvidenceResolver interface {
	ResolveEvidence(ctx context.Context, sourceType, sourceRef string, clusterID int64) (EvidenceItem, error)
}

// Evidence returns the evidence timeline for an incident: at least one
// evidence block derived from the incident snapshot, enriched by the attached
// evidence resolver when available. Resolution failure (missing source record)
// never breaks the detail view: the snapshot fallback is always returned.
func (s *Service) Evidence(ctx context.Context, id int64) ([]EvidenceItem, error) {
	record, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	item := snapshotEvidence(record)
	if s.evidenceResolver == nil {
		return []EvidenceItem{item}, nil
	}
	resolved, err := s.evidenceResolver.ResolveEvidence(ctx, record.SourceType, record.SourceRef, record.ClusterID)
	if err != nil {
		return []EvidenceItem{item}, nil
	}
	if resolved.Title == "" {
		resolved.Title = record.Title
	}
	if resolved.Summary == "" {
		resolved.Summary = record.Summary
	}
	if resolved.Severity == "" {
		resolved.Severity = record.Severity
	}
	if resolved.Resource.Kind == "" || resolved.Resource.Name == "" {
		resolved.Resource = record.Resource
	}
	if resolved.ObservedAt == "" {
		resolved.ObservedAt = record.ObservedAt.UTC().Format("2006-01-02T15:04:05.000Z")
	}
	if resolved.DeepLink == "" {
		resolved.DeepLink = IncidentDeepLink(record.SourceType)
	}
	return []EvidenceItem{resolved}, nil
}

// snapshotEvidence builds a minimal evidence block from the incident snapshot.
func snapshotEvidence(record Incident) EvidenceItem {
	return EvidenceItem{
		SourceType: record.SourceType,
		SourceRef:  record.SourceRef,
		Title:      record.Title,
		Summary:    record.Summary,
		Severity:   record.Severity,
		Resource:   record.Resource,
		ObservedAt: record.ObservedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
		DeepLink:   IncidentDeepLink(record.SourceType),
	}
}

// IncidentDeepLink returns the frontend route for the source module list. The
// receiving views do not support deep query focus yet; the evidence block
// carries the stable source_ref for manual location.
func IncidentDeepLink(sourceType string) string {
	switch strings.TrimSpace(sourceType) {
	case SourceTypeDiagnosis:
		return "/diagnoses"
	case SourceTypeAlert:
		return "/alerts"
	case SourceTypeInspection:
		return "/inspection"
	case SourceTypeSignal:
		return "/aiops/overview"
	default:
		return "/incidents"
	}
}
