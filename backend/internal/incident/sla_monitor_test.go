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
	err               error
}
type slaEnqueuerStub struct {
	events []struct {
		IncidentID int64
		EventType  string
		Payload    []byte
	}
	enabled bool
	err     error
}

func (s *slaCandidateStub) ListSLAEligible(_ context.Context, eventType string, _, _ time.Time, _ int) ([]SLACandidate, error) {
	if s.err != nil {
		return nil, s.err
	}
	if eventType == SLAEventBreached {
		s.breachedCalled = true
		return s.items, nil
	}
	s.approachingCalled = true
	return nil, nil
}
func (e *slaEnqueuerStub) EnqueueSLA(_ context.Context, id int64, eventType string, payload []byte) error {
	if e.err != nil {
		return e.err
	}
	e.events = append(e.events, struct {
		IncidentID int64
		EventType  string
		Payload    []byte
	}{id, eventType, payload})
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
