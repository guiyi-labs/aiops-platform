package incident

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

type slaCandidateStub struct {
	items             []SLACandidate
	breachedCalled    bool
	approachingCalled bool
	escalationItems   map[int][]SLACandidate
	err               error
}
type slaEnqueuerStub struct {
	events []struct {
		IncidentID int64
		EventType  string
		Level      int
		Payload    []byte
	}
	enabled bool
	err     error
}

func (s *slaCandidateStub) ListSLAEligible(_ context.Context, eventType string, level int, _, _ time.Time, _ int) ([]SLACandidate, error) {
	if s.err != nil {
		return nil, s.err
	}
	if eventType == SLAEventBreached {
		s.breachedCalled = true
		return s.items, nil
	}
	if eventType == SLAEventApproaching {
		s.approachingCalled = true
		return nil, nil
	}
	if items, ok := s.escalationItems[level]; ok {
		return items, nil
	}
	return nil, nil
}
func (e *slaEnqueuerStub) EnqueueSLA(_ context.Context, id int64, eventType string, level int, payload []byte) error {
	if e.err != nil {
		return e.err
	}
	e.events = append(e.events, struct {
		IncidentID int64
		EventType  string
		Level      int
		Payload    []byte
	}{id, eventType, level, payload})
	return nil
}

func TestSLAMonitorEvaluateEnqueuesBreachedThenApproaching(t *testing.T) {
	now := time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
	candidates := &slaCandidateStub{}
	enqueuer := &slaEnqueuerStub{}
	monitor := NewSLAMonitor(SLAMonitorConfig{Enabled: true, ApproachingWindow: 15 * time.Minute, BatchSize: 20}, candidates, enqueuer, nil)
	monitor.now = func() time.Time { return now }

	if err := monitor.EvaluateOnce(context.Background()); err != nil {
		t.Fatalf("EvaluateOnce() error = %v", err)
	}
	if len(enqueuer.events) != 0 {
		t.Fatalf("expected no events when queue is empty, got %d", len(enqueuer.events))
	}

	candidates.items = []SLACandidate{{
		IncidentID: 5, Number: "INC-000005", Title: "node not ready", Severity: "critical",
		Status: "confirmed", Summary: "kubelet down", SLADueAt: now.Add(-time.Hour), ObservedAt: now.Add(-2 * time.Hour),
	}}
	enqueuer2 := &slaEnqueuerStub{}
	monitor2 := NewSLAMonitor(SLAMonitorConfig{Enabled: true, ApproachingWindow: 15 * time.Minute, BatchSize: 20}, &slaCandidateStub{items: candidates.items}, enqueuer2, nil)
	monitor2.now = func() time.Time { return now }
	if err := monitor2.EvaluateOnce(context.Background()); err != nil {
		t.Fatalf("EvaluateOnce() error = %v", err)
	}
	if len(enqueuer2.events) != 1 || enqueuer2.events[0].EventType != SLAEventBreached || enqueuer2.events[0].IncidentID != 5 {
		t.Fatalf("events = %#v", enqueuer2.events)
	}
	var payload struct {
		IncidentNumber string `json:"incident_number"`
		Event          string `json:"event"`
		DeepLink       string `json:"deep_link"`
	}
	if err := json.Unmarshal(enqueuer2.events[0].Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.IncidentNumber != "INC-000005" || payload.Event != "incident.sla_breached" || payload.DeepLink != "/incidents/5" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestSLAMonitorEnqueuesBoundedEscalations(t *testing.T) {
	now := time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
	firstCandidate := SLACandidate{IncidentID: 8, Number: "INC-000008", Title: "api unavailable", Severity: "critical", Status: "confirmed", SLADueAt: now.Add(-time.Hour), ObservedAt: now.Add(-2 * time.Hour)}
	finalCandidate := SLACandidate{IncidentID: 9, Number: "INC-000009", Title: "database unavailable", Severity: "critical", Status: "open", SLADueAt: now.Add(-3 * time.Hour), ObservedAt: now.Add(-4 * time.Hour)}
	candidates := &slaCandidateStub{escalationItems: map[int][]SLACandidate{
		SLAEscalationLevelFirst: {firstCandidate}, SLAEscalationLevelFinal: {finalCandidate},
	}}
	enqueuer := &slaEnqueuerStub{}
	monitor := NewSLAMonitor(SLAMonitorConfig{
		Enabled: true, ApproachingWindow: 15 * time.Minute, FirstEscalationAfter: 30 * time.Minute,
		FinalEscalationAfter: 2 * time.Hour, BatchSize: 20,
	}, candidates, enqueuer, nil)
	monitor.now = func() time.Time { return now }

	if err := monitor.EvaluateOnce(context.Background()); err != nil {
		t.Fatalf("EvaluateOnce() error = %v", err)
	}
	if len(enqueuer.events) != 2 {
		t.Fatalf("events = %#v, want two escalation stages", enqueuer.events)
	}
	if enqueuer.events[0].EventType != SLAEventEscalated || enqueuer.events[0].Level != SLAEscalationLevelFirst || enqueuer.events[1].Level != SLAEscalationLevelFinal {
		t.Fatalf("events = %#v", enqueuer.events)
	}
	var payload struct {
		Level  int    `json:"escalation_level"`
		Stage  string `json:"escalation_stage"`
		Reason string `json:"escalation_reason"`
	}
	if err := json.Unmarshal(enqueuer.events[1].Payload, &payload); err != nil {
		t.Fatalf("decode escalation payload: %v", err)
	}
	if payload.Level != SLAEscalationLevelFinal || payload.Stage != "final" || payload.Reason == "" {
		t.Fatalf("escalation payload = %#v", payload)
	}
}

func TestSLAMonitorPropagatesErrors(t *testing.T) {
	now := time.Now().UTC()
	monitor := NewSLAMonitor(SLAMonitorConfig{Enabled: true, ApproachingWindow: time.Minute, BatchSize: 20},
		&slaCandidateStub{err: errors.New("db down")}, &slaEnqueuerStub{}, nil)
	monitor.now = func() time.Time { return now }
	if err := monitor.EvaluateOnce(context.Background()); err == nil {
		t.Fatal("expected candidate error to propagate")
	}

	candidate := SLACandidate{IncidentID: 1, Number: "INC-000001", Title: "t", Severity: "high", Status: "open", SLADueAt: now.Add(5 * time.Minute), ObservedAt: now.Add(-time.Hour)}
	enqueuer := &slaEnqueuerStub{err: errors.New("enqueue failed")}
	monitor2 := NewSLAMonitor(SLAMonitorConfig{Enabled: true, ApproachingWindow: 15 * time.Minute, BatchSize: 20},
		&slaCandidateStub{items: []SLACandidate{candidate}}, enqueuer, nil)
	monitor2.now = func() time.Time { return now }
	if err := monitor2.EvaluateOnce(context.Background()); err == nil {
		t.Fatal("expected enqueue error to propagate")
	}
}

func TestSLAMonitorDisabledIsNoop(t *testing.T) {
	enqueuer := &slaEnqueuerStub{}
	monitor := NewSLAMonitor(SLAMonitorConfig{Enabled: false}, &slaCandidateStub{}, enqueuer, nil)
	if err := monitor.EvaluateOnce(context.Background()); err != nil {
		t.Fatalf("disabled EvaluateOnce() error = %v", err)
	}
	if len(enqueuer.events) != 0 {
		t.Fatalf("disabled monitor must not enqueue, got %d", len(enqueuer.events))
	}
}

func TestSLAMonitorRunLifecycle(t *testing.T) {
	candidates := &slaCandidateStub{items: []SLACandidate{{IncidentID: 1, Number: "INC-000001"}}}
	enqueuer := &slaEnqueuerStub{}
	monitor := NewSLAMonitor(SLAMonitorConfig{Enabled: true, PollInterval: time.Hour, BatchSize: 50}, candidates, enqueuer, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		monitor.Run(ctx)
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after context cancellation")
	}
	if !candidates.breachedCalled {
		t.Error("Run did not evaluate breached reminders before stopping")
	}
}
