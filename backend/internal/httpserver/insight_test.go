package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newInsightTestEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := insightHandler{}
	api := r.Group("/api/v1/aiops")
	api.GET("/insight", h.runbook)
	return r
}

func TestInsightHandler_RunbookReturns200(t *testing.T) {
	r := newInsightTestEngine()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/aiops/insight?cluster_id=7&domain=network&kind=Deployment&namespace=prod&name=api&code=NET-EXPOSE", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body["read_only"] != true {
		t.Fatalf("runbook must be read-only: %v", body["read_only"])
	}
	diagnoses, ok := body["diagnoses"].([]any)
	if !ok || len(diagnoses) == 0 {
		t.Fatalf("expected diagnosis routes, got %v", body["diagnoses"])
	}
	ops, ok := body["operations"].([]any)
	if !ok || len(ops) == 0 {
		t.Fatalf("expected deployment operation candidates, got %v", body["operations"])
	}
}

func TestInsightHandler_MissingFieldsReturns400(t *testing.T) {
	r := newInsightTestEngine()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/insight?cluster_id=1&domain=image", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without kind/name, got %d", w.Code)
	}
}

func TestInsightHandler_InvalidClusterReturns400(t *testing.T) {
	r := newInsightTestEngine()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/aiops/insight?cluster_id=0&domain=cis&kind=Deployment&name=x", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid cluster_id, got %d", w.Code)
	}
}
