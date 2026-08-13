package httpserver

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/slo"
)

func TestSLOUnavailableAndQueryValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := sloHandler{service: nil}
	router := gin.New()
	router.GET("/api/v1/aiops/slos", h.listSLODefinitions)
	router.GET("/api/v1/aiops/slos/:id", h.getSLODefinition)
	router.GET("/api/v1/aiops/slos/:id/evaluate", h.evaluateSLO)
	router.GET("/api/v1/aiops/slos/:id/evaluations", h.listSLOEvaluations)
	paths := []string{"/api/v1/aiops/slos", "/api/v1/aiops/slos/1", "/api/v1/aiops/slos/1/evaluate", "/api/v1/aiops/slos/1/evaluations"}
	for _, path := range paths {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("path=%s status=%d body=%s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestSLOListDefinitionQueryValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := slo.NewService(slo.NopRepository{}, nil)
	h := sloHandler{service: svc}
	router := gin.New()
	router.GET("/api/v1/aiops/slos", h.listSLODefinitions)

	cases := []string{
		"/api/v1/aiops/slos?cluster_id=abc",
		"/api/v1/aiops/slos?enabled=maybe",
		"/api/v1/aiops/slos?owner_id=-1",
		"/api/v1/aiops/slos?limit=0",
	}
	for _, path := range cases {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_QUERY") {
			t.Fatalf("path=%s status=%d body=%s", path, rec.Code, rec.Body.String())
		}
	}

	// valid queries return 200
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/aiops/slos?cluster_id=1&namespace=default&template=latency&enabled=true&owner_id=1&limit=50", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSLOEvaluationQueryValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := slo.NewService(slo.NopRepository{}, nil)
	h := sloHandler{service: svc}
	router := gin.New()
	router.GET("/api/v1/aiops/slos/:id/evaluations", h.listSLOEvaluations)

	// invalid id -> INVALID_ID
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/aiops/slos/abc/evaluations", nil))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_ID") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	cases := []string{
		"/api/v1/aiops/slos/1/evaluations?version=0",
		"/api/v1/aiops/slos/1/evaluations?start=not-a-time",
		"/api/v1/aiops/slos/1/evaluations?end=not-a-time",
		"/api/v1/aiops/slos/1/evaluations?limit=0",
	}
	for _, path := range cases {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_QUERY") {
			t.Fatalf("path=%s status=%d body=%s", path, rec.Code, rec.Body.String())
		}
	}

	okRec := httptest.NewRecorder()
	router.ServeHTTP(okRec, httptest.NewRequest(http.MethodGet, "/api/v1/aiops/slos/1/evaluations?version=1&state=ok&limit=100", nil))
	if okRec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", okRec.Code, okRec.Body.String())
	}
}

func TestSLOListTemplates(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := sloHandler{}
	router := gin.New()
	router.GET("/api/v1/aiops/slos/templates", h.listSLITemplates)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/aiops/slos/templates", nil))
	if rec.Code != http.StatusOK || !contains(rec.Body.String(), "template_version") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestWriteSLOErrorMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{err: slo.ErrDefinitionNotFound, status: http.StatusNotFound, code: "SLO_NOT_FOUND"},
		{err: slo.ErrDefinitionDisabled, status: http.StatusConflict, code: "SLO_DISABLED"},
		{err: slo.ErrEvaluatorUnavailable, status: http.StatusServiceUnavailable, code: "SLO_EVALUATOR_UNAVAILABLE"},
		{err: slo.ErrEvaluationInvalidInput, status: http.StatusBadRequest, code: "SLO_INVALID_INPUT"},
		{err: slo.ErrDuplicateDefinition, status: http.StatusConflict, code: "SLO_DUPLICATE"},
		{err: errors.New("objective must be between 0 and 1"), status: http.StatusBadRequest, code: "SLO_INVALID_INPUT"},
		{err: errors.New("boom"), status: http.StatusInternalServerError, code: "SLO_INTERNAL_ERROR"},
	}
	for _, tt := range cases {
		router := gin.New()
		router.GET("/x", func(c *gin.Context) { writeSLOError(c, tt.err) })
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
		if rec.Code != tt.status || !contains(rec.Body.String(), tt.code) {
			t.Fatalf("err=%v status=%d body=%s want status=%d code=%s", tt.err, rec.Code, rec.Body.String(), tt.status, tt.code)
		}
	}
}
