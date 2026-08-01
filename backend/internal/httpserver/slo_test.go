package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/requestctx"
	"k8s-aiops.local/backend/internal/slo"
)

func newSLOTestEngine(t *testing.T, svc *slo.Service) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := sloHandler{service: svc}
	api := r.Group("/api/v1/aiops")
	api.GET("/slos/templates", h.listSLITemplates)
	api.GET("/slos", h.listSLODefinitions)
	api.POST("/slos", h.createSLODefinition)
	api.GET("/slos/:id", h.getSLODefinition)
	api.PATCH("/slos/:id", h.patchSLODefinition)
	api.DELETE("/slos/:id", h.deleteSLODefinition)
	api.POST("/slos/:id/evaluate", h.evaluateSLO)
	api.GET("/slos/:id/evaluations", h.listSLOEvaluations)
	return r
}

// withTestActor middleware injects a fake authenticated actor for handler
// tests that exercise create/patch paths.
func withTestActor() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), requestctx.Metadata{
			ActorID:          1,
			ActorName:        "alice",
			ActorDisplayName: "Alice",
		}))
		c.Next()
	}
}

// TestSLOHandler_ListTemplatesReturns200 verifies the templates catalog
// endpoint works without a service (it does not need the repository).
func TestSLOHandler_ListTemplatesReturns200(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := sloHandler{service: nil}
	r.GET("/api/v1/aiops/slos/templates", h.listSLITemplates)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/slos/templates", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Items           []slo.TemplateDescriptor `json:"items"`
		TemplateVersion string                   `json:"template_version"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Items) != 3 {
		t.Errorf("items: want 3 templates, got %d", len(resp.Items))
	}
	if resp.TemplateVersion != slo.TemplateVersion {
		t.Errorf("template_version: want %s, got %s", slo.TemplateVersion, resp.TemplateVersion)
	}
}

// TestSLOHandler_ListDefinitionsReturns200 verifies list with an empty
// repository returns an empty page.
func TestSLOHandler_ListDefinitionsReturns200(t *testing.T) {
	svc := slo.NewService(slo.NopRepository{}, nil)
	r := newSLOTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/slos", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// TestSLOHandler_ListDefinitionsInvalidClusterID verifies bad cluster_id is
// rejected with 400.
func TestSLOHandler_ListDefinitionsInvalidClusterID(t *testing.T) {
	svc := slo.NewService(slo.NopRepository{}, nil)
	r := newSLOTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/slos?cluster_id=abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestSLOHandler_GetDefinitionNotFound verifies that a get on a missing ID
// returns 404 (NopRepository returns ErrDefinitionNotFound).
func TestSLOHandler_GetDefinitionNotFound(t *testing.T) {
	svc := slo.NewService(slo.NopRepository{}, nil)
	r := newSLOTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/slos/42", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// TestSLOHandler_GetDefinitionInvalidID verifies non-numeric ID is 400.
func TestSLOHandler_GetDefinitionInvalidID(t *testing.T) {
	svc := slo.NewService(slo.NopRepository{}, nil)
	r := newSLOTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/slos/abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestSLOHandler_CreateDefinitionInvalidBody verifies a malformed JSON body
// is rejected with 400.
func TestSLOHandler_CreateDefinitionInvalidBody(t *testing.T) {
	svc := slo.NewService(slo.NopRepository{}, nil)
	r := newSLOTestEngine(t, svc)
	r.Use(withTestActor())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/aiops/slos", nil) // no body
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestSLOHandler_EvaluateRequiresEvaluator verifies evaluate returns 503
// when the service has no evaluator configured.
func TestSLOHandler_EvaluateRequiresEvaluator(t *testing.T) {
	// Build a service backed by a memory repository so GetDefinition
	// succeeds but EvaluateSLO returns ErrEvaluatorUnavailable.
	repo := &sloMemoryRepo{}
	svc := slo.NewService(repo, nil)
	// Pre-seed a definition so GetDefinition succeeds.
	def := &slo.Definition{
		ID: 1, ClusterID: 1, Enabled: true, Version: 1,
		Service:         slo.ServiceRef{Kind: "Deployment", Namespace: "default", Name: "api"},
		Template:        slo.TemplateRequestSuccessRatio,
		TemplateVersion: slo.TemplateVersion,
		Objective:       0.99, RollingWindowSeconds: 3600,
		MissingDataPolicy: slo.MissingDataUnavailable,
		FastBurnRate:      14.4, FastBurnWindowSeconds: 3600,
		SlowBurnRate: 1.0, SlowBurnWindowSeconds: 21600,
		Owner:   slo.ActorRef{ID: 1, Name: "alice"},
		Creator: slo.ActorRef{ID: 1, Name: "alice"},
	}
	repo.store(1, def)
	r := newSLOTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/aiops/slos/1/evaluate", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}

// TestSLOHandler_EvaluateDefinitionNotFound verifies evaluate on a missing
// SLO returns 404.
func TestSLOHandler_EvaluateDefinitionNotFound(t *testing.T) {
	svc := slo.NewService(slo.NopRepository{}, nil)
	r := newSLOTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/aiops/slos/999/evaluate", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// TestSLOHandler_ListEvaluationsReturns200 verifies list evaluations on a
// missing SLO returns 200 with an empty page (NopRepository).
func TestSLOHandler_ListEvaluationsReturns200(t *testing.T) {
	svc := slo.NewService(slo.NopRepository{}, nil)
	r := newSLOTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/slos/1/evaluations", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// TestSLOHandler_ListEvaluationsInvalidVersion verifies a bad version query
// param is rejected with 400.
func TestSLOHandler_ListEvaluationsInvalidVersion(t *testing.T) {
	svc := slo.NewService(slo.NopRepository{}, nil)
	r := newSLOTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/slos/1/evaluations?version=abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestSLOHandler_DeleteReturns404OnMissing verifies delete on a missing SLO
// returns 404 (NopRepository.DeleteDefinition is a no-op, so we verify the
// happy-path 204 separately).
func TestSLOHandler_DeleteReturns204(t *testing.T) {
	svc := slo.NewService(slo.NopRepository{}, nil)
	r := newSLOTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/aiops/slos/1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	// NopRepository.DeleteDefinition always returns nil -> 204.
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

// TestSLOHandler_NilServiceReturns503 verifies every endpoint returns 503
// when the service is nil.
func TestSLOHandler_NilServiceReturns503(t *testing.T) {
	r := newSLOTestEngine(t, nil)
	// The list/templates endpoints are registered via newSLOTestEngine but
	// the handler short-circuits with 503 when service is nil. Templates is
	// the exception — it does not need the service. Skip it here.
	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/aiops/slos"},
		{http.MethodPost, "/api/v1/aiops/slos"},
		{http.MethodGet, "/api/v1/aiops/slos/1"},
		{http.MethodPatch, "/api/v1/aiops/slos/1"},
		{http.MethodDelete, "/api/v1/aiops/slos/1"},
		{http.MethodPost, "/api/v1/aiops/slos/1/evaluate"},
		{http.MethodGet, "/api/v1/aiops/slos/1/evaluations"},
	}
	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusServiceUnavailable {
				t.Errorf("%s %s: expected 503, got %d", ep.method, ep.path, w.Code)
			}
		})
	}
}

// sloMemoryRepo is a tiny in-memory slo.Repository for handler tests that
// need GetDefinition to succeed.
type sloMemoryRepo struct {
	defs map[int64]*slo.Definition
}

func (r *sloMemoryRepo) store(id int64, def *slo.Definition) {
	if r.defs == nil {
		r.defs = make(map[int64]*slo.Definition)
	}
	r.defs[id] = def
}

func (r *sloMemoryRepo) CreateDefinition(_ context.Context, def *slo.Definition) error { return nil }
func (r *sloMemoryRepo) GetDefinition(_ context.Context, id int64) (slo.Definition, error) {
	if d, ok := r.defs[id]; ok {
		return *d, nil
	}
	return slo.Definition{}, slo.ErrDefinitionNotFound
}
func (r *sloMemoryRepo) ListDefinitions(_ context.Context, _ slo.DefinitionFilter) ([]slo.Definition, int64, error) {
	return nil, 0, nil
}
func (r *sloMemoryRepo) UpdateDefinition(_ context.Context, _ int64, _ slo.PatchDefinitionInput, _ time.Time) (slo.Definition, error) {
	return slo.Definition{}, slo.ErrDefinitionNotFound
}
func (r *sloMemoryRepo) DeleteDefinition(_ context.Context, _ int64) error { return nil }
func (r *sloMemoryRepo) InsertEvaluation(_ context.Context, _ *slo.Evaluation) error {
	return nil
}
func (r *sloMemoryRepo) LatestEvaluation(_ context.Context, _ int64) (slo.Evaluation, error) {
	return slo.Evaluation{}, nil
}
func (r *sloMemoryRepo) ListEvaluations(_ context.Context, _ slo.EvaluationFilter) ([]slo.Evaluation, int64, error) {
	return nil, 0, nil
}
