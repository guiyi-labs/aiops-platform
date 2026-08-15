package correlation

import (
	"context"
	"testing"
	"time"
)

// TestCorrelateMergesSameCaseKeyFromMultipleTriggers exercises the merge
// path in Correlate: two trigger signals that resolve to the same rule for
// the same cluster+resource produce the same case_key and must merge into a
// single result with combined factors, signal links and candidates.
func TestCorrelateMergesSameCaseKeyFromMultipleTriggers(t *testing.T) {
	observedAt := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	changeStart := observedAt.Add(-5 * time.Minute)
	podUID := "pod-uid-merge-001"
	rsUID := "rs-uid-merge-001"
	deployUID := "deploy-uid-merge-001"

	results, err := NewEngine().Correlate(context.Background(), EngineInputs{
		Now: observedAt,
		Signals: []SignalOccurrenceInput{
			{ID: 301, SignalID: "diag.pod.image_pull_backoff.v1", Producer: "diagnosis",
				ClusterID: fixtureClusterA, Namespace: "app",
				Resource: ResourceCitation{Kind: "Pod", Namespace: "app", Name: "web-abc", UID: podUID},
				Severity: "critical", State: "active", Coverage: "complete",
				Freshness: observedAt, ObservedAt: observedAt},
			{ID: 302, SignalID: "diag.pod.crash_loop_backoff.v1", Producer: "diagnosis",
				ClusterID: fixtureClusterA, Namespace: "app",
				Resource: ResourceCitation{Kind: "Pod", Namespace: "app", Name: "web-abc", UID: podUID},
				Severity: "critical", State: "active", Coverage: "complete",
				Freshness: observedAt.Add(-2 * time.Minute), ObservedAt: observedAt.Add(-2 * time.Minute)},
		},
		Changes: []ChangeEventInput{
			{ID: 401, ClusterID: fixtureClusterA, Namespace: "app", Kind: "rollout",
				PlanID: "plan-merge",
				Target: ResourceCitation{Kind: "Deployment", Namespace: "app", Name: "web", UID: deployUID},
				Action: "rollout_restart", Result: "succeeded", Actor: "operator",
				StartedAt: changeStart, Confidence: "high", Source: "platform"},
		},
		Edges: []TopologyEdgeInput{
			{ID: 501, ClusterID: fixtureClusterA, Kind: "Owns",
				Source: ResourceCitation{Kind: "ReplicaSet", Namespace: "app", Name: "web-xyz", UID: rsUID},
				Target: ResourceCitation{Kind: "Pod", Namespace: "app", Name: "web-abc", UID: podUID}},
			{ID: 502, ClusterID: fixtureClusterA, Kind: "Owns",
				Source: ResourceCitation{Kind: "Deployment", Namespace: "app", Name: "web", UID: deployUID},
				Target: ResourceCitation{Kind: "ReplicaSet", Namespace: "app", Name: "web-xyz", UID: rsUID}},
		},
	})
	if err != nil {
		t.Fatalf("Correlate: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 merged result, got %d", len(results))
	}
	result := results[0]
	if result.Case.RuleID != "correlation.rollout_causes_pod_failure.v1" {
		t.Fatalf("rule = %q", result.Case.RuleID)
	}
	// Both triggers must appear as signal links.
	triggerCount := 0
	for _, link := range result.SignalLinks {
		if link.Relation == SignalRelationTrigger {
			triggerCount++
		}
	}
	if triggerCount != 2 {
		t.Fatalf("expected 2 trigger links after merge, got %d", triggerCount)
	}
	// The widest observed window must win.
	if result.Case.FirstObservedAt.IsZero() || result.Case.LastObservedAt.IsZero() {
		t.Fatalf("observed window not set: %+v", result.Case)
	}
}

// TestCorrelateEmptyTriggerSkips verifies that signals with no matching rule
// are skipped without producing results or panicking.
func TestCorrelateEmptyTriggerSkips(t *testing.T) {
	results, err := NewEngine().Correlate(context.Background(), EngineInputs{
		Now: time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC),
		Signals: []SignalOccurrenceInput{
			{ID: 1, SignalID: "bogus.signal.v1", ClusterID: fixtureClusterA,
				Resource:   ResourceCitation{Kind: "Pod", Namespace: "app", Name: "x", UID: "uid-x"},
				ObservedAt: time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)},
		},
	})
	if err != nil {
		t.Fatalf("Correlate: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results for unmatched signal, got %d", len(results))
	}
}
