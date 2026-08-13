package incident

import (
	"context"
	"encoding/csv"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"
)

// SourceInfo is the source-derived data used to build an incident workspace
// from a diagnosis or a client-observed finding.
type SourceInfo struct {
	Title      string
	Summary    string
	Severity   string
	Resource   ResourceRef
	ObservedAt time.Time
}

// SourceResolver resolves a source reference into the data needed to build an
// incident. The diagnosis-backed resolver is authoritative for diagnosis
// sources and the alert-backed resolver for alert instances; a nil resolver
// (or one returning ErrInvalidSource) falls back to the caller-provided
// finding fields.
type SourceResolver interface {
	Resolve(ctx context.Context, sourceType, sourceRef string, clusterID int64) (SourceInfo, error)
}

// Service is the M98 incident workspace application service.
type Service struct {
	repo             Repository
	resolver         SourceResolver
	evidenceResolver EvidenceResolver
}

func NewService(repo Repository) *Service { return &Service{repo: repo} }

// WithResolver attaches a source resolver. Returns the receiver for chaining.
func (s *Service) WithResolver(resolver SourceResolver) *Service {
	s.resolver = resolver
	return s
}

// WithEvidenceResolver attaches the evidence timeline resolver. Returns the
// receiver for chaining.
func (s *Service) WithEvidenceResolver(resolver EvidenceResolver) *Service {
	s.evidenceResolver = resolver
	return s
}

// CreateInput is the validated input for opening an incident workspace.
type CreateInput struct {
	SourceType string
	SourceRef  string
	ClusterID  int64
	Title      string
	Severity   string
	Summary    string
	ObservedAt time.Time
	Resource   ResourceRef
}

// Create opens a new incident. Diagnosis sources are enriched by the source
// resolver (which deduplicates by (source_type, source_ref) at the database
// level); finding sources carry their own metadata.
func (s *Service) Create(ctx context.Context, input CreateInput) (Incident, error) {
	input.SourceType = strings.TrimSpace(input.SourceType)
	input.SourceRef = strings.TrimSpace(input.SourceRef)
	input.Title = strings.TrimSpace(input.Title)
	input.Severity = strings.TrimSpace(input.Severity)
	input.Summary = strings.TrimSpace(input.Summary)
	input.Resource.Kind = strings.TrimSpace(input.Resource.Kind)
	input.Resource.Namespace = strings.TrimSpace(input.Resource.Namespace)
	input.Resource.Name = strings.TrimSpace(input.Resource.Name)
	input.Resource.UID = strings.TrimSpace(input.Resource.UID)

	if input.SourceType != SourceTypeDiagnosis && input.SourceType != SourceTypeFinding && input.SourceType != SourceTypeAlert && input.SourceType != SourceTypeInspection && input.SourceType != SourceTypeSignal {
		return Incident{}, ErrInvalidSource
	}
	if input.SourceRef == "" {
		return Incident{}, ErrInvalidSource
	}
	if input.ClusterID <= 0 {
		return Incident{}, ErrInvalidSource
	}

	if s.resolver != nil {
		if info, err := s.resolver.Resolve(ctx, input.SourceType, input.SourceRef, input.ClusterID); err == nil {
			input.Title = info.Title
			input.Summary = info.Summary
			input.Severity = info.Severity
			input.Resource = info.Resource
			input.ObservedAt = info.ObservedAt
		} else if !errors.Is(err, ErrInvalidSource) {
			return Incident{}, err
		}
	}

	if input.Title == "" {
		return Incident{}, ErrInvalidTitle
	}
	if !ValidSeverity(input.Severity) {
		return Incident{}, ErrInvalidSource
	}
	if input.Resource.Kind == "" || input.Resource.Name == "" {
		return Incident{}, ErrInvalidSource
	}
	if input.ObservedAt.IsZero() {
		input.ObservedAt = time.Now().UTC()
	}

	record := Incident{
		Title:      input.Title,
		SourceType: input.SourceType,
		SourceRef:  input.SourceRef,
		ClusterID:  input.ClusterID,
		Resource:   input.Resource,
		Severity:   input.Severity,
		Status:     StatusOpen,
		Summary:    input.Summary,
		ObservedAt: input.ObservedAt,
		SLADueAt:   SLADeadline(input.Severity, input.ObservedAt),
	}
	if err := s.repo.Create(ctx, &record); err != nil {
		return Incident{}, err
	}
	return s.repo.Get(ctx, record.ID)
}

// Get returns a single incident with followers and timeline.
func (s *Service) Get(ctx context.Context, id int64) (Incident, error) {
	return s.repo.Get(ctx, id)
}

// List returns incidents matching the filter.
func (s *Service) List(ctx context.Context, filter ListFilter) ([]Incident, error) {
	return s.repo.List(ctx, filter)
}

// Summary returns the incident count board.
func (s *Service) Summary(ctx context.Context) (Summary, error) {
	return s.repo.Summary(ctx)
}

// Transition moves an incident through the status machine with CAS versioning.
func (s *Service) Transition(ctx context.Context, id, expectedVersion int64, toStatus string, actor ActorRef, comment string) (Incident, error) {
	toStatus = strings.TrimSpace(toStatus)
	if toStatus != StatusOpen && toStatus != StatusConfirmed && toStatus != StatusResolved && toStatus != StatusDismissed {
		return Incident{}, ErrInvalidTransition
	}
	return s.repo.Transition(ctx, id, expectedVersion, toStatus, actor, strings.TrimSpace(comment))
}

// Assign hands the incident to another platform user with CAS versioning.
func (s *Service) Assign(ctx context.Context, id, expectedVersion, assigneeUserID int64, actor ActorRef, comment string) (Incident, error) {
	if assigneeUserID <= 0 {
		return Incident{}, ErrAssigneeNotFound
	}
	return s.repo.Assign(ctx, id, expectedVersion, assigneeUserID, actor, strings.TrimSpace(comment))
}

// AddFollower subscribes a user to incident updates.
func (s *Service) AddFollower(ctx context.Context, id, userID int64, actor ActorRef) (Incident, error) {
	if userID <= 0 {
		return Incident{}, ErrFollowerDuplicate
	}
	return s.repo.AddFollower(ctx, id, userID, actor)
}

// RemoveFollower unsubscribes a user from incident updates.
func (s *Service) RemoveFollower(ctx context.Context, id, userID int64, actor ActorRef) (Incident, error) {
	if userID <= 0 {
		return Incident{}, ErrFollowerNotFound
	}
	return s.repo.RemoveFollower(ctx, id, userID, actor)
}

// AddNote appends a human note to the incident timeline.
func (s *Service) AddNote(ctx context.Context, id, expectedVersion int64, actor ActorRef, content string) (Incident, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return Incident{}, ErrInvalidNote
	}
	return s.repo.AddNote(ctx, id, expectedVersion, actor, content)
}

// SetPostmortem writes the read-only postmortem view (resolved only).
func (s *Service) SetPostmortem(ctx context.Context, id, expectedVersion int64, actor ActorRef, content string) (Incident, error) {
	return s.repo.SetPostmortem(ctx, id, expectedVersion, actor, strings.TrimSpace(content))
}

// ExportCSV writes a redacted, formula-safe CSV snapshot of the incidents
// matching the filter. Free-text cells are protected against CSV injection,
// mirroring the audit-log export contract.
func (s *Service) ExportCSV(ctx context.Context, filter ListFilter, destination io.Writer) (ExportResult, error) {
	incidents, err := s.repo.List(ctx, filter)
	if err != nil {
		return ExportResult{}, err
	}
	if err := writeIncidentCSV(destination, incidents); err != nil {
		return ExportResult{}, err
	}
	return ExportResult{Rows: len(incidents)}, nil
}

// ExportOne writes a redacted, formula-safe CSV snapshot of a single incident.
func (s *Service) ExportOne(ctx context.Context, id int64, destination io.Writer) (ExportResult, error) {
	item, err := s.repo.Get(ctx, id)
	if err != nil {
		return ExportResult{}, err
	}
	if err := writeIncidentCSV(destination, []Incident{item}); err != nil {
		return ExportResult{}, err
	}
	return ExportResult{Rows: 1}, nil
}

func writeIncidentCSV(destination io.Writer, incidents []Incident) error {
	if _, err := io.WriteString(destination, "\uFEFF"); err != nil {
		return err
	}
	writer := csv.NewWriter(destination)
	writer.UseCRLF = true
	header := []string{"number", "title", "source_type", "source_ref", "cluster_id",
		"resource_kind", "resource_namespace", "resource_name", "resource_uid",
		"severity", "status", "summary", "assignee_id", "assignee_name",
		"observed_at", "sla_due_at", "resolved_at", "postmortem", "version", "created_at", "updated_at"}
	if err := writer.Write(header); err != nil {
		return err
	}
	for _, item := range incidents {
		assigneeID, assigneeName := "", ""
		if item.Assignee != nil {
			assigneeID = strconv.FormatInt(item.Assignee.ID, 10)
			assigneeName = safeCSVCell(item.Assignee.Name)
		}
		resolvedAt := ""
		if item.ResolvedAt != nil {
			resolvedAt = item.ResolvedAt.UTC().Format(time.RFC3339Nano)
		}
		row := []string{
			safeCSVCell(item.Number), safeCSVCell(item.Title), safeCSVCell(item.SourceType), safeCSVCell(item.SourceRef),
			strconv.FormatInt(item.ClusterID, 10), safeCSVCell(item.Resource.Kind), safeCSVCell(item.Resource.Namespace),
			safeCSVCell(item.Resource.Name), safeCSVCell(item.Resource.UID), safeCSVCell(item.Severity), safeCSVCell(item.Status),
			safeCSVCell(item.Summary), assigneeID, assigneeName, item.ObservedAt.UTC().Format(time.RFC3339Nano),
			item.SLADueAt.UTC().Format(time.RFC3339Nano), resolvedAt, safeCSVCell(item.Postmortem),
			strconv.FormatInt(item.Version, 10), item.CreatedAt.UTC().Format(time.RFC3339Nano), item.UpdatedAt.UTC().Format(time.RFC3339Nano),
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return err
	}
	return nil
}

// ExportResult describes a completed CSV export.
type ExportResult struct {
	Rows int
}

// safeCSVCell mirrors the audit export redaction: it quotes cells that start
// with a formula character so exported files cannot be used for injection.
func safeCSVCell(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if trimmed == "" {
		return value
	}
	switch trimmed[0] {
	case '=', '+', '-', '@':
		return "'" + value
	default:
		return value
	}
}
