package diagnosis

import (
	"fmt"
	"testing"
	"time"

	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

func TestWithNarrativeNodeNotReadyTimeline(t *testing.T) {
	var node k8sgateway.Node
	mustDecode(t, `{"metadata":{"name":"worker-1","uid":"node-1"},"status":{"conditions":[
		{"type":"Ready","status":"False","reason":"KubeletNotReady","message":"runtime is unavailable","lastTransitionTime":"2026-07-26T10:00:00Z"},
		{"type":"MemoryPressure","status":"True","reason":"KubeletHasInsufficientMemory","message":"memory pressure","lastTransitionTime":"2026-07-26T10:01:00Z"}]}}`, &node)
	record, matched := EvaluateNodeNotReady(7, node, time.Date(2026, 7, 26, 10, 2, 0, 0, time.UTC))
	if !matched {
		t.Fatal("rule did not match")
	}
	augmented := WithNarrative(record)
	if len(augmented.Timeline) != 2 {
		t.Fatalf("timeline = %d entries, want 2", len(augmented.Timeline))
	}
	first, second := augmented.Timeline[0], augmented.Timeline[1]
	if first.OccurredAt != "2026-07-26T10:00:00Z" || second.OccurredAt != "2026-07-26T10:01:00Z" {
		t.Fatalf("timeline not ascending: %q then %q", first.OccurredAt, second.OccurredAt)
	}
	if first.Type != "node_condition" || first.Category != CategoryResourceState {
		t.Fatalf("first entry = %#v", first)
	}
	if first.Source != "node.status.conditions" || first.Missing {
		t.Fatalf("first entry metadata = %#v", first)
	}
	if len(first.Integrity) != 64 {
		t.Fatalf("integrity = %q, want sha256 hex", first.Integrity)
	}
	card := augmented.RootCauseCard
	if card == nil {
		t.Fatal("root cause card missing")
	}
	if card.FirstObservedAt != "2026-07-26T10:00:00Z" {
		t.Fatalf("first_observed_at = %q", card.FirstObservedAt)
	}
	if card.Conclusion != record.Summary || card.Severity != "critical" || card.Status != "open" {
		t.Fatalf("card summary = %#v", card)
	}
	if card.Confidence != "deterministic" || card.ConfidenceSource != RuleNodeNotReady {
		t.Fatalf("card confidence = %#v", card)
	}
	if card.Resource.Kind != "Node" || card.Resource.Name != "worker-1" {
		t.Fatalf("card resource = %#v", card.Resource)
	}
	wantRefs := []string{
		evidenceRef(record, 0, record.Evidence[0]),
		evidenceRef(record, 1, record.Evidence[1]),
	}
	if fmt.Sprint(card.KeyEvidenceRefs) != fmt.Sprint(wantRefs) {
		t.Fatalf("key_evidence_refs = %v, want %v", card.KeyEvidenceRefs, wantRefs)
	}
}

func TestWithNarrativeDeploymentUnavailableTimeline(t *testing.T) {
	var deployment k8sgateway.Deployment
	mustDecode(t, `{"metadata":{"name":"api","namespace":"demo","uid":"deploy-1"},"spec":{"replicas":3},"status":{"replicas":1,"readyReplicas":1,"availableReplicas":1,"updatedReplicas":1,"unavailableReplicas":2}}`, &deployment)
	observedAt := time.Date(2026, 7, 18, 9, 30, 0, 0, time.UTC)
	record, matched := EvaluateDeploymentReplicasUnavailable(7, deployment, observedAt)
	if !matched {
		t.Fatal("rule did not match")
	}
	augmented := WithNarrative(record)
	if len(augmented.Timeline) != 1 {
		t.Fatalf("timeline = %d entries, want 1", len(augmented.Timeline))
	}
	entry := augmented.Timeline[0]
	if entry.OccurredAt != observedAt.UTC().Format(time.RFC3339) {
		t.Fatalf("occurred_at = %q, want observedAt fallback", entry.OccurredAt)
	}
	if entry.Summary != "desired=3 ready=1 available=1" {
		t.Fatalf("summary = %q", entry.Summary)
	}
	if entry.Missing {
		t.Fatalf("deployment_status entry must not be missing: %#v", entry)
	}
	card := augmented.RootCauseCard
	if card == nil || card.FirstObservedAt != entry.OccurredAt {
		t.Fatalf("card = %#v", card)
	}
	if len(card.KeyEvidenceRefs) != 1 || card.KeyEvidenceRefs[0] != evidenceRef(record, 0, record.Evidence[0]) {
		t.Fatalf("key_evidence_refs = %v", card.KeyEvidenceRefs)
	}
}

func TestWithNarrativePodOOMKilledTimeline(t *testing.T) {
	var pod k8sgateway.Pod
	mustDecode(t, `{"metadata":{"name":"memory-api","namespace":"demo","uid":"pod-oom"},"status":{"phase":"Running","containerStatuses":[{"name":"app","restartCount":3,"lastState":{"terminated":{"reason":"OOMKilled","exitCode":137,"finishedAt":"2026-07-17T02:00:00Z"}}}]}}`, &pod)
	event := k8sgateway.Event{Type: "Warning", Reason: "OOMKilled", Message: "container was killed due to out of memory", Count: 2, LastTimestamp: "2026-07-17T02:05:00Z"}
	event.Metadata.Name = "oom-killed"
	record, matched := EvaluatePodOOMKilled(7, pod, []k8sgateway.Event{event}, time.Date(2026, 7, 17, 2, 10, 0, 0, time.UTC))
	if !matched {
		t.Fatal("rule did not match")
	}
	augmented := WithNarrative(record)
	if len(augmented.Timeline) != 2 {
		t.Fatalf("timeline = %d entries, want 2", len(augmented.Timeline))
	}
	termination, eventEntry := augmented.Timeline[0], augmented.Timeline[1]
	if termination.Type != "container_termination" || termination.OccurredAt != "2026-07-17T02:00:00Z" {
		t.Fatalf("termination entry = %#v", termination)
	}
	if eventEntry.Type != "event" || eventEntry.Category != CategoryEvent || eventEntry.OccurredAt != "2026-07-17T02:05:00Z" {
		t.Fatalf("event entry = %#v", eventEntry)
	}
	if eventEntry.Ref != "event/oom-killed" {
		t.Fatalf("event ref = %q", eventEntry.Ref)
	}
	card := augmented.RootCauseCard
	if card == nil || card.FirstObservedAt != "2026-07-17T02:00:00Z" {
		t.Fatalf("card = %#v", card)
	}
	if len(card.KeyEvidenceRefs) != 2 {
		t.Fatalf("key_evidence_refs = %v", card.KeyEvidenceRefs)
	}
}

func TestWithNarrativeServiceNoEndpointsTimeline(t *testing.T) {
	var service k8sgateway.ServiceResource
	var endpoints k8sgateway.Endpoints
	mustDecode(t, `{"metadata":{"name":"api","namespace":"demo","uid":"service-1"},"spec":{"type":"ClusterIP","selector":{"app":"api"},"ports":[{"port":80,"targetPort":8080,"protocol":"TCP"}]}}`, &service)
	mustDecode(t, `{"metadata":{"name":"api","namespace":"demo"},"subsets":[{"notReadyAddresses":[{"ip":"10.0.0.9"}]}]}`, &endpoints)
	observedAt := time.Date(2026, 7, 17, 3, 0, 0, 0, time.UTC)
	record, matched := EvaluateServiceNoEndpoints(7, service, endpoints, observedAt)
	if !matched {
		t.Fatal("rule did not match")
	}
	augmented := WithNarrative(record)
	if len(augmented.Timeline) != 2 {
		t.Fatalf("timeline = %d entries, want 2", len(augmented.Timeline))
	}
	if augmented.Timeline[0].Type != "service_spec" || augmented.Timeline[1].Type != "endpoints" {
		t.Fatalf("stable order broken: %#v", augmented.Timeline)
	}
	for _, entry := range augmented.Timeline {
		if entry.Missing || entry.OccurredAt != observedAt.UTC().Format(time.RFC3339) {
			t.Fatalf("entry = %#v", entry)
		}
	}
	if augmented.Timeline[1].Summary != "Service 无可用 Endpoints" {
		t.Fatalf("summary = %q", augmented.Timeline[1].Summary)
	}
	card := augmented.RootCauseCard
	if card == nil || card.Resource.Kind != "Service" || len(card.KeyEvidenceRefs) != 2 {
		t.Fatalf("card = %#v", card)
	}
}

func TestWithNarrativeMissingConditionMarked(t *testing.T) {
	var node k8sgateway.Node
	mustDecode(t, `{"metadata":{"name":"orphan-1"},"status":{"conditions":[{"type":"MemoryPressure","status":"True","lastTransitionTime":"2026-07-26T10:00:00Z"}]}}`, &node)
	record, matched := EvaluateNodeNotReady(7, node, time.Date(2026, 7, 26, 10, 5, 0, 0, time.UTC))
	if !matched {
		t.Fatal("rule did not match")
	}
	augmented := WithNarrative(record)
	var missing *TimelineEntry
	for i := range augmented.Timeline {
		if augmented.Timeline[i].Missing {
			missing = &augmented.Timeline[i]
		}
	}
	if missing == nil {
		t.Fatal("missing Ready condition not propagated")
	}
	if missing.Type != "node_condition" || missing.MissingReason != "ReadyConditionMissing" {
		t.Fatalf("missing entry = %#v", missing)
	}
	if missing.OccurredAt != "" {
		t.Fatalf("missing entry occurred_at = %q, want empty", missing.OccurredAt)
	}
	card := augmented.RootCauseCard
	if card == nil || card.FirstObservedAt != "2026-07-26T10:00:00Z" {
		t.Fatalf("card = %#v", card)
	}
	if len(card.KeyEvidenceRefs) != 1 {
		t.Fatalf("missing entry must be excluded from key refs: %v", card.KeyEvidenceRefs)
	}
}

func TestWithNarrativeIsPureProjection(t *testing.T) {
	var node k8sgateway.Node
	mustDecode(t, `{"metadata":{"name":"worker-1"},"status":{"conditions":[{"type":"Ready","status":"False","reason":"KubeletNotReady","lastTransitionTime":"2026-07-26T10:00:00Z"}]}}`, &node)
	record, matched := EvaluateNodeNotReady(7, node, time.Date(2026, 7, 26, 10, 2, 0, 0, time.UTC))
	if !matched {
		t.Fatal("rule did not match")
	}
	WithNarrative(record)
	if len(record.Timeline) != 0 || record.RootCauseCard != nil {
		t.Fatalf("WithNarrative mutated its argument: %#v", record)
	}
}
