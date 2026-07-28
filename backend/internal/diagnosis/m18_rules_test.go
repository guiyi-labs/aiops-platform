package diagnosis

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

type m18Fixtures struct {
	Node []struct {
		Name     string          `json:"name"`
		Match    bool            `json:"match"`
		Resource k8sgateway.Node `json:"resource"`
	} `json:"node"`
	PersistentVolumeClaim []struct {
		Name     string                           `json:"name"`
		Match    bool                             `json:"match"`
		Resource k8sgateway.PersistentVolumeClaim `json:"resource"`
		Events   []k8sgateway.Event               `json:"events"`
	} `json:"persistent_volume_claim"`
	HorizontalPodAutoscaler []struct {
		Name     string                             `json:"name"`
		Match    bool                               `json:"match"`
		Resource k8sgateway.HorizontalPodAutoscaler `json:"resource"`
	} `json:"horizontal_pod_autoscaler"`
	Ingress []struct {
		Name      string                                `json:"name"`
		Match     bool                                  `json:"match"`
		Resource  k8sgateway.Ingress                    `json:"resource"`
		Services  map[string]k8sgateway.ServiceResource `json:"services"`
		Endpoints map[string]k8sgateway.Endpoints       `json:"endpoints"`
	} `json:"ingress"`
}

func TestM18ReplayableDiagnosisFixtures(t *testing.T) {
	raw, err := os.ReadFile("testdata/m18-fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures m18Fixtures
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	observedAt := time.Date(2026, 7, 27, 8, 30, 0, 0, time.UTC)

	for _, fixture := range fixtures.Node {
		t.Run("node/"+fixture.Name, func(t *testing.T) {
			record, matched := EvaluateNodePressure(7, fixture.Resource, observedAt)
			assertM18Fixture(t, fixture.Match, RuleNodePressure, record, matched)
		})
	}
	for _, fixture := range fixtures.PersistentVolumeClaim {
		t.Run("pvc/"+fixture.Name, func(t *testing.T) {
			record, matched := EvaluatePersistentVolumeClaimPending(7, fixture.Resource, fixture.Events, observedAt)
			assertM18Fixture(t, fixture.Match, RulePersistentVolumeClaimPending, record, matched)
		})
	}
	for _, fixture := range fixtures.HorizontalPodAutoscaler {
		t.Run("hpa/"+fixture.Name, func(t *testing.T) {
			record, matched := EvaluateHorizontalPodAutoscalerSaturated(7, fixture.Resource, observedAt)
			assertM18Fixture(t, fixture.Match, RuleHorizontalPodAutoscalerSaturated, record, matched)
		})
	}
	for _, fixture := range fixtures.Ingress {
		t.Run("ingress/"+fixture.Name, func(t *testing.T) {
			states := make(map[string]IngressBackendState, len(fixture.Services))
			for name, service := range fixture.Services {
				states[name] = IngressBackendState{Service: service, Endpoints: fixture.Endpoints[name]}
			}
			record, matched := EvaluateIngressBackendUnavailable(7, fixture.Resource, IngressServiceRoutes(fixture.Resource), states, observedAt)
			assertM18Fixture(t, fixture.Match, RuleIngressBackendUnavailable, record, matched)
		})
	}
}

func assertM18Fixture(t *testing.T, wantMatch bool, ruleID string, record Record, matched bool) {
	t.Helper()
	if matched != wantMatch {
		t.Fatalf("matched = %v, want %v; record = %#v", matched, wantMatch, record)
	}
	if wantMatch && (record.RuleID != ruleID || len(record.Evidence) == 0 || !record.ObservedAt.Equal(time.Date(2026, 7, 27, 8, 30, 0, 0, time.UTC))) {
		t.Fatalf("record = %#v", record)
	}
}

func TestNodeNotReadyRemainsAuthoritativeOverPressure(t *testing.T) {
	var node k8sgateway.Node
	if err := json.Unmarshal([]byte(`{"metadata":{"name":"worker"},"status":{"conditions":[{"type":"Ready","status":"False"},{"type":"DiskPressure","status":"True"}]}}`), &node); err != nil {
		t.Fatal(err)
	}
	if _, matched := EvaluateNodePressure(1, node, time.Now()); matched {
		t.Fatal("pressure rule matched a non-Ready Node")
	}
	record, matched := EvaluateNodeNotReady(1, node, time.Now())
	if !matched || record.RuleID != RuleNodeNotReady {
		t.Fatalf("record = %#v, matched = %v", record, matched)
	}
}
