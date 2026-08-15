package incident

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"
)

// FuzzSLAMonitorStateMachine exercises the SLA reminder state machine with
// arbitrary escalation windows and candidate fields. It pins the escalation
// contract: a first escalation is emitted only when
// 0 < FirstEscalationAfter < FinalEscalationAfter, the final escalation
// always follows, stage names are first/final only, and arbitrary candidate
// data never panics while producing valid JSON payloads.
func FuzzSLAMonitorStateMachine(f *testing.F) {
	seeds := []struct {
		firstNS, finalNS int64
		incidentID       int64
		level            int
	}{
		{int64(time.Minute), int64(2 * time.Minute), 1, SLAEscalationLevelFirst},
		{int64(2 * time.Minute), int64(time.Minute), 2, SLAEscalationLevelFinal},
		{int64(0), int64(0), 3, 0},
		{int64(-time.Minute), int64(time.Minute), 4, SLAEscalationLevelFirst},
		{int64(time.Hour), int64(2 * time.Hour), 5, SLAEscalationLevelFinal},
	}
	for _, seed := range seeds {
		f.Add(seed.firstNS, seed.finalNS, seed.incidentID, seed.level)
	}

	f.Fuzz(func(t *testing.T, firstNS, finalNS int64, incidentID int64, level int) {
		now := time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
		candidates := &slaCandidateStub{
			items: []SLACandidate{{
				IncidentID: incidentID, Number: "INC-000001", Title: "demo", Severity: SeverityWarning,
				Status: StatusOpen, Summary: "sla demo", SLADueAt: now.Add(time.Hour),
				ObservedAt: now.Add(-time.Minute), AssigneeID: 2, AssigneeName: "ops",
			}},
			escalationItems: map[int][]SLACandidate{
				SLAEscalationLevelFirst: {{
					IncidentID: incidentID, Number: "INC-000001", Title: "demo", Severity: SeverityWarning,
					Status: StatusOpen, Summary: "sla demo", SLADueAt: now.Add(-time.Hour),
					ObservedAt: now.Add(-2 * time.Hour),
				}},
				SLAEscalationLevelFinal: {{
					IncidentID: incidentID, Number: "INC-000001", Title: "demo", Severity: SeverityWarning,
					Status: StatusOpen, Summary: "sla demo", SLADueAt: now.Add(-3 * time.Hour),
					ObservedAt: now.Add(-4 * time.Hour), AssigneeID: -1, AssigneeName: "",
				}},
			},
		}
		enqueuer := &slaEnqueuerStub{}
		monitor := NewSLAMonitor(SLAMonitorConfig{
			Enabled: true, ApproachingWindow: time.Hour,
			FirstEscalationAfter: time.Duration(firstNS), FinalEscalationAfter: time.Duration(finalNS), BatchSize: 100,
		}, candidates, enqueuer, nil)
		monitor.now = func() time.Time { return now }

		if err := monitor.EvaluateOnce(context.Background()); err != nil {
			t.Fatalf("EvaluateOnce: %v", err)
		}

		// Enqueued events must all carry a valid, parseable payload with
		// the expected fields and a deep link.
		for _, event := range enqueuer.events {
			payload, err := parseSLAPayload(event.Payload)
			if err != nil {
				t.Fatalf("payload for event=%s level=%d: %v", event.EventType, event.Level, err)
			}
			if payload["incident_id"] == nil || payload["deep_link"] != "/incidents/"+strconv.FormatInt(incidentID, 10) {
				t.Fatalf("payload missing identity fields: %v", payload)
			}
			if event.EventType == SLAEventEscalated {
				stage, ok := payload["escalation_stage"].(string)
				if !ok || (stage != "first" && stage != "final") {
					t.Fatalf("invalid escalation stage %v", payload["escalation_stage"])
				}
				if event.Level != SLAEscalationLevelFirst && event.Level != SLAEscalationLevelFinal {
					t.Fatalf("invalid escalation level %d", event.Level)
				}
			}
		}

		// Escalation ordering contract: when monotonic windows are
		// configured, a first escalation must precede the final one.
		first, final := time.Duration(firstNS), time.Duration(finalNS)
		firstOK := first > 0 && final > first
		sawFirst, sawFinal := false, false
		for _, event := range enqueuer.events {
			if event.Level == SLAEscalationLevelFirst {
				sawFirst = true
			}
			if event.Level == SLAEscalationLevelFinal {
				if !firstOK && sawFirst {
					t.Fatalf("final escalation emitted without monotonic windows (first=%v final=%v)", first, final)
				}
				sawFinal = true
			}
		}
		if firstOK && !sawFirst {
			t.Fatalf("first escalation not emitted with first=%v final=%v", first, final)
		}
		if firstOK && !sawFinal {
			t.Fatalf("final escalation not emitted with first=%v final=%v", first, final)
		}
	})
}

func parseSLAPayload(data []byte) (map[string]any, error) {
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}
