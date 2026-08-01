package kubernetes

import (
	"context"
	"errors"
	"testing"

	"k8s-aiops.local/backend/internal/apiquery"
	"k8s-aiops.local/backend/internal/cluster"
)

// TestIsCustomResourceBrowsable covers the whitelist lookup for both
// namespaced and cluster-scoped entries, plus the not-found path.
func TestIsCustomResourceBrowsable(t *testing.T) {
	svc := NewService(credentialStub{enabled: true}, &gatewayStub{}, nil)

	if namespaced, ok := svc.IsCustomResourceBrowsable("cert-manager.io", "v1", "certificates"); !ok || !namespaced {
		t.Fatalf("cert-manager.io/v1/certificates = (namespaced=%v, ok=%v), want (true, true)", namespaced, ok)
	}
	if namespaced, ok := svc.IsCustomResourceBrowsable("cert-manager.io", "v1", "clusterissuers"); !ok || namespaced {
		t.Fatalf("cert-manager.io/v1/clusterissuers = (namespaced=%v, ok=%v), want (false, true)", namespaced, ok)
	}
	if _, ok := svc.IsCustomResourceBrowsable("example.com", "v1", "widgets"); ok {
		t.Fatalf("example.com/v1/widgets should not be browsable")
	}
	// Version mismatch: v1alpha1 is not whitelisted even if the resource is.
	if _, ok := svc.IsCustomResourceBrowsable("cert-manager.io", "v1alpha1", "certificates"); ok {
		t.Fatalf("cert-manager.io/v1alpha1/certificates should not be browsable (version pinned)")
	}
	// Core group is never whitelisted (covered by typed endpoints).
	if _, ok := svc.IsCustomResourceBrowsable("", "v1", "pods"); ok {
		t.Fatalf("core/v1/pods should not be browsable as a custom resource")
	}
}

// TestCustomResourcesNamespacedListAndRedaction verifies that a namespaced CRD
// list builds the namespace-scoped path, pages, and redacts sensitive fields.
func TestCustomResourcesNamespacedListAndRedaction(t *testing.T) {
	body := []byte(`{"apiVersion":"cert-manager.io/v1","kind":"CertificateList","items":[` +
		`{"metadata":{"name":"api-cert","namespace":"demo"},"spec":{"secretName":"api-tls"}},` +
		`{"metadata":{"name":"db-cert","namespace":"demo"},"spec":{"secretName":"db-tls","password":"super-secret"}},` +
		`{"metadata":{"name":"cache-cert","namespace":"demo"}}]}`)
	gateway := &gatewayStub{body: body}
	svc := NewService(credentialStub{enabled: true}, gateway, nil)

	response, err := svc.CustomResources(context.Background(), 7, "cert-manager.io", "v1", "certificates", "demo", apiquery.ListQuery{Page: 1, Limit: 2})
	if err != nil {
		t.Fatalf("CustomResources error: %v", err)
	}
	if gateway.path != "/apis/cert-manager.io/v1/namespaces/demo/certificates" {
		t.Fatalf("path = %q, want namespaced collection path", gateway.path)
	}
	if response.Total != 3 {
		t.Fatalf("total = %d, want 3", response.Total)
	}
	if len(response.Items) != 2 {
		t.Fatalf("len(items) = %d, want 2 (limit)", len(response.Items))
	}
	if response.Remaining != 1 {
		t.Fatalf("remaining = %d, want 1", response.Remaining)
	}
	// Verify redaction: the password field on db-cert (item index 1 in the
	// raw list) must be redacted. Item ordering follows the raw list order.
	dbCert := findItemByName(t, response.Items, "db-cert")
	if pw, ok := dbCert["spec"].(map[string]interface{})["password"]; ok {
		if pw != "<redacted>" {
			t.Fatalf("password = %v, want <redacted>", pw)
		}
	} else {
		t.Fatalf("spec.password missing after redaction")
	}
	// secretName is not a sensitive field and must be preserved.
	apiCert := findItemByName(t, response.Items, "api-cert")
	if apiCert["spec"].(map[string]interface{})["secretName"] != "api-tls" {
		t.Fatalf("secretName not preserved: %#v", apiCert["spec"])
	}
}

// TestCustomResourcesClusterScopedIgnoresNamespace verifies that a
// cluster-scoped CRD uses the cluster-wide path regardless of namespace.
func TestCustomResourcesClusterScopedIgnoresNamespace(t *testing.T) {
	body := []byte(`{"apiVersion":"cert-manager.io/v1","kind":"ClusterIssuerList","items":[{"metadata":{"name":"letsencrypt"}}]}`)
	gateway := &gatewayStub{body: body}
	svc := NewService(credentialStub{enabled: true}, gateway, nil)

	// namespace is non-empty but should be ignored for cluster-scoped CRDs.
	response, err := svc.CustomResources(context.Background(), 7, "cert-manager.io", "v1", "clusterissuers", "demo", apiquery.ListQuery{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("CustomResources error: %v", err)
	}
	if gateway.path != "/apis/cert-manager.io/v1/clusterissuers" {
		t.Fatalf("path = %q, want cluster-wide collection path", gateway.path)
	}
	if response.Total != 1 || len(response.Items) != 1 {
		t.Fatalf("response = %#v, want 1 item", response)
	}
}

// TestCustomResourcesAllNamespacesWhenNamespaceEmpty verifies that a namespaced
// CRD with an empty namespace lists across all namespaces (cluster-wide path).
func TestCustomResourcesAllNamespacesWhenNamespaceEmpty(t *testing.T) {
	body := []byte(`{"items":[{"metadata":{"name":"mon","namespace":"a"}},{"metadata":{"name":"mon","namespace":"b"}}]}`)
	gateway := &gatewayStub{body: body}
	svc := NewService(credentialStub{enabled: true}, gateway, nil)

	response, err := svc.CustomResources(context.Background(), 7, "monitoring.coreos.com", "v1", "servicemonitors", "", apiquery.ListQuery{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("CustomResources error: %v", err)
	}
	if gateway.path != "/apis/monitoring.coreos.com/v1/servicemonitors" {
		t.Fatalf("path = %q, want cluster-wide collection path", gateway.path)
	}
	if response.Total != 2 {
		t.Fatalf("total = %d, want 2", response.Total)
	}
}

// TestCustomResourcesNotWhitelistedReturnsErr verifies that a non-whitelisted
// GVR returns ErrCustomResourceNotWhitelisted without hitting the gateway.
func TestCustomResourcesNotWhitelistedReturnsErr(t *testing.T) {
	gateway := &gatewayStub{body: []byte(`{"items":[]}`)}
	svc := NewService(credentialStub{enabled: true}, gateway, nil)

	_, err := svc.CustomResources(context.Background(), 7, "example.com", "v1", "widgets", "demo", apiquery.ListQuery{Page: 1, Limit: 20})
	if !errors.Is(err, ErrCustomResourceNotWhitelisted) {
		t.Fatalf("err = %v, want ErrCustomResourceNotWhitelisted", err)
	}
	if gateway.path != "" {
		t.Fatalf("gateway was called (path=%q); non-whitelisted GVR must short-circuit", gateway.path)
	}
}

// TestCustomResourcesPropagatesClusterDisabled verifies that credential errors
// (e.g. disabled cluster) propagate from the gateway/credential layer.
func TestCustomResourcesPropagatesClusterDisabled(t *testing.T) {
	gateway := &gatewayStub{body: []byte(`{"items":[]}`)}
	svc := NewService(credentialStub{enabled: false}, gateway, nil)

	_, err := svc.CustomResources(context.Background(), 7, "cert-manager.io", "v1", "certificates", "demo", apiquery.ListQuery{Page: 1, Limit: 20})
	if !errors.Is(err, cluster.ErrDisabled) {
		t.Fatalf("err = %v, want cluster.ErrDisabled", err)
	}
}

// TestCustomResourcesPropagatesNotFound verifies that a 404 from the Kubernetes
// API maps to ErrResourceNotFound.
func TestCustomResourcesPropagatesNotFound(t *testing.T) {
	gateway := &gatewayStub{err: cluster.APIStatusError{StatusCode: 404}}
	svc := NewService(credentialStub{enabled: true}, gateway, nil)

	_, err := svc.CustomResources(context.Background(), 7, "cert-manager.io", "v1", "certificates", "demo", apiquery.ListQuery{Page: 1, Limit: 20})
	if !errors.Is(err, ErrResourceNotFound) {
		t.Fatalf("err = %v, want ErrResourceNotFound", err)
	}
}

// TestCustomResourcesForwardsSelectors verifies that label/field selectors and
// name filter are forwarded to the Kubernetes API query string.
func TestCustomResourcesForwardsSelectors(t *testing.T) {
	body := []byte(`{"items":[{"metadata":{"name":"api","namespace":"demo"}}]}`)
	gateway := &gatewayStub{body: body}
	svc := NewService(credentialStub{enabled: true}, gateway, nil)

	_, err := svc.CustomResources(context.Background(), 7, "cert-manager.io", "v1", "certificates", "demo", apiquery.ListQuery{Page: 1, Limit: 20, Name: "api", LabelSelector: "team=platform", FieldSelector: "status.phase=Ready"})
	if err != nil {
		t.Fatalf("CustomResources error: %v", err)
	}
	if gateway.query.Get("labelSelector") != "team=platform" || gateway.query.Get("fieldSelector") != "status.phase=Ready" {
		t.Fatalf("selectors not forwarded: %v", gateway.query)
	}
}

// TestCustomResourceDetailNamespacedAndRedaction verifies the single-instance
// read path for a namespaced CRD: correct path + redaction.
func TestCustomResourceDetailNamespacedAndRedaction(t *testing.T) {
	body := []byte(`{"apiVersion":"cert-manager.io/v1","kind":"Certificate","metadata":{"name":"api-cert","namespace":"demo"},"spec":{"secretName":"api-tls","password":"super-secret"}}`)
	gateway := &gatewayStub{body: body}
	svc := NewService(credentialStub{enabled: true}, gateway, nil)

	item, err := svc.CustomResource(context.Background(), 7, "cert-manager.io", "v1", "certificates", "demo", "api-cert")
	if err != nil {
		t.Fatalf("CustomResource error: %v", err)
	}
	if gateway.path != "/apis/cert-manager.io/v1/namespaces/demo/certificates/api-cert" {
		t.Fatalf("path = %q, want namespaced item path", gateway.path)
	}
	spec, ok := item["spec"].(map[string]interface{})
	if !ok {
		t.Fatalf("spec missing: %#v", item)
	}
	if spec["password"] != "<redacted>" {
		t.Fatalf("password = %v, want <redacted>", spec["password"])
	}
	if spec["secretName"] != "api-tls" {
		t.Fatalf("secretName = %v, want api-tls", spec["secretName"])
	}
}

// TestCustomResourceDetailClusterScoped verifies the single-instance read path
// for a cluster-scoped CRD uses the cluster-wide item path.
func TestCustomResourceDetailClusterScoped(t *testing.T) {
	body := []byte(`{"apiVersion":"cert-manager.io/v1","kind":"ClusterIssuer","metadata":{"name":"letsencrypt"}}`)
	gateway := &gatewayStub{body: body}
	svc := NewService(credentialStub{enabled: true}, gateway, nil)

	_, err := svc.CustomResource(context.Background(), 7, "cert-manager.io", "v1", "clusterissuers", "demo", "letsencrypt")
	if err != nil {
		t.Fatalf("CustomResource error: %v", err)
	}
	if gateway.path != "/apis/cert-manager.io/v1/clusterissuers/letsencrypt" {
		t.Fatalf("path = %q, want cluster-scoped item path (namespace omitted)", gateway.path)
	}
}

// TestCustomResourceDetailNotWhitelistedReturnsErr verifies the detail path
// rejects non-whitelisted GVRs without hitting the gateway.
func TestCustomResourceDetailNotWhitelistedReturnsErr(t *testing.T) {
	gateway := &gatewayStub{body: []byte(`{}`)}
	svc := NewService(credentialStub{enabled: true}, gateway, nil)

	_, err := svc.CustomResource(context.Background(), 7, "example.com", "v1", "widgets", "demo", "my-widget")
	if !errors.Is(err, ErrCustomResourceNotWhitelisted) {
		t.Fatalf("err = %v, want ErrCustomResourceNotWhitelisted", err)
	}
	if gateway.path != "" {
		t.Fatalf("gateway was called (path=%q); non-whitelisted GVR must short-circuit", gateway.path)
	}
}

// TestCustomResourceDetailPropagatesNotFound verifies 404 mapping on the detail
// path.
func TestCustomResourceDetailPropagatesNotFound(t *testing.T) {
	gateway := &gatewayStub{err: cluster.APIStatusError{StatusCode: 404}}
	svc := NewService(credentialStub{enabled: true}, gateway, nil)

	_, err := svc.CustomResource(context.Background(), 7, "cert-manager.io", "v1", "certificates", "demo", "missing")
	if !errors.Is(err, ErrResourceNotFound) {
		t.Fatalf("err = %v, want ErrResourceNotFound", err)
	}
}

// TestCustomResourcesRedactsDataAndStringData verifies that the M22 redaction
// rules for Secret-like data/stringData fields apply to CRD manifests too.
func TestCustomResourcesRedactsDataAndStringData(t *testing.T) {
	body := []byte(`{"items":[{"metadata":{"name":"cfg"},"data":{"token":"abc"},"stringData":{"password":"xyz"}}]}`)
	gateway := &gatewayStub{body: body}
	svc := NewService(credentialStub{enabled: true}, gateway, nil)

	response, err := svc.CustomResources(context.Background(), 7, "cert-manager.io", "v1", "certificates", "demo", apiquery.ListQuery{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("CustomResources error: %v", err)
	}
	if len(response.Items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(response.Items))
	}
	if response.Items[0]["data"] != "<redacted>" {
		t.Fatalf("data = %v, want <redacted>", response.Items[0]["data"])
	}
	if response.Items[0]["stringData"] != "<redacted>" {
		t.Fatalf("stringData = %v, want <redacted>", response.Items[0]["stringData"])
	}
}

// TestCustomResourceListPathBuilding is a table test for the path builders
// covering namespaced/cluster-scoped and empty-namespace cases.
func TestCustomResourceListPathBuilding(t *testing.T) {
	cases := []struct {
		name       string
		group      string
		version    string
		resource   string
		namespace  string
		namespaced bool
		want       string
	}{
		{"namespaced with ns", "cert-manager.io", "v1", "certificates", "demo", true, "/apis/cert-manager.io/v1/namespaces/demo/certificates"},
		{"namespaced empty ns lists all", "cert-manager.io", "v1", "certificates", "", true, "/apis/cert-manager.io/v1/certificates"},
		{"cluster-scoped ignores ns", "cert-manager.io", "v1", "clusterissuers", "demo", false, "/apis/cert-manager.io/v1/clusterissuers"},
		{"cluster-scoped empty ns", "cert-manager.io", "v1", "clusterissuers", "", false, "/apis/cert-manager.io/v1/clusterissuers"},
		{"group with dots preserved", "monitoring.coreos.com", "v1", "servicemonitors", "ops", true, "/apis/monitoring.coreos.com/v1/namespaces/ops/servicemonitors"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := customResourceListPath(tc.group, tc.version, tc.resource, tc.namespace, tc.namespaced)
			if got != tc.want {
				t.Fatalf("customResourceListPath = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestCustomResourceItemPathBuilding is a table test for the single-item path
// builder.
func TestCustomResourceItemPathBuilding(t *testing.T) {
	cases := []struct {
		label      string
		group      string
		version    string
		resource   string
		namespace  string
		name       string
		namespaced bool
		want       string
	}{
		{"namespaced", "cert-manager.io", "v1", "certificates", "demo", "api-cert", true, "/apis/cert-manager.io/v1/namespaces/demo/certificates/api-cert"},
		{"cluster-scoped", "cert-manager.io", "v1", "clusterissuers", "", "letsencrypt", false, "/apis/cert-manager.io/v1/clusterissuers/letsencrypt"},
		{"name with slash escaped", "batch", "v1", "jobs", "demo", "nightly/cleanup", true, "/apis/batch/v1/namespaces/demo/jobs/nightly%2Fcleanup"},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			got := customResourcePath(tc.group, tc.version, tc.resource, tc.namespace, tc.name, tc.namespaced)
			if got != tc.want {
				t.Fatalf("customResourcePath = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestCustomResourceNameExtractor verifies the metadata.name extractor used by
// filterAndPage / countNamed.
func TestCustomResourceNameExtractor(t *testing.T) {
	if name := customResourceName(map[string]interface{}{"metadata": map[string]interface{}{"name": "api"}}); name != "api" {
		t.Fatalf("name = %q, want api", name)
	}
	if name := customResourceName(map[string]interface{}{"metadata": map[string]interface{}{}}); name != "" {
		t.Fatalf("name = %q, want empty", name)
	}
	if name := customResourceName(map[string]interface{}{}); name != "" {
		t.Fatalf("name = %q, want empty", name)
	}
}

// TestCustomResourcesNameFilterAndSort verifies the name filter + sort
// interactions on a namespaced CRD list.
func TestCustomResourcesNameFilterAndSort(t *testing.T) {
	body := []byte(`{"items":[` +
		`{"metadata":{"name":"beta","namespace":"demo"}},` +
		`{"metadata":{"name":"alpha","namespace":"demo"}},` +
		`{"metadata":{"name":"gamma","namespace":"demo"}}]}`)

	t.Run("name filter narrows total", func(t *testing.T) {
		gateway := &gatewayStub{body: body}
		svc := NewService(credentialStub{enabled: true}, gateway, nil)
		// "al" matches only "alpha" (substring, case-insensitive).
		response, err := svc.CustomResources(context.Background(), 7, "cert-manager.io", "v1", "certificates", "demo", apiquery.ListQuery{Page: 1, Limit: 20, Name: "al"})
		if err != nil {
			t.Fatalf("CustomResources error: %v", err)
		}
		if response.Total != 1 {
			t.Fatalf("total = %d, want 1 (only alpha matches 'al')", response.Total)
		}
		if len(response.Items) != 1 || customResourceName(response.Items[0]) != "alpha" {
			t.Fatalf("items = %#v, want [alpha]", response.Items)
		}
	})

	t.Run("sort by name ascending", func(t *testing.T) {
		gateway := &gatewayStub{body: body}
		svc := NewService(credentialStub{enabled: true}, gateway, nil)
		response, err := svc.CustomResources(context.Background(), 7, "cert-manager.io", "v1", "certificates", "demo", apiquery.ListQuery{Page: 1, Limit: 20, SortBy: "name", Ascending: true})
		if err != nil {
			t.Fatalf("CustomResources error: %v", err)
		}
		if response.Total != 3 {
			t.Fatalf("total = %d, want 3", response.Total)
		}
		got := []string{
			customResourceName(response.Items[0]),
			customResourceName(response.Items[1]),
			customResourceName(response.Items[2]),
		}
		want := []string{"alpha", "beta", "gamma"}
		if got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
			t.Fatalf("sorted items = %v, want %v", got, want)
		}
	})
}

// findItemByName locates a redacted list item by metadata.name, failing the
// test if not found.
func findItemByName(t *testing.T, items []map[string]interface{}, name string) map[string]interface{} {
	t.Helper()
	for _, item := range items {
		if customResourceName(item) == name {
			return item
		}
	}
	t.Fatalf("item %q not found in response", name)
	return nil
}
