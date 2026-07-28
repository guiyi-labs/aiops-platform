package diagnosis

import (
	"time"

	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

func EvaluateHorizontalPodAutoscalerSaturated(clusterID int64, hpa k8sgateway.HorizontalPodAutoscaler, observedAt time.Time) (Record, bool) {
	atMaximum := hpa.Spec.MaxReplicas > 0 && (hpa.Status.CurrentReplicas >= hpa.Spec.MaxReplicas || hpa.Status.DesiredReplicas >= hpa.Spec.MaxReplicas)
	var limited *k8sgateway.WorkloadCondition
	for index := range hpa.Status.Conditions {
		condition := &hpa.Status.Conditions[index]
		if condition.Type == "ScalingLimited" && condition.Status == "True" && condition.Reason == "TooManyReplicas" {
			limited = condition
			break
		}
	}
	if !atMaximum || limited == nil {
		return Record{}, false
	}
	minReplicas := int32(1)
	if hpa.Spec.MinReplicas != nil {
		minReplicas = *hpa.Spec.MinReplicas
	}
	metrics := make([]map[string]any, 0, len(hpa.Spec.Metrics))
	for _, metric := range hpa.Spec.Metrics {
		metrics = append(metrics, summarizeHPAMetric(metric))
	}
	evidence := []Evidence{
		{Type: "hpa_scale", Source: "horizontalpodautoscaler.spec,status", Content: map[string]any{
			"target_api_version": hpa.Spec.ScaleTargetRef.APIVersion, "target_kind": hpa.Spec.ScaleTargetRef.Kind,
			"target_name": hpa.Spec.ScaleTargetRef.Name, "min_replicas": minReplicas, "max_replicas": hpa.Spec.MaxReplicas,
			"current_replicas": hpa.Status.CurrentReplicas, "desired_replicas": hpa.Status.DesiredReplicas, "metrics": metrics,
		}},
		{Type: "hpa_condition", Source: "horizontalpodautoscaler.status.conditions", Content: map[string]any{
			"type": limited.Type, "status": limited.Status, "reason": limited.Reason,
			"message": limited.Message, "last_transition_time": limited.LastTransitionTime,
		}},
	}
	return Record{
		ClusterID: clusterID, RuleID: RuleHorizontalPodAutoscalerSaturated, Severity: "high", Status: "open",
		Resource:        ResourceRef{Kind: "HorizontalPodAutoscaler", Namespace: hpa.Metadata.Namespace, Name: hpa.Metadata.Name, UID: hpa.Metadata.UID},
		Summary:         "The HorizontalPodAutoscaler reached maxReplicas and is limited by TooManyReplicas.",
		RootCauses:      []string{"Observed load requires more replicas than the configured maximum", "The scaling target may be constrained below the capacity required by its metric targets"},
		Recommendations: []string{"Validate the declared metrics and current workload demand", "Confirm cluster capacity and workload safety before raising maxReplicas", "Investigate sustained load or inefficient workload resource usage"},
		Evidence:        evidence, ObservedAt: observedAt.UTC(),
	}, true
}

func summarizeHPAMetric(metric k8sgateway.HPAMetricSpec) map[string]any {
	result := map[string]any{"type": metric.Type}
	switch {
	case metric.Resource != nil:
		result["name"] = metric.Resource.Name
		result["target"] = metric.Resource.Target
	case metric.ContainerResource != nil:
		result["name"] = metric.ContainerResource.Name
		result["container"] = metric.ContainerResource.Container
		result["target"] = metric.ContainerResource.Target
	case metric.Pods != nil:
		result["name"] = metric.Pods.Metric.Name
		result["target"] = metric.Pods.Target
	case metric.Object != nil:
		result["name"] = metric.Object.Metric.Name
		result["target"] = metric.Object.Target
	case metric.External != nil:
		result["name"] = metric.External.Metric.Name
		result["target"] = metric.External.Target
	}
	return result
}
