package diagnosis

import (
	"time"

	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

func EvaluateServiceNoEndpoints(clusterID int64, service k8sgateway.ServiceResource, endpoints k8sgateway.Endpoints, observedAt time.Time) (Record, bool) {
	if service.Spec.ExternalName != "" || len(service.Spec.Selector) == 0 {
		return Record{}, false
	}
	ready, notReady := 0, 0
	for _, subset := range endpoints.Subsets {
		ready += len(subset.Addresses)
		notReady += len(subset.NotReadyAddresses)
	}
	if ready > 0 {
		return Record{}, false
	}
	ports := make([]map[string]any, 0, len(service.Spec.Ports))
	for _, port := range service.Spec.Ports {
		ports = append(ports, map[string]any{"name": port.Name, "port": port.Port, "protocol": port.Protocol, "target_port": port.TargetPort})
	}
	sourceAPI := endpoints.SourceAPI
	if sourceAPI == "" {
		sourceAPI = "core/v1"
	}
	evidence := []Evidence{
		{Type: "service_spec", Source: "service.spec", Content: map[string]any{
			"type": service.Spec.Type, "selector": service.Spec.Selector, "ports": ports,
		}},
		{Type: "endpoints", Source: "endpoints.subsets", Content: map[string]any{
			"ready_addresses": ready, "not_ready_addresses": notReady, "subset_count": len(endpoints.Subsets), "source_api": sourceAPI,
		}},
	}
	return Record{
		ClusterID: clusterID, RuleID: RuleServiceNoEndpoints, Severity: "high", Status: "open",
		Resource: ResourceRef{Kind: "Service", Namespace: service.Metadata.Namespace, Name: service.Metadata.Name, UID: service.Metadata.UID},
		Summary:  "Service 使用标签选择器，但当前没有可接收流量的 Ready Endpoint。",
		RootCauses: []string{
			"Service selector 与目标 Pod 标签不匹配",
			"匹配的 Pod 尚未 Ready 或已停止运行",
			"Service targetPort 与容器实际监听端口配置不一致",
		},
		Recommendations: []string{
			"对比 Service selector 与目标 Pod labels",
			"检查匹配 Pod 的 Ready 状态、Readiness Probe 和相关 Event",
			"核对 Service targetPort 与容器监听端口",
			"修正配置后确认 Endpoints 出现 Ready addresses",
		},
		Evidence: evidence, ObservedAt: observedAt.UTC(),
	}, true
}
