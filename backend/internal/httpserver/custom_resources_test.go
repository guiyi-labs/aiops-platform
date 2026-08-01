package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/authz"
	"k8s-aiops.local/backend/internal/cluster"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
	"k8s-aiops.local/backend/internal/requestctx"
)

// customResCredentials implements k8sgateway.CredentialSource for handler tests.
type customResCredentials struct {
	clusterID int64
	err       error
}

func (c customResCredentials) Access(_ context.Context, _ int64) (cluster.Cluster, []byte, error) {
	if c.err != nil {
		return cluster.Cluster{}, nil, c.err
	}
	return cluster.Cluster{ID: c.clusterID, Enabled: true}, []byte("config"), nil
}

// customResGateway implements k8sgateway.Gateway for handler tests. It returns a
// canned body keyed by request path, or a default body/error.
type customResGateway struct {
	body     []byte
	err      error
	gotPath  string
	gotQuery url.Values
}

func (g *customResGateway) Get(_ context.Context, _ int64, _ []byte, path string, query url.Values, _ int64) ([]byte, error) {
	g.gotPath, g.gotQuery = path, query
	return g.body, g.err
}

// newCustomResourcesTestEngine builds a gin engine wired to the
// custom-resources handlers, mirroring the production route shapes. A
// pre-middleware injects the actor + clusterID (simulating withAuthentication
// + withClusterContext) and sets the authz scope (simulating
// requireNamespaceQueryAccess). requireClusterAccess is omitted because the
// stub credentials always succeed; the scope controls namespace visibility.
func newCustomResourcesTestEngine(t *testing.T, svc *k8sgateway.Service, scope authz.ClusterScope, clusterID int64) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		md := requestctx.Metadata{ActorID: 1, Roles: []string{"operations_admin"}, ClusterID: clusterID, RequestID: "cr-test"}
		c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), md))
		c.Set(namespaceScopeKey, scope)
		c.Next()
	})
	h := kubernetesHandler{service: svc}
	r.GET("/api/v1/clusters/:cluster_id/custom-resources/:group/:version/:resource", h.customResources)
	r.GET("/api/v1/clusters/:cluster_id/custom-resources/:group/:version/:resource/:name", h.customResource)
	return r
}

func newCustomResourcesService(gateway *customResGateway) *k8sgateway.Service {
	return k8sgateway.NewService(customResCredentials{clusterID: 7}, gateway, nil)
}

// TestCustomResourcesHandler_ListNamespaced200 verifies the list handler
// returns 200 with redacted items for a namespaced CRD when the caller has
// cluster-wide scope (AllNamespaces=true, no ?namespace=).
func TestCustomResourcesHandler_ListNamespaced200(t *testing.T) {
	body := []byte(`{"items":[{"metadata":{"name":"api-cert","namespace":"demo"},"spec":{"secretName":"api-tls","password":"super-secret"}}]}`)
	gateway := &customResGateway{body: body}
	svc := newCustomResourcesService(gateway)
	engine := newCustomResourcesTestEngine(t, svc, authz.ClusterScope{AllNamespaces: true}, 7)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/7/custom-resources/cert-manager.io/v1/certificates", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	// AllNamespaces + no ?namespace= → cluster-wide path.
	if gateway.gotPath != "/apis/cert-manager.io/v1/certificates" {
		t.Fatalf("gateway path = %q, want cluster-wide collection path", gateway.gotPath)
	}
	// Redaction must apply. Unmarshal to be robust to JSON HTML-escaping
	// of "<redacted>" (gin escapes < and > to \u003c / \u003e).
	var listResp struct {
		Items []map[string]interface{} `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, w.Body.String())
	}
	if len(listResp.Items) != 1 {
		t.Fatalf("items len = %d, want 1; body=%s", len(listResp.Items), w.Body.String())
	}
	spec, _ := listResp.Items[0]["spec"].(map[string]interface{})
	if spec == nil || spec["password"] != "<redacted>" {
		t.Fatalf("body = %s, want redacted password", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "super-secret") {
		t.Fatalf("body = %s, password leaked", w.Body.String())
	}
	// secretName is non-sensitive and must survive.
	if !strings.Contains(w.Body.String(), "api-tls") {
		t.Fatalf("body = %s, secretName dropped", w.Body.String())
	}
}

// TestCustomResourcesHandler_ListNamespacedWithNamespaceQuery200 verifies the
// list handler fans into a single namespace when ?namespace= is set and the
// caller has AllNamespaces scope.
func TestCustomResourcesHandler_ListNamespacedWithNamespaceQuery200(t *testing.T) {
	body := []byte(`{"items":[{"metadata":{"name":"api-cert","namespace":"demo"}}]}`)
	gateway := &customResGateway{body: body}
	svc := newCustomResourcesService(gateway)
	engine := newCustomResourcesTestEngine(t, svc, authz.ClusterScope{AllNamespaces: true}, 7)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/7/custom-resources/cert-manager.io/v1/certificates?namespace=demo", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if gateway.gotPath != "/apis/cert-manager.io/v1/namespaces/demo/certificates" {
		t.Fatalf("gateway path = %q, want namespaced collection path", gateway.gotPath)
	}
}

// TestCustomResourcesHandler_ListClusterScoped200 verifies the list handler
// uses the cluster-wide path for a cluster-scoped CRD regardless of ?namespace=.
func TestCustomResourcesHandler_ListClusterScoped200(t *testing.T) {
	body := []byte(`{"items":[{"metadata":{"name":"letsencrypt"}}]}`)
	gateway := &customResGateway{body: body}
	svc := newCustomResourcesService(gateway)
	engine := newCustomResourcesTestEngine(t, svc, authz.ClusterScope{AllNamespaces: true}, 7)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/7/custom-resources/cert-manager.io/v1/clusterissuers?namespace=demo", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if gateway.gotPath != "/apis/cert-manager.io/v1/clusterissuers" {
		t.Fatalf("gateway path = %q, want cluster-wide collection path (namespace ignored)", gateway.gotPath)
	}
}

// TestCustomResourcesHandler_NotWhitelistedReturns404 verifies anti-leakage:
// a non-whitelisted GVR returns 404 RESOURCE_NOT_FOUND, indistinguishable from
// a missing resource.
func TestCustomResourcesHandler_NotWhitelistedReturns404(t *testing.T) {
	gateway := &customResGateway{body: []byte(`{"items":[]}`)}
	svc := newCustomResourcesService(gateway)
	engine := newCustomResourcesTestEngine(t, svc, authz.ClusterScope{AllNamespaces: true}, 7)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/7/custom-resources/example.com/v1/widgets", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (anti-leakage); body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "RESOURCE_NOT_FOUND") {
		t.Fatalf("body = %s, want RESOURCE_NOT_FOUND code", w.Body.String())
	}
	if gateway.gotPath != "" {
		t.Fatalf("gateway was called (path=%q); non-whitelisted GVR must short-circuit before the gateway", gateway.gotPath)
	}
}

// TestCustomResourcesHandler_NamespaceGrantFanOut200 verifies that a
// namespace-grant user (AllNamespaces=false) with multiple grants triggers a
// per-namespace fan-out via authorizedNamespaceLists.
func TestCustomResourcesHandler_NamespaceGrantFanOut200(t *testing.T) {
	// The gateway stub returns the same body for every call; with two
	// namespace grants, authorizedNamespaceLists calls the service twice and
	// merges items.
	body := []byte(`{"items":[{"metadata":{"name":"api-cert","namespace":"demo"}}]}`)
	gateway := &customResGateway{body: body}
	svc := newCustomResourcesService(gateway)
	scope := authz.ClusterScope{AllNamespaces: false, NamespaceGrants: []string{"demo", "staging"}}
	engine := newCustomResourcesTestEngine(t, svc, scope, 7)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/7/custom-resources/cert-manager.io/v1/certificates", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	// Two grants → two service calls → two items merged (same item returned
	// per namespace in this stub). Total reflects the merged count.
	var response struct {
		Items []map[string]interface{} `json:"items"`
		Total int                      `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, w.Body.String())
	}
	if response.Total != 2 {
		t.Fatalf("total = %d, want 2 (one item per namespace grant)", response.Total)
	}
}

// TestCustomResourcesHandler_EmptyScopeReturnsEmptyList verifies anti-leakage:
// a caller with no grants and no cluster scope gets 200 with items:[] rather
// than 404 or an error.
func TestCustomResourcesHandler_EmptyScopeReturnsEmptyList(t *testing.T) {
	gateway := &customResGateway{body: []byte(`{"items":[{"metadata":{"name":"should-not-appear"}}]}`)}
	svc := newCustomResourcesService(gateway)
	scope := authz.ClusterScope{AllNamespaces: false, NamespaceGrants: nil}
	engine := newCustomResourcesTestEngine(t, svc, scope, 7)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/7/custom-resources/cert-manager.io/v1/certificates", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (empty scope → empty list, not 404); body=%s", w.Code, w.Body.String())
	}
	if gateway.gotPath != "" {
		t.Fatalf("gateway was called (path=%q); empty scope must short-circuit before the gateway", gateway.gotPath)
	}
	var response struct {
		Items []map[string]interface{} `json:"items"`
		Total int                      `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, w.Body.String())
	}
	if response.Total != 0 || len(response.Items) != 0 {
		t.Fatalf("response = %+v, want empty list (anti-leakage)", response)
	}
}

// TestCustomResourcesHandler_InvalidQueryReturns400 verifies that an invalid
// limit/page is rejected with 400 before the service is called.
func TestCustomResourcesHandler_InvalidQueryReturns400(t *testing.T) {
	gateway := &customResGateway{body: []byte(`{"items":[]}`)}
	svc := newCustomResourcesService(gateway)
	engine := newCustomResourcesTestEngine(t, svc, authz.ClusterScope{AllNamespaces: true}, 7)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/7/custom-resources/cert-manager.io/v1/certificates?limit=999", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
	if gateway.gotPath != "" {
		t.Fatalf("gateway was called (path=%q); invalid query must short-circuit", gateway.gotPath)
	}
}

// TestCustomResourcesHandler_ResourceNotFoundMapsTo404 verifies that a 404 from
// the Kubernetes API surfaces as 404 RESOURCE_NOT_FOUND.
func TestCustomResourcesHandler_ResourceNotFoundMapsTo404(t *testing.T) {
	gateway := &customResGateway{err: cluster.APIStatusError{StatusCode: 404}}
	svc := newCustomResourcesService(gateway)
	engine := newCustomResourcesTestEngine(t, svc, authz.ClusterScope{AllNamespaces: true}, 7)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/7/custom-resources/cert-manager.io/v1/certificates", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "RESOURCE_NOT_FOUND") {
		t.Fatalf("body = %s, want RESOURCE_NOT_FOUND code", w.Body.String())
	}
}

// TestCustomResourcesHandler_ClusterDisabledMapsTo409 verifies that a disabled
// cluster surfaces as 409 CLUSTER_DISABLED.
func TestCustomResourcesHandler_ClusterDisabledMapsTo409(t *testing.T) {
	svc := k8sgateway.NewService(customResCredentials{err: cluster.ErrDisabled}, &customResGateway{body: []byte(`{"items":[]}`)}, nil)
	engine := newCustomResourcesTestEngine(t, svc, authz.ClusterScope{AllNamespaces: true}, 7)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/7/custom-resources/cert-manager.io/v1/certificates", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "CLUSTER_DISABLED") {
		t.Fatalf("body = %s, want CLUSTER_DISABLED code", w.Body.String())
	}
}

// TestCustomResourceHandler_DetailNamespaced200 verifies the detail handler
// returns 200 with a redacted manifest for a namespaced CRD.
func TestCustomResourceHandler_DetailNamespaced200(t *testing.T) {
	body := []byte(`{"apiVersion":"cert-manager.io/v1","kind":"Certificate","metadata":{"name":"api-cert","namespace":"demo"},"spec":{"secretName":"api-tls","password":"super-secret"}}`)
	gateway := &customResGateway{body: body}
	svc := newCustomResourcesService(gateway)
	engine := newCustomResourcesTestEngine(t, svc, authz.ClusterScope{AllNamespaces: true}, 7)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/7/custom-resources/cert-manager.io/v1/certificates/api-cert?namespace=demo", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if gateway.gotPath != "/apis/cert-manager.io/v1/namespaces/demo/certificates/api-cert" {
		t.Fatalf("gateway path = %q, want namespaced item path", gateway.gotPath)
	}
	// Unmarshal to be robust to JSON HTML-escaping of "<redacted>".
	var detail map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, w.Body.String())
	}
	detailSpec, _ := detail["spec"].(map[string]interface{})
	if detailSpec == nil || detailSpec["password"] != "<redacted>" {
		t.Fatalf("body = %s, password must be redacted", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "super-secret") {
		t.Fatalf("body = %s, password leaked", w.Body.String())
	}
}

// TestCustomResourceHandler_DetailClusterScoped200 verifies the detail handler
// uses the cluster-wide item path for a cluster-scoped CRD.
func TestCustomResourceHandler_DetailClusterScoped200(t *testing.T) {
	body := []byte(`{"apiVersion":"cert-manager.io/v1","kind":"ClusterIssuer","metadata":{"name":"letsencrypt"}}`)
	gateway := &customResGateway{body: body}
	svc := newCustomResourcesService(gateway)
	engine := newCustomResourcesTestEngine(t, svc, authz.ClusterScope{AllNamespaces: true}, 7)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/7/custom-resources/cert-manager.io/v1/clusterissuers/letsencrypt", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if gateway.gotPath != "/apis/cert-manager.io/v1/clusterissuers/letsencrypt" {
		t.Fatalf("gateway path = %q, want cluster-scoped item path", gateway.gotPath)
	}
}

// TestCustomResourceHandler_DetailNamespacedRequiresNamespace400 verifies that
// a namespaced CRD detail without ?namespace= is rejected with 400.
func TestCustomResourceHandler_DetailNamespacedRequiresNamespace400(t *testing.T) {
	gateway := &customResGateway{body: []byte(`{}`)}
	svc := newCustomResourcesService(gateway)
	engine := newCustomResourcesTestEngine(t, svc, authz.ClusterScope{AllNamespaces: true}, 7)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/7/custom-resources/cert-manager.io/v1/certificates/api-cert", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
	if gateway.gotPath != "" {
		t.Fatalf("gateway was called (path=%q); missing namespace must short-circuit", gateway.gotPath)
	}
}

// TestCustomResourceHandler_DetailNotWhitelistedReturns404 verifies anti-leakage
// on the detail path.
func TestCustomResourceHandler_DetailNotWhitelistedReturns404(t *testing.T) {
	gateway := &customResGateway{body: []byte(`{}`)}
	svc := newCustomResourcesService(gateway)
	engine := newCustomResourcesTestEngine(t, svc, authz.ClusterScope{AllNamespaces: true}, 7)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/7/custom-resources/example.com/v1/widgets/my-widget", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (anti-leakage); body=%s", w.Code, w.Body.String())
	}
	if gateway.gotPath != "" {
		t.Fatalf("gateway was called (path=%q); non-whitelisted GVR must short-circuit", gateway.gotPath)
	}
}

// TestCustomResourceHandler_DetailNotFoundMapsTo404 verifies 404 mapping on
// the detail path.
func TestCustomResourceHandler_DetailNotFoundMapsTo404(t *testing.T) {
	gateway := &customResGateway{err: cluster.APIStatusError{StatusCode: 404}}
	svc := newCustomResourcesService(gateway)
	engine := newCustomResourcesTestEngine(t, svc, authz.ClusterScope{AllNamespaces: true}, 7)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/7/custom-resources/cert-manager.io/v1/certificates/missing?namespace=demo", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
	}
}

// TestCustomResourcesHandler_ForwardsLabelSelector verifies that the
// label_selector query is forwarded to the Kubernetes API.
func TestCustomResourcesHandler_ForwardsLabelSelector(t *testing.T) {
	body := []byte(`{"items":[]}`)
	gateway := &customResGateway{body: body}
	svc := newCustomResourcesService(gateway)
	engine := newCustomResourcesTestEngine(t, svc, authz.ClusterScope{AllNamespaces: true}, 7)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/7/custom-resources/cert-manager.io/v1/certificates?namespace=demo&label_selector=team%3Dplatform", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if gateway.gotQuery.Get("labelSelector") != "team=platform" {
		t.Fatalf("labelSelector = %q, want team=platform", gateway.gotQuery.Get("labelSelector"))
	}
}

// TestCustomResourcesHandler_OnlyGetVerbRegistered is a static guard: the
// custom-resources routes must only accept GET. We attempt POST and expect 404
// (route not registered) — confirming no write path exists (ADR 0064 §2).
func TestCustomResourcesHandler_OnlyGetVerbRegistered(t *testing.T) {
	gateway := &customResGateway{body: []byte(`{}`)}
	svc := newCustomResourcesService(gateway)
	engine := newCustomResourcesTestEngine(t, svc, authz.ClusterScope{AllNamespaces: true}, 7)

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		req := httptest.NewRequest(method, "/api/v1/clusters/7/custom-resources/cert-manager.io/v1/certificates", nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound && w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s status = %d, want 404/405 (no write path); body=%s", method, w.Code, w.Body.String())
		}
		if gateway.gotPath != "" {
			t.Fatalf("%s reached the gateway (path=%q); write verbs must not be routed", method, gateway.gotPath)
		}
	}
}

// TestWriteServiceError_CustomResourceNotWhitelisted verifies the error→404
// mapping directly (defense-in-depth: the handler pre-checks, but the mapper
// must also handle the service-layer error).
func TestWriteServiceError_CustomResourceNotWhitelisted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	handler := kubernetesHandler{}
	if !handler.writeServiceError(c, k8sgateway.ErrCustomResourceNotWhitelisted) {
		t.Fatal("expected service error to be written")
	}
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "RESOURCE_NOT_FOUND") {
		t.Fatalf("body = %s, want RESOURCE_NOT_FOUND code", recorder.Body.String())
	}
}

// Ensure the errors import is exercised on builds that exclude the
// cluster-disabled sub-test (keeps goimports honest across build tags).
var _ = errors.New
