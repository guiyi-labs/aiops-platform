package optimization

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"testing"
	"time"

	"k8s-aiops.local/backend/internal/capacity"
	"k8s-aiops.local/backend/internal/cluster"
	"k8s-aiops.local/backend/internal/gitopsdrift"
	"k8s-aiops.local/backend/internal/imagepolicy"
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

	col := NewCollector(lister, nil, nil)
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

	col := NewCollector(lister, fakeMetrics{cpu: 50_000_000, mem: 33_554_432}, nil) // 50m / 32Mi
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
	col := NewCollector(lister, nil, nil) // no metrics source
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
	col := NewCollector(lister, nil, nil)
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

	in, err := NewCollector(lister, nil, nil).CollectNetPolicy(context.Background(), 7)
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

	in, err := NewCollector(lister, nil, nil).CollectNetPolicy(context.Background(), 7)
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
	if _, err := NewCollector(lister, nil, nil).CollectNetPolicy(context.Background(), 7); err == nil {
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

// TestCollectImagePolicy_MapsControllersAndInitContainers checks the mapping
// from live workload specs to image usages, including init containers and the
// decomposition of the image reference.
func TestCollectImagePolicy_MapsControllersAndInitContainers(t *testing.T) {
	lister := &fakeLister{data: map[string][]json.RawMessage{
		"/apis/apps/v1/deployments": {raw(`{"metadata":{"namespace":"shop","name":"api"},"spec":{"template":{"spec":{
			"initContainers":[{"name":"migrate","image":"registry.io/team/migrate:v1.4.2"}],
			"containers":[{"name":"app","image":"registry.io/team/api:latest","imagePullPolicy":"Always"}]}}}}`)},
		"/apis/apps/v1/statefulsets": {raw(`{"metadata":{"namespace":"shop","name":"db"},"spec":{"template":{"spec":{
			"containers":[{"name":"pg","image":"registry.io:5000/team/pg:16"}]}}}}`)},
		"/apis/apps/v1/daemonsets": {raw(`{"metadata":{"namespace":"kube-system","name":"agent"},"spec":{"template":{"spec":{
			"containers":[{"name":"agent","image":"registry.io/team/agent@sha256:abc"}]}}}}`)},
		"/apis/batch/v1/jobs": {raw(`{"metadata":{"namespace":"shop","name":"backup"},"spec":{"template":{"spec":{
			"containers":[{"name":"backup","image":"registry.io/team/backup:v2"}]}}}}`)},
	}}

	in, err := NewCollector(lister, nil, nil).CollectImagePolicy(context.Background(), 7)
	if err != nil {
		t.Fatalf("CollectImagePolicy: %v", err)
	}
	if len(in.Usages) != 5 {
		t.Fatalf("usages = %d, want 5: %+v", len(in.Usages), in.Usages)
	}

	byContainer := map[string]imagepolicy.ImageUsage{}
	for _, u := range in.Usages {
		byContainer[u.Container.WorkloadKind+"/"+u.Container.WorkloadName+"/"+u.Container.Container] = u
	}

	app, ok := byContainer["Deployment/api/app"]
	if !ok {
		t.Fatalf("deployment container missing: %+v", in.Usages)
	}
	if app.Image.Repository != "registry.io/team/api" || app.Image.Tag != "latest" {
		t.Fatalf("app image = %+v, want repo/tag split", app.Image)
	}
	if app.Image.PullPolicy != "Always" {
		t.Fatalf("pull policy = %q, want Always", app.Image.PullPolicy)
	}
	if app.Container.Namespace != "shop" {
		t.Fatalf("namespace = %q, want shop", app.Container.Namespace)
	}

	// Init containers carry the same supply-chain risk and must be collected.
	if _, ok := byContainer["Deployment/api/migrate"]; !ok {
		t.Fatalf("init container missing: %+v", in.Usages)
	}

	// A registry port must not be mistaken for a tag.
	pg := byContainer["StatefulSet/db/pg"]
	if pg.Image.Repository != "registry.io:5000/team/pg" || pg.Image.Tag != "16" {
		t.Fatalf("registry-port image parsed as %+v", pg.Image)
	}

	agent := byContainer["DaemonSet/agent/agent"]
	if agent.Image.Digest != "sha256:abc" {
		t.Fatalf("digest = %q, want sha256:abc", agent.Image.Digest)
	}

	if _, ok := byContainer["Job/backup/backup"]; !ok {
		t.Fatalf("job container missing: %+v", in.Usages)
	}
}

// TestCollectImagePolicy_SkipsOwnedPods is the double-counting guard: Pods
// created by a controller are already represented by that controller's pod
// template, so only standalone Pods may be added.
func TestCollectImagePolicy_SkipsOwnedPods(t *testing.T) {
	lister := &fakeLister{data: map[string][]json.RawMessage{
		"/apis/apps/v1/deployments": {raw(`{"metadata":{"namespace":"shop","name":"api"},"spec":{"template":{"spec":{
			"containers":[{"name":"app","image":"registry.io/team/api:v1"}]}}}}`)},
		"/api/v1/pods": {
			raw(`{"metadata":{"namespace":"shop","name":"api-abc123","ownerReferences":[{"kind":"ReplicaSet"}]},"spec":{
				"containers":[{"name":"app","image":"registry.io/team/api:v1"}]}}`),
			raw(`{"metadata":{"namespace":"ops","name":"debug"},"spec":{
				"containers":[{"name":"shell","image":"registry.io/team/debug:latest"}]}}`),
		},
	}}

	in, err := NewCollector(lister, nil, nil).CollectImagePolicy(context.Background(), 7)
	if err != nil {
		t.Fatalf("CollectImagePolicy: %v", err)
	}
	if len(in.Usages) != 2 {
		t.Fatalf("usages = %d, want 2 (controller + standalone pod only): %+v", len(in.Usages), in.Usages)
	}
	for _, u := range in.Usages {
		if u.Container.WorkloadKind == "Pod" && u.Container.WorkloadName != "debug" {
			t.Fatalf("controller-owned pod was collected: %+v", u.Container)
		}
	}
}

// TestCollectImagePolicy_SkipsContainersWithoutImage keeps malformed objects
// from producing empty-repository findings.
func TestCollectImagePolicy_SkipsContainersWithoutImage(t *testing.T) {
	lister := &fakeLister{data: map[string][]json.RawMessage{
		"/apis/apps/v1/deployments": {raw(`{"metadata":{"namespace":"shop","name":"api"},"spec":{"template":{"spec":{
			"containers":[{"name":"app"},{"name":"sidecar","image":"registry.io/team/proxy:v1"}]}}}}`)},
	}}

	in, err := NewCollector(lister, nil, nil).CollectImagePolicy(context.Background(), 7)
	if err != nil {
		t.Fatalf("CollectImagePolicy: %v", err)
	}
	if len(in.Usages) != 1 || in.Usages[0].Container.Container != "sidecar" {
		t.Fatalf("usages = %+v, want only the sidecar", in.Usages)
	}
}

// TestCollectImagePolicy_ToleratesMissingCollections mirrors a cluster with no
// Jobs or standalone Pods: absent collections are not an error.
func TestCollectImagePolicy_ToleratesMissingCollections(t *testing.T) {
	lister := &fakeLister{data: map[string][]json.RawMessage{
		"/apis/apps/v1/deployments": {raw(`{"metadata":{"namespace":"shop","name":"api"},"spec":{"template":{"spec":{
			"containers":[{"name":"app","image":"registry.io/team/api:v1"}]}}}}`)},
	}}

	in, err := NewCollector(lister, nil, nil).CollectImagePolicy(context.Background(), 7)
	if err != nil {
		t.Fatalf("CollectImagePolicy: %v", err)
	}
	if len(in.Usages) != 1 {
		t.Fatalf("usages = %d, want 1", len(in.Usages))
	}
}

// TestCollectImagePolicy_PropagatesListFailure ensures a broken cluster
// connection surfaces as an error (502 COLLECT_FAILED) instead of being
// reported as a clean, image-free cluster.
func TestCollectImagePolicy_PropagatesListFailure(t *testing.T) {
	for _, path := range []string{"/apis/apps/v1/deployments", "/api/v1/pods"} {
		lister := failingLister{failOn: path}
		in, err := NewCollector(lister, nil, nil).CollectImagePolicy(context.Background(), 7)
		if err == nil {
			t.Fatalf("expected the %s failure to propagate", path)
		}
		if len(in.Usages) != 0 {
			t.Fatalf("a failed collection must not return a partial bundle: %+v", in)
		}
	}
}

// TestCollectGitOpsDrift_MapsResourcesAndManagers verifies the GitOps drift
// collector captures each scanned kind with its last-applied-configuration
// (decoded from the annotation JSON string) and live spec/data, detects the
// manager, and records GitOps-managed namespaces.
func TestCollectGitOpsDrift_MapsResourcesAndManagers(t *testing.T) {
	lister := &fakeLister{data: map[string][]json.RawMessage{
		"/api/v1/namespaces": {
			raw(`{"metadata":{"name":"gitops","annotations":{"kustomize.toolkit.fluxcd.io/name":"app"}}}`),
			raw(`{"metadata":{"name":"shop"}}`),
		},
		"/apis/apps/v1/deployments": {raw(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{
			"namespace":"shop","name":"api","uid":"u1",
			"annotations":{"kubectl.kubernetes.io/last-applied-configuration":"{\"apiVersion\":\"apps/v1\",\"kind\":\"Deployment\",\"metadata\":{\"name\":\"api\"},\"spec\":{\"replicas\":3}}"}},"spec":{"replicas":3}}`)},
		"/apis/apps/v1/statefulsets": {raw(`{"apiVersion":"apps/v1","kind":"StatefulSet","metadata":{
			"namespace":"shop","name":"db","uid":"u2",
			"annotations":{"argocd.argoproj.io/tracking-id":"app:db"}},"spec":{"replicas":1}}`)},
		"/apis/apps/v1/daemonsets": {raw(`{"apiVersion":"apps/v1","kind":"DaemonSet","metadata":{
			"namespace":"kube-system","name":"agent","uid":"u3"},"spec":{}}`)},
		"/api/v1/configmaps": {raw(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{
			"namespace":"shop","name":"cfg","uid":"u4",
			"annotations":{"kubectl.kubernetes.io/last-applied-configuration":"{\"apiVersion\":\"v1\",\"kind\":\"ConfigMap\",\"data\":{\"KEY\":\"v1\"}}"}},
			"data":{"KEY":"v1"}}`)},
		"/api/v1/secrets": {raw(`{"apiVersion":"v1","kind":"Secret","metadata":{
			"namespace":"shop","name":"sec","uid":"u5"},"data":{}}`)},
	}}

	in, err := NewCollector(lister, nil, nil).CollectGitOpsDrift(context.Background(), 7)
	if err != nil {
		t.Fatalf("CollectGitOpsDrift: %v", err)
	}
	if len(in.Resources) != 5 {
		t.Fatalf("resources = %d, want 5: %+v", len(in.Resources), in.Resources)
	}

	byName := map[string]gitopsdrift.ManagedResource{}
	for _, r := range in.Resources {
		byName[r.Namespace+"/"+r.Name] = r
	}

	dep := byName["shop/api"]
	if dep.Kind != "Deployment" || dep.UID != "u1" {
		t.Fatalf("deployment = %+v", dep)
	}
	if dep.Manager != gitopsdrift.ManagerKubectl {
		t.Fatalf("deployment manager = %q, want kubectl", dep.Manager)
	}
	if len(dep.AppliedConfig) == 0 {
		t.Fatal("deployment missing applied config")
	}
	if string(dep.LiveBody) != `{"replicas":3}` {
		t.Fatalf("deployment live body = %s, want {\"replicas\":3}", dep.LiveBody)
	}

	if byName["shop/db"].Manager != gitopsdrift.ManagerArgoCD {
		t.Fatalf("statefulset manager = %q, want argocd", byName["shop/db"].Manager)
	}

	cfg := byName["shop/cfg"]
	if len(cfg.AppliedConfig) == 0 {
		t.Fatal("configmap missing applied config")
	}
	if string(cfg.LiveBody) != `{"KEY":"v1"}` {
		t.Fatalf("configmap live body = %s, want {\"KEY\":\"v1\"}", cfg.LiveBody)
	}

	// No last-applied annotation and no GitOps annotation -> unknown manager.
	if byName["kube-system/agent"].Manager != "" {
		t.Fatalf("agent manager = %q, want empty", byName["kube-system/agent"].Manager)
	}

	// Only the namespace carrying a Flux annotation is recorded as managed.
	if len(in.ManagedNamespaces) != 1 || in.ManagedNamespaces[0] != "gitops" {
		t.Fatalf("managed namespaces = %+v, want [gitops]", in.ManagedNamespaces)
	}
}

// TestCollectGitOpsDrift_PropagatesListFailure ensures a broken cluster
// connection surfaces as an error (502 COLLECT_FAILED) rather than a partial
// or empty bundle.
func TestCollectGitOpsDrift_PropagatesListFailure(t *testing.T) {
	for _, path := range []string{"/api/v1/namespaces", "/apis/apps/v1/deployments"} {
		lister := failingLister{failOn: path}
		in, err := NewCollector(lister, nil, nil).CollectGitOpsDrift(context.Background(), 7)
		if err == nil {
			t.Fatalf("expected the %s failure to propagate", path)
		}
		if len(in.Resources) != 0 {
			t.Fatalf("a failed collection must not return a partial bundle: %+v", in)
		}
	}
}

// fakeUsage is a test UsageSeriesSource keyed by "<node>:<metric>".
type fakeUsage struct {
	series map[string][]capacity.Sample
}

func (f fakeUsage) NodeUsageSeries(_ context.Context, _ int64, node, metric string, _, _ time.Time) ([]capacity.Sample, error) {
	return f.series[node+":"+metric], nil
}

// TestCollectCapacity_AggregatesNodeCapacityAndUsage verifies the collector
// sums node allocatable capacity and point-wise aggregates per-node usage into
// a single cluster series.
func TestCollectCapacity_AggregatesNodeCapacityAndUsage(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	lister := &fakeLister{data: map[string][]json.RawMessage{
		"/api/v1/nodes": {
			raw(`{"metadata":{"name":"n1","uid":"n1"},"status":{"allocatable":{"cpu":"4","memory":"8Gi"}}}`),
			raw(`{"metadata":{"name":"n2","uid":"n2"},"status":{"allocatable":{"cpu":"4","memory":"8Gi"}}}`),
		},
	}}
	usage := fakeUsage{series: map[string][]capacity.Sample{
		"n1:cpu": {
			{Timestamp: now.Add(-2 * time.Hour), Value: 1_000_000_000},
			{Timestamp: now.Add(-1 * time.Hour), Value: 2_000_000_000},
		},
		"n2:cpu": {
			{Timestamp: now.Add(-2 * time.Hour), Value: 1_000_000_000},
			{Timestamp: now.Add(-1 * time.Hour), Value: 2_000_000_000},
		},
		"n1:memory": {
			{Timestamp: now.Add(-2 * time.Hour), Value: 4 * 1024 * 1024 * 1024},
			{Timestamp: now.Add(-1 * time.Hour), Value: 6 * 1024 * 1024 * 1024},
		},
		"n2:memory": {
			{Timestamp: now.Add(-2 * time.Hour), Value: 4 * 1024 * 1024 * 1024},
			{Timestamp: now.Add(-1 * time.Hour), Value: 6 * 1024 * 1024 * 1024},
		},
	}}

	in, err := NewCollector(lister, nil, usage).CollectCapacity(context.Background(), 7)
	if err != nil {
		t.Fatalf("CollectCapacity: %v", err)
	}
	// 2 nodes x 4 cores = 8 cores = 8e9 nanocores.
	if in.CPU.Capacity != 8_000_000_000 {
		t.Errorf("cpu capacity = %d, want 8e9", in.CPU.Capacity)
	}
	// 2 nodes x 8Gi = 16Gi bytes.
	if in.Memory.Capacity != 16*1024*1024*1024 {
		t.Errorf("mem capacity = %d, want 16Gi", in.Memory.Capacity)
	}
	// n1 and n2 share timestamps, so they sum into one cluster point per tick.
	if len(in.CPU.Samples) != 2 {
		t.Fatalf("cpu samples = %d, want 2 (aggregated)", len(in.CPU.Samples))
	}
	if in.CPU.Samples[1].Value != 4_000_000_000 {
		t.Errorf("latest aggregated cpu = %v, want 4e9", in.CPU.Samples[1].Value)
	}
	if len(in.Memory.Samples) != 2 {
		t.Fatalf("mem samples = %d, want 2 (aggregated)", len(in.Memory.Samples))
	}
}

// TestCollectCapacity_NoUsageSource_CapacityOnly verifies that without a
// metrics history source the collector still reports capacity but no series.
func TestCollectCapacity_NoUsageSource_CapacityOnly(t *testing.T) {
	lister := &fakeLister{data: map[string][]json.RawMessage{
		"/api/v1/nodes": {
			raw(`{"metadata":{"name":"n1","uid":"n1"},"status":{"allocatable":{"cpu":"2","memory":"4Gi"}}}`),
		},
	}}

	in, err := NewCollector(lister, nil, nil).CollectCapacity(context.Background(), 7)
	if err != nil {
		t.Fatalf("CollectCapacity: %v", err)
	}
	if in.CPU.Capacity != 2_000_000_000 {
		t.Errorf("cpu capacity = %d, want 2e9", in.CPU.Capacity)
	}
	if len(in.CPU.Samples) != 0 || len(in.Memory.Samples) != 0 {
		t.Errorf("expected no samples without a usage source")
	}
}

// TestCollectCapacity_PropagatesListFailure ensures a broken node List surfaces
// as an error rather than a partial bundle.
func TestCollectCapacity_PropagatesListFailure(t *testing.T) {
	lister := failingLister{failOn: "/api/v1/nodes"}
	in, err := NewCollector(lister, nil, nil).CollectCapacity(context.Background(), 7)
	if err == nil {
		t.Fatalf("expected the /api/v1/nodes failure to propagate")
	}
	if in.CPU.Capacity != 0 || len(in.CPU.Samples) != 0 {
		t.Fatalf("a failed collection must not return a partial bundle: %+v", in)
	}
}

// TestCollectPolicy_ControllerTemplate extracts the pod template of a
// Deployment: resource requests/limits, security context, probes and host
// access flags all map into the policy workload model.
func TestCollectPolicy_ControllerTemplate(t *testing.T) {
	lister := &fakeLister{data: map[string][]json.RawMessage{
		"/apis/apps/v1/deployments": {
			raw(`{"metadata":{"namespace":"prod","name":"web","uid":"u-web"},"spec":{"template":{"spec":{
				"hostNetwork":true,
				"containers":[{
					"name":"app",
					"resources":{"requests":{"cpu":"100m","memory":"128Mi"},"limits":{"cpu":"1"}},
					"securityContext":{"privileged":true,"allowPrivilegeEscalation":true,"runAsNonRoot":false},
					"livenessProbe":{"httpGet":{"path":"/healthz"}},
					"readinessProbe":{"httpGet":{"path":"/ready"}},
					"startupProbe":{"httpGet":{"path":"/startup"}}
				}]
			}}}}`),
		},
		"/api/v1/pods": {},
	}}

	in, err := NewCollector(lister, nil, nil).CollectPolicy(context.Background(), 7)
	if err != nil {
		t.Fatalf("CollectPolicy: %v", err)
	}
	if len(in.Workloads) != 1 {
		t.Fatalf("workloads = %d, want 1", len(in.Workloads))
	}
	wl := in.Workloads[0]
	if wl.Kind != "Deployment" || wl.Namespace != "prod" || wl.Name != "web" || !wl.HostNetwork {
		t.Fatalf("workload = %+v, want Deployment prod/web with hostNetwork", wl)
	}
	if len(wl.Containers) != 1 {
		t.Fatalf("containers = %d, want 1", len(wl.Containers))
	}
	c := wl.Containers[0]
	if !c.CPURequest || !c.MemoryRequest || !c.HasResourceLimits {
		t.Fatalf("resource flags = %+v, want all true", c)
	}
	if c.Privileged == nil || !*c.Privileged {
		t.Fatalf("privileged = %v, want true", c.Privileged)
	}
	if c.AllowPrivilegeEscalation == nil || !*c.AllowPrivilegeEscalation {
		t.Fatalf("allowPrivilegeEscalation = %v, want true", c.AllowPrivilegeEscalation)
	}
	if c.RunAsNonRoot == nil || *c.RunAsNonRoot {
		t.Fatalf("runAsNonRoot = %v, want false", c.RunAsNonRoot)
	}
	if !c.LivenessProbe || !c.ReadinessProbe || !c.StartupProbe {
		t.Fatalf("probes = %+v, want all present", c)
	}
}

// TestCollectPolicy_SkipsOwnedPods verifies that bare Pods are only included
// when they have no owner, and that an owned Pod does not double-count.
func TestCollectPolicy_SkipsOwnedPods(t *testing.T) {
	lister := &fakeLister{data: map[string][]json.RawMessage{
		"/apis/apps/v1/deployments": {},
		"/api/v1/pods": {
			raw(`{"metadata":{"namespace":"prod","name":"owned","ownerReferences":[{"kind":"ReplicaSet"}],"uid":"p1"},"spec":{"hostPID":true}}`),
			raw(`{"metadata":{"namespace":"prod","name":"standalone","uid":"p2"},"spec":{"hostIPC":true}}`),
		},
	}}

	in, err := NewCollector(lister, nil, nil).CollectPolicy(context.Background(), 7)
	if err != nil {
		t.Fatalf("CollectPolicy: %v", err)
	}
	if len(in.Workloads) != 1 {
		t.Fatalf("workloads = %d, want 1 (only the standalone pod)", len(in.Workloads))
	}
	wl := in.Workloads[0]
	if wl.Kind != "Pod" || wl.Name != "standalone" || !wl.HostIPC || wl.HostPID {
		t.Fatalf("workload = %+v, want Pod standalone with hostIPC", wl)
	}
}

// TestCollectPolicy_PropagatesListFailure ensures a broken deployment List
// surfaces as an error rather than a partial bundle.
func TestCollectPolicy_PropagatesListFailure(t *testing.T) {
	lister := failingLister{failOn: "/apis/apps/v1/deployments"}
	in, err := NewCollector(lister, nil, nil).CollectPolicy(context.Background(), 7)
	if err == nil {
		t.Fatalf("expected the deployments failure to propagate")
	}
	if len(in.Workloads) != 0 {
		t.Fatalf("a failed collection must not return a partial bundle: %+v", in)
	}
}

// TestCollectHPA_MapsScalingFields extracts scaling bounds, target metric and
// current utilization from an autoscaling/v2 HPA.
func TestCollectHPA_MapsScalingFields(t *testing.T) {
	lister := &fakeLister{data: map[string][]json.RawMessage{
		"/apis/autoscaling/v2/horizontalpodautoscalers": {
			raw(`{"metadata":{"namespace":"prod","name":"web","uid":"u-web"},"spec":{
				"minReplicas":2,"maxReplicas":10,
				"metrics":[{"type":"Resource","resource":{"name":"cpu","target":{"averageUtilization":80}}}]
			},"status":{"currentReplicas":4,"currentMetrics":[{"type":"Resource","resource":{"name":"cpu","current":{"averageUtilization":65}}}]}}`),
		},
	}}

	in, err := NewCollector(lister, nil, nil).CollectHPA(context.Background(), 7)
	if err != nil {
		t.Fatalf("CollectHPA: %v", err)
	}
	if len(in.HPAs) != 1 {
		t.Fatalf("hpas = %d, want 1", len(in.HPAs))
	}
	h := in.HPAs[0]
	if h.Namespace != "prod" || h.Name != "web" {
		t.Fatalf("hpa identity = %+v, want prod/web", h)
	}
	if h.MinReplicas == nil || *h.MinReplicas != 2 {
		t.Fatalf("min_replicas = %v, want 2", h.MinReplicas)
	}
	if h.MaxReplicas != 10 || h.CurrentReplicas != 4 {
		t.Fatalf("replicas = %d/%d, want 10/4", h.MaxReplicas, h.CurrentReplicas)
	}
	if h.TargetMetric != "cpu" || h.TargetValue != 80 {
		t.Fatalf("target = %q/%v, want cpu/80", h.TargetMetric, h.TargetValue)
	}
	if h.CurrentUtilizationPct == nil || *h.CurrentUtilizationPct != 65 {
		t.Fatalf("current_utilization_pct = %v, want 65", h.CurrentUtilizationPct)
	}
}

// TestCollectHPA_PropagatesListFailure ensures a broken HPA List surfaces as
// an error rather than a partial bundle.
func TestCollectHPA_PropagatesListFailure(t *testing.T) {
	lister := failingLister{failOn: "/apis/autoscaling/v2/horizontalpodautoscalers"}
	in, err := NewCollector(lister, nil, nil).CollectHPA(context.Background(), 7)
	if err == nil {
		t.Fatalf("expected the hpa list failure to propagate")
	}
	if len(in.HPAs) != 0 {
		t.Fatalf("a failed collection must not return a partial bundle: %+v", in)
	}
}

// TestCollectPDB_MapsWorkloadsAndBudgets extracts workload replicas/labels and
// PDB budget/status fields.
func TestCollectPDB_MapsWorkloadsAndBudgets(t *testing.T) {
	lister := &fakeLister{data: map[string][]json.RawMessage{
		"/apis/apps/v1/deployments": {
			raw(`{"metadata":{"namespace":"prod","name":"web","uid":"u-web","labels":{"app":"web"}},"spec":{"replicas":3}}`),
		},
		"/apis/apps/v1/statefulsets": {},
		"/apis/apps/v1/daemonsets":   {},
		"/apis/policy/v1/poddisruptionbudgets": {
			raw(`{"metadata":{"namespace":"prod","name":"web-pdb","uid":"u-pdb"},"spec":{"minAvailable":1,"selector":{"matchLabels":{"app":"web"}}},"status":{"expectedPods":3,"disruptionsAllowed":2}}`),
		},
	}}

	in, err := NewCollector(lister, nil, nil).CollectPDB(context.Background(), 7)
	if err != nil {
		t.Fatalf("CollectPDB: %v", err)
	}
	if len(in.Workloads) != 1 {
		t.Fatalf("workloads = %d, want 1", len(in.Workloads))
	}
	w := in.Workloads[0]
	if w.Kind != "Deployment" || w.Namespace != "prod" || w.Name != "web" || w.Replicas != 3 {
		t.Fatalf("workload = %+v, want Deployment prod/web replicas=3", w)
	}
	if w.Labels["app"] != "web" {
		t.Fatalf("workload labels = %+v, want app=web", w.Labels)
	}
	if len(in.PDBs) != 1 {
		t.Fatalf("pdbs = %d, want 1", len(in.PDBs))
	}
	p := in.PDBs[0]
	if p.MinAvailable != "1" || p.SelectorLabels["app"] != "web" {
		t.Fatalf("pdb spec = %+v, want minAvailable=1 selector app=web", p)
	}
	if p.ExpectedPods != 3 || p.DisruptionsAllowed != 2 {
		t.Fatalf("pdb status = %d/%d, want 3/2", p.ExpectedPods, p.DisruptionsAllowed)
	}
}

// TestCollectPDB_PropagatesListFailure ensures a broken PDB List surfaces as
// an error rather than a partial bundle.
func TestCollectPDB_PropagatesListFailure(t *testing.T) {
	lister := failingLister{failOn: "/apis/policy/v1/poddisruptionbudgets"}
	in, err := NewCollector(lister, nil, nil).CollectPDB(context.Background(), 7)
	if err == nil {
		t.Fatalf("expected the pdb list failure to propagate")
	}
	if len(in.Workloads) != 0 || len(in.PDBs) != 0 {
		t.Fatalf("a failed collection must not return a partial bundle: %+v", in)
	}
}

// TestCollectIngress_MapsExposure extracts hosts, TLS, ingress class and
// backend Service references, plus the Services for resolution.
func TestCollectIngress_MapsExposure(t *testing.T) {
	lister := &fakeLister{data: map[string][]json.RawMessage{
		"/apis/networking.k8s.io/v1/ingresses": {
			raw(`{"metadata":{"namespace":"prod","name":"web","uid":"u-web"},"spec":{
				"ingressClassName":"nginx",
				"tls":[{"hosts":["web.example.com"],"secretName":"web-tls"}],
				"rules":[{"host":"web.example.com","http":{"paths":[{"path":"/","pathType":"Prefix","backend":{"service":{"name":"web-svc"}}}]}}]
			}}`),
		},
		"/api/v1/services": {
			raw(`{"metadata":{"namespace":"prod","name":"web-svc"}}`),
		},
	}}

	in, err := NewCollector(lister, nil, nil).CollectIngress(context.Background(), 7)
	if err != nil {
		t.Fatalf("CollectIngress: %v", err)
	}
	if len(in.Ingresses) != 1 {
		t.Fatalf("ingresses = %d, want 1", len(in.Ingresses))
	}
	ig := in.Ingresses[0]
	if ig.Namespace != "prod" || ig.Name != "web" || ig.IngressClassName != "nginx" {
		t.Fatalf("ingress = %+v, want prod/web nginx", ig)
	}
	if !ig.HasTLS {
		t.Fatal("has_tls = false, want true")
	}
	if len(ig.Hosts) != 1 || ig.Hosts[0] != "web.example.com" {
		t.Fatalf("hosts = %+v, want ['web.example.com']", ig.Hosts)
	}
	if len(ig.Backends) != 1 || ig.Backends[0].Name != "web-svc" {
		t.Fatalf("backends = %+v, want ['web-svc']", ig.Backends)
	}
	if len(in.Services) != 1 || in.Services[0].Name != "web-svc" {
		t.Fatalf("services = %+v, want ['web-svc']", in.Services)
	}
}

// TestCollectIngress_PropagatesListFailure ensures a broken Ingress List
// surfaces as an error rather than a partial bundle.
func TestCollectIngress_PropagatesListFailure(t *testing.T) {
	lister := failingLister{failOn: "/apis/networking.k8s.io/v1/ingresses"}
	in, err := NewCollector(lister, nil, nil).CollectIngress(context.Background(), 7)
	if err == nil {
		t.Fatalf("expected the ingress list failure to propagate")
	}
	if len(in.Ingresses) != 0 {
		t.Fatalf("a failed collection must not return a partial bundle: %+v", in)
	}
}
