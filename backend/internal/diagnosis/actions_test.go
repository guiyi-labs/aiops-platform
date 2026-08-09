package diagnosis

import (
	"testing"
	"time"

	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

func TestBuildActionAreaPodIncludesControlledAction(t *testing.T) {
	var pod k8sgateway.Pod
	mustDecode(t, `{"metadata":{"name":"memory-api","namespace":"demo"},"status":{"containerStatuses":[{"name":"app","restartCount":3,"lastState":{"terminated":{"reason":"OOMKilled","exitCode":137,"finishedAt":"2026-07-17T02:00:00Z"}}}]}}`, &pod)
	record, matched := EvaluatePodOOMKilled(7, pod, []k8sgateway.Event{}, time.Date(2026, 7, 17, 2, 10, 0, 0, time.UTC))
	if !matched {
		t.Fatal("rule did not match")
	}
	actions := buildActionArea(record)
	if len(actions) != len(record.Recommendations)+1 {
		t.Fatalf("actions = %d, want recommendations+1", len(actions))
	}
	for i, action := range actions[:len(record.Recommendations)] {
		if action.Kind != ActionKindAdvisory || action.Title != record.Recommendations[i] {
			t.Fatalf("advisory %d = %#v", i, action)
		}
	}
	controlled := actions[len(record.Recommendations)]
	if controlled.Kind != ActionKindControlled || controlled.Action != "deployment.rollout_restart" {
		t.Fatalf("controlled = %#v", controlled)
	}
	if !controlled.RequiresDryRun || !controlled.RequiresConfirmation {
		t.Fatalf("controlled action must require dry-run and confirmation: %#v", controlled)
	}
}

func TestBuildActionAreaServiceAdvisoryOnly(t *testing.T) {
	var service k8sgateway.ServiceResource
	var endpoints k8sgateway.Endpoints
	mustDecode(t, `{"metadata":{"name":"api","namespace":"demo"},"spec":{"type":"ClusterIP","selector":{"app":"api"}}}`, &service)
	mustDecode(t, `{"subsets":[{"notReadyAddresses":[{"ip":"10.0.0.9"}]}]}`, &endpoints)
	record, matched := EvaluateServiceNoEndpoints(7, service, endpoints, time.Date(2026, 7, 17, 3, 0, 0, 0, time.UTC))
	if !matched {
		t.Fatal("rule did not match")
	}
	actions := buildActionArea(record)
	if len(actions) != len(record.Recommendations) {
		t.Fatalf("actions = %d, want %d advisory only", len(actions), len(record.Recommendations))
	}
	for _, action := range actions {
		if action.Kind != ActionKindAdvisory {
			t.Fatalf("unexpected kind %q", action.Kind)
		}
	}
}

func TestWithNarrativeCarriesActions(t *testing.T) {
	var node k8sgateway.Node
	mustDecode(t, `{"metadata":{"name":"worker-1"},"status":{"conditions":[{"type":"Ready","status":"False","reason":"KubeletNotReady","lastTransitionTime":"2026-07-26T10:00:00Z"}]}}`, &node)
	record, matched := EvaluateNodeNotReady(7, node, time.Date(2026, 7, 26, 10, 2, 0, 0, time.UTC))
	if !matched {
		t.Fatal("rule did not match")
	}
	augmented := WithNarrative(record)
	if len(augmented.Actions) != len(record.Recommendations) {
		t.Fatalf("actions = %d entries, want advisory-only %d", len(augmented.Actions), len(record.Recommendations))
	}
	for _, action := range augmented.Actions {
		if action.Kind != ActionKindAdvisory {
			t.Fatalf("unexpected kind %q", action.Kind)
		}
	}
}

func TestBuildActionAreaEmptyRecommendations(t *testing.T) {
	record := Record{Resource: ResourceRef{Kind: "Pod"}}
	actions := buildActionArea(record)
	if len(actions) != 1 || actions[0].Kind != ActionKindControlled {
		t.Fatalf("pod with no recommendations must still surface the controlled capability: %#v", actions)
	}
	serviceRecord := Record{Resource: ResourceRef{Kind: "Service"}}
	if actions := buildActionArea(serviceRecord); len(actions) != 0 {
		t.Fatalf("service without recommendations must have no actions: %#v", actions)
	}
}
