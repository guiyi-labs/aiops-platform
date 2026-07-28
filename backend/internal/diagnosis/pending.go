package diagnosis

import (
	"fmt"
	"strings"
	"time"

	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

// EvaluatePodPending matches a Pod that remains in the Kubernetes Pending phase.
// Scheduling conditions and FailedScheduling events are retained as evidence.
func EvaluatePodPending(clusterID int64, pod k8sgateway.Pod, events []k8sgateway.Event, observedAt time.Time) (Record, bool) {
	if pod.Status.Phase != "Pending" {
		return Record{}, false
	}
	evidence := []Evidence{
		{Type: "pod_status", Source: "pod.status", Content: map[string]any{
			"phase": pod.Status.Phase, "reason": pod.Status.Reason, "message": pod.Status.Message,
		}},
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type != "PodScheduled" || condition.Status != "False" {
			continue
		}
		evidence = append(evidence, Evidence{Type: "pod_condition", Source: "pod.status.conditions", Content: map[string]any{
			"type": condition.Type, "status": condition.Status, "reason": condition.Reason,
			"message": condition.Message, "last_transition_time": condition.LastTransitionTime,
		}})
	}
	for _, event := range events {
		message := strings.ToLower(event.Message)
		if event.Type != "Warning" || (event.Reason != "FailedScheduling" && !strings.Contains(message, "schedule")) {
			continue
		}
		evidence = append(evidence, Evidence{Type: "event", Source: fmt.Sprintf("event/%s", event.Metadata.Name), Content: map[string]any{
			"reason": event.Reason, "message": event.Message, "count": event.Count,
			"first_timestamp": event.FirstTimestamp, "last_timestamp": event.LastTimestamp,
		}})
		if len(evidence) >= 6 {
			break
		}
	}
	return Record{
		ClusterID: clusterID, RuleID: RulePodPending, Severity: "high", Status: "open",
		Resource: ResourceRef{Kind: "Pod", Namespace: pod.Metadata.Namespace, Name: pod.Metadata.Name, UID: pod.Metadata.UID},
		Summary:  "Pod 处于 Pending，尚未完成调度或依赖资源准备。",
		RootCauses: []string{
			"集群节点资源不足或调度约束不满足",
			"NodeSelector、亲和性、污点容忍或拓扑约束阻止调度",
			"关联 PVC、镜像或其他启动前依赖尚未就绪",
		},
		Recommendations: []string{
			"检查 PodScheduled 条件和 FailedScheduling Event 的具体原因",
			"对比节点可分配资源、污点和 Pod 的调度约束",
			"检查 PVC、StorageClass 以及其他启动前依赖的状态",
		},
		Evidence: evidence, ObservedAt: observedAt.UTC(),
	}, true
}
