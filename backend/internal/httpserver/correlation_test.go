package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/correlation"
)

func newCorrelationTestEngine(t *testing.T, svc *correlation.Service) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := correlationHandler{service: svc}
	api := r.Group("/api/v1/aiops/correlation")
	api.GET("/rules", h.listCorrelationRules)
	api.GET("/cases", h.listCorrelationCases)
	api.GET("/cases/timeline", h.listCorrelationTimeline)
	api.GET("/cases/:id", h.getCorrelationCase)
	api.GET("/cases/:id/graph", h.getCorrelationCaseGraph)
	api.GET("/cases/:id/actions", h.listCorrelationActions)
	return r
}

// TestCorrelationHandler_ListRulesReturns200 verifies the rule catalog
// endpoint works without a service (it reads the static catalog).
func TestCorrelationHandler_ListRulesReturns200(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := correlationHandler{service: nil}
	r.GET("/api/v1/aiops/correlation/rules", h.listCorrelationRules)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/correlation/rules", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Items              []correlation.RuleDescriptor `json:"items"`
		CorrelationVersion string                       `json:"correlation_version"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Items) == 0 {
		t.Error("expected at least one rule in catalog")
	}
	if resp.CorrelationVersion != correlation.CorrelationVersion {
		t.Errorf("correlation_version: want %s, got %s", correlation.CorrelationVersion, resp.CorrelationVersion)
	}
}

// TestCorrelationHandler_ListCasesMissingClusterIDReturns400 verifies
// cluster_id is required.
func TestCorrelationHandler_ListCasesMissingClusterIDReturns400(t *testing.T) {
	svc := correlation.NewService(correlation.NopRepository{}, nil, nil)
	r := newCorrelationTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/correlation/cases", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestCorrelationHandler_ListCasesReturns200 verifies list with an empty
// repository returns an empty page.
func TestCorrelationHandler_ListCasesReturns200(t *testing.T) {
	svc := correlation.NewService(correlation.NopRepository{}, nil, nil)
	r := newCorrelationTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/correlation/cases?cluster_id=1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp correlation.CaseListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 0 {
		t.Errorf("expected 0 total, got %d", resp.Total)
	}
}

// TestCorrelationHandler_TimelineReturns200 verifies the timeline endpoint.
func TestCorrelationHandler_TimelineReturns200(t *testing.T) {
	svc := correlation.NewService(correlation.NopRepository{}, nil, nil)
	r := newCorrelationTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/correlation/cases/timeline?cluster_id=1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// TestCorrelationHandler_GetCaseNotFoundReturns404 verifies a non-existent
// case returns 404.
func TestCorrelationHandler_GetCaseNotFoundReturns404(t *testing.T) {
	svc := correlation.NewService(correlation.NopRepository{}, nil, nil)
	r := newCorrelationTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/correlation/cases/999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// TestCorrelationHandler_GetCaseGraphNotFoundReturns404 verifies the graph
// endpoint returns 404 for a non-existent case.
func TestCorrelationHandler_GetCaseGraphNotFoundReturns404(t *testing.T) {
	svc := correlation.NewService(correlation.NopRepository{}, nil, nil)
	r := newCorrelationTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/correlation/cases/999/graph", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// TestCorrelationHandler_ListActionsNotFoundReturns404 verifies the actions
// endpoint returns 404 for a non-existent case.
func TestCorrelationHandler_ListActionsNotFoundReturns404(t *testing.T) {
	svc := correlation.NewService(correlation.NopRepository{}, nil, nil)
	r := newCorrelationTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/correlation/cases/999/actions", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// TestCorrelationHandler_InvalidIDReturns400 verifies a non-numeric :id
// returns 400.
func TestCorrelationHandler_InvalidIDReturns400(t *testing.T) {
	svc := correlation.NewService(correlation.NopRepository{}, nil, nil)
	r := newCorrelationTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/correlation/cases/abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestCorrelationHandler_ServiceUnavailableReturns503 verifies that a nil
// service yields 503 on the query endpoints.
func TestCorrelationHandler_ServiceUnavailableReturns503(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := correlationHandler{service: nil}
	r.GET("/api/v1/aiops/correlation/cases", h.listCorrelationCases)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/correlation/cases?cluster_id=1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}
