package diagnosis

import (
	"time"

	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

// EvaluateDeploymentReplicasUnavailable matches a Deployment whose desired
// replicas are not both ready and available.
func EvaluateDeploymentReplicasUnavailable(clusterID int64, deployment k8sgateway.Deployment, observedAt time.Time) (Record, bool) {
	desired := int32(1)
	if deployment.Spec.Replicas != nil {
		desired = *deployment.Spec.Replicas
	}
	ready := deployment.Status.ReadyReplicas
	available := deployment.Status.AvailableReplicas
	if desired <= ready && desired <= available {
		return Record{}, false
	}
	evidence := []Evidence{{Type: "deployment_status", Source: "deployment.status", Content: map[string]any{
		"desired_replicas":     desired,
		"replicas":             deployment.Status.Replicas,
		"ready_replicas":       ready,
		"available_replicas":   available,
		"updated_replicas":     deployment.Status.UpdatedReplicas,
		"unavailable_replicas": deployment.Status.UnavailableReplicas,
	}}}
	return Record{
		ClusterID: clusterID, RuleID: RuleDeploymentReplicasUnavailable, Severity: "high", Status: "open",
		Resource: ResourceRef{Kind: "Deployment", Namespace: deployment.Metadata.Namespace, Name: deployment.Metadata.Name, UID: deployment.Metadata.UID},
		Summary:  "Deployment 的 Ready 或 Available 副本数低于期望值。",
		RootCauses: []string{
			"Pod 处于 Pending、未通过就绪检查或反复重启",
			"集群容量或调度约束导致副本无法完成调度",
			"滚动发布停滞，当前版本无法达到 Available 状态",
		},
		Recommendations: []string{
			"对比期望、已更新、Ready、Available 与 Unavailable 副本计数",
			"检查 Deployment 关联 Pod、Event、就绪探针和镜像拉取状态",
			"修改副本数或镜像配置前，先确认节点容量与滚动发布 Conditions",
		},
		Evidence: evidence, ObservedAt: observedAt.UTC(),
	}, true
}
