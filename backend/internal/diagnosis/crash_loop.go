package diagnosis

import (
	"fmt"
	"strings"
	"time"

	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

func EvaluateCrashLoopBackOff(clusterID int64, pod k8sgateway.Pod, events []k8sgateway.Event, observedAt time.Time) (Record, bool) {
	evidence := make([]Evidence, 0, 6)
	matched := false
	for _, status := range pod.Status.ContainerStatuses {
		if status.State.Waiting == nil || status.State.Waiting.Reason != "CrashLoopBackOff" {
			continue
		}
		matched = true
		content := map[string]any{
			"container": status.Name, "reason": status.State.Waiting.Reason,
			"message": status.State.Waiting.Message, "restart_count": status.RestartCount,
		}
		if status.LastState.Terminated != nil {
			terminated := status.LastState.Terminated
			content["last_termination"] = map[string]any{
				"reason": terminated.Reason, "message": terminated.Message, "exit_code": terminated.ExitCode,
				"signal": terminated.Signal, "started_at": terminated.StartedAt, "finished_at": terminated.FinishedAt,
			}
		}
		evidence = append(evidence, Evidence{Type: "container_state", Source: "pod.status.containerStatuses", Content: content})
	}
	if !matched {
		return Record{}, false
	}
	for _, event := range events {
		message := strings.ToLower(event.Message)
		if event.Type != "Warning" || (event.Reason != "BackOff" && !strings.Contains(message, "back-off restarting")) {
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
		ClusterID: clusterID, RuleID: RuleCrashLoopBackOff, Severity: "high", Status: "open",
		Resource: ResourceRef{Kind: "Pod", Namespace: pod.Metadata.Namespace, Name: pod.Metadata.Name, UID: pod.Metadata.UID},
		Summary:  "Pod 容器反复启动失败，当前处于 CrashLoopBackOff 状态。",
		RootCauses: []string{
			"应用启动命令、参数或配置错误导致进程退出",
			"依赖服务、配置文件或 Secret 不可用",
			"健康检查配置不当或资源限制触发进程终止",
		},
		Recommendations: []string{
			"检查 last_termination 的退出码、原因和结束时间",
			"读取 previous 容器日志定位上一次退出前的错误",
			"核对 ConfigMap、Secret、启动参数和依赖服务连通性",
			"检查资源限制与健康检查配置，修正后观察重启次数和新 Event",
		},
		Evidence: evidence, ObservedAt: observedAt.UTC(),
	}, true
}
