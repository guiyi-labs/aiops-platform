package httpserver

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/apiquery"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
	"k8s-aiops.local/backend/internal/metricshistory"
	"k8s-aiops.local/backend/internal/servicemesh"
)

type meshCRDNoop struct{}

func (meshCRDNoop) CustomResources(_ context.Context, _ int64, _, _, _, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[map[string]interface{}], error) {
	return apiquery.ListResponse[map[string]interface{}]{}, nil
}
func (meshCRDNoop) CustomResource(_ context.Context, _ int64, _, _, _, _, _ string) (map[string]interface{}, error) {
	return nil, servicemesh.ErrIstioNotInstalled
}

func newServiceMeshTestEngine(t *testing.T, svc *servicemesh.Service) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", int64(1))
		c.Set("workspace_id", int64(1))
		c.Set("workspace_roles", map[int64][]string{1: {"viewer"}})
		c.Next()
	})
	h := servicemeshHandler{service: svc}
	cluster := r.Group("/api/v1/clusters/:cluster_id/servicemesh")
	{
		cluster.GET("/virtualservices", h.listVirtualServices)
		cluster.GET("/virtualservices/namespaces/:namespace/name/:name", h.getVirtualService)
		cluster.GET("/destinationrules", h.listDestinationRules)
		cluster.GET("/destinationrules/namespaces/:namespace/name/:name", h.getDestinationRule)
		cluster.GET("/traffic-metrics", h.trafficMetrics)
	}
	return r
}

func TestServiceMesh_ListVirtualServices503WhenNil(t *testing.T) {
	r := newServiceMeshTestEngine(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/servicemesh/virtualservices", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when service=nil, got %d", w.Code)
	}
}

func TestServiceMesh_ListVirtualServicesBadClusterID(t *testing.T) {
	svc := servicemesh.NewService(meshCRDNoop{}, nil)
	r := newServiceMeshTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/abc/servicemesh/virtualservices", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad cluster_id, got %d", w.Code)
	}
}

func TestServiceMesh_ListVirtualServices200(t *testing.T) {
	svc := servicemesh.NewService(meshCRDNoop{}, nil)
	r := newServiceMeshTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/servicemesh/virtualservices?limit=10", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestServiceMesh_ListDestinationRulesInvalidLimit(t *testing.T) {
	svc := servicemesh.NewService(meshCRDNoop{}, nil)
	r := newServiceMeshTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/servicemesh/destinationrules?limit=nope", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid limit, got %d", w.Code)
	}
}

func TestServiceMesh_GetVirtualServiceMissingName(t *testing.T) {
	svc := servicemesh.NewService(meshCRDNoop{}, nil)
	r := newServiceMeshTestEngine(t, svc)
	// handler route requires both namespace and name; sending empty params.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/servicemesh/virtualservices/namespaces/prod/name/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	// Gin will 404 when trailing name segment is empty or not matched, so
	// both 404 (no name param parsed) and 400 (handler sees "") are OK.
	if w.Code != http.StatusBadRequest && w.Code != http.StatusNotFound {
		t.Fatalf("expected 400/404 for missing name, got %d", w.Code)
	}
}

func TestServiceMesh_TrafficMetricsBadWindowStart(t *testing.T) {
	svc := servicemesh.NewService(nil, nil)
	r := newServiceMeshTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/servicemesh/traffic-metrics?window_start=not-a-date", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad window_start, got %d", w.Code)
	}
}

func TestServiceMesh_TrafficMetricsBadTopK(t *testing.T) {
	svc := servicemesh.NewService(nil, nil)
	r := newServiceMeshTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/servicemesh/traffic-metrics?top_k=9999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad top_k, got %d", w.Code)
	}
}

func TestServiceMesh_TrafficMetricsDataUnavailable(t *testing.T) {
	svc := servicemesh.NewService(meshCRDNoop{}, nil)
	r := newServiceMeshTestEngine(t, svc)
	end := time.Now().UTC()
	start := end.Add(-time.Hour)
	url := "/api/v1/clusters/1/servicemesh/traffic-metrics?window_start=" +
		strings.ReplaceAll(start.Format(time.RFC3339), ":", "%3A") +
		"&window_end=" + strings.ReplaceAll(end.Format(time.RFC3339), ":", "%3A")
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for nil metrics reader, got %d: %s", w.Code, w.Body.String())
	}
}

// --- M109 supplemental: error branches + success paths for mesh handlers ---

type meshCRDDynamic struct {
	listErr  error
	getErr   error
	listItem map[string]interface{}
	getItem  map[string]interface{}
}

func (d meshCRDDynamic) CustomResources(_ context.Context, _ int64, _, _, _, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[map[string]interface{}], error) {
	if d.listErr != nil {
		return apiquery.ListResponse[map[string]interface{}]{}, d.listErr
	}
	items := []map[string]interface{}{}
	if d.listItem != nil {
		items = append(items, d.listItem)
	}
	return apiquery.ListResponse[map[string]interface{}]{Items: items, Total: len(items)}, nil
}

func (d meshCRDDynamic) CustomResource(_ context.Context, _ int64, _, _, _, _, _ string) (map[string]interface{}, error) {
	if d.getErr != nil {
		return nil, d.getErr
	}
	if d.getItem == nil {
		return nil, servicemesh.ErrIstioNotInstalled
	}
	return d.getItem, nil
}

func meshVSMap(name string) map[string]interface{} {
	return map[string]interface{}{
		"metadata": map[string]interface{}{
			"name": name, "namespace": "default", "uid": "uid-1", "creationTimestamp": "2026-01-01T00:00:00Z",
		},
		"spec": map[string]interface{}{
			"hosts":    []interface{}{"reviews.example.com"},
			"gateways": []interface{}{"mesh"},
			"http":     []interface{}{map[string]interface{}{"route": []interface{}{}}},
		},
	}
}

func meshDRMap(name string) map[string]interface{} {
	return map[string]interface{}{
		"metadata": map[string]interface{}{
			"name": name, "namespace": "default", "uid": "uid-2", "creationTimestamp": "2026-01-01T00:00:00Z",
		},
		"spec": map[string]interface{}{
			"host":          "reviews",
			"subsets":       []interface{}{},
			"trafficPolicy": map[string]interface{}{},
		},
	}
}

type meshMetricsReader struct{}

func (meshMetricsReader) Query(context.Context, metricshistory.SeriesQuery) (metricshistory.SeriesResponse, error) {
	return metricshistory.SeriesResponse{}, nil
}

func TestServiceMesh_ListVirtualServicesBadOffset(t *testing.T) {
	svc := servicemesh.NewService(meshCRDNoop{}, nil)
	r := newServiceMeshTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/servicemesh/virtualservices?offset=-1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !contains(w.Body.String(), "INVALID_QUERY") {
		t.Fatalf("expected 400 INVALID_QUERY, got %d: %s", w.Code, w.Body.String())
	}
}

func TestServiceMesh_ListVirtualServicesServerError(t *testing.T) {
	svc := servicemesh.NewService(meshCRDDynamic{listErr: errors.New("crd down")}, nil)
	r := newServiceMeshTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/servicemesh/virtualservices", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError || !contains(w.Body.String(), "SERVICEMESH_FAILED") {
		t.Fatalf("expected 500 SERVICEMESH_FAILED, got %d: %s", w.Code, w.Body.String())
	}
}

func TestServiceMesh_ListVirtualServicesIstioMissing(t *testing.T) {
	svc := servicemesh.NewService(meshCRDDynamic{listErr: k8sgateway.ErrResourceNotFound}, nil)
	r := newServiceMeshTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/servicemesh/virtualservices", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound || !contains(w.Body.String(), "SERVICEMESH_NOT_INSTALLED") {
		t.Fatalf("expected 404 SERVICEMESH_NOT_INSTALLED, got %d: %s", w.Code, w.Body.String())
	}
}

func TestServiceMesh_GetVirtualServiceSuccess(t *testing.T) {
	svc := servicemesh.NewService(meshCRDDynamic{getItem: meshVSMap("reviews")}, nil)
	r := newServiceMeshTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/servicemesh/virtualservices/namespaces/default/name/reviews", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !contains(w.Body.String(), `"name":"reviews"`) {
		t.Fatalf("expected 200 with reviews, got %d: %s", w.Code, w.Body.String())
	}
}

func TestServiceMesh_GetVirtualServiceNotFound(t *testing.T) {
	svc := servicemesh.NewService(meshCRDNoop{}, nil)
	r := newServiceMeshTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/servicemesh/virtualservices/namespaces/default/name/reviews", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound || !contains(w.Body.String(), "SERVICEMESH_NOT_INSTALLED") {
		t.Fatalf("expected 404 SERVICEMESH_NOT_INSTALLED, got %d: %s", w.Code, w.Body.String())
	}
}

func TestServiceMesh_GetVirtualServiceServerError(t *testing.T) {
	svc := servicemesh.NewService(meshCRDDynamic{getErr: errors.New("crd down")}, nil)
	r := newServiceMeshTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/servicemesh/virtualservices/namespaces/default/name/reviews", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError || !contains(w.Body.String(), "SERVICEMESH_FAILED") {
		t.Fatalf("expected 500 SERVICEMESH_FAILED, got %d: %s", w.Code, w.Body.String())
	}
}

func TestServiceMesh_ListDestinationRulesSuccess(t *testing.T) {
	svc := servicemesh.NewService(meshCRDDynamic{listItem: meshDRMap("reviews-dr")}, nil)
	r := newServiceMeshTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/servicemesh/destinationrules", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !contains(w.Body.String(), `"total":1`) {
		t.Fatalf("expected 200 with total 1, got %d: %s", w.Code, w.Body.String())
	}
}

func TestServiceMesh_GetDestinationRuleSuccess(t *testing.T) {
	svc := servicemesh.NewService(meshCRDDynamic{getItem: meshDRMap("reviews-dr")}, nil)
	r := newServiceMeshTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/servicemesh/destinationrules/namespaces/default/name/reviews-dr", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !contains(w.Body.String(), `"name":"reviews-dr"`) {
		t.Fatalf("expected 200 with reviews-dr, got %d: %s", w.Code, w.Body.String())
	}
}

func TestServiceMesh_GetDestinationRuleServerError(t *testing.T) {
	svc := servicemesh.NewService(meshCRDDynamic{getErr: errors.New("crd down")}, nil)
	r := newServiceMeshTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/servicemesh/destinationrules/namespaces/default/name/reviews-dr", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError || !contains(w.Body.String(), "SERVICEMESH_FAILED") {
		t.Fatalf("expected 500 SERVICEMESH_FAILED, got %d: %s", w.Code, w.Body.String())
	}
}

func TestServiceMesh_TrafficMetricsBadWindowEnd(t *testing.T) {
	svc := servicemesh.NewService(nil, nil)
	r := newServiceMeshTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/servicemesh/traffic-metrics?window_end=not-a-date", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !contains(w.Body.String(), "INVALID_QUERY") {
		t.Fatalf("expected 400 for bad window_end, got %d", w.Code)
	}
}

func TestServiceMesh_TrafficMetricsSuccess(t *testing.T) {
	svc := servicemesh.NewService(meshCRDNoop{}, meshMetricsReader{})
	r := newServiceMeshTestEngine(t, svc)
	end := time.Now().UTC()
	start := end.Add(-time.Hour)
	url := "/api/v1/clusters/1/servicemesh/traffic-metrics?window_start=" +
		strings.ReplaceAll(start.Format(time.RFC3339), ":", "%3A") +
		"&window_end=" + strings.ReplaceAll(end.Format(time.RFC3339), ":", "%3A") +
		"&namespace=default&service_name=reviews&top_k=5"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for traffic metrics, got %d: %s", w.Code, w.Body.String())
	}
}

func TestServiceMesh_TrafficMetricsServerError(t *testing.T) {
	svc := servicemesh.NewService(meshCRDNoop{}, meshMetricsErrReader{})
	r := newServiceMeshTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/servicemesh/traffic-metrics", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	// metrics reader is non-nil but all queries fail -> partial success is empty
	// 200, not an error; only a nil reader or invalid window produces 503/400.
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 partial (fail-soft), got %d: %s", w.Code, w.Body.String())
	}
}

type meshMetricsErrReader struct{}

func (meshMetricsErrReader) Query(context.Context, metricshistory.SeriesQuery) (metricshistory.SeriesResponse, error) {
	return metricshistory.SeriesResponse{}, errors.New("metrics down")
}

func TestServiceMesh_WriteMeshErrorTable(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{servicemesh.ErrIstioNotInstalled, http.StatusNotFound, "SERVICEMESH_NOT_INSTALLED"},
		{servicemesh.ErrMeshDataUnavailable, http.StatusServiceUnavailable, "SERVICEMESH_TRAFFIC_UNAVAILABLE"},
		{servicemesh.ErrInvalidWindow, http.StatusBadRequest, "INVALID_QUERY"},
		{k8sgateway.ErrResourceNotFound, http.StatusNotFound, "RESOURCE_NOT_FOUND"},
		{errors.New("boom"), http.StatusInternalServerError, "SERVICEMESH_FAILED"},
	}
	for _, tc := range cases {
		gin.SetMode(gin.TestMode)
		router := gin.New()
		router.GET("/t", func(c *gin.Context) { writeServiceMeshError(c, tc.err) })
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/t", nil))
		if rec.Code != tc.status || !contains(rec.Body.String(), tc.code) {
			t.Fatalf("err=%v want %d %s got %d %s", tc.err, tc.status, tc.code, rec.Code, rec.Body.String())
		}
	}
}
