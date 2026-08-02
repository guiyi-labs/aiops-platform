package optimization

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"testing"

	"k8s-aiops.local/backend/internal/cluster"
	"k8s-aiops.local/backend/internal/kubernetes"
)

// fakeLister returns canned list items per API path; an unknown path means
// "resource type not installed" and yields no items (no error), mirroring how
// the production collector tolerates uninstalled CRDs.
type fakeLister struct {
	data  map[string][]json.RawMessage
	calls []string
}

func (f *fakeLister) List(_ context.Context, _ int64, path string) ([]json.RawMessage, error) {
	f.calls = append(f.calls, path)
	if items, ok := f.data[path]; ok {
		return items, nil
	}
	return nil, nil
}

func raw(s string) json.RawMessage { return json.RawMessage(s) }

// fakeMetrics returns a fixed p95 for every pod container.
type fakeMetrics struct{ cpu, mem int64 }

func (m fakeMetrics) PodContainerP95(_ context.Context, _ int64, _, _, _ string) (int64, int64, bool) {
	return m.cpu, m.mem, true
}

func TestCollectCIS_MapsWorkloadRBACNamespace(t *testing.T) {
	lister := &fakeLister{data: map[string][]json.RawMessage{
		"/api/v1/pods": {raw(`{
			"apiVersion":"v1","kind":"Pod",
			"metadata":{"namespace":"default","name":"priv","uid":"u1","labels":{"app":"x"}},
			"spec":{
				"hostNetwork":true,
				"containers":[{"name":"c1","securityContext":{"privileged":true,"capabilities":{"drop":["NET_RAW"]}},"volumeMounts":[{"name":"hp"}]}],
				"volumes":[{"name":"hp","hostPath":{"path":"/data"}}]
			}}`)},
		"/apis/rbac.authorization.k8s.io/v1/clusterrolebindings": {raw(`{
			"apiVersion":"rbac.authorization.k8s.io/v1","kind":"ClusterRoleBinding",
			"metadata":{"name":"bad","uid":"b1"},
			"roleRef":{"kind":"ClusterRole","name":"cluster-admin"},
			"subjects":[{"kind":"User","name":"alice"}]}`)},
		"/apis/rbac.authorization.k8s.io/v1/clusterroles": {raw(`{
			"apiVersion":"rbac.authorization.k8s.io/v1","kind":"ClusterRole",
			"metadata":{"name":"cluster-admin"},
			"rules":[{"apiGroups":["*"],"resources":["*"],"verbs":["*"]}]}`)},
		"/api/v1/namespaces": {raw(`{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"default","uid":"ns1"}}`)},
	}}

	col := NewCollector(lister, nil)
	in, err := col.CollectCIS(context.Background(), 7)
	if err != nil {
		t.Fatalf("CollectCIS: %v", err)
	}

	if len(in.Workloads) != 1 {
		t.Fatalf("workloads = %d, want 1", len(in.Workloads))
	}
	c := in.Workloads[0].Containers[0]
	if c.Privileged == nil || !*c.Privileged {
		t.Errorf("expected privileged container")
	}
	if !c.HostNetwork {
		t.Errorf("expected hostNetwork=true")
	}
	if c.HostPathVolumes != 1 {
		t.Errorf("HostPathVolumes = %d, want 1", c.HostPathVolumes)
	}

	if len(in.Bindings) != 1 {
		t.Fatalf("bindings = %d, want 1", len(in.Bindings))
	}
	b := in.Bindings[0]
	if b.RoleName != "cluster-admin" || len(b.Subjects) != 1 || b.Subjects[0].Name != "alice" {
		t.Errorf("unexpected binding: %+v", b)
	}
	if len(b.RoleRules) != 1 || len(b.RoleRules[0].Verbs) != 1 || b.RoleRules[0].Verbs[0] != "*" {
		t.Errorf("role rules not resolved: %+v", b.RoleRules)
	}

	if len(in.Namespaces) != 1 || in.Namespaces[0].Enforce != "" {
		t.Errorf("namespace PSA not mapped: %+v", in.Namespaces)
	}
}

func TestCollectFinOps_CollectsRequestsLimitsAndP95(t *testing.T) {
	lister := &fakeLister{data: map[string][]json.RawMessage{
		"/apis/apps/v1/deployments": {raw(`{
			"apiVersion":"apps/v1","kind":"Deployment",
			"metadata":{"namespace":"default","name":"web"},
			"spec":{"replicas":2,"selector":{"matchLabels":{"app":"web"}},
				"template":{"spec":{"containers":[{"name":"c1",
					"resources":{"requests":{"cpu":"100m","memory":"64Mi"},"limits":{"cpu":"200m","memory":"128Mi"}}}]}}}}`)},
		"/api/v1/pods": {
			raw(`{"apiVersion":"v1","kind":"Pod","metadata":{"namespace":"default","name":"web-aaa","labels":{"app":"web"}}}`),
			raw(`{"apiVersion":"v1","kind":"Pod","metadata":{"namespace":"default","name":"web-bbb","labels":{"app":"web"}}}`),
		},
	}}

	col := NewCollector(lister, fakeMetrics{cpu: 50_000_000, mem: 33_554_432}) // 50m / 32Mi
	inputs, err := col.CollectFinOps(context.Background(), 7)
	if err != nil {
		t.Fatalf("CollectFinOps: %v", err)
	}
	if len(inputs) != 1 {
		t.Fatalf("inputs = %d, want 1", len(inputs))
	}
	in := inputs[0]
	if in.WorkloadKind != "Deployment" || in.WorkloadName != "web" || in.ContainerName != "c1" {
		t.Errorf("unexpected identity: %+v", in)
	}
	if in.Replicas != 2 {
		t.Errorf("replicas = %d, want 2", in.Replicas)
	}
	if in.Requests.CPURequest != 100_000_000 {
		t.Errorf("CPURequest = %d, want 100000000 (100m)", in.Requests.CPURequest)
	}
	if in.Limits.CPULimit != 200_000_000 {
		t.Errorf("CPULimit = %d, want 200000000 (200m)", in.Limits.CPULimit)
	}
	if in.CPUUsageP95 != 50_000_000 {
		t.Errorf("CPUUsageP95 = %d, want 50000000", in.CPUUsageP95)
	}
	if in.MemUsageP95 != 33_554_432 {
		t.Errorf("MemUsageP95 = %d, want 33554432", in.MemUsageP95)
	}
}

func TestCollectFinOps_NoMetrics_NoUsage(t *testing.T) {
	lister := &fakeLister{data: map[string][]json.RawMessage{
		"/apis/apps/v1/deployments": {raw(`{
			"apiVersion":"apps/v1","kind":"Deployment",
			"metadata":{"namespace":"default","name":"web"},
			"spec":{"replicas":1,"selector":{"matchLabels":{"app":"web"}},
				"template":{"spec":{"containers":[{"name":"c1","resources":{"requests":{"cpu":"100m"}}}]}}}}`)},
	}}
	col := NewCollector(lister, nil) // no metrics source
	inputs, err := col.CollectFinOps(context.Background(), 7)
	if err != nil {
		t.Fatalf("CollectFinOps: %v", err)
	}
	if len(inputs) != 1 {
		t.Fatalf("inputs = %d, want 1", len(inputs))
	}
	if inputs[0].CPUUsageP95 != 0 || inputs[0].MemUsageP95 != 0 {
		t.Errorf("expected zero usage without a metrics source, got %+v", inputs[0])
	}
	if inputs[0].Requests.CPURequest != 100_000_000 {
		t.Errorf("requests should still be collected: %+v", inputs[0])
	}
}

func TestCollectDeprecatedAPI_ScansObjects(t *testing.T) {
	lister := &fakeLister{data: map[string][]json.RawMessage{
		"/api/v1/pods": {raw(`{"apiVersion":"v1","kind":"Pod","metadata":{"namespace":"default","name":"p1","uid":"u1"}}`)},
		// An object served from the apps/v1 list path but advertising a removed
		// apiVersion in its own body — the collector reads the item's apiVersion.
		"/apis/apps/v1/deployments": {raw(`{"apiVersion":"apps/v1beta1","kind":"Deployment","metadata":{"namespace":"default","name":"legacy","uid":"u2"}}`)},
	}}
	col := NewCollector(lister, nil)
	objs, err := col.CollectDeprecatedAPI(context.Background(), 7)
	if err != nil {
		t.Fatalf("CollectDeprecatedAPI: %v", err)
	}
	if len(objs) != 2 {
		t.Fatalf("objects = %d, want 2", len(objs))
	}
	// verify the deprecated one kept its apiVersion/kind from the body
	var foundLegacy bool
	for _, o := range objs {
		if o.Name == "legacy" {
			foundLegacy = true
			if o.APIVersion != "apps/v1beta1" || o.Kind != "Deployment" {
				t.Errorf("legacy object mis-mapped: %+v", o)
			}
		}
	}
	if !foundLegacy {
		t.Errorf("legacy object not collected; got %+v", objs)
	}
}

// kubernetesLister adapter: prove the Gateway + CredentialSource wiring maps a
// raw List response into items (no real cluster / network).
type stubGateway struct{ body []byte }

func (g stubGateway) Get(_ context.Context, _ int64, _ []byte, _ string, _ url.Values, _ int64) ([]byte, error) {
	return g.body, nil
}

type stubCreds struct{}

func (stubCreds) Access(_ context.Context, _ int64) (cluster.Cluster, []byte, error) {
	return cluster.Cluster{}, []byte("kubeconfig"), nil
}

func TestKubernetesLister_AdaptsGateway(t *testing.T) {
	body := []byte(`{"items":[{"apiVersion":"v1","kind":"Pod","metadata":{"name":"a"}},{"apiVersion":"v1","kind":"Pod","metadata":{"name":"b"}}]}`)
	lister := newKubernetesLister(stubGateway{body: body}, stubCreds{})
	items, err := lister.List(context.Background(), 7, "/api/v1/pods")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
}

var _ kubernetes.Gateway = stubGateway{}
var _ kubernetes.CredentialSource = stubCreds{}

func TestCollectNetPolicy_MapsNamespacesPodsServicesAndPolicies(t *testing.T) {
	lister := &fakeLister{data: map[string][]json.RawMessage{
		"/api/v1/namespaces": {raw(`{"metadata":{"name":"shop","uid":"ns1","labels":{"tier":"app"}}}`)},
		"/api/v1/pods": {raw(`{
			"metadata":{"namespace":"shop","name":"web-1","uid":"p1","labels":{"app":"web"}},
			"spec":{"hostNetwork":true,"containers":[
				{"name":"c1","ports":[{"name":"http","containerPort":8080,"protocol":"TCP"}]},
				{"name":"c2","ports":[{"containerPort":9090}]}
			]}}`)},
		"/api/v1/services": {raw(`{
			"metadata":{"namespace":"shop","name":"web","uid":"s1"},
			"spec":{"type":"NodePort","selector":{"app":"web"},"clusterIP":"10.0.0.7",
				"ports":[
					{"name":"http","port":80,"targetPort":"http","protocol":"TCP","nodePort":31080},
					{"name":"metrics","port":9090,"targetPort":9090}
				]}}`)},
		"/apis/networking.k8s.io/v1/networkpolicies": {raw(`{
			"metadata":{"namespace":"shop","name":"allow-gw","uid":"np1"},
			"spec":{
				"podSelector":{"matchLabels":{"app":"web"}},
				"policyTypes":["Ingress","Egress"],
				"ingress":[{
					"from":[
						{"namespaceSelector":{},"podSelector":{"matchLabels":{"app":"gw"}}},
						{"ipBlock":{"cidr":"0.0.0.0/0","except":["10.0.0.0/8"]}}
					],
					"ports":[{"protocol":"TCP","port":"http"},{"port":8000,"endPort":8100}]
				}],
				"egress":[{"to":[{"ipBlock":{"cidr":"0.0.0.0/0"}}]}]
			}}`)},
	}}

	in, err := NewCollector(lister, nil).CollectNetPolicy(context.Background(), 7)
	if err != nil {
		t.Fatalf("CollectNetPolicy: %v", err)
	}

	if len(in.Namespaces) != 1 || in.Namespaces[0].Labels["tier"] != "app" {
		t.Fatalf("namespaces = %+v", in.Namespaces)
	}
	if len(in.Pods) != 1 {
		t.Fatalf("pods = %d, want 1", len(in.Pods))
	}
	pod := in.Pods[0]
	if !pod.HostNetwork || pod.Labels["app"] != "web" {
		t.Fatalf("pod identity not mapped: %+v", pod)
	}
	if len(pod.Ports) != 2 || pod.Ports[0].Name != "http" || pod.Ports[0].ContainerPort != 8080 || pod.Ports[1].ContainerPort != 9090 {
		t.Fatalf("container ports across containers not flattened: %+v", pod.Ports)
	}

	if len(in.Services) != 1 {
		t.Fatalf("services = %d, want 1", len(in.Services))
	}
	svc := in.Services[0]
	if svc.Type != "NodePort" || svc.Selector["app"] != "web" || svc.ClusterIP != "10.0.0.7" {
		t.Fatalf("service not mapped: %+v", svc)
	}
	// targetPort is an IntOrString: both spellings must survive as strings.
	if svc.Ports[0].TargetPort != "http" || svc.Ports[0].NodePort != 31080 {
		t.Fatalf("named targetPort not mapped: %+v", svc.Ports[0])
	}
	if svc.Ports[1].TargetPort != "9090" {
		t.Fatalf("numeric targetPort = %q, want 9090", svc.Ports[1].TargetPort)
	}

	if len(in.Policies) != 1 {
		t.Fatalf("policies = %d, want 1", len(in.Policies))
	}
	policy := in.Policies[0]
	if policy.PodSelector.MatchLabels["app"] != "web" || policy.PodSelector.HasExpressions {
		t.Fatalf("policy podSelector not mapped: %+v", policy.PodSelector)
	}
	if len(policy.Ingress) != 1 || len(policy.Ingress[0].Peers) != 2 {
		t.Fatalf("ingress rule not mapped: %+v", policy.Ingress)
	}
	peer := policy.Ingress[0].Peers[0]
	// An empty namespaceSelector must stay distinguishable from an absent one.
	if peer.NamespaceSelector == nil || !peer.NamespaceSelector.SelectsAll() {
		t.Fatalf("empty namespaceSelector lost: %+v", peer.NamespaceSelector)
	}
	if peer.PodSelector == nil || peer.PodSelector.MatchLabels["app"] != "gw" {
		t.Fatalf("peer podSelector not mapped: %+v", peer.PodSelector)
	}
	if cidr := policy.Ingress[0].Peers[1]; cidr.IPBlockCIDR != "0.0.0.0/0" || len(cidr.IPBlockExcept) != 1 {
		t.Fatalf("ipBlock not mapped: %+v", cidr)
	}
	if ports := policy.Ingress[0].Ports; len(ports) != 2 || ports[0].Port != "http" || ports[1].Port != "8000" || ports[1].EndPort != 8100 {
		t.Fatalf("policy ports not mapped: %+v", ports)
	}
	if len(policy.Egress) != 1 || policy.Egress[0].Peers[0].IPBlockCIDR != "0.0.0.0/0" {
		t.Fatalf("egress rule not mapped: %+v", policy.Egress)
	}
}

// TestCollectNetPolicy_ToleratesMissingNetworkingAPI verifies that a cluster
// without the NetworkPolicy API (or without list permission on it) still
// produces a usable reachability bundle instead of failing the whole scan.
func TestCollectNetPolicy_ToleratesMissingNetworkingAPI(t *testing.T) {
	lister := &fakeLister{data: map[string][]json.RawMessage{
		"/api/v1/namespaces": {raw(`{"metadata":{"name":"shop"}}`)},
		"/api/v1/pods":       {raw(`{"metadata":{"namespace":"shop","name":"web-1","labels":{"app":"web"}}}`)},
		"/api/v1/services":   {raw(`{"metadata":{"namespace":"shop","name":"web"},"spec":{"selector":{"app":"web"},"ports":[{"port":80}]}}`)},
	}}

	in, err := NewCollector(lister, nil).CollectNetPolicy(context.Background(), 7)
	if err != nil {
		t.Fatalf("CollectNetPolicy: %v", err)
	}
	if len(in.Policies) != 0 {
		t.Fatalf("policies = %d, want 0", len(in.Policies))
	}
	if len(in.Pods) != 1 || len(in.Services) != 1 {
		t.Fatalf("bundle should still carry pods and services: %+v", in)
	}
	// An absent targetPort must stay empty so the analyzer defaults it to the
	// service port rather than treating "" as a named port.
	if in.Services[0].Ports[0].TargetPort != "" {
		t.Fatalf("absent targetPort = %q, want empty", in.Services[0].Ports[0].TargetPort)
	}
}

// TestCollectNetPolicy_PropagatesPodListFailure ensures a broken cluster
// connection surfaces as an error (the handler turns it into 502
// COLLECT_FAILED) rather than being reported as a clean, empty cluster.
func TestCollectNetPolicy_PropagatesPodListFailure(t *testing.T) {
	lister := failingLister{failOn: "/api/v1/pods"}
	if _, err := NewCollector(lister, nil).CollectNetPolicy(context.Background(), 7); err == nil {
		t.Fatal("expected the pod list failure to propagate")
	}
}

type failingLister struct{ failOn string }

func (f failingLister) List(_ context.Context, _ int64, path string) ([]json.RawMessage, error) {
	if path == f.failOn {
		return nil, errors.New("connection refused")
	}
	return nil, nil
}
