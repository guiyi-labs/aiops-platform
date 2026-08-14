package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/slo"
)

func TestBurnSummary_UnavailableAndQueryValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := sloHandler{service: nil}
	router := gin.New()
	router.GET("/api/v1/aiops/slos/burn-summary", h.burnSummary)

	if rec := httptest.NewRecorder(); true {
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/aiops/slos/burn-summary", nil))
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503 for nil service, got %d: %s", rec.Code, rec.Body.String())
		}
	}
}

func TestBurnSummary_QueryValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := slo.NewService(slo.NopRepository{}, nil)
	h := sloHandler{service: svc}
	router := gin.New()
	router.GET("/api/v1/aiops/slos/burn-summary", h.burnSummary)

	cases := []string{
		"/api/v1/aiops/slos/burn-summary?cluster_id=abc",
		"/api/v1/aiops/slos/burn-summary?limit=0",
		"/api/v1/aiops/slos/burn-summary?limit=999",
	}
	for _, path := range cases {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "INVALID_QUERY") {
			t.Fatalf("path=%s status=%d body=%s", path, rec.Code, rec.Body.String())
		}
	}

	// Valid query over empty definitions returns 200 with empty summary.
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/aiops/slos/burn-summary?limit=50", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Fatalf("expected empty items, got %s", rec.Body.String())
	}
}
