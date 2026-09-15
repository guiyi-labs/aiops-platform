package diagnosis

// Covers summarizeHPAMetric, the pure projection used to build the HPA
// saturation evidence payload. Each HPA metric source (Resource,
// ContainerResource, Pods, Object, External) has its own branch.

import (
	"testing"
	"time"

	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

func TestSummarizeHPAMetricPerSource(t *testing.T) {
	cases := []struct {
		name   string
		raw    string
		fields map[string]any
	}{
		{
			name: "resource",
			raw:  `{"type":"Resource","resource":{"name":"cpu","target":{"type":"Utilization","averageUtilization":70}}}`,
			fields: map[string]any{
				"type": "Resource", "name": "cpu",
			},
		},
		{
			name: "container resource",
			raw:  `{"type":"ContainerResource","containerResource":{"name":"memory","container":"app","target":{"type":"AverageValue"}}}`,
			fields: map[string]any{
				"type": "ContainerResource", "name": "memory", "container": "app",
			},
		},
		{
			name: "pods",
			raw:  `{"type":"Pods","pods":{"metric":{"name":"requests_per_second"},"target":{"type":"AverageValue"}}}`,
			fields: map[string]any{
				"type": "Pods", "name": "requests_per_second",
			},
		},
		{
			name: "object",
			raw:  `{"type":"Object","object":{"describedObject":{"kind":"Service","name":"api"},"metric":{"name":"hits"},"target":{"type":"Value"}}}`,
			fields: map[string]any{
				"type": "Object", "name": "hits",
			},
		},
		{
			name: "external",
			raw:  `{"type":"External","external":{"metric":{"name":"queue_depth"},"target":{"type":"Value"}}}`,
			fields: map[string]any{
				"type": "External", "name": "queue_depth",
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var hpa k8sgateway.HorizontalPodAutoscaler
			mustDecode(t, `{"spec":{"metrics":[`+testCase.raw+`]}}`, &hpa)
			if len(hpa.Spec.Metrics) != 1 {
				t.Fatalf("metrics = %#v", hpa.Spec.Metrics)
			}
			got := summarizeHPAMetric(hpa.Spec.Metrics[0])
			for key, want := range testCase.fields {
				if got[key] != want {
					t.Fatalf("metric[%q] = %#v, want %#v (full: %#v)", key, got[key], want, got)
				}
			}
			// Every branch must carry the target through untouched.
			if _, ok := got["target"]; !ok {
				t.Fatalf("target missing from %#v", got)
			}
		})
	}
}

func TestSummarizeHPAMetricWithoutKnownSource(t *testing.T) {
	// A metric spec whose source is absent (or an unknown future type) must
	// degrade to just the type instead of panicking.
	got := summarizeHPAMetric(k8sgateway.HPAMetricSpec{Type: "Unknown"})
	if len(got) != 1 || got["type"] != "Unknown" {
		t.Fatalf("metric = %#v", got)
	}
	if _, ok := got["name"]; ok {
		t.Fatalf("unexpected name in %#v", got)
	}
}

func TestEvaluateHPASaturatedRequiresBothSignals(t *testing.T) {
	observedAt := time.Date(2026, 7, 27, 8, 30, 0, 0, time.UTC)

	// At maxReplicas but without the ScalingLimited/TooManyReplicas condition.
	var atMaxOnly k8sgateway.HorizontalPodAutoscaler
	mustDecode(t, `{"metadata":{"name":"api","namespace":"demo"},"spec":{"maxReplicas":4,"metrics":[{"type":"Resource","resource":{"name":"cpu","target":{"type":"Utilization","averageUtilization":70}}}]},"status":{"currentReplicas":4,"desiredReplicas":4}}`, &atMaxOnly)
	if _, matched := EvaluateHorizontalPodAutoscalerSaturated(7, atMaxOnly, observedAt); matched {
		t.Fatal("saturation matched without the ScalingLimited condition")
	}

	// Limited by TooManyReplicas but still below maxReplicas.
	var limitedOnly k8sgateway.HorizontalPodAutoscaler
	mustDecode(t, `{"metadata":{"name":"api","namespace":"demo"},"spec":{"maxReplicas":10},"status":{"currentReplicas":2,"desiredReplicas":2,"conditions":[{"type":"ScalingLimited","status":"True","reason":"TooManyReplicas"}]}}`, &limitedOnly)
	if _, matched := EvaluateHorizontalPodAutoscalerSaturated(7, limitedOnly, observedAt); matched {
		t.Fatal("saturation matched below maxReplicas")
	}

	// Both signals present: the metric summary must reach the evidence.
	var saturated k8sgateway.HorizontalPodAutoscaler
	mustDecode(t, `{"metadata":{"name":"api","namespace":"demo","uid":"hpa-1"},"spec":{"maxReplicas":4,"metrics":[{"type":"External","external":{"metric":{"name":"queue_depth"},"target":{"type":"Value"}}}]},"status":{"currentReplicas":4,"desiredReplicas":4,"conditions":[{"type":"ScalingLimited","status":"True","reason":"TooManyReplicas","message":"above maximum"}]}}`, &saturated)
	record, matched := EvaluateHorizontalPodAutoscalerSaturated(7, saturated, observedAt)
	if !matched || record.RuleID != RuleHorizontalPodAutoscalerSaturated {
		t.Fatalf("record = %#v, matched = %v", record, matched)
	}
	// minReplicas unset must default to 1 in the evidence payload.
	metrics, ok := record.Evidence[0].Content["metrics"].([]map[string]any)
	if !ok || len(metrics) != 1 || metrics[0]["name"] != "queue_depth" {
		t.Fatalf("metrics evidence = %#v", record.Evidence[0].Content["metrics"])
	}
	if record.Evidence[0].Content["min_replicas"] != int32(1) {
		t.Fatalf("min replicas = %#v", record.Evidence[0].Content["min_replicas"])
	}
}
