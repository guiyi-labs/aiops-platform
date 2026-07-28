package diagnosis

import (
	"time"

	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

// EvaluateNodeNotReady matches a Node whose Ready condition is not True.
// The complete condition set is retained so operators can distinguish a
// kubelet failure from a transient pressure or network condition.
func EvaluateNodeNotReady(clusterID int64, node k8sgateway.Node, observedAt time.Time) (Record, bool) {
	readyFound := false
	ready := false
	evidence := make([]Evidence, 0, len(node.Status.Conditions)+1)
	for _, condition := range node.Status.Conditions {
		if condition.Type == "Ready" {
			readyFound = true
			ready = condition.Status == "True"
		}
		evidence = append(evidence, Evidence{Type: "node_condition", Source: "node.status.conditions", Content: map[string]any{
			"type": condition.Type, "status": condition.Status, "reason": condition.Reason,
			"message": condition.Message, "last_transition_time": condition.LastTransitionTime,
		}})
	}
	if readyFound && ready {
		return Record{}, false
	}
	if !readyFound {
		evidence = append(evidence, Evidence{Type: "node_condition", Source: "node.status.conditions", Content: map[string]any{
			"type": "Ready", "status": "Missing", "reason": "ReadyConditionMissing",
			"message": "Node does not report a Ready condition", "last_transition_time": "",
		}})
	}
	return Record{
		ClusterID: clusterID, RuleID: RuleNodeNotReady, Severity: "critical", Status: "open",
		Resource: ResourceRef{Kind: "Node", Name: node.Metadata.Name, UID: node.Metadata.UID},
		Summary:  "Node 未处于 Ready 状态，可能无法继续接收或承载工作负载。",
		RootCauses: []string{
			"kubelet 或容器运行时异常",
			"节点网络、磁盘、内存或 PID 压力阻止节点恢复就绪",
			"节点失联、被隔离，或 Ready Condition 长时间处于 Unknown",
		},
		Recommendations: []string{
			"检查 Ready Condition 的 Reason、Message 与最近转换时间",
			"检查 kubelet、容器运行时、节点压力 Conditions 和近期 Event",
			"确认节点连通性与容量恢复后，再解除隔离或重新调度工作负载",
		},
		Evidence: evidence, ObservedAt: observedAt.UTC(),
	}, true
}
