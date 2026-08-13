package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"k8s-aiops.local/backend/internal/alert"
	"k8s-aiops.local/backend/internal/diagnosis"
	"k8s-aiops.local/backend/internal/incident"
	"k8s-aiops.local/backend/internal/inspection"
	"k8s-aiops.local/backend/internal/signal"
)

// diagnosisRecordReader reads a persisted diagnosis record by id.
type diagnosisRecordReader interface {
	Get(ctx context.Context, id int64) (diagnosis.Record, error)
}

// incidentResolver dispatches incident source enrichment by source type:
// diagnosis sources resolve from persisted diagnosis records; alert sources
// resolve the linked diagnosis of a firing alert instance; inspection
// sources resolve a persisted inspection result; signal sources resolve a
// normalized signal occurrence. Finding sources carry their own metadata and
// are not resolvable here (ErrInvalidSource falls back to caller-provided
// fields).
type incidentResolver struct {
	diagnosisRecords diagnosisRecordReader
	alerts           alertResolver
	inspections      inspectionResultReader
	signals          signalOccurrenceReader
}

// alertResolver fetches a cluster-scoped alert instance plus its rule-metric
// label for building an incident from a firing alert.
type alertResolver interface {
	Get(ctx context.Context, clusterID, id int64) (alert.Instance, error)
}

// alertServiceAdapter adapts *alert.Service to the narrow alertResolver
// interface used by the incident resolver.
type alertServiceAdapter struct {
	svc *alert.Service
}

func (a alertServiceAdapter) Get(ctx context.Context, clusterID, id int64) (alert.Instance, error) {
	return a.svc.GetInstance(ctx, clusterID, id)
}

// inspectionResultReader fetches a persisted inspection result by id.
type inspectionResultReader interface {
	Get(ctx context.Context, id int64) (inspection.ResultView, error)
}

// inspectionServiceAdapter adapts *inspection.Service to the narrow
// inspectionResultReader interface used by the incident resolver.
type inspectionServiceAdapter struct {
	svc *inspection.Service
}

func (a inspectionServiceAdapter) Get(ctx context.Context, id int64) (inspection.ResultView, error) {
	return a.svc.GetResult(ctx, id)
}

// signalOccurrenceReader fetches a normalized signal occurrence by id.
type signalOccurrenceReader interface {
	Get(ctx context.Context, id int64) (signal.Occurrence, error)
}

// signalServiceAdapter adapts *signal.Service to the narrow
// signalOccurrenceReader interface used by the incident resolver.
type signalServiceAdapter struct {
	svc *signal.Service
}

func (a signalServiceAdapter) Get(ctx context.Context, id int64) (signal.Occurrence, error) {
	return a.svc.Get(ctx, id)
}

func NewIncidentResolver(records *diagnosis.GormRepository, alerts *alert.Service, inspections *inspection.Service, signals *signal.Service) *incidentResolver {
	return &incidentResolver{
		diagnosisRecords: records,
		alerts:           alertServiceAdapter{svc: alerts},
		inspections:      inspectionServiceAdapter{svc: inspections},
		signals:          signalServiceAdapter{svc: signals},
	}
}

func (r *incidentResolver) Resolve(ctx context.Context, sourceType, sourceRef string, clusterID int64) (incident.SourceInfo, error) {
	switch sourceType {
	case incident.SourceTypeDiagnosis:
		return r.resolveDiagnosis(ctx, sourceRef)
	case incident.SourceTypeAlert:
		return r.resolveAlert(ctx, clusterID, sourceRef)
	case incident.SourceTypeInspection:
		return r.resolveInspection(ctx, clusterID, sourceRef)
	case incident.SourceTypeSignal:
		return r.resolveSignal(ctx, clusterID, sourceRef)
	default:
		return incident.SourceInfo{}, incident.ErrInvalidSource
	}
}

func (r *incidentResolver) resolveSignal(ctx context.Context, clusterID int64, sourceRef string) (incident.SourceInfo, error) {
	const prefix = "signal:"
	if !strings.HasPrefix(sourceRef, prefix) {
		return incident.SourceInfo{}, incident.ErrInvalidSource
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(sourceRef, prefix), 10, 64)
	if err != nil || id < 1 {
		return incident.SourceInfo{}, incident.ErrInvalidSource
	}
	if r.signals == nil {
		return incident.SourceInfo{}, incident.ErrInvalidSource
	}
	occ, err := r.signals.Get(ctx, id)
	if err != nil {
		if errors.Is(err, signal.ErrSignalNotFound) {
			return incident.SourceInfo{}, incident.ErrInvalidSource
		}
		return incident.SourceInfo{}, fmt.Errorf("resolve signal occurrence %d: %w", id, err)
	}
	// Anti-leakage: the caller's cluster must own the occurrence.
	if occ.ClusterID != clusterID {
		return incident.SourceInfo{}, incident.ErrInvalidSource
	}
	title := occ.SignalID
	if occ.Resource.Name != "" {
		title += " " + occ.Resource.Name
	}
	return incident.SourceInfo{
		Title:    "Signal " + title,
		Summary:  occ.SignalCode + " (" + string(occ.State) + ", " + string(occ.Coverage) + ")",
		Severity: normalizeIncidentSeverity(string(occ.Severity)),
		Resource: incident.ResourceRef{
			Kind:      occ.Resource.Kind,
			Namespace: occ.Resource.Namespace,
			Name:      occ.Resource.Name,
			UID:       occ.Resource.UID,
		},
		ObservedAt: occ.ObservedAt,
	}, nil
}

func (r *incidentResolver) resolveInspection(ctx context.Context, clusterID int64, sourceRef string) (incident.SourceInfo, error) {
	const prefix = "inspection:"
	if !strings.HasPrefix(sourceRef, prefix) {
		return incident.SourceInfo{}, incident.ErrInvalidSource
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(sourceRef, prefix), 10, 64)
	if err != nil || id < 1 {
		return incident.SourceInfo{}, incident.ErrInvalidSource
	}
	if r.inspections == nil {
		return incident.SourceInfo{}, incident.ErrInvalidSource
	}
	result, err := r.inspections.Get(ctx, id)
	if err != nil {
		if errors.Is(err, inspection.ErrResultNotFound) {
			return incident.SourceInfo{}, incident.ErrInvalidSource
		}
		return incident.SourceInfo{}, fmt.Errorf("resolve inspection result %d: %w", id, err)
	}
	// Anti-leakage: the caller's cluster must own the result.
	if result.ClusterID != clusterID {
		return incident.SourceInfo{}, incident.ErrInvalidSource
	}
	title := result.RuleCode
	if result.ResourceName != "" {
		title += " " + result.ResourceName
	}
	return incident.SourceInfo{
		Title:    "Inspection " + title,
		Summary:  result.SignalCode + " (" + result.State + ")",
		Severity: normalizeIncidentSeverity(result.Severity),
		Resource: incident.ResourceRef{
			Kind:      result.ResourceKind,
			Namespace: result.Namespace,
			Name:      result.ResourceName,
			UID:       result.ResourceUID,
		},
		ObservedAt: result.ObservedAt,
	}, nil
}

// normalizeIncidentSeverity maps inspection severities (critical/warning/info)
// onto the incident severity vocabulary (info/warning/high/critical).
func normalizeIncidentSeverity(severity string) string {
	switch severity {
	case "critical":
		return incident.SeverityCritical
	case "warning":
		return incident.SeverityWarning
	default:
		return incident.SeverityInfo
	}
}

func (r *incidentResolver) resolveDiagnosis(ctx context.Context, sourceRef string) (incident.SourceInfo, error) {
	const prefix = "diagnosis:"
	if !strings.HasPrefix(sourceRef, prefix) {
		return incident.SourceInfo{}, incident.ErrInvalidSource
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(sourceRef, prefix), 10, 64)
	if err != nil || id < 1 {
		return incident.SourceInfo{}, incident.ErrInvalidSource
	}
	record, err := r.diagnosisRecords.Get(ctx, id)
	if err != nil {
		if errors.Is(err, diagnosis.ErrRecordNotFound) {
			return incident.SourceInfo{}, incident.ErrInvalidSource
		}
		return incident.SourceInfo{}, err
	}
	return incident.SourceInfo{
		Title:    record.Resource.Name + " " + record.RuleID,
		Summary:  record.Summary,
		Severity: record.Severity,
		Resource: incident.ResourceRef{
			Kind:      record.Resource.Kind,
			Namespace: record.Resource.Namespace,
			Name:      record.Resource.Name,
			UID:       record.Resource.UID,
		},
		ObservedAt: record.ObservedAt,
	}, nil
}

func (r *incidentResolver) resolveAlert(ctx context.Context, clusterID int64, sourceRef string) (incident.SourceInfo, error) {
	const prefix = "alert:"
	if !strings.HasPrefix(sourceRef, prefix) {
		return incident.SourceInfo{}, incident.ErrInvalidSource
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(sourceRef, prefix), 10, 64)
	if err != nil || id < 1 {
		return incident.SourceInfo{}, incident.ErrInvalidSource
	}
	instance, err := r.alerts.Get(ctx, clusterID, id)
	if err != nil {
		if errors.Is(err, alert.ErrAlertNotFound) || errors.Is(err, alert.ErrRuleNotFound) {
			return incident.SourceInfo{}, incident.ErrInvalidSource
		}
		return incident.SourceInfo{}, fmt.Errorf("resolve alert instance %d: %w", id, err)
	}
	// A firing alert instance is backed by a diagnosis record created at first
	// firing; reuse it for severity, resource and narrative so the incident
	// carries the same evidence as the underlying diagnosis.
	if instance.DiagnosisID > 0 {
		record, err := r.diagnosisRecords.Get(ctx, instance.DiagnosisID)
		if err == nil {
			return incident.SourceInfo{
				Title:    "Alert " + record.Resource.Name + " " + record.RuleID,
				Summary:  record.Summary,
				Severity: record.Severity,
				Resource: incident.ResourceRef{
					Kind:      record.Resource.Kind,
					Namespace: record.Resource.Namespace,
					Name:      record.Resource.Name,
					UID:       record.Resource.UID,
				},
				ObservedAt: instance.FirstFiredAt,
			}, nil
		}
		if !errors.Is(err, diagnosis.ErrRecordNotFound) {
			return incident.SourceInfo{}, fmt.Errorf("resolve alert diagnosis %d: %w", instance.DiagnosisID, err)
		}
	}
	return incident.SourceInfo{}, incident.ErrInvalidSource
}
