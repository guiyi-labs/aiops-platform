package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestMetricsUsesRouteTemplatesAndStatusClasses(t *testing.T) {
	metrics := NewMetrics()
	metrics.Observe("GET", "/api/v1/users/:user_id", 200, 10*time.Millisecond)
	metrics.Observe("GET", "/api/v1/users/:user_id", 403, 20*time.Millisecond)
	output := metrics.Render()
	if !strings.Contains(output, `route="/api/v1/users/:user_id"`) || strings.Contains(output, "secret-user") || !strings.Contains(output, `status_class="2xx"`) || !strings.Contains(output, `status_class="4xx"`) {
		t.Fatalf("metrics output = %s", output)
	}
}

func TestRequestMetricsUsesRegisteredRouteTemplate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	metrics := NewMetrics()
	router := gin.New()
	router.Use(requestMetrics(metrics))
	router.GET("/api/v1/users/:user_id", func(c *gin.Context) { c.Status(http.StatusOK) })

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/users/secret-user", nil))

	output := metrics.Render()
	if recorder.Code != http.StatusOK || !strings.Contains(output, `route="/api/v1/users/:user_id"`) || strings.Contains(output, "secret-user") {
		t.Fatalf("status=%d metrics=%s", recorder.Code, output)
	}
}
