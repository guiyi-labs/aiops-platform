package main

import (
	"context"

	"k8s-aiops.local/backend/internal/apiquery"
	"k8s-aiops.local/backend/internal/eventstream"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

// kubernetesEventLister adapts *k8sgateway.Service to eventstream.EventLister.
// It calls the existing read-only Events() gateway method (ADR 0004) and
// projects k8sgateway.Event into the SSE-friendly eventstream.EventSummary.
// No Watch API, no field selectors beyond namespace — bounded polling only
// (ADR 0066).
//
// The adapter is intentionally a separate file from main.go so the import
// graph stays clean: the eventstream package does not depend on the kubernetes
// package, only this adapter does.
type kubernetesEventLister struct {
	gateway *k8sgateway.Service
}

// ListEvents returns recent events for a cluster/namespace pair. An empty
// namespace means cluster-wide (only valid when the caller has cluster-wide
// scope, enforced by the handler before subscribing).
func (l kubernetesEventLister) ListEvents(ctx context.Context, clusterID int64, namespace string, limit int) ([]eventstream.EventSummary, error) {
	if l.gateway == nil {
		return nil, eventstream.ErrClusterMissing
	}
	pageLimit := limit
	if pageLimit < 1 {
		pageLimit = eventstream.DefaultListLimit
	}
	if pageLimit > eventstream.MaxListLimit {
		pageLimit = eventstream.MaxListLimit
	}
	response, err := l.gateway.Events(ctx, clusterID, namespace, apiquery.ListQuery{Page: 1, Limit: pageLimit})
	if err != nil {
		return nil, err
	}
	summaries := make([]eventstream.EventSummary, 0, len(response.Items))
	for _, ev := range response.Items {
		summaries = append(summaries, eventstream.EventSummary{
			UID:            ev.Metadata.UID,
			Name:           ev.Metadata.Name,
			Namespace:      ev.Metadata.Namespace,
			Kind:           ev.InvolvedObject.Kind,
			Type:           ev.Type,
			Reason:         ev.Reason,
			Message:        ev.Message,
			Count:          ev.Count,
			LastTimestamp:  ev.LastTimestamp,
			FirstTimestamp: ev.FirstTimestamp,
			ClusterID:      clusterID,
		})
	}
	return summaries, nil
}

// Compile-time assertion that the adapter satisfies eventstream.EventLister.
var _ eventstream.EventLister = kubernetesEventLister{}
