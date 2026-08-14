package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

func TestCapacityPreview_BadBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/optimization/capacity/preview", capacityPreviewHandler{kubernetes: &k8sgateway.Service{}}.preview)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/optimization/capacity/preview", strings.NewReader(`not json`)))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "INVALID_BODY") {
		t.Fatalf("bad body: code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCapacityPreview_MissingClusterID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/optimization/capacity/preview", capacityPreviewHandler{kubernetes: &k8sgateway.Service{}}.preview)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/optimization/capacity/preview", strings.NewReader(`{"cpu_request_nanocores":1000000000}`)))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "INVALID_CLUSTER") {
		t.Fatalf("missing cluster: code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCapacityPreview_AllZeroRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/optimization/capacity/preview", capacityPreviewHandler{kubernetes: &k8sgateway.Service{}}.preview)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/optimization/capacity/preview", strings.NewReader(`{"cluster_id":1}`)))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "INVALID_REQUEST") {
		t.Fatalf("all-zero requests: code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCapacityPreview_NegativeRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/optimization/capacity/preview", capacityPreviewHandler{kubernetes: &k8sgateway.Service{}}.preview)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/optimization/capacity/preview",
		strings.NewReader(`{"cluster_id":1,"cpu_request_nanocores":-100}`)))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "INVALID_REQUEST") {
		t.Fatalf("negative request: code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCapacityPreview_NilKubernetes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/optimization/capacity/preview", capacityPreviewHandler{}.preview)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/optimization/capacity/preview",
		strings.NewReader(`{"cluster_id":1,"cpu_request_nanocores":1000000000}`)))
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "COLLECTOR_UNAVAILABLE") {
		t.Fatalf("nil k8s: code=%d body=%s", w.Code, w.Body.String())
	}
}
