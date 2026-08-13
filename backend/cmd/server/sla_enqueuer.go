package main

import (
	"context"

	"k8s-aiops.local/backend/internal/incident"
	"k8s-aiops.local/backend/internal/notification"
)

// slaEnqueuer adapts the notification outbox to the incident SLA monitor.
type slaEnqueuer struct{ service *notification.Service }

func (e slaEnqueuer) EnqueueSLA(ctx context.Context, incidentID int64, eventType string, payload []byte) error {
	return e.service.Enqueue(ctx, notification.EnqueueInput{
		IncidentID: incidentID,
		EventType:  eventType,
		Payload:    string(payload),
	})
}

var _ incident.SLAEnqueuer = slaEnqueuer{}
