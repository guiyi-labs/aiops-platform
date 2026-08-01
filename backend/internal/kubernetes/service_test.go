package kubernetes

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"testing"

	"k8s-aiops.local/backend/internal/apiquery"
	"k8s-aiops.local/backend/internal/cluster"
)

type credentialStub struct{ enabled bool }

func (s credentialStub) Access(context.Context, int64) (cluster.Cluster, []byte, error) {
	if !s.enabled {
		return cluster.Cluster{}, nil, cluster.ErrDisabled
	}
	return cluster.Cluster{ID: 7, Enabled: true}, []byte("config"), nil
}

type gatewayStub struct {
	body      []byte
	err       error
	path      string
	paths     []string
	query     url.Values
	responses map[string]gatewayResponse
	patchBody []byte
	dryRun    string
	patchType string
}

func (s *gatewayStub) Patch(_ context.Context, _ int64, _ []byte, path string, query url.Values, contentType string, body []byte, _ int64) ([]byte, error) {
	s.path, s.query, s.patchBody, s.patchType = path, query, append([]byte(nil), body...), contentType
	if response, ok := s.responses["PATCH "+path]; ok {
		return response.body, response.err
	}
	return s.body, s.err
}

type gatewayResponse struct {
	body []byte
	err  error
}

func (s *gatewayStub) Get(_ context.Context, _ int64, _ []byte, path string, query url.Values, _ int64) ([]byte, error) {
	s.path, s.query = path, query
	s.paths = append(s.paths, path)
	if response, ok := s.responses[path]; ok {
		return response.body, response.err
	}
	return s.body, s.err
}

func TestPodsFiltersAndPaginates(t *testing.T) {
	gateway := &gatewayStub{body: []byte(`{"items":[{"metadata":{"name":"api-2","namespace":"prod"}},{"metadata":{"name":"api-1","namespace":"prod"}},{"metadata":{"name":"worker","namespace":"prod"}}]}`)}
	service := NewService(credentialStub{enabled: true}, gateway, nil)
	response, err := service.Pods(context.Background(), 7, "prod", apiquery.ListQuery{Page: 1, Limit: 1, Name: "api", SortBy: "name", Ascending: true})
	if err != nil {
		t.Fatal(err)
	}
	if response.Total != 2 || response.Remaining != 1 || len(response.Items) != 1 || response.Items[0].Metadata.Name != "api-1" {
		t.Fatalf("response = %#v", response)
	}
	if gateway.path != "/api/v1/namespaces/prod/pods" {
		t.Fatalf("path = %q", gateway.path)
	}
}

func TestSelectorsAreForwarded(t *testing.T) {
	gateway := &gatewayStub{body: []byte(`{"items":[]}`)}
	service := NewService(credentialStub{enabled: true}, gateway, nil)
	_, err := service.Namespaces(context.Background(), 7, apiquery.ListQuery{Page: 1, Limit: 20, LabelSelector: "team=platform", FieldSelector: "status.phase=Active"})
	if err != nil {
		t.Fatal(err)
	}
	if gateway.query.Get("labelSelector") != "team=platform" || gateway.query.Get("fieldSelector") != "status.phase=Active" {
		t.Fatalf("query = %v", gateway.query)
	}
}

func TestNodeMetricsExposeFixedContract(t *testing.T) {
	gateway := &gatewayStub{body: []byte(`{"kind":"NodeMetricsList","apiVersion":"metrics.k8s.io/v1beta1","metadata":{"resourceVersion":"ignored"},"items":[{"metadata":{"name":"worker-1","labels":{"role":"worker"}},"timestamp":"2026-07-27T06:00:00Z","window":"30s","usage":{"cpu":"125m","memory":"512Mi"},"unexpected":"hidden"}]}`)}
	response, err := NewService(credentialStub{enabled: true}, gateway, nil).NodeMetrics(context.Background(), 7, apiquery.ListQuery{Page: 1, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(response)
	if gateway.path != "/apis/metrics.k8s.io/v1beta1/nodes" || response.Total != 1 || strings.Contains(string(encoded), "unexpected") || strings.Contains(string(encoded), "apiVersion") {
		t.Fatalf("path=%q response=%s", gateway.path, encoded)
	}
	if response.Items[0].Usage.CPU != "125m" || response.Items[0].Usage.Memory != "512Mi" {
		t.Fatalf("usage = %#v", response.Items[0].Usage)
	}
}

func TestPodMetricsUseBoundedNamespacePathAndNormalizeContainers(t *testing.T) {
	gateway := &gatewayStub{body: []byte(`{"items":[{"metadata":{"name":"api-1","namespace":"team one"},"timestamp":"2026-07-27T06:00:00Z","window":"20s","containers":null}]}`)}
	response, err := NewService(credentialStub{enabled: true}, gateway, nil).PodMetrics(context.Background(), 7, "team one", apiquery.ListQuery{Page: 1, Limit: 20, Name: "api"})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(response)
	if gateway.path != "/apis/metrics.k8s.io/v1beta1/namespaces/team%20one/pods" || string(encoded) != `{"items":[{"metadata":{"name":"api-1","namespace":"team one"},"timestamp":"2026-07-27T06:00:00Z","window":"20s","containers":[]}],"total":1,"remaining":0}` {
		t.Fatalf("path=%q response=%s", gateway.path, encoded)
	}
}

func TestMetricsAPINotFoundHasExplicitCapabilityError(t *testing.T) {
	gateway := &gatewayStub{err: cluster.APIStatusError{StatusCode: 404}}
	_, err := NewService(credentialStub{enabled: true}, gateway, nil).NodeMetrics(context.Background(), 7, apiquery.ListQuery{Page: 1, Limit: 20})
	if !errors.Is(err, ErrMetricsAPIUnavailable) {
		t.Fatalf("error = %v", err)
	}
}

func TestPodMetricsKeepResourceNotFoundWhenMetricsAPIExists(t *testing.T) {
	podPath := "/apis/metrics.k8s.io/v1beta1/namespaces/missing/pods"
	gateway := &gatewayStub{responses: map[string]gatewayResponse{
		podPath:                        {err: cluster.APIStatusError{StatusCode: 404}},
		"/apis/metrics.k8s.io/v1beta1": {body: []byte(`{"groupVersion":"metrics.k8s.io/v1beta1"}`)},
	}}
	_, err := NewService(credentialStub{enabled: true}, gateway, nil).PodMetrics(context.Background(), 7, "missing", apiquery.ListQuery{Page: 1, Limit: 20})
	if !errors.Is(err, ErrResourceNotFound) || errors.Is(err, ErrMetricsAPIUnavailable) {
		t.Fatalf("error = %v", err)
	}
	if len(gateway.paths) != 2 || gateway.paths[1] != "/apis/metrics.k8s.io/v1beta1" {
		t.Fatalf("paths = %v", gateway.paths)
	}
}

func TestEventsPreserveModernObservationFieldsAndFilterByResourceName(t *testing.T) {
	gateway := &gatewayStub{body: []byte(`{"items":[{"metadata":{"name":"checkout.1","namespace":"payments"},"type":"Warning","reason":"BackOff","message":"restarting failed container","action":"BackOff","eventTime":"2026-07-26T12:00:00Z","reportingComponent":"kubelet","reportingInstance":"worker-1","series":{"count":4,"lastObservedTime":"2026-07-26T12:03:00Z"},"involvedObject":{"kind":"Pod","namespace":"payments","name":"checkout-1"}},{"metadata":{"name":"worker.1"},"type":"Normal","reason":"Pulled","involvedObject":{"kind":"Pod","namespace":"payments","name":"worker-1"}}]}`)}
	response, err := NewService(credentialStub{enabled: true}, gateway, nil).Events(context.Background(), 7, "payments", apiquery.ListQuery{Page: 1, Limit: 100, Name: "checkout"})
	if err != nil {
		t.Fatal(err)
	}
	if gateway.path != "/api/v1/namespaces/payments/events" || response.Total != 1 || len(response.Items) != 1 {
		t.Fatalf("path=%q response=%#v", gateway.path, response)
	}
	event := response.Items[0]
	if event.EventTime != "2026-07-26T12:00:00Z" || event.Series.Count != 4 || event.Series.LastObservedTime != "2026-07-26T12:03:00Z" || event.ReportingComponent != "kubelet" || event.ReportingInstance != "worker-1" {
		t.Fatalf("event observation fields = %#v", event)
	}
}

func TestLogsAreBoundedAndScoped(t *testing.T) {
	gateway := &gatewayStub{body: []byte("line one\nline two")}
	service := NewService(credentialStub{enabled: true}, gateway, nil)
	logs, err := service.Logs(context.Background(), 7, "prod", "api-1", "app", true, 50)
	if err != nil || logs != "line one\nline two" {
		t.Fatalf("Logs() = %q, %v", logs, err)
	}
	if gateway.path != "/api/v1/namespaces/prod/pods/api-1/log" || gateway.query.Get("container") != "app" || gateway.query.Get("previous") != "true" || gateway.query.Get("tailLines") != "50" {
		t.Fatalf("request = %q %v", gateway.path, gateway.query)
	}
}

func TestDisabledClusterIsRejected(t *testing.T) {
	service := NewService(credentialStub{}, &gatewayStub{}, nil)
	_, err := service.Pods(context.Background(), 7, "", apiquery.ListQuery{Page: 1, Limit: 20})
	if err != cluster.ErrDisabled {
		t.Fatalf("error = %v, want ErrDisabled", err)
	}
}

func TestWorkloadResourcePaths(t *testing.T) {
	tests := []struct {
		name     string
		body     []byte
		call     func(*Service) error
		wantPath string
	}{
		{name: "nodes", body: []byte(`{"items":[]}`), call: func(service *Service) error {
			_, err := service.Nodes(context.Background(), 7, apiquery.ListQuery{Page: 1, Limit: 20})
			return err
		}, wantPath: "/api/v1/nodes"},
		{name: "deployments", body: []byte(`{"items":[]}`), call: func(service *Service) error {
			_, err := service.Deployments(context.Background(), 7, "demo", apiquery.ListQuery{Page: 1, Limit: 20})
			return err
		}, wantPath: "/apis/apps/v1/namespaces/demo/deployments"},
		{name: "services", body: []byte(`{"items":[]}`), call: func(service *Service) error {
			_, err := service.Services(context.Background(), 7, "demo", apiquery.ListQuery{Page: 1, Limit: 20})
			return err
		}, wantPath: "/api/v1/namespaces/demo/services"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gateway := &gatewayStub{body: tt.body}
			if err := tt.call(NewService(credentialStub{enabled: true}, gateway, nil)); err != nil {
				t.Fatal(err)
			}
			if gateway.path != tt.wantPath {
				t.Fatalf("path = %q, want %q", gateway.path, tt.wantPath)
			}
		})
	}
}

func TestExtendedReadOnlyResourcePaths(t *testing.T) {
	tests := []struct {
		name     string
		body     []byte
		call     func(*Service) error
		wantPath string
	}{
		{name: "ingress list", body: []byte(`{"items":[]}`), call: func(service *Service) error {
			_, err := service.Ingresses(context.Background(), 7, "team one", apiquery.ListQuery{Page: 1, Limit: 20})
			return err
		}, wantPath: "/apis/networking.k8s.io/v1/namespaces/team%20one/ingresses"},
		{name: "ingress detail", body: []byte(`{"metadata":{"name":"web"}}`), call: func(service *Service) error {
			_, err := service.Ingress(context.Background(), 7, "team one", "web/canary")
			return err
		}, wantPath: "/apis/networking.k8s.io/v1/namespaces/team%20one/ingresses/web%2Fcanary"},
		{name: "endpoint slice list", body: []byte(`{"items":[]}`), call: func(service *Service) error {
			_, err := service.EndpointSlices(context.Background(), 7, "team one", apiquery.ListQuery{Page: 1, Limit: 20})
			return err
		}, wantPath: "/apis/discovery.k8s.io/v1/namespaces/team%20one/endpointslices"},
		{name: "pvc list", body: []byte(`{"items":[]}`), call: func(service *Service) error {
			_, err := service.PersistentVolumeClaims(context.Background(), 7, "team one", apiquery.ListQuery{Page: 1, Limit: 20})
			return err
		}, wantPath: "/api/v1/namespaces/team%20one/persistentvolumeclaims"},
		{name: "pvc detail", body: []byte(`{"metadata":{"name":"cache"}}`), call: func(service *Service) error {
			_, err := service.PersistentVolumeClaim(context.Background(), 7, "team one", "cache/v2")
			return err
		}, wantPath: "/api/v1/namespaces/team%20one/persistentvolumeclaims/cache%2Fv2"},
		{name: "storage class list", body: []byte(`{"items":[]}`), call: func(service *Service) error {
			_, err := service.StorageClasses(context.Background(), 7, apiquery.ListQuery{Page: 1, Limit: 20})
			return err
		}, wantPath: "/apis/storage.k8s.io/v1/storageclasses"},
		{name: "storage class detail", body: []byte(`{"metadata":{"name":"standard"}}`), call: func(service *Service) error {
			_, err := service.StorageClass(context.Background(), 7, "fast/local")
			return err
		}, wantPath: "/apis/storage.k8s.io/v1/storageclasses/fast%2Flocal"},
		{name: "config map list", body: []byte(`{"items":[]}`), call: func(service *Service) error {
			_, err := service.ConfigMaps(context.Background(), 7, "team one", apiquery.ListQuery{Page: 1, Limit: 20})
			return err
		}, wantPath: "/api/v1/namespaces/team%20one/configmaps"},
		{name: "config map detail", body: []byte(`{"metadata":{"name":"runtime"}}`), call: func(service *Service) error {
			_, err := service.ConfigMap(context.Background(), 7, "team one", "runtime/v2")
			return err
		}, wantPath: "/api/v1/namespaces/team%20one/configmaps/runtime%2Fv2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gateway := &gatewayStub{body: tt.body}
			if err := tt.call(NewService(credentialStub{enabled: true}, gateway, nil)); err != nil {
				t.Fatal(err)
			}
			if gateway.path != tt.wantPath {
				t.Fatalf("path = %q, want %q", gateway.path, tt.wantPath)
			}
		})
	}
}

func TestM22PodDisruptionBudgetPathsUsePolicyAPIGroup(t *testing.T) {
	tests := []struct {
		name     string
		body     []byte
		call     func(*Service) error
		wantPath string
	}{
		{name: "list", body: []byte(`{"items":[]}`), call: func(service *Service) error {
			_, err := service.PodDisruptionBudgets(context.Background(), 7, "team one", apiquery.ListQuery{Page: 1, Limit: 20})
			return err
		}, wantPath: "/apis/policy/v1/namespaces/team%20one/poddisruptionbudgets"},
		{name: "detail", body: []byte(`{"metadata":{"name":"api-budget"}}`), call: func(service *Service) error {
			_, err := service.PodDisruptionBudget(context.Background(), 7, "team one", "api/budget")
			return err
		}, wantPath: "/apis/policy/v1/namespaces/team%20one/poddisruptionbudgets/api%2Fbudget"},
		{name: "manifest", body: []byte(`{"metadata":{"name":"api-budget"}}`), call: func(service *Service) error {
			_, err := service.Manifest(context.Background(), 7, "PodDisruptionBudget", "team one", "api/budget")
			return err
		}, wantPath: "/apis/policy/v1/namespaces/team%20one/poddisruptionbudgets/api%2Fbudget"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gateway := &gatewayStub{body: tt.body}
			if err := tt.call(NewService(credentialStub{enabled: true}, gateway, nil)); err != nil {
				t.Fatal(err)
			}
			if gateway.path != tt.wantPath {
				t.Fatalf("path = %q, want %q", gateway.path, tt.wantPath)
			}
		})
	}
}

func TestM17ReadOnlyResourcePaths(t *testing.T) {
	tests := []struct {
		name     string
		body     []byte
		call     func(*Service) error
		wantPath string
	}{
		{name: "stateful set list", body: []byte(`{"items":[]}`), call: func(service *Service) error {
			_, err := service.StatefulSets(context.Background(), 7, "team one", apiquery.ListQuery{Page: 1, Limit: 20})
			return err
		}, wantPath: "/apis/apps/v1/namespaces/team%20one/statefulsets"},
		{name: "stateful set detail", body: []byte(`{"metadata":{"name":"db"}}`), call: func(service *Service) error {
			_, err := service.StatefulSet(context.Background(), 7, "team one", "db/v2")
			return err
		}, wantPath: "/apis/apps/v1/namespaces/team%20one/statefulsets/db%2Fv2"},
		{name: "daemon set list", body: []byte(`{"items":[]}`), call: func(service *Service) error {
			_, err := service.DaemonSets(context.Background(), 7, "team one", apiquery.ListQuery{Page: 1, Limit: 20})
			return err
		}, wantPath: "/apis/apps/v1/namespaces/team%20one/daemonsets"},
		{name: "daemon set detail", body: []byte(`{"metadata":{"name":"agent"}}`), call: func(service *Service) error {
			_, err := service.DaemonSet(context.Background(), 7, "team one", "agent/v2")
			return err
		}, wantPath: "/apis/apps/v1/namespaces/team%20one/daemonsets/agent%2Fv2"},
		{name: "replica set list", body: []byte(`{"items":[]}`), call: func(service *Service) error {
			_, err := service.ReplicaSets(context.Background(), 7, "team one", apiquery.ListQuery{Page: 1, Limit: 20})
			return err
		}, wantPath: "/apis/apps/v1/namespaces/team%20one/replicasets"},
		{name: "replica set detail", body: []byte(`{"metadata":{"name":"api"}}`), call: func(service *Service) error {
			_, err := service.ReplicaSet(context.Background(), 7, "team one", "api/v2")
			return err
		}, wantPath: "/apis/apps/v1/namespaces/team%20one/replicasets/api%2Fv2"},
		{name: "job list", body: []byte(`{"items":[]}`), call: func(service *Service) error {
			_, err := service.Jobs(context.Background(), 7, "team one", apiquery.ListQuery{Page: 1, Limit: 20})
			return err
		}, wantPath: "/apis/batch/v1/namespaces/team%20one/jobs"},
		{name: "job detail", body: []byte(`{"metadata":{"name":"backup"}}`), call: func(service *Service) error {
			_, err := service.Job(context.Background(), 7, "team one", "backup/v2")
			return err
		}, wantPath: "/apis/batch/v1/namespaces/team%20one/jobs/backup%2Fv2"},
		{name: "cron job list", body: []byte(`{"items":[]}`), call: func(service *Service) error {
			_, err := service.CronJobs(context.Background(), 7, "team one", apiquery.ListQuery{Page: 1, Limit: 20})
			return err
		}, wantPath: "/apis/batch/v1/namespaces/team%20one/cronjobs"},
		{name: "cron job detail", body: []byte(`{"metadata":{"name":"cleanup"}}`), call: func(service *Service) error {
			_, err := service.CronJob(context.Background(), 7, "team one", "cleanup/v2")
			return err
		}, wantPath: "/apis/batch/v1/namespaces/team%20one/cronjobs/cleanup%2Fv2"},
		{name: "hpa list", body: []byte(`{"items":[]}`), call: func(service *Service) error {
			_, err := service.HorizontalPodAutoscalers(context.Background(), 7, "team one", apiquery.ListQuery{Page: 1, Limit: 20})
			return err
		}, wantPath: "/apis/autoscaling/v2/namespaces/team%20one/horizontalpodautoscalers"},
		{name: "hpa detail", body: []byte(`{"metadata":{"name":"api"}}`), call: func(service *Service) error {
			_, err := service.HorizontalPodAutoscaler(context.Background(), 7, "team one", "api/v2")
			return err
		}, wantPath: "/apis/autoscaling/v2/namespaces/team%20one/horizontalpodautoscalers/api%2Fv2"},
		{name: "quota list", body: []byte(`{"items":[]}`), call: func(service *Service) error {
			_, err := service.ResourceQuotas(context.Background(), 7, "team one", apiquery.ListQuery{Page: 1, Limit: 20})
			return err
		}, wantPath: "/api/v1/namespaces/team%20one/resourcequotas"},
		{name: "quota detail", body: []byte(`{"metadata":{"name":"team"}}`), call: func(service *Service) error {
			_, err := service.ResourceQuota(context.Background(), 7, "team one", "team/v2")
			return err
		}, wantPath: "/api/v1/namespaces/team%20one/resourcequotas/team%2Fv2"},
		{name: "limit range list", body: []byte(`{"items":[]}`), call: func(service *Service) error {
			_, err := service.LimitRanges(context.Background(), 7, "team one", apiquery.ListQuery{Page: 1, Limit: 20})
			return err
		}, wantPath: "/api/v1/namespaces/team%20one/limitranges"},
		{name: "limit range detail", body: []byte(`{"metadata":{"name":"defaults"}}`), call: func(service *Service) error {
			_, err := service.LimitRange(context.Background(), 7, "team one", "defaults/v2")
			return err
		}, wantPath: "/api/v1/namespaces/team%20one/limitranges/defaults%2Fv2"},
		{name: "secret list", body: []byte(`{"items":[]}`), call: func(service *Service) error {
			_, err := service.Secrets(context.Background(), 7, "team one", apiquery.ListQuery{Page: 1, Limit: 20})
			return err
		}, wantPath: "/api/v1/namespaces/team%20one/secrets"},
		{name: "secret detail", body: []byte(`{"metadata":{"name":"runtime"}}`), call: func(service *Service) error {
			_, err := service.Secret(context.Background(), 7, "team one", "runtime/v2")
			return err
		}, wantPath: "/api/v1/namespaces/team%20one/secrets/runtime%2Fv2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gateway := &gatewayStub{body: tt.body}
			if err := tt.call(NewService(credentialStub{enabled: true}, gateway, nil)); err != nil {
				t.Fatal(err)
			}
			if gateway.path != tt.wantPath {
				t.Fatalf("path = %q, want %q", gateway.path, tt.wantPath)
			}
		})
	}
}

func TestEndpointSlicesExposeFixedContract(t *testing.T) {
	const unsafeValue = "ENDPOINT_SLICE_FIELD_MUST_NOT_ESCAPE"
	gateway := &gatewayStub{body: []byte(`{"items":[{"metadata":{"name":"api-a","namespace":"demo","labels":{"kubernetes.io/service-name":"api"}},"addressType":"IPv4","serviceName":"spoofed","ports":[{"name":"http","protocol":"TCP","port":8080,"appProtocol":"http"}],"endpoints":[{"addresses":["10.0.0.8"],"conditions":{"ready":false,"serving":true,"terminating":false},"nodeName":"worker-1","targetRef":{"kind":"Pod","namespace":"demo","name":"api-1","uid":"pod-uid"},"deprecatedTopology":{"unsafe":"` + unsafeValue + `"}}],"unsafe":"` + unsafeValue + `"}]}`)}
	response, err := NewService(credentialStub{enabled: true}, gateway, nil).EndpointSlices(context.Background(), 7, "", apiquery.ListQuery{Page: 1, Limit: 20, SortBy: "name", Ascending: true})
	if err != nil {
		t.Fatal(err)
	}
	if gateway.path != "/apis/discovery.k8s.io/v1/endpointslices" || response.Total != 1 || response.Remaining != 0 || len(response.Items) != 1 {
		t.Fatalf("path=%q response=%#v", gateway.path, response)
	}
	item := response.Items[0]
	if item.ServiceName != "api" || item.AddressType != "IPv4" || len(item.Ports) != 1 || item.Ports[0].Name != "http" || item.Ports[0].Protocol != "TCP" || item.Ports[0].Port != 8080 {
		t.Fatalf("endpoint slice identity and ports = %#v", item)
	}
	endpoint := item.Endpoints[0]
	if endpoint.Conditions.Ready == nil || *endpoint.Conditions.Ready || endpoint.Conditions.Serving == nil || !*endpoint.Conditions.Serving || endpoint.NodeName == nil || *endpoint.NodeName != "worker-1" || endpoint.TargetRef == nil || endpoint.TargetRef.Kind != "Pod" || endpoint.TargetRef.Namespace != "demo" || endpoint.TargetRef.Name != "api-1" {
		t.Fatalf("endpoint slice endpoint = %#v", endpoint)
	}
	encoded, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), unsafeValue) || strings.Contains(string(encoded), "appProtocol") || strings.Contains(string(encoded), "spoofed") {
		t.Fatalf("fixed EndpointSlice response = %s", encoded)
	}
}

func TestEndpointSlicesNormalizeEmptyCollections(t *testing.T) {
	gateway := &gatewayStub{body: []byte(`{"items":[{"metadata":{"name":"empty","namespace":"demo","labels":{"kubernetes.io/service-name":"empty"}},"addressType":"IPv4","ports":null,"endpoints":null}]}`)}
	response, err := NewService(credentialStub{enabled: true}, gateway, nil).EndpointSlices(context.Background(), 7, "demo", apiquery.ListQuery{Page: 1, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(response.Items[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"endpoints":[]`) || strings.Contains(string(encoded), `"endpoints":null`) {
		t.Fatalf("normalized EndpointSlice response = %s", encoded)
	}
}

func TestSensitiveResourceFieldsNeverSerialize(t *testing.T) {
	const configValue = "CONFIG_VALUE_MUST_NOT_ESCAPE"
	const binaryValue = "BINARY_VALUE_MUST_NOT_ESCAPE"
	configGateway := &gatewayStub{body: []byte(`{"metadata":{"name":"runtime","namespace":"demo"},"immutable":true,"data":{"zeta":"` + configValue + `","alpha":"safe-looking"},"binaryData":{"payload":"` + binaryValue + `"}}`)}
	configMap, err := NewService(credentialStub{enabled: true}, configGateway, nil).ConfigMap(context.Background(), 7, "demo", "runtime")
	if err != nil {
		t.Fatal(err)
	}
	encodedConfigMap, err := json.Marshal(configMap)
	if err != nil {
		t.Fatal(err)
	}
	configJSON := string(encodedConfigMap)
	if strings.Contains(configJSON, configValue) || strings.Contains(configJSON, binaryValue) || !strings.Contains(configJSON, `"dataKeys":["alpha","zeta"]`) || !strings.Contains(configJSON, `"binaryDataKeys":["payload"]`) {
		t.Fatalf("sanitized ConfigMap response = %s", configJSON)
	}

	const parameterValue = "STORAGE_PARAMETER_MUST_NOT_ESCAPE"
	storageGateway := &gatewayStub{body: []byte(`{"metadata":{"name":"standard"},"provisioner":"example.csi.local","parameters":{"api-key":"` + parameterValue + `"}}`)}
	storageClass, err := NewService(credentialStub{enabled: true}, storageGateway, nil).StorageClass(context.Background(), 7, "standard")
	if err != nil {
		t.Fatal(err)
	}
	encodedStorageClass, err := json.Marshal(storageClass)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encodedStorageClass), parameterValue) || strings.Contains(string(encodedStorageClass), "parameters") {
		t.Fatalf("sanitized StorageClass response = %s", encodedStorageClass)
	}

	const secretValue = "SECRET_VALUE_MUST_NOT_ESCAPE"
	const annotationValue = "SECRET_ANNOTATION_MUST_NOT_ESCAPE"
	secretGateway := &gatewayStub{body: []byte(`{"metadata":{"name":"runtime","namespace":"demo","uid":"secret-1","annotations":{"credential":"` + annotationValue + `"},"labels":{"team":"platform"}},"type":"Opaque","immutable":true,"data":{"zeta":"` + secretValue + `","alpha":"safe-looking"}}`)}
	secret, err := NewService(credentialStub{enabled: true}, secretGateway, nil).Secret(context.Background(), 7, "demo", "runtime")
	if err != nil {
		t.Fatal(err)
	}
	encodedSecret, err := json.Marshal(secret)
	if err != nil {
		t.Fatal(err)
	}
	secretJSON := string(encodedSecret)
	if strings.Contains(secretJSON, secretValue) || strings.Contains(secretJSON, annotationValue) || strings.Contains(secretJSON, "annotations") || strings.Contains(secretJSON, "labels") || !strings.Contains(secretJSON, `"dataKeys":["alpha","zeta"]`) {
		t.Fatalf("sanitized Secret response = %s", secretJSON)
	}
}

func TestHPAContractDropsMetricSelectorsAndNormalizesCollections(t *testing.T) {
	const selectorValue = "HPA_SELECTOR_MUST_NOT_ESCAPE"
	gateway := &gatewayStub{body: []byte(`{"items":[{"metadata":{"name":"api","namespace":"demo"},"spec":{"scaleTargetRef":{"apiVersion":"apps/v1","kind":"Deployment","name":"api"},"minReplicas":1,"maxReplicas":5,"metrics":[{"type":"Resource","resource":{"name":"cpu","target":{"type":"Utilization","averageUtilization":70}}},{"type":"External","external":{"metric":{"name":"queue_depth","selector":{"matchLabels":{"unsafe":"` + selectorValue + `"}}},"target":{"type":"AverageValue","averageValue":"10"}}}]},"status":{"currentReplicas":2,"desiredReplicas":3,"conditions":null}}]}`)}
	response, err := NewService(credentialStub{enabled: true}, gateway, nil).HorizontalPodAutoscalers(context.Background(), 7, "demo", apiquery.ListQuery{Page: 1, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(response.Items[0])
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	if strings.Contains(text, selectorValue) || strings.Contains(text, "selector") || !strings.Contains(text, `"averageUtilization":70`) || !strings.Contains(text, `"metric":{"name":"queue_depth"}`) || !strings.Contains(text, `"conditions":[]`) {
		t.Fatalf("sanitized HPA response = %s", text)
	}
}

func TestPatchDeploymentIsBoundedAndSupportsServerDryRun(t *testing.T) {
	gateway := &gatewayStub{body: []byte(`{"metadata":{"name":"api","namespace":"demo","uid":"uid-1","resourceVersion":"12"}}`)}
	service := NewService(credentialStub{enabled: true}, gateway, nil)
	patch := []byte(`{"metadata":{"resourceVersion":"11"},"spec":{"template":{"metadata":{"annotations":{"k8s-aiops.local/restarted-at":"2026-07-17T12:00:00Z"}}}}}`)
	item, err := service.PatchDeployment(context.Background(), 7, "demo", "api", patch, true)
	if err != nil {
		t.Fatal(err)
	}
	if item.Metadata.ResourceVersion != "12" || gateway.path != "/apis/apps/v1/namespaces/demo/deployments/api" || gateway.query.Get("dryRun") != "All" {
		t.Fatalf("item=%#v path=%q query=%v", item, gateway.path, gateway.query)
	}
	if gateway.patchType != "application/strategic-merge-patch+json" || string(gateway.patchBody) != string(patch) {
		t.Fatalf("type=%q body=%s", gateway.patchType, gateway.patchBody)
	}
}

func TestPatchCronJobIsBoundedAndSupportsServerDryRun(t *testing.T) {
	gateway := &gatewayStub{body: []byte(`{"metadata":{"name":"cleanup","namespace":"demo","uid":"uid-2","resourceVersion":"22"},"spec":{"suspend":true}}`)}
	service := NewService(credentialStub{enabled: true}, gateway, nil)
	patch := []byte(`{"metadata":{"uid":"uid-2","resourceVersion":"21"},"spec":{"suspend":true}}`)
	item, err := service.PatchCronJob(context.Background(), 7, "team one", "nightly/cleanup", patch, true)
	if err != nil {
		t.Fatal(err)
	}
	if item.Metadata.ResourceVersion != "22" || item.Spec.Suspend == nil || !*item.Spec.Suspend || gateway.path != "/apis/batch/v1/namespaces/team%20one/cronjobs/nightly%2Fcleanup" || gateway.query.Get("dryRun") != "All" {
		t.Fatalf("item=%#v path=%q query=%v", item, gateway.path, gateway.query)
	}
	if gateway.patchType != "application/strategic-merge-patch+json" || string(gateway.patchBody) != string(patch) {
		t.Fatalf("type=%q body=%s", gateway.patchType, gateway.patchBody)
	}
}

func TestServiceDiagnosisResourcePaths(t *testing.T) {
	tests := []struct {
		name     string
		body     []byte
		call     func(*Service) error
		wantPath string
	}{
		{name: "service", body: []byte(`{"metadata":{"name":"api"}}`), call: func(service *Service) error {
			_, err := service.GetService(context.Background(), 7, "demo", "api")
			return err
		}, wantPath: "/api/v1/namespaces/demo/services/api"},
		{name: "node", body: []byte(`{"metadata":{"name":"worker-1"}}`), call: func(service *Service) error {
			_, err := service.Node(context.Background(), 7, "worker-1")
			return err
		}, wantPath: "/api/v1/nodes/worker-1"},
		{name: "deployment", body: []byte(`{"metadata":{"name":"api"}}`), call: func(service *Service) error {
			_, err := service.Deployment(context.Background(), 7, "demo", "api")
			return err
		}, wantPath: "/apis/apps/v1/namespaces/demo/deployments/api"},
		{name: "endpoint slices", body: []byte(`{"items":[]}`), call: func(service *Service) error {
			_, err := service.ServiceEndpoints(context.Background(), 7, "demo", "api")
			return err
		}, wantPath: "/apis/discovery.k8s.io/v1/namespaces/demo/endpointslices"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gateway := &gatewayStub{body: tt.body}
			if err := tt.call(NewService(credentialStub{enabled: true}, gateway, nil)); err != nil {
				t.Fatal(err)
			}
			if gateway.path != tt.wantPath {
				t.Fatalf("path = %q, want %q", gateway.path, tt.wantPath)
			}
		})
	}
}

func TestServiceEndpointsPrefersEndpointSlices(t *testing.T) {
	gateway := &gatewayStub{body: []byte(`{"items":[{"metadata":{"name":"api-a"},"addressType":"IPv4","endpoints":[{"addresses":["10.0.0.1","10.0.0.2"],"conditions":{"ready":true}},{"addresses":["10.0.0.3"],"conditions":{"ready":false}},{"addresses":["10.0.0.4"],"conditions":{}}]}]}`)}
	service := NewService(credentialStub{enabled: true}, gateway, nil)
	endpoints, err := service.ServiceEndpoints(context.Background(), 7, "demo", "api")
	if err != nil {
		t.Fatal(err)
	}
	if endpoints.SourceAPI != "discovery.k8s.io/v1" || len(endpoints.Subsets) != 1 || len(endpoints.Subsets[0].Addresses) != 3 || len(endpoints.Subsets[0].NotReadyAddresses) != 1 {
		t.Fatalf("endpoints = %#v", endpoints)
	}
	if gateway.query.Get("labelSelector") != "kubernetes.io/service-name=api" || len(gateway.paths) != 1 {
		t.Fatalf("paths=%v query=%v", gateway.paths, gateway.query)
	}
}

func TestServiceEndpointsFallsBackOnlyWhenDiscoveryAPIIsMissing(t *testing.T) {
	discoveryPath := "/apis/discovery.k8s.io/v1/namespaces/demo/endpointslices"
	legacyPath := "/api/v1/namespaces/demo/endpoints/api"
	gateway := &gatewayStub{responses: map[string]gatewayResponse{
		discoveryPath: {err: cluster.APIStatusError{StatusCode: 404}},
		legacyPath:    {body: []byte(`{"metadata":{"name":"api"},"subsets":[{"addresses":[{"ip":"10.0.0.8"}]}]}`)},
	}}
	endpoints, err := NewService(credentialStub{enabled: true}, gateway, nil).ServiceEndpoints(context.Background(), 7, "demo", "api")
	if err != nil || endpoints.SourceAPI != "core/v1" || len(endpoints.Subsets) != 1 || len(gateway.paths) != 2 {
		t.Fatalf("endpoints=%#v paths=%v err=%v", endpoints, gateway.paths, err)
	}

	forbidden := &gatewayStub{responses: map[string]gatewayResponse{discoveryPath: {err: cluster.APIStatusError{StatusCode: 403}}}}
	_, err = NewService(credentialStub{enabled: true}, forbidden, nil).ServiceEndpoints(context.Background(), 7, "demo", "api")
	var status cluster.APIStatusError
	if !errors.As(err, &status) || status.StatusCode != 403 || len(forbidden.paths) != 1 {
		t.Fatalf("paths=%v err=%v", forbidden.paths, err)
	}
}

func TestContainersListsContainerInfo(t *testing.T) {
	podPath := "/api/v1/namespaces/prod/pods/api-1"
	gateway := &gatewayStub{responses: map[string]gatewayResponse{
		podPath: {body: []byte(`{"metadata":{"name":"api-1","namespace":"prod"},"spec":{"containers":[{"name":"app","image":"app:1.0"},{"name":"sidecar","image":"sidecar:2.0"}],"initContainers":[{"name":"migrate","image":"migrate:1.0"}]},"status":{"containerStatuses":[{"name":"app","ready":true,"restartCount":0,"state":{"running":{}},"lastState":{}},{"name":"sidecar","ready":false,"restartCount":3,"state":{"waiting":{"reason":"ImagePullBackOff"}},"lastState":{"terminated":{"exitCode":1}}}],"initContainerStatuses":[{"name":"migrate","ready":true,"restartCount":1,"state":{"terminated":{"exitCode":0}}}]}}`)},
	}}
	service := NewService(credentialStub{enabled: true}, gateway, nil)
	containers, err := service.Containers(context.Background(), 7, "prod", "api-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(containers) != 3 {
		t.Fatalf("expected 3 containers, got %d", len(containers))
	}
	if containers[0].Name != "app" || containers[0].Ready != true || containers[0].State != "running" {
		t.Fatalf("unexpected app container: %#v", containers[0])
	}
	if containers[1].Name != "sidecar" || containers[1].Ready != false || containers[1].State != "waiting" || containers[1].RestartCount != 3 {
		t.Fatalf("unexpected sidecar container: %#v", containers[1])
	}
	if containers[2].Name != "migrate" || !containers[2].IsInit || containers[2].State != "terminated" || containers[2].RestartCount != 1 {
		t.Fatalf("unexpected init container: %#v", containers[2])
	}
}

func TestLogsSinceSupportsSinceSecondsAndDisclosesTruncation(t *testing.T) {
	podPath := "/api/v1/namespaces/prod/pods/api-1"
	logPath := "/api/v1/namespaces/prod/pods/api-1/log"
	gateway := &gatewayStub{responses: map[string]gatewayResponse{
		podPath: {body: []byte(`{"metadata":{"name":"api-1"},"spec":{"containers":[{"name":"app"}]},"status":{"containerStatuses":[{"name":"app","ready":true}]}}`)},
		logPath: {body: []byte("2026-07-29T10:00:00Z line one\n2026-07-29T10:00:05Z line two\n")},
	}}
	service := NewService(credentialStub{enabled: true}, gateway, nil)
	log, err := service.LogsSince(context.Background(), 7, "prod", "api-1", "app", false, 200, 3600, "")
	if err != nil {
		t.Fatal(err)
	}
	if log.Container != "app" || len(log.Lines) != 2 || log.Truncated {
		t.Fatalf("unexpected log: %#v", log)
	}
	if log.Lines[0].Timestamp != "2026-07-29T10:00:00Z" || log.Lines[0].Message != "line one" {
		t.Fatalf("unexpected first line: %#v", log.Lines[0])
	}
	if gateway.query.Get("sinceSeconds") != "3600" {
		t.Fatalf("sinceSeconds not forwarded: %v", gateway.query)
	}
}

func TestLogsSinceDisallowsBothSinceSecondsAndSinceTime(t *testing.T) {
	gateway := &gatewayStub{}
	service := NewService(credentialStub{enabled: true}, gateway, nil)
	_, err := service.LogsSince(context.Background(), 7, "prod", "api-1", "app", false, 200, 3600, "2026-07-29T00:00:00Z")
	if err == nil {
		t.Fatal("expected mutually exclusive log window parameters to be rejected")
	}
}

func TestLogsSinceDetectsTruncation(t *testing.T) {
	podPath := "/api/v1/namespaces/prod/pods/api-1"
	logPath := "/api/v1/namespaces/prod/pods/api-1/log"
	longLine := strings.Repeat("x", 1<<20)
	gateway := &gatewayStub{responses: map[string]gatewayResponse{
		podPath: {body: []byte(`{"metadata":{"name":"api-1"},"spec":{"containers":[{"name":"app"}]},"status":{"containerStatuses":[{"name":"app","ready":true}]}}`)},
		logPath: {body: []byte("2026-07-29T10:00:00Z " + longLine)},
	}}
	service := NewService(credentialStub{enabled: true}, gateway, nil)
	log, err := service.LogsSince(context.Background(), 7, "prod", "api-1", "app", false, 200, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if !log.Truncated || log.TruncationReason != "body_limit" {
		t.Fatalf("expected truncated with body_limit, got %#v", log)
	}
}

func TestAllContainerLogsFetchesAllContainers(t *testing.T) {
	podPath := "/api/v1/namespaces/prod/pods/api-1"
	gateway := &gatewayStub{responses: map[string]gatewayResponse{
		podPath: {body: []byte(`{"metadata":{"name":"api-1"},"spec":{"containers":[{"name":"app"},{"name":"sidecar"}]},"status":{"containerStatuses":[{"name":"app","ready":true},{"name":"sidecar","ready":true}]}}`)},
	}}
	g := gateway
	service := NewService(credentialStub{enabled: true}, g, nil)
	resp, err := service.AllContainerLogs(context.Background(), 7, "prod", "api-1", false, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Containers) != 2 {
		t.Fatalf("expected 2 container logs, got %d", len(resp.Containers))
	}
	if resp.Previous {
		t.Fatal("expected previous=false")
	}
	if resp.Containers[0].Container != "app" || resp.Containers[1].Container != "sidecar" {
		t.Fatalf("unexpected container order: %v, %v", resp.Containers[0].Container, resp.Containers[1].Container)
	}
	for _, c := range resp.Containers {
		if c.TruncationReason == "fetch_error" {
			t.Logf("container %s had fetch error (expected without mock): %s", c.Container, c.TruncationReason)
		}
	}
}

func TestParseLogLinesHandlesTimestampedAndUntimestamped(t *testing.T) {
	input := "2026-07-29T10:00:00Z app started\n2026-07-29T10:00:05Z processing request\nno timestamp here\n"
	lines := parseLogLines(input)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if lines[0].Timestamp != "2026-07-29T10:00:00Z" || lines[0].Message != "app started" {
		t.Fatalf("line 0 = %#v", lines[0])
	}
	if lines[1].Timestamp != "2026-07-29T10:00:05Z" || lines[1].Message != "processing request" {
		t.Fatalf("line 1 = %#v", lines[1])
	}
	if lines[2].Timestamp != "" || lines[2].Message != "no timestamp here" {
		t.Fatalf("line 2 = %#v", lines[2])
	}
}

func TestParseLogLinesHandlesEmpty(t *testing.T) {
	lines := parseLogLines("")
	if lines != nil {
		t.Fatalf("expected nil for empty input, got %v", lines)
	}
}

func TestManifestRejectsNonApprovedKind(t *testing.T) {
	gateway := &gatewayStub{responses: map[string]gatewayResponse{}}
	service := NewService(credentialStub{enabled: true}, gateway, nil)
	_, err := service.Manifest(context.Background(), 7, "Secret", "default", "my-secret")
	if err != ErrResourceNotFound {
		t.Fatalf("expected ErrResourceNotFound, got %v", err)
	}
}

func TestManifestFetchesAndRedacts(t *testing.T) {
	manifestJSON := `{
		"metadata":{"name":"api-1","namespace":"prod"},
		"spec":{"containers":[{"name":"app","image":"app:1.0","env":[{"name":"DB_PASSWORD","value":"super-secret"}]}]},
		"data":{"config":"sensitive-data"}
	}`
	gateway := &gatewayStub{responses: map[string]gatewayResponse{
		"/api/v1/namespaces/prod/pods/api-1": {body: []byte(manifestJSON)},
	}}
	service := NewService(credentialStub{enabled: true}, gateway, nil)
	result, err := service.Manifest(context.Background(), 7, "Pod", "prod", "api-1")
	if err != nil {
		t.Fatal(err)
	}
	if result["metadata"] == nil {
		t.Fatal("expected metadata to be present")
	}
	if result["data"] != "<redacted>" {
		t.Fatalf("expected data to be <redacted>, got %v", result["data"])
	}
	spec := result["spec"].(map[string]interface{})
	containers := spec["containers"].([]interface{})
	firstContainer := containers[0].(map[string]interface{})
	env := firstContainer["env"].([]interface{})
	firstEnv := env[0].(map[string]interface{})
	if firstEnv["value"] != "<redacted>" {
		t.Fatalf("expected env value to be <redacted>, got %v", firstEnv["value"])
	}
}

func TestManifestRedactsSensitiveFieldNames(t *testing.T) {
	manifestJSON := `{"metadata":{"name":"test"},"password":"admin123","nested":{"secret":"my-secret"}}`
	gateway := &gatewayStub{responses: map[string]gatewayResponse{
		"/apis/apps/v1/namespaces/default/deployments/test": {body: []byte(manifestJSON)},
	}}
	service := NewService(credentialStub{enabled: true}, gateway, nil)
	result, err := service.Manifest(context.Background(), 7, "Deployment", "default", "test")
	if err != nil {
		t.Fatal(err)
	}
	if result["password"] != "<redacted>" {
		t.Fatalf("expected password to be <redacted>, got %v", result["password"])
	}
	nested := result["nested"].(map[string]interface{})
	if nested["secret"] != "<redacted>" {
		t.Fatalf("expected nested secret to be <redacted>, got %v", nested["secret"])
	}
}

func TestManifestPathMapping(t *testing.T) {
	tests := []struct {
		kind      string
		namespace string
		name      string
		wantPath  string
	}{
		{"Pod", "prod", "api-1", "/api/v1/namespaces/prod/pods/api-1"},
		{"PersistentVolume", "", "my-pv", "/api/v1/persistentvolumes/my-pv"},
		{"ClusterRole", "", "admin", "/apis/rbac.authorization.k8s.io/v1/clusterroles/admin"},
		{"Deployment", "prod", "api", "/apis/apps/v1/namespaces/prod/deployments/api"},
		{"Ingress", "default", "web", "/apis/networking.k8s.io/v1/namespaces/default/ingresses/web"},
		{"Role", "kube-system", "reader", "/apis/rbac.authorization.k8s.io/v1/namespaces/kube-system/roles/reader"},
	}
	for _, tt := range tests {
		got := manifestPath(tt.kind, tt.namespace, tt.name)
		if got != tt.wantPath {
			t.Errorf("manifestPath(%s, %s, %s) = %s, want %s", tt.kind, tt.namespace, tt.name, got, tt.wantPath)
		}
	}
}

func TestVeleroCapabilityReturnsNotInstalledWhenAPIGroupMissing(t *testing.T) {
	gateway := &gatewayStub{responses: map[string]gatewayResponse{
		"/apis/velero.io/v1": {err: cluster.APIStatusError{StatusCode: 404}},
	}}
	service := NewService(credentialStub{enabled: true}, gateway, nil)
	capability, err := service.VeleroCapability(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if capability.Installed {
		t.Fatalf("expected Installed=false, got %#v", capability)
	}
}

func TestVeleroCapabilityReturnsInstalledWhenAPIGroupExists(t *testing.T) {
	gateway := &gatewayStub{responses: map[string]gatewayResponse{
		"/apis/velero.io/v1": {body: []byte(`{"groupVersion":"velero.io/v1","kind":"APIResourceList"}`)},
	}}
	service := NewService(credentialStub{enabled: true}, gateway, nil)
	capability, err := service.VeleroCapability(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if !capability.Installed || capability.Version != "v1" {
		t.Fatalf("expected Installed=true Version=v1, got %#v", capability)
	}
}

func TestBackupsReturnsVeleroUnavailableWhenAPIGroupMissing(t *testing.T) {
	gateway := &gatewayStub{responses: map[string]gatewayResponse{
		"/apis/velero.io/v1/backups": {err: cluster.APIStatusError{StatusCode: 404}},
		"/apis/velero.io/v1":         {err: cluster.APIStatusError{StatusCode: 404}},
	}}
	service := NewService(credentialStub{enabled: true}, gateway, nil)
	_, err := service.Backups(context.Background(), 7, "", apiquery.ListQuery{Page: 1, Limit: 20})
	if !errors.Is(err, ErrVeleroUnavailable) {
		t.Fatalf("expected ErrVeleroUnavailable, got %v", err)
	}
}

func TestBackupsProjectsBoundedShapeAndPaginates(t *testing.T) {
	backupListJSON := `{"items":[
		{"metadata":{"name":"backup-a","namespace":"velero","creationTimestamp":"2026-07-29T10:00:00Z"},"spec":{"includedNamespaces":["prod"],"storageLocation":"default","ttl":"720h"},"status":{"phase":"Completed","expiration":"2026-08-28T10:00:00Z","startTimestamp":"2026-07-29T10:00:00Z","completionTimestamp":"2026-07-29T10:05:00Z","errors":0,"warnings":2}},
		{"metadata":{"name":"backup-b","namespace":"velero","creationTimestamp":"2026-07-29T11:00:00Z"},"spec":{"includedNamespaces":["staging"],"storageLocation":"default","ttl":"720h"},"status":{"phase":"Failed","failureReason":"snapshot timeout","errors":3,"warnings":1}}
	]}`
	gateway := &gatewayStub{responses: map[string]gatewayResponse{
		"/apis/velero.io/v1/backups": {body: []byte(backupListJSON)},
	}}
	service := NewService(credentialStub{enabled: true}, gateway, nil)
	response, err := service.Backups(context.Background(), 7, "", apiquery.ListQuery{Page: 1, Limit: 10, SortBy: "name", Ascending: true})
	if err != nil {
		t.Fatal(err)
	}
	if response.Total != 2 || len(response.Items) != 2 {
		t.Fatalf("response = %#v", response)
	}
	first := response.Items[0]
	if first.Name != "backup-a" || first.Phase != "Completed" || len(first.IncludedNamespaces) != 1 || first.IncludedNamespaces[0] != "prod" || first.StorageLocation != "default" || first.TTL != "720h" || first.Expiration != "2026-08-28T10:00:00Z" || first.Errors != 0 || first.Warnings != 2 {
		t.Fatalf("first backup = %#v", first)
	}
	second := response.Items[1]
	if second.Phase != "Failed" || second.FailureReason != "snapshot timeout" || second.Errors != 3 {
		t.Fatalf("second backup = %#v", second)
	}
	if gateway.path != "/apis/velero.io/v1/backups" {
		t.Fatalf("path = %q", gateway.path)
	}
}

func TestBackupsUsesNamespaceScopedPath(t *testing.T) {
	gateway := &gatewayStub{responses: map[string]gatewayResponse{
		"/apis/velero.io/v1/namespaces/velero/backups": {body: []byte(`{"items":[]}`)},
	}}
	service := NewService(credentialStub{enabled: true}, gateway, nil)
	response, err := service.Backups(context.Background(), 7, "velero", apiquery.ListQuery{Page: 1, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if response.Total != 0 {
		t.Fatalf("expected empty list, got %#v", response)
	}
	if gateway.path != "/apis/velero.io/v1/namespaces/velero/backups" {
		t.Fatalf("path = %q", gateway.path)
	}
}

func TestBackupReturnsProjectedSingleResource(t *testing.T) {
	backupJSON := `{"metadata":{"name":"backup-a","namespace":"velero","creationTimestamp":"2026-07-29T10:00:00Z"},"spec":{"includedNamespaces":["prod"],"storageLocation":"default","ttl":"720h"},"status":{"phase":"Completed","startTimestamp":"2026-07-29T10:00:00Z","completionTimestamp":"2026-07-29T10:05:00Z","errors":0,"warnings":2}}`
	gateway := &gatewayStub{responses: map[string]gatewayResponse{
		"/apis/velero.io/v1/namespaces/velero/backups/backup-a": {body: []byte(backupJSON)},
	}}
	service := NewService(credentialStub{enabled: true}, gateway, nil)
	backup, err := service.Backup(context.Background(), 7, "velero", "backup-a")
	if err != nil {
		t.Fatal(err)
	}
	if backup.Name != "backup-a" || backup.Namespace != "velero" || backup.Phase != "Completed" || backup.StorageLocation != "default" || backup.TTL != "720h" || len(backup.IncludedNamespaces) != 1 || backup.Errors != 0 || backup.Warnings != 2 {
		t.Fatalf("backup = %#v", backup)
	}
}

func TestBackupKeepsResourceNotFoundWhenVeleroInstalledButBackupMissing(t *testing.T) {
	gateway := &gatewayStub{responses: map[string]gatewayResponse{
		"/apis/velero.io/v1/namespaces/velero/backups/missing": {err: cluster.APIStatusError{StatusCode: 404}},
		"/apis/velero.io/v1": {body: []byte(`{"groupVersion":"velero.io/v1"}`)},
	}}
	service := NewService(credentialStub{enabled: true}, gateway, nil)
	_, err := service.Backup(context.Background(), 7, "velero", "missing")
	if !errors.Is(err, ErrResourceNotFound) || errors.Is(err, ErrVeleroUnavailable) {
		t.Fatalf("expected ErrResourceNotFound without ErrVeleroUnavailable, got %v", err)
	}
}
