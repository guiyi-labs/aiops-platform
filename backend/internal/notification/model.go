package notification

import (
	"encoding/json"
	"errors"
	"time"
)

var ErrDeliveryNotFound = errors.New("notification delivery not found")
var ErrDeliveryNotRetryable = errors.New("notification delivery is not retryable")

// Event types produced by the diagnosis trigger and the incident SLA monitor.
const (
	EventTypeDiagnosisCreated      = "diagnosis.created"
	EventTypeDiagnosisStatusChange = "diagnosis.status_changed"
	EventTypeDiagnosisAssigned     = "diagnosis.assigned"
	EventTypeIncidentSLAApproach   = "incident.sla_approaching"
	EventTypeIncidentSLABreached   = "incident.sla_breached"
	EventTypeIncidentSLAEscalated  = "incident.sla_escalated"
)

type Delivery struct {
	ID              int64           `json:"id"`
	DiagnosisID     int64           `json:"diagnosis_id,omitempty"`
	IncidentID      int64           `json:"incident_id,omitempty"`
	EventType       string          `json:"event_type"`
	EscalationLevel int             `json:"escalation_level,omitempty"`
	Status          string          `json:"status"`
	Attempts        int             `json:"attempts"`
	NextAttemptAt   time.Time       `json:"next_attempt_at"`
	DeliveredAt     *time.Time      `json:"delivered_at,omitempty"`
	LastError       string          `json:"last_error,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
	Payload         json.RawMessage `json:"-"`
}

// EnqueueInput is a single outbox delivery. Exactly one of DiagnosisID and
// IncidentID should be set; incident events are deduplicated per
// (incident_id, event_type, escalation_level) by a partial unique index.
type EnqueueInput struct {
	DiagnosisID     int64
	IncidentID      int64
	EventType       string
	EscalationLevel int
	Payload         string
}

type ListFilter struct {
	DiagnosisID     int64
	IncidentID      int64
	EventType       string
	EscalationLevel *int
	Status          string
	Limit           int
}

type ListResponse struct {
	Items     []Delivery `json:"items"`
	Total     int64      `json:"total"`
	Remaining int64      `json:"remaining"`
}

type Envelope struct {
	ID         int64           `json:"id"`
	EventType  string          `json:"event_type"`
	OccurredAt time.Time       `json:"occurred_at"`
	Data       json.RawMessage `json:"data"`
}
