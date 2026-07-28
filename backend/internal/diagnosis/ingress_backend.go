package diagnosis

import (
	"time"

	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

type IngressServiceRoute struct {
	Host        string
	Path        string
	ServiceName string
	PortName    string
	PortNumber  int32
}

type IngressBackendState struct {
	Service   k8sgateway.ServiceResource
	Endpoints k8sgateway.Endpoints
}

func IngressServiceRoutes(ingress k8sgateway.Ingress) []IngressServiceRoute {
	routes := make([]IngressServiceRoute, 0)
	if ingress.Spec.DefaultBackend != nil && ingress.Spec.DefaultBackend.Service != nil {
		backend := ingress.Spec.DefaultBackend.Service
		routes = append(routes, IngressServiceRoute{Path: "<default>", ServiceName: backend.Name, PortName: backend.Port.Name, PortNumber: backend.Port.Number})
	}
	for _, rule := range ingress.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, path := range rule.HTTP.Paths {
			if path.Backend.Service == nil {
				continue
			}
			backend := path.Backend.Service
			routes = append(routes, IngressServiceRoute{Host: rule.Host, Path: path.Path, ServiceName: backend.Name, PortName: backend.Port.Name, PortNumber: backend.Port.Number})
		}
	}
	return routes
}

func EvaluateIngressBackendUnavailable(clusterID int64, ingress k8sgateway.Ingress, routes []IngressServiceRoute, backends map[string]IngressBackendState, observedAt time.Time) (Record, bool) {
	evidence := make([]Evidence, 0)
	for _, route := range routes {
		backend, exists := backends[route.ServiceName]
		if !exists || backend.Service.Spec.ExternalName != "" {
			continue
		}
		ready, notReady := 0, 0
		for _, subset := range backend.Endpoints.Subsets {
			ready += len(subset.Addresses)
			notReady += len(subset.NotReadyAddresses)
		}
		if ready > 0 {
			continue
		}
		sourceAPI := backend.Endpoints.SourceAPI
		if sourceAPI == "" {
			sourceAPI = "core/v1"
		}
		evidence = append(evidence, Evidence{Type: "ingress_backend", Source: "ingress.spec + service + endpoints", Content: map[string]any{
			"host": route.Host, "path": route.Path, "service_name": route.ServiceName,
			"service_port_name": route.PortName, "service_port_number": route.PortNumber,
			"service_type": backend.Service.Spec.Type, "service_selector": backend.Service.Spec.Selector,
			"ready_addresses": ready, "not_ready_addresses": notReady,
			"subset_count": len(backend.Endpoints.Subsets), "source_api": sourceAPI,
		}})
	}
	if len(evidence) == 0 {
		return Record{}, false
	}
	return Record{
		ClusterID: clusterID, RuleID: RuleIngressBackendUnavailable, Severity: "high", Status: "open",
		Resource:        ResourceRef{Kind: "Ingress", Namespace: ingress.Metadata.Namespace, Name: ingress.Metadata.Name, UID: ingress.Metadata.UID},
		Summary:         "The Ingress has at least one Service backend with no Ready endpoint addresses.",
		RootCauses:      []string{"The backend Service selector has no Ready Pods", "Backend Pods are failing readiness or the Service port mapping is incorrect"},
		Recommendations: []string{"Inspect each unavailable backend route and its exact Service", "Check backend Pod readiness, selector labels, and Service target ports", "Confirm Ready endpoint addresses appear before directing production traffic"},
		Evidence:        evidence, ObservedAt: observedAt.UTC(),
	}, true
}
