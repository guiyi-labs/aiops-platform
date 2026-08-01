package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/aiinvestigator"
	"k8s-aiops.local/backend/internal/requestctx"
)

// newInvestigatorTestEngine builds a gin engine wired to the investigator
// handler, mirroring the route shapes registered in router.go.
func newInvestigatorTestEngine(t *testing.T, svc *aiinvestigator.Service) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := aiInvestigatorHandler{service: svc}
	api := r.Group("/api/v1/aiops/investigator")
	api.GET("/runbooks", h.listRunbooks)
	api.GET("/cases/:case_id/investigations", h.listInvestigations)
	api.GET("/investigations/:id", h.getInvestigation)
	api.POST("/cases/:case_id/investigations", h.generateInvestigation)
	return r
}

// withActor attaches actor metadata to the request context, mirroring what
// the authentication middleware does in production.
func withActor(req *http.Request, actorID int64, actorName string) *http.Request {
	return req.WithContext(requestctx.WithMetadata(req.Context(), requestctx.Metadata{
		ActorID:   actorID,
		ActorName: actorName,
		RequestID: "investigator-test",
	}))
}

// handlerCaseReader is a test-only aiinvestigator.CaseReader for the
// httpserver package.
type handlerCaseReader struct {
	ctx   aiinvestigator.CaseContext
	codes map[string]bool
	err   error
}

func (r handlerCaseReader) GetCase(context.Context, int64) (aiinvestigator.CaseContext, error) {
	if r.err != nil {
		return aiinvestigator.CaseContext{}, r.err
	}
	return r.ctx, nil
}

func (r handlerCaseReader) EligibleActionCodes(context.Context, int64) (map[string]bool, error) {
	return r.codes, nil
}

func handlerCaseContext() aiinvestigator.CaseContext {
	return aiinvestigator.CaseContext{
		CaseID:               42,
		ClusterID:            1,
		RuleID:               "pod_failure_with_rollout",
		PrimaryResourceKind:  "Pod",
		PrimaryResourceName:  "web-abc",
		PrimaryResourceUID:   "uid-123",
		Confidence:           "candidate",
		EvidenceCompleteness: "partial",
		SignalLinks: []aiinvestigator.SignalLinkContext{
			{SignalOccurrenceID: 100, Relation: "trigger", SignalID: "pod_crashloop", Producer: "diagnosis", ObservedAt: "2026-07-31T10:00:00Z"},
		},
		ChangeCandidates: []aiinvestigator.ChangeCandidateContext{
			{ChangeEventID: 200, RuleID: "rollout_preceded", Confidence: "candidate", Rank: 1, ReasonCode: "temporal_proximity"},
		},
	}
}

func TestInvestigatorHandler_ListRunbooksReturns200(t *testing.T) {
	svc := aiinvestigator.NewService(aiinvestigator.NopRepository{}, nil, nil)
	r := newInvestigatorTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/investigator/runbooks", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Items               []aiinvestigator.RunbookDescriptor `json:"items"`
		InvestigatorVersion string                             `json:"investigator_version"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Items) == 0 {
		t.Error("expected at least one runbook in catalog")
	}
	if resp.InvestigatorVersion != aiinvestigator.InvestigatorVersion {
		t.Errorf("investigator_version: want %s, got %s", aiinvestigator.InvestigatorVersion, resp.InvestigatorVersion)
	}
}

func TestInvestigatorHandler_ListInvestigationsInvalidCaseIDReturns400(t *testing.T) {
	svc := aiinvestigator.NewService(aiinvestigator.NopRepository{}, nil, nil)
	r := newInvestigatorTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/investigator/cases/abc/investigations", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestInvestigatorHandler_ListInvestigationsNegativeCaseIDReturns400(t *testing.T) {
	svc := aiinvestigator.NewService(aiinvestigator.NopRepository{}, nil, nil)
	r := newInvestigatorTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/investigator/cases/0/investigations", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for case_id=0, got %d: %s", w.Code, w.Body.String())
	}
}

func TestInvestigatorHandler_ListInvestigationsBadLimitReturns400(t *testing.T) {
	svc := aiinvestigator.NewService(aiinvestigator.NopRepository{}, nil, nil)
	r := newInvestigatorTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/investigator/cases/42/investigations?limit=0", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for limit=0, got %d: %s", w.Code, w.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/aiops/investigator/cases/42/investigations?limit=999", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for limit>200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestInvestigatorHandler_ListInvestigationsReturns200(t *testing.T) {
	svc := aiinvestigator.NewService(aiinvestigator.NopRepository{}, nil, nil)
	r := newInvestigatorTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/investigator/cases/42/investigations", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp aiinvestigator.InvestigationListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 0 {
		t.Errorf("expected 0 total from NopRepository, got %d", resp.Total)
	}
}

func TestInvestigatorHandler_GetInvestigationInvalidIDReturns400(t *testing.T) {
	svc := aiinvestigator.NewService(aiinvestigator.NopRepository{}, nil, nil)
	r := newInvestigatorTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/investigator/investigations/notanint", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestInvestigatorHandler_GetInvestigationNotFoundReturns404(t *testing.T) {
	svc := aiinvestigator.NewService(aiinvestigator.NopRepository{}, nil, nil)
	r := newInvestigatorTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/investigator/investigations/77", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestInvestigatorHandler_GenerateInvalidCaseIDReturns400(t *testing.T) {
	svc := aiinvestigator.NewService(aiinvestigator.NopRepository{}, nil, nil)
	r := newInvestigatorTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/aiops/investigator/cases/xyz/investigations", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestInvestigatorHandler_GenerateCaseNotFoundReturns404(t *testing.T) {
	// NopCaseReader returns ErrCaseNotFound, so generation must 404.
	svc := aiinvestigator.NewService(aiinvestigator.NopRepository{}, nil, nil)
	r := newInvestigatorTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/aiops/investigator/cases/999/investigations", nil)
	req = withActor(req, 1, "alice")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing case, got %d: %s", w.Code, w.Body.String())
	}
	var resp errorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Code != "CASE_NOT_FOUND" {
		t.Errorf("error code = %q, want CASE_NOT_FOUND", resp.Code)
	}
}

func TestInvestigatorHandler_GenerateSuccessReturns200(t *testing.T) {
	reader := handlerCaseReader{
		ctx:   handlerCaseContext(),
		codes: map[string]bool{"deployment.rollback": true},
	}
	svc := aiinvestigator.NewService(aiinvestigator.NopRepository{}, nil, reader)
	r := newInvestigatorTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/aiops/investigator/cases/42/investigations", nil)
	req = withActor(req, 7, "alice")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var inv aiinvestigator.Investigation
	if err := json.Unmarshal(w.Body.Bytes(), &inv); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if inv.Status != aiinvestigator.InvestigationStatusCompleted {
		t.Errorf("Status = %q, want completed", inv.Status)
	}
	if inv.CaseID != 42 {
		t.Errorf("CaseID = %d, want 42", inv.CaseID)
	}
	if inv.Provider != "nop" {
		t.Errorf("Provider = %q, want nop", inv.Provider)
	}
	if len(inv.Citations) == 0 {
		t.Errorf("investigation must have at least one citation")
	}
	if inv.Actor.ID != 7 || inv.Actor.Name != "alice" {
		t.Errorf("Actor = %+v, want {7 alice}", inv.Actor)
	}
}

func TestInvestigatorHandler_GenerateProviderFailureReturns200WithFailedInvestigation(t *testing.T) {
	// When the case reader fails, the service returns the error and the
	// handler maps it. Use a case reader that returns ErrCaseNotFound.
	reader := handlerCaseReader{err: aiinvestigator.ErrCaseNotFound}
	svc := aiinvestigator.NewService(aiinvestigator.NopRepository{}, nil, reader)
	r := newInvestigatorTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/aiops/investigator/cases/42/investigations", nil)
	req = withActor(req, 1, "alice")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for case not found, got %d: %s", w.Code, w.Body.String())
	}
}

func TestInvestigatorHandler_GeneratePreservesActorFromContext(t *testing.T) {
	reader := handlerCaseReader{
		ctx:   handlerCaseContext(),
		codes: map[string]bool{},
	}
	svc := aiinvestigator.NewService(aiinvestigator.NopRepository{}, nil, reader)
	r := newInvestigatorTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/aiops/investigator/cases/42/investigations", nil)
	req = withActor(req, 99, "bob")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var inv aiinvestigator.Investigation
	_ = json.Unmarshal(w.Body.Bytes(), &inv)
	if inv.Actor.ID != 99 || inv.Actor.Name != "bob" {
		t.Errorf("Actor = %+v, want {99 bob}", inv.Actor)
	}
}
