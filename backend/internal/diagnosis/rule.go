package diagnosis

import (
	"fmt"
	"strings"
	"time"

	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

func EvaluateImagePullBackOff(clusterID int64, pod k8sgateway.Pod, events []k8sgateway.Event, observedAt time.Time) (Record, bool) {
	images := make(map[string]string, len(pod.Spec.Containers))
	for _, container := range pod.Spec.Containers {
		images[container.Name] = container.Image
	}
	evidence := make([]Evidence, 0, 4)
	matched := false
	for _, status := range pod.Status.ContainerStatuses {
		if status.State.Waiting == nil {
			continue
		}
		reason := status.State.Waiting.Reason
		if reason != "ImagePullBackOff" && reason != "ErrImagePull" {
			continue
		}
		matched = true
		evidence = append(evidence, Evidence{Type: "container_state", Source: "pod.status.containerStatuses", Content: map[string]any{
			"container": status.Name, "image": images[status.Name], "reason": reason,
			"message": status.State.Waiting.Message, "restart_count": status.RestartCount,
		}})
	}
	if !matched {
		return Record{}, false
	}
	for _, event := range events {
		message := strings.ToLower(event.Message)
		if event.Type != "Warning" || (event.Reason != "Failed" && event.Reason != "BackOff" && !strings.Contains(message, "pull")) {
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
		ClusterID: clusterID, RuleID: RuleImagePullBackOff, Severity: "high", Status: "open",
		Resource:        ResourceRef{Kind: "Pod", Namespace: pod.Metadata.Namespace, Name: pod.Metadata.Name, UID: pod.Metadata.UID},
		Summary:         "Pod 无法拉取容器镜像，容器处于 ImagePullBackOff/ErrImagePull 状态。",
		RootCauses:      []string{"镜像名称或标签不存在", "镜像仓库网络或 DNS 不可达", "私有仓库 imagePullSecret 缺失、失效或未绑定到 ServiceAccount"},
		Recommendations: []string{"核对工作负载中的镜像仓库、名称和 tag", "从集群节点验证仓库域名解析和网络连通性", "检查 Namespace 中的 imagePullSecret 以及 Pod/ServiceAccount 引用关系", "修正配置后观察新 Event，确认镜像拉取成功"},
		Evidence:        evidence, ObservedAt: observedAt.UTC(),
	}, true
}
