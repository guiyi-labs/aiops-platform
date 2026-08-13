package incident

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"go.uber.org/zap"
)

// SLA reminder event types emitted by the monitor into the notification outbox.
const (
	SLAEventApproaching = "incident.sla_approaching"
	SLAEventBreached    = "incident.sla_breached"
)

// SLAEnqueuer enqueues incident SLA reminder events into the platform
// notification outbox. Implemented in cmd/server over the notification
// service; kept as an interface to avoid an incident -> notification import.
type SLAEnqueuer interface {
	EnqueueSLA(ctx context.Context, incidentID int64, eventType string, payload []byte) error
}

// SLACandidateLister lists incidents that still need an SLA reminder of the
// given event type. Implemented by the incident GormRepository.
type SLACandidateLister interface {
	ListSLAEligible(ctx context.Context, eventType string, dueAfter, dueBefore time.Time, limit int) ([]SLACandidate, error)
}

// SLACandidate is the minimal incident projection needed to build a reminder.
type SLACandidate struct {
	IncidentID   int64
	Number       string
	Title        string
	Severity     string
	Status       string
	Summary      string
	AssigneeID   int64
	AssigneeName string
	ObservedAt   time.Time
	SLADueAt     time.Time
}

// SLAMonitorConfig configures the SLA reminder background monitor.
type SLAMonitorConfig struct {
	Enabled           bool
	PollInterval      time.Duration
	ApproachingWindow time.Duration
	BatchSize         int
}

// SLAMonitor periodically scans open/confirmed incidents whose SLA deadline is
// approaching or already breached and enqueues one reminder per incident per
// event type. Deduplication is enforced by the outbox partial unique index on
// (incident_id, event_type), so repeated evaluations never duplicate events.
type SLAMonitor struct {
	config     SLAMonitorConfig
	candidates SLACandidateLister
	enqueuer   SLAEnqueuer
	logger     *zap.Logger
	now        func() time.Time
}

func NewSLAMonitor(config SLAMonitorConfig, candidates SLACandidateLister, enqueuer SLAEnqueuer, logger *zap.Logger) *SLAMonitor {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &SLAMonitor{config: config, candidates: candidates, enqueuer: enqueuer, logger: logger, now: time.Now}
}

// Run evaluates once, then on every poll interval until the context ends.
func (m *SLAMonitor) Run(ctx context.Context) {
	if !m.config.Enabled {
		return
	}
	m.evaluate(ctx)
	ticker := time.NewTicker(m.config.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.evaluate(ctx)
		}
	}
}

func (m *SLAMonitor) evaluate(ctx context.Context) {
	if err := m.EvaluateOnce(ctx); err != nil && ctx.Err() == nil {
		m.logger.Error("evaluate incident SLA reminders", zap.Error(err))
	}
}

// EvaluateOnce scans for breached then approaching deadlines. Breached
// reminders always take precedence; an approaching reminder is only emitted
// for incidents whose deadline is still in the future within the window.
func (m *SLAMonitor) EvaluateOnce(ctx context.Context) error {
	if !m.config.Enabled {
		return nil
	}
	now := m.now().UTC()
	if err := m.evaluateEvent(ctx, SLAEventBreached, time.Unix(0, 0).UTC(), now); err != nil {
		return err
	}
	return m.evaluateEvent(ctx, SLAEventApproaching, now, now.Add(m.config.ApproachingWindow))
}

func (m *SLAMonitor) evaluateEvent(ctx context.Context, eventType string, dueAfter, dueBefore time.Time) error {
	candidates, err := m.candidates.ListSLAEligible(ctx, eventType, dueAfter, dueBefore, m.config.BatchSize)
	if err != nil {
		return err
	}
	changedAt := m.now().UTC()
	for _, candidate := range candidates {
		payload, err := slaPayload(candidate, eventType, changedAt)
		if err != nil {
			return err
		}
		if err := m.enqueuer.EnqueueSLA(ctx, candidate.IncidentID, eventType, payload); err != nil {
			return err
		}
	}
	return nil
}

// slaPayload builds the webhook payload for one SLA reminder event.
func slaPayload(candidate SLACandidate, eventType string, changedAt time.Time) ([]byte, error) {
	payload := map[string]any{
		"incident_id":     candidate.IncidentID,
		"incident_number": candidate.Number,
		"title":           candidate.Title,
		"severity":        candidate.Severity,
		"status":          candidate.Status,
		"summary":         candidate.Summary,
		"sla_due_at":      candidate.SLADueAt.UTC().Format(time.RFC3339Nano),
		"observed_at":     candidate.ObservedAt.UTC().Format(time.RFC3339Nano),
		"changed_at":      changedAt.UTC().Format(time.RFC3339Nano),
		"event":           eventType,
		"deep_link":       "/incidents/" + strconv.FormatInt(candidate.IncidentID, 10),
	}
	if candidate.AssigneeID > 0 {
		payload["assignee_user_id"] = candidate.AssigneeID
		payload["assignee_name"] = candidate.AssigneeName
	}
	return json.Marshal(payload)
}
