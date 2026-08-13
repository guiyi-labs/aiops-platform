package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/correlation"
	"k8s-aiops.local/backend/internal/incident"
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

// caseViewRepoStub returns one canned case view for GetCase.
type caseViewRepoStub struct {
	view correlation.CaseView
}

func (r caseViewRepoStub) GetCase(context.Context, int64) (correlation.CaseView, error) {
	return r.view, nil
}

func (caseViewRepoStub) UpsertResult(context.Context, *correlation.CorrelationResult) (correlation.Case, error) {
	return correlation.Case{}, nil
}
func (caseViewRepoStub) ListCases(context.Context, correlation.CaseFilter) ([]correlation.Case, int64, error) {
	return nil, 0, nil
}
func (caseViewRepoStub) ListTimeline(context.Context, correlation.CaseFilter) ([]correlation.Case, int64, error) {
	return nil, 0, nil
}
func (caseViewRepoStub) ListSignalLinks(context.Context, int64) ([]correlation.SignalLink, error) {
	return nil, nil
}
func (caseViewRepoStub) ListResourceLinks(context.Context, int64) ([]correlation.ResourceLink, error) {
	return nil, nil
}
func (caseViewRepoStub) ListChangeCandidates(context.Context, int64) ([]correlation.ChangeCandidate, error) {
	return nil, nil
}
func (caseViewRepoStub) ResolveCaseStatus(context.Context, int64, correlation.CaseStatus, time.Time) error {
	return nil
}

// TestCorrelationHandler_CaseViewIncludesLinkedIncident verifies the M108
// bidirectional deep link enrichment: the case view carries the incident
// workspace opened from this correlation case.
func TestCorrelationHandler_CaseViewIncludesLinkedIncident(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := caseViewRepoStub{view: correlation.CaseView{
		Case: correlation.Case{ID: 1, CaseKey: "k", ClusterID: 1, RuleID: "r", PrimaryResource: correlation.ResourceCitation{Kind: "Node", Name: "K8S-W1"}},
	}}
	svc := correlation.NewService(repo, nil, nil)
	h := correlationHandler{
		service: svc,
		incidentBySource: func(_ context.Context, sourceRef string) (*incident.Incident, error) {
			if sourceRef != "correlation:1" {
				return nil, errors.New("unexpected source ref " + sourceRef)
			}
			return &incident.Incident{ID: 7, Number: "INC-000007", Title: "case-linked", Status: "open"}, nil
		},
	}
	r := gin.New()
	r.GET("/api/v1/aiops/correlation/cases/:id", h.getCorrelationCase)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/correlation/cases/1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Case     correlation.CaseView `json:"case"`
		Incident *struct {
			ID     int64  `json:"id"`
			Number string `json:"number"`
			Status string `json:"status"`
		} `json:"incident"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Incident == nil {
		t.Fatal("expected incident enrichment, got null")
	}
	if resp.Incident.ID != 7 || resp.Incident.Number != "INC-000007" {
		t.Errorf("incident = %+v, want id=7 number=INC-000007", resp.Incident)
	}
}

// TestCorrelationHandler_CaseViewOmitsIncident verifies the case view stays
// unchanged when no incident is linked (enrichment never fails the view).
func TestCorrelationHandler_CaseViewOmitsIncident(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := caseViewRepoStub{view: correlation.CaseView{
		Case: correlation.Case{ID: 1, CaseKey: "k", ClusterID: 1, RuleID: "r", PrimaryResource: correlation.ResourceCitation{Kind: "Node", Name: "K8S-W1"}},
	}}
	svc := correlation.NewService(repo, nil, nil)
	h := correlationHandler{
		service: svc,
		incidentBySource: func(_ context.Context, _ string) (*incident.Incident, error) {
			return nil, incident.ErrNotFound
		},
	}
	r := gin.New()
	r.GET("/api/v1/aiops/correlation/cases/:id", h.getCorrelationCase)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/correlation/cases/1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &map[string]any{}); err != nil {
		t.Fatalf("decode: %v", err)
	}
}
