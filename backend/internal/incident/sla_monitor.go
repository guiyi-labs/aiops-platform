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
	SLAEventEscalated   = "incident.sla_escalated"

	SLAEscalationLevelBase  = 0
	SLAEscalationLevelFirst = 1
	SLAEscalationLevelFinal = 2
)

// SLAEnqueuer enqueues incident SLA reminder events into the platform
// notification outbox. Implemented in cmd/server over the notification
// service; kept as an interface to avoid an incident -> notification import.
type SLAEnqueuer interface {
	EnqueueSLA(ctx context.Context, incidentID int64, eventType string, escalationLevel int, payload []byte) error
}

// SLACandidateLister lists incidents that still need an SLA reminder of the
// given event type. Implemented by the incident GormRepository.
type SLACandidateLister interface {
	ListSLAEligible(ctx context.Context, eventType string, escalationLevel int, dueAfter, dueBefore time.Time, limit int) ([]SLACandidate, error)
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
	Enabled              bool
	PollInterval         time.Duration
	ApproachingWindow    time.Duration
	FirstEscalationAfter time.Duration
	FinalEscalationAfter time.Duration
	BatchSize            int
}

// SLAMonitor periodically scans open/confirmed incidents whose SLA deadline is
// approaching, breached, or still unresolved after a bounded escalation delay.
// Deduplication is enforced by the outbox partial unique index on
// (incident_id, event_type, escalation_level), so repeated evaluations never
// duplicate an escalation stage.
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

// EvaluateOnce scans for breached, approaching, and then bounded escalation
// stages. A first escalation is emitted only between the first and final delay;
// the final escalation is emitted after the final delay.
func (m *SLAMonitor) EvaluateOnce(ctx context.Context) error {
	if !m.config.Enabled {
		return nil
	}
	now := m.now().UTC()
	if err := m.evaluateEvent(ctx, SLAEventBreached, SLAEscalationLevelBase, time.Unix(0, 0).UTC(), now); err != nil {
		return err
	}
	if err := m.evaluateEvent(ctx, SLAEventApproaching, SLAEscalationLevelBase, now, now.Add(m.config.ApproachingWindow)); err != nil {
		return err
	}
	if m.config.FirstEscalationAfter <= 0 || m.config.FinalEscalationAfter <= m.config.FirstEscalationAfter {
		return nil
	}
	firstWindowStart := now.Add(-m.config.FinalEscalationAfter).Add(time.Nanosecond)
	firstWindowEnd := now.Add(-m.config.FirstEscalationAfter)
	if err := m.evaluateEvent(ctx, SLAEventEscalated, SLAEscalationLevelFirst, firstWindowStart, firstWindowEnd); err != nil {
		return err
	}
	return m.evaluateEvent(ctx, SLAEventEscalated, SLAEscalationLevelFinal, time.Unix(0, 0).UTC(), now.Add(-m.config.FinalEscalationAfter))
}

func (m *SLAMonitor) evaluateEvent(ctx context.Context, eventType string, escalationLevel int, dueAfter, dueBefore time.Time) error {
	candidates, err := m.candidates.ListSLAEligible(ctx, eventType, escalationLevel, dueAfter, dueBefore, m.config.BatchSize)
	if err != nil {
		return err
	}
	changedAt := m.now().UTC()
	for _, candidate := range candidates {
		payload, err := slaPayload(candidate, eventType, escalationLevel, changedAt)
		if err != nil {
			return err
		}
		if err := m.enqueuer.EnqueueSLA(ctx, candidate.IncidentID, eventType, escalationLevel, payload); err != nil {
			return err
		}
	}
	return nil
}

// slaPayload builds the webhook payload for one SLA reminder event.
func slaPayload(candidate SLACandidate, eventType string, escalationLevel int, changedAt time.Time) ([]byte, error) {
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
	if eventType == SLAEventEscalated {
		payload["escalation_level"] = escalationLevel
		if escalationLevel == SLAEscalationLevelFinal {
			payload["escalation_stage"] = "final"
		} else {
			payload["escalation_stage"] = "first"
		}
		payload["escalation_reason"] = "incident remains unresolved after SLA breach"
	}
	if candidate.AssigneeID > 0 {
		payload["assignee_user_id"] = candidate.AssigneeID
		payload["assignee_name"] = candidate.AssigneeName
	}
	return json.Marshal(payload)
}
