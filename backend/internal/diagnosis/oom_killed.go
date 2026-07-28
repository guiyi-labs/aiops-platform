package diagnosis

import (
	"fmt"
	"strings"
	"time"

	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

// EvaluatePodOOMKilled matches current or previous container termination caused by OOMKilled.
func EvaluatePodOOMKilled(clusterID int64, pod k8sgateway.Pod, events []k8sgateway.Event, observedAt time.Time) (Record, bool) {
	evidence := make([]Evidence, 0, 6)
	matched := false
	for _, status := range pod.Status.ContainerStatuses {
		termination := status.State.Terminated
		source := "state.terminated"
		if termination == nil || !strings.EqualFold(termination.Reason, "OOMKilled") {
			termination = status.LastState.Terminated
			source = "last_state.terminated"
		}
		if termination == nil || !strings.EqualFold(termination.Reason, "OOMKilled") {
			continue
		}
		matched = true
		evidence = append(evidence, Evidence{Type: "container_termination", Source: "pod.status.containerStatuses." + source, Content: map[string]any{
			"container": status.Name, "reason": termination.Reason, "message": termination.Message,
			"exit_code": termination.ExitCode, "signal": termination.Signal, "restart_count": status.RestartCount,
			"started_at": termination.StartedAt, "finished_at": termination.FinishedAt,
		}})
	}
	if !matched {
		return Record{}, false
	}
	for _, event := range events {
		message := strings.ToLower(event.Message)
		if event.Type != "Warning" || (!strings.Contains(strings.ToLower(event.Reason), "oom") && !strings.Contains(message, "out of memory")) {
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
		ClusterID: clusterID, RuleID: RulePodOOMKilled, Severity: "critical", Status: "open",
		Resource: ResourceRef{Kind: "Pod", Namespace: pod.Metadata.Namespace, Name: pod.Metadata.Name, UID: pod.Metadata.UID},
		Summary:  "Pod 容器曾因 OOMKilled 被系统终止，存在内存压力或内存限制配置问题。",
		RootCauses: []string{
			"容器实际内存使用超过 limits.memory",
			"节点内存压力导致容器被内核或 kubelet 驱逐",
			"应用内存泄漏或并发峰值超过当前资源配置",
		},
		Recommendations: []string{
			"对比容器内存使用趋势与 requests/limits 配置",
			"检查 Node MemoryPressure、驱逐 Event 和同节点其他工作负载",
			"结合应用堆、缓存和并发配置评估扩容或限流方案",
		},
		Evidence: evidence, ObservedAt: observedAt.UTC(),
	}, true
}
