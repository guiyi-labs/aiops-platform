package kubernetes

import (
	"context"
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/kubernetes/fake"

	"k8s-aiops.local/backend/internal/cluster"
)

// fakeDiscoveryProvider implements DiscoveryProvider for tests. It returns a
// pre-built discovery interface or an error.
type fakeDiscoveryProvider struct {
	disco discovery.DiscoveryInterface
	err   error
}

func (f fakeDiscoveryProvider) Discovery(clusterID int64, kubeconfig []byte) (discovery.DiscoveryInterface, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.disco, nil
}

// fakeCredentials implements CredentialSource for tests.
type fakeCredentials struct {
	clusterID  int64
	kubeconfig []byte
	err        error
}

func (f fakeCredentials) Access(ctx context.Context, clusterID int64) (cluster.Cluster, []byte, error) {
	if f.err != nil {
		return cluster.Cluster{}, nil, f.err
	}
	return cluster.Cluster{ID: f.clusterID}, f.kubeconfig, nil
}

// newFakeDiscovery builds a fake.FakeDiscovery preloaded with the given
// APIResourceLists. fake.FakeDiscovery fully implements discovery.DiscoveryInterface,
// so we don't need a hand-written stub.
func newFakeDiscovery(resources []*metav1.APIResourceList) *fakediscovery.FakeDiscovery {
	clientset := fake.NewSimpleClientset()
	fakeDisco, ok := clientset.Discovery().(*fakediscovery.FakeDiscovery)
	if !ok {
		panic("fake clientset discovery is not *fakediscovery.FakeDiscovery")
	}
	fakeDisco.Resources = resources
	return fakeDisco
}

// TestAPIResourcesNilDiscoveryReturnsWhitelistOnly verifies that when the
// Service is constructed without a DiscoveryProvider (e.g. route-contract
// tests), APIResources returns only the fixed whitelist without error.
func TestAPIResourcesNilDiscoveryReturnsWhitelistOnly(t *testing.T) {
	svc := NewService(fakeCredentials{}, nil, nil)
	resources, err := svc.APIResources(context.Background(), 1)
	if err != nil {
		t.Fatalf("APIResources error: %v", err)
	}
	if len(resources) != len(fixedAPIResources) {
		t.Fatalf("len = %d, want %d (whitelist only)", len(resources), len(fixedAPIResources))
	}
	for _, r := range resources {
		if r.Source != "whitelist" {
			t.Fatalf("source = %q, want whitelist (no discovery provider)", r.Source)
		}
	}
}

// TestAPIResourcesDiscoveryMergesCRDs verifies that dynamically discovered
// CRDs are merged on top of the fixed whitelist, with subresources and
// non-listable resources excluded.
func TestAPIResourcesDiscoveryMergesCRDs(t *testing.T) {
	fakeDisco := newFakeDiscovery([]*metav1.APIResourceList{
		{
			GroupVersion: "custom.io/v1",
			APIResources: []metav1.APIResource{
				{Name: "widgets", Kind: "Widget", Namespaced: true, Verbs: []string{"list", "get"}},
				{Name: "widgets/status", Kind: "Widget", Namespaced: true, Verbs: []string{"get"}}, // subresource, must be skipped
				{Name: "gadgets", Kind: "Gadget", Namespaced: false, Verbs: []string{"list"}},
			},
		},
		{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{Name: "pods", Kind: "Pod", Namespaced: true, Verbs: []string{"list", "get"}},    // already in whitelist, must be deduped
				{Name: "bindings", Kind: "Binding", Namespaced: true, Verbs: []string{"create"}}, // no list/get, must be skipped
			},
		},
	})
	svc := NewService(fakeCredentials{}, nil, fakeDiscoveryProvider{disco: fakeDisco})
	resources, err := svc.APIResources(context.Background(), 1)
	if err != nil {
		t.Fatalf("APIResources error: %v", err)
	}

	// Expect: whitelist (all) + widgets + gadgets (NOT pods — deduped; NOT
	// widgets/status — subresource; NOT bindings — no list/get verb).
	widgetFound := false
	gadgetFound := false
	podCount := 0
	for _, r := range resources {
		switch r.Group + "/" + r.Resource {
		case "custom.io/widgets":
			widgetFound = true
			if !r.Namespaced {
				t.Fatalf("widgets should be namespaced")
			}
			if r.Source != "discovery" {
				t.Fatalf("widgets source = %q, want discovery", r.Source)
			}
		case "custom.io/gadgets":
			gadgetFound = true
		case "/pods":
			podCount++
		}
	}
	if !widgetFound {
		t.Fatal("custom.io/widgets not merged from discovery")
	}
	if !gadgetFound {
		t.Fatal("custom.io/gadgets not merged from discovery")
	}
	if podCount != 1 {
		t.Fatalf("pods count = %d, want 1 (deduped, not duplicated from discovery)", podCount)
	}

	// Verify no subresources present.
	for _, r := range resources {
		if contains(r.Resource, "/") {
			t.Fatalf("subresource leaked into output: %s", r.Resource)
		}
	}
}

// TestAPIResourcesDiscoveryErrorFallsBackToWhitelist verifies that when the
// discovery API returns an error, the whitelist is still returned (graceful
// degradation).
func TestAPIResourcesDiscoveryErrorFallsBackToWhitelist(t *testing.T) {
	svc := NewService(fakeCredentials{}, nil, fakeDiscoveryProvider{err: errors.New("discovery unavailable")})
	resources, err := svc.APIResources(context.Background(), 1)
	if err != nil {
		t.Fatalf("APIResources error: %v", err)
	}
	if len(resources) != len(fixedAPIResources) {
		t.Fatalf("len = %d, want %d (whitelist fallback)", len(resources), len(fixedAPIResources))
	}
}

// TestAPIResourcesCredentialErrorFallsBackToWhitelist verifies that when the
// credential source returns an error, the whitelist is still returned.
func TestAPIResourcesCredentialErrorFallsBackToWhitelist(t *testing.T) {
	svc := NewService(fakeCredentials{err: errors.New("credential not found")}, nil, fakeDiscoveryProvider{})
	resources, err := svc.APIResources(context.Background(), 1)
	if err != nil {
		t.Fatalf("APIResources error: %v", err)
	}
	if len(resources) != len(fixedAPIResources) {
		t.Fatalf("len = %d, want %d (whitelist fallback)", len(resources), len(fixedAPIResources))
	}
}

// TestAPIResourcesSorted verifies the output is sorted by group, version,
// resource for stable frontend rendering.
func TestAPIResourcesSorted(t *testing.T) {
	svc := NewService(fakeCredentials{}, nil, nil)
	resources, err := svc.APIResources(context.Background(), 1)
	if err != nil {
		t.Fatalf("APIResources error: %v", err)
	}
	for i := 1; i < len(resources); i++ {
		prev := resources[i-1]
		curr := resources[i]
		if prev.Group > curr.Group {
			t.Fatalf("not sorted by group at index %d: %q > %q", i, prev.Group, curr.Group)
		}
		if prev.Group == curr.Group && prev.Version > curr.Version {
			t.Fatalf("not sorted by version at index %d", i)
		}
		if prev.Group == curr.Group && prev.Version == curr.Version && prev.Resource > curr.Resource {
			t.Fatalf("not sorted by resource at index %d: %q > %q", i, prev.Resource, curr.Resource)
		}
	}
}

// TestFixedAPIResourcesWhitelistCoversCoreKinds verifies the whitelist
// contains the core operator resources the console renders by default.
func TestFixedAPIResourcesWhitelistCoversCoreKinds(t *testing.T) {
	wantKinds := map[string]bool{
		"Pod": false, "Deployment": false, "Service": false,
		"ConfigMap": false, "Namespace": false, "Node": false,
		"StatefulSet": false, "DaemonSet": false, "Job": false,
		"Ingress": false, "PersistentVolumeClaim": false,
		"NetworkPolicy": false, "PodDisruptionBudget": false,
	}
	for _, r := range fixedAPIResources {
		if _, ok := wantKinds[r.Kind]; ok {
			wantKinds[r.Kind] = true
		}
	}
	for kind, found := range wantKinds {
		if !found {
			t.Errorf("whitelist missing required kind %q", kind)
		}
	}
}

// contains is a substring check (avoids importing strings just for one call).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || indexOf(s, substr) >= 0)
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
