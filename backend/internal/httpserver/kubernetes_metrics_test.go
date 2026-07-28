package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

func TestMetricsAPIUnavailableHasExplicitHTTPError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/v1/clusters/7/metrics/nodes", nil)

	handler := kubernetesHandler{}
	if !handler.writeServiceError(context, k8sgateway.ErrMetricsAPIUnavailable) {
		t.Fatal("expected service error to be written")
	}
	if recorder.Code != http.StatusFailedDependency || !strings.Contains(recorder.Body.String(), `"code":"METRICS_API_UNAVAILABLE"`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
