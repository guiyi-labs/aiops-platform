package main

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"k8s-aiops.local/backend/internal/diagnosis"
	"k8s-aiops.local/backend/internal/incident"
)

// diagnosisIncidentResolver enriches incident workspaces from persisted
// diagnosis records. Finding sources carry their own metadata and are not
// resolvable here (ErrInvalidSource falls back to caller-provided fields).
type diagnosisIncidentResolver struct {
	records *diagnosis.GormRepository
}

func (r *diagnosisIncidentResolver) Resolve(ctx context.Context, sourceType, sourceRef string) (incident.SourceInfo, error) {
	if sourceType != incident.SourceTypeDiagnosis {
		return incident.SourceInfo{}, incident.ErrInvalidSource
	}
	const prefix = "diagnosis:"
	if !strings.HasPrefix(sourceRef, prefix) {
		return incident.SourceInfo{}, incident.ErrInvalidSource
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(sourceRef, prefix), 10, 64)
	if err != nil || id < 1 {
		return incident.SourceInfo{}, incident.ErrInvalidSource
	}
	record, err := r.records.Get(ctx, id)
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
