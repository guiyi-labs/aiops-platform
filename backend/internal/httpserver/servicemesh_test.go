package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/apiquery"
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
