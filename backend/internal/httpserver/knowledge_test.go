package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/knowledge"
)

type knowledgeNopRepo struct{}

func (knowledgeNopRepo) Insert(ctx context.Context, entry knowledge.Entry) (knowledge.Entry, error) {
	return entry, nil
}

func TestKnowledgeSearchRecallsCaseAfterRuleRename(t *testing.T) {
	repo := knowledge.NewInMemoryRepository()
	entry, err := repo.Insert(context.Background(), knowledge.Entry{
		RuleID: "pod.image_pull.v1", Severity: "high", ResourceKind: "Pod",
		ResourceName: "broken", Summary: "ImagePullBackOff image registry unavailable",
		RootCauses: []string{"image registry unavailable"}, NotedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	retriever := knowledge.NewRetriever(repo, knowledge.DefaultConfig()).WithEmbeddingProvider(knowledge.NewLexicalEmbeddingProvider(0))
	handler := knowledgeHandler{repo: repo, retriever: retriever}
	engine := gin.New()
	engine.GET("/knowledge", handler.listKnowledge)
	query := url.Values{"q": {entry.Summary}, "rule_id": {"pod.image_pull.v2"}, "limit": {"1"}}
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/knowledge?"+query.Encode(), nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body.String())
	}
	var result knowledge.ListResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].ID != entry.ID {
		t.Fatalf("renamed rule lost historical case: %+v", result)
	}
}

func TestKnowledgeSearchValidatesQueryAndUnavailableRetriever(t *testing.T) {
	for _, tc := range []struct {
		query string
		want  int
	}{
		{"q=" + url.QueryEscape(strings.Repeat("故", 501)), http.StatusBadRequest},
		{"q=故障", http.StatusServiceUnavailable},
	} {
		t.Run(tc.query[:8], func(t *testing.T) {
			engine := newKnowledgeTestEngine(t, knowledgeNopRepo{})
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/aiops/knowledge?"+tc.query, nil))
			if response.Code != tc.want {
				t.Fatalf("want %d, got %d", tc.want, response.Code)
			}
		})
	}
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

func TestInspectKnowledgeValidatesAndExposesActualStages(t *testing.T) {
	repo := knowledge.NewInMemoryRepository()
	entry, err := repo.Insert(context.Background(), knowledge.Entry{
		RuleID: "pod.image_pull.v1", Severity: "high", ResourceKind: "Pod", ResourceName: "web",
		Summary: "ImagePullBackOff image registry unavailable", NotedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	retriever := knowledge.NewRetriever(repo, knowledge.DefaultConfig()).WithEmbeddingProvider(knowledge.NewLexicalEmbeddingProvider(0))
	engine := gin.New()
	engine.GET("/inspect", knowledgeHandler{repo: repo, retriever: retriever}.inspectKnowledge)
	for _, query := range []string{"", "q=", "q=x&unknown=y", "q=x&q=y", "q=" + url.QueryEscape(strings.Repeat("故", 501)), "q=x&rule_id=" + strings.Repeat("x", 201)} {
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/inspect?"+query, nil))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid query accepted: %q status %d", query, response.Code)
		}
	}
	query := url.Values{"q": {entry.Summary}, "rule_id": {"pod.image_pull.v2"}}
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/inspect?"+query.Encode(), nil))
	if response.Code != 200 {
		t.Fatalf("status %d: %s", response.Code, response.Body.String())
	}
	var trace knowledge.RetrievalTrace
	if err := json.Unmarshal(response.Body.Bytes(), &trace); err != nil {
		t.Fatal(err)
	}
	if len(trace.Structured) != 0 || len(trace.Results) != 1 || trace.Results[0].ID != entry.ID {
		t.Fatalf("unexpected stages: %+v", trace)
	}
	count, _ := repo.Count(context.Background())
	if count != 1 {
		t.Fatalf("inspection changed library: %d", count)
	}
}
