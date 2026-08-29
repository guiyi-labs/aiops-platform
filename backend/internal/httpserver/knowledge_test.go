package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/knowledge"
)

type knowledgeNopRepo struct{}

func (knowledgeNopRepo) Insert(ctx context.Context, entry knowledge.Entry) (knowledge.Entry, error) {
	return entry, nil
}
func (knowledgeNopRepo) ListByFilter(ctx context.Context, filter knowledge.Filter) (knowledge.ListResponse, error) {
	return knowledge.ListResponse{Items: []knowledge.Entry{}, Total: 0}, nil
}
func (knowledgeNopRepo) Count(ctx context.Context) (int64, error) { return 0, nil }

func newKnowledgeTestEngine(t *testing.T, repo knowledge.Repository) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := knowledgeHandler{repo: repo}
	api := r.Group("/api/v1/aiops")
	api.GET("/knowledge", h.listKnowledge)
	api.GET("/knowledge/stats", h.knowledgeStats)
	return r
}

func TestKnowledgeHandler_ListReturns200(t *testing.T) {
	r := newKnowledgeTestEngine(t, knowledgeNopRepo{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/knowledge?limit=10", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestKnowledgeHandler_StatsReturns200(t *testing.T) {
	r := newKnowledgeTestEngine(t, knowledgeNopRepo{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/knowledge/stats", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestKnowledgeHandler_ListReturns503WhenRepoNil(t *testing.T) {
	r := newKnowledgeTestEngine(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/knowledge", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestKnowledgeHandler_ListRejectsBothSeverityFilters(t *testing.T) {
	r := newKnowledgeTestEngine(t, knowledgeNopRepo{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/knowledge?severity=high&min_severity=warning", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestKnowledgeHandler_ListRejectsBadSeverity(t *testing.T) {
	r := newKnowledgeTestEngine(t, knowledgeNopRepo{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/knowledge?severity=criticalish", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestKnowledgeHandler_ListRejectsBadLimit(t *testing.T) {
	r := newKnowledgeTestEngine(t, knowledgeNopRepo{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/knowledge?limit=999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

var _ knowledge.Repository = knowledgeNopRepo{}
