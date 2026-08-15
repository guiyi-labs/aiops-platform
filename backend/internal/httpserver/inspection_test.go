package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"k8s-aiops.local/backend/internal/inspection"
)

// --- inspection test fakes ---

type inspectionRepoNoop struct{}

func (inspectionRepoNoop) UpsertRuleOverride(_ context.Context, _ *inspection.RuleOverride) error {
	return nil
}
func (inspectionRepoNoop) ListRuleOverrides(_ context.Context, _ int64) ([]inspection.RuleOverride, error) {
	return nil, nil
}
func (inspectionRepoNoop) GetRuleOverride(_ context.Context, _ int64, _ string) (*inspection.RuleOverride, error) {
	return nil, nil
}

func (inspectionRepoNoop) CreatePlan(_ context.Context, _ *inspection.Plan) error { return nil }
func (inspectionRepoNoop) GetPlan(_ context.Context, id int64) (inspection.Plan, error) {
	return inspection.Plan{}, inspection.ErrPlanNotFound
}
func (inspectionRepoNoop) ListPlans(_ context.Context, _ inspection.PlanListFilter) ([]inspection.Plan, error) {
	return nil, nil
}
func (inspectionRepoNoop) UpdatePlan(_ context.Context, _ int64, _ inspection.PatchPlanInput) (inspection.Plan, error) {
	return inspection.Plan{}, inspection.ErrPlanNotFound
}
func (inspectionRepoNoop) DeletePlan(_ context.Context, _, _ int64) error { return nil }
func (inspectionRepoNoop) TouchPlanRun(_ context.Context, _ int64, _, _ *gorm.DeletedAt) error {
	return nil
}

func (inspectionRepoNoop) CreateTask(_ context.Context, _ *inspection.Task) error { return nil }
func (inspectionRepoNoop) GetTask(_ context.Context, id int64) (inspection.Task, error) {
	return inspection.Task{}, inspection.ErrTaskNotFound
}
func (inspectionRepoNoop) ListTasks(_ context.Context, _ inspection.TaskListFilter) (inspection.ListResponse[inspection.Task], error) {
	return inspection.ListResponse[inspection.Task]{}, nil
}
func (inspectionRepoNoop) UpdateTaskStatus(_ context.Context, _ int64, _ inspection.PatchTaskInput) error {
	return nil
}

func (inspectionRepoNoop) CreateResults(_ context.Context, _ []inspection.Result) error { return nil }
func (inspectionRepoNoop) ListResults(_ context.Context, _ inspection.ListFilter) (inspection.ListResponse[inspection.Result], error) {
	return inspection.ListResponse[inspection.Result]{}, nil
}
func (inspectionRepoNoop) GetResult(_ context.Context, id int64) (inspection.Result, error) {
	return inspection.Result{}, inspection.ErrResultNotFound
}

func (inspectionRepoNoop) Coverage(_ context.Context, windowDays int, now time.Time) (inspection.CoverageSummary, error) {
	summary := inspection.CoverageSummary{
		Scope:      "inspection:plans+tasks+results:" + strconv.Itoa(windowDays) + "d",
		ObservedAt: now.UTC().Format(time.RFC3339),
		WindowDays: windowDays,
		BySeverity: map[string]int{},
	}
	// An empty window is never a healthy state (fail-closed).
	summary.FailClosed = true
	summary.EmptyNote = "window contains no inspection findings (fail-closed)"
	return summary, nil
}

type inspectionExecutorNoop struct{}

func (inspectionExecutorNoop) Execute(_ context.Context, _ int64, _ inspection.RuleDescriptor) ([]inspection.Finding, error) {
	return nil, nil
}

type inspectionClusterListerNoop struct{}

func (inspectionClusterListerNoop) List(_ context.Context) ([]struct {
	ID   int64
	Name string
}, error) {
	return []struct {
		ID   int64
		Name string
	}{{ID: 1, Name: "test"}}, nil
}

func newInspectionTestEngine(t *testing.T, svc *inspection.Service) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", int64(1))
		c.Set("workspace_id", int64(1))
		c.Set("workspace_roles", map[int64][]string{1: {"admin"}})
		c.Next()
	})
	h := inspectionHandler{service: svc}
	api := r.Group("/api/v1/aiops")
	inspectionRoutes := api.Group("/inspection")
	{
		inspectionRoutes.GET("/rules", h.listRules)
		inspectionRoutes.GET("/plans", h.listPlans)
		inspectionRoutes.POST("/plans", h.createPlan)
		inspectionRoutes.GET("/plans/:id", h.getPlan)
		inspectionRoutes.DELETE("/plans/:id", h.deletePlan)
		inspectionRoutes.POST("/run-once", h.runOnce)
		inspectionRoutes.GET("/tasks", h.listTasks)
		inspectionRoutes.GET("/tasks/:id", h.getTask)
		inspectionRoutes.GET("/results", h.listResults)
		inspectionRoutes.GET("/results/:id", h.getResult)
		inspectionRoutes.GET("/coverage", h.coverage)
	}
	perCluster := r.Group("/api/v1/clusters/:cluster_id/aiops/inspection")
	{
		perCluster.GET("/rules", h.effectiveRules)
	}
	return r
}

func mustNewInspectionService(t *testing.T) *inspection.Service {
	t.Helper()
	svc, err := inspection.NewService(
		inspection.Config{MaxConcurrentClusters: 1, PerClusterTimeout: 5 * time.Second, MaxTaskResults: 100},
		inspectionRepoNoop{},
		inspectionExecutorNoop{},
		inspectionClusterListerNoop{},
		zap.NewNop(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

func TestInspection_ListRulesCatalogReturns200(t *testing.T) {
	r := newInspectionTestEngine(t, mustNewInspectionService(t))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/inspection/rules", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	rules, ok := body["items"].([]interface{})
	if !ok {
		t.Fatalf("items missing")
	}
	if len(rules) != 8 {
		t.Errorf("catalog items = %d, want 8", len(rules))
	}
}

func TestInspection_ListRules503WhenServiceNil(t *testing.T) {
	r := newInspectionTestEngine(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/inspection/rules", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestInspection_CreatePlanValidation(t *testing.T) {
	r := newInspectionTestEngine(t, mustNewInspectionService(t))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/aiops/inspection/plans", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty plan, got %d", w.Code)
	}
}

func TestInspection_ListPlans200(t *testing.T) {
	r := newInspectionTestEngine(t, mustNewInspectionService(t))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/inspection/plans?limit=10", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestInspection_RunOnceInvalidRuleCodes(t *testing.T) {
	r := newInspectionTestEngine(t, mustNewInspectionService(t))
	body := `{"cluster_ids":[1], "rule_codes":["not_a_real_rule_code"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/aiops/inspection/run-once", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid rule_codes, got %d: %s", w.Code, w.Body.String())
	}
}

func TestInspection_PerClusterEffectiveBadCluster(t *testing.T) {
	r := newInspectionTestEngine(t, mustNewInspectionService(t))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/abc/aiops/inspection/rules", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad cluster_id, got %d", w.Code)
	}
}

func TestInspection_ListResultsInvalidLimit(t *testing.T) {
	r := newInspectionTestEngine(t, mustNewInspectionService(t))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/inspection/results?limit=-5", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid limit, got %d", w.Code)
	}
}

func TestInspection_CoverageReturnsValidJSON(t *testing.T) {
	r := newInspectionTestEngine(t, mustNewInspectionService(t))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/inspection/coverage", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result struct {
		Scope        string  `json:"scope"`
		PlanTotal    int     `json:"plan_total"`
		FailClosed   bool    `json:"fail_closed"`
		RuleCoverage float64 `json:"rule_coverage"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	// The noop repo returns empty data; coverage must report fail-closed.
	if !result.FailClosed {
		t.Fatalf("fail_closed should be true with empty repo: %+v", result)
	}
	if result.Scope == "" {
		t.Fatalf("scope should not be empty")
	}
}

func TestInspection_CoverageBadWindow(t *testing.T) {
	r := newInspectionTestEngine(t, mustNewInspectionService(t))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/inspection/coverage?window_days=0", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "INVALID_WINDOW") {
		t.Fatalf("expected 400 for window_days=0, got %d: %s", w.Code, w.Body.String())
	}
}

func TestInspection_GetPlanNotFound(t *testing.T) {
	svc := mustNewInspectionService(t)
	r := newInspectionTestEngine(t, svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/aiops/inspection/plans/999", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("get plan missing = %d, body=%s", w.Code, w.Body.String())
	}
}

func TestInspection_GetPlanInvalidID(t *testing.T) {
	svc := mustNewInspectionService(t)
	r := newInspectionTestEngine(t, svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/aiops/inspection/plans/abc", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("get plan invalid = %d", w.Code)
	}
}

func TestInspection_DeletePlanReturns204(t *testing.T) {
	svc := mustNewInspectionService(t)
	r := newInspectionTestEngine(t, svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/v1/aiops/inspection/plans/7", nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete plan = %d, body=%s", w.Code, w.Body.String())
	}
}

func TestInspection_DeletePlanInvalidID(t *testing.T) {
	svc := mustNewInspectionService(t)
	r := newInspectionTestEngine(t, svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/v1/aiops/inspection/plans/abc", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("delete plan invalid = %d", w.Code)
	}
}

func TestInspection_ListTasksReturns200(t *testing.T) {
	svc := mustNewInspectionService(t)
	r := newInspectionTestEngine(t, svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/aiops/inspection/tasks", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("list tasks = %d, body=%s", w.Code, w.Body.String())
	}
}

func TestInspection_ListTasksBadPlanID(t *testing.T) {
	svc := mustNewInspectionService(t)
	r := newInspectionTestEngine(t, svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/aiops/inspection/tasks?plan_id=abc", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("list tasks bad plan_id = %d", w.Code)
	}
}

func TestInspection_ListTasksBadLimit(t *testing.T) {
	svc := mustNewInspectionService(t)
	r := newInspectionTestEngine(t, svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/aiops/inspection/tasks?limit=0", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("list tasks bad limit = %d", w.Code)
	}
}

func TestInspection_GetTaskNotFound(t *testing.T) {
	svc := mustNewInspectionService(t)
	r := newInspectionTestEngine(t, svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/aiops/inspection/tasks/999", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("get task missing = %d, body=%s", w.Code, w.Body.String())
	}
}

func TestInspection_GetTaskInvalidID(t *testing.T) {
	svc := mustNewInspectionService(t)
	r := newInspectionTestEngine(t, svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/aiops/inspection/tasks/abc", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("get task invalid = %d", w.Code)
	}
}

func TestInspection_NilServiceReturns503(t *testing.T) {
	h := inspectionHandler{service: nil}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", int64(1))
		c.Next()
	})
	r.GET("/api/v1/aiops/inspection/rules", h.listRules)
	r.GET("/api/v1/aiops/inspection/plans", h.listPlans)
	r.POST("/api/v1/aiops/inspection/plans", h.createPlan)
	r.GET("/api/v1/aiops/inspection/plans/:id", h.getPlan)
	r.DELETE("/api/v1/aiops/inspection/plans/:id", h.deletePlan)
	r.POST("/api/v1/aiops/inspection/run-once", h.runOnce)
	r.GET("/api/v1/aiops/inspection/tasks", h.listTasks)
	r.GET("/api/v1/aiops/inspection/tasks/:id", h.getTask)
	r.GET("/api/v1/aiops/inspection/results", h.listResults)
	r.GET("/api/v1/aiops/inspection/results/:id", h.getResult)
	r.GET("/api/v1/aiops/inspection/coverage", h.coverage)
	for _, path := range []string{
		"/api/v1/aiops/inspection/rules",
		"/api/v1/aiops/inspection/plans",
		"/api/v1/aiops/inspection/plans/1",
		"/api/v1/aiops/inspection/tasks",
		"/api/v1/aiops/inspection/tasks/1",
		"/api/v1/aiops/inspection/results",
		"/api/v1/aiops/inspection/results/1",
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("nil service GET %s = %d, want 503", path, w.Code)
		}
	}
	// POST/DELETE also need 503 for nil service.
	for _, tt := range []struct {
		method, path string
	}{
		{http.MethodPost, "/api/v1/aiops/inspection/plans"},
		{http.MethodDelete, "/api/v1/aiops/inspection/plans/1"},
		{http.MethodPost, "/api/v1/aiops/inspection/run-once"},
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(tt.method, tt.path, nil))
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("nil service %s %s = %d, want 503", tt.method, tt.path, w.Code)
		}
	}
}

func TestInspection_ListResultsReturns200(t *testing.T) {
	svc := mustNewInspectionService(t)
	r := newInspectionTestEngine(t, svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/aiops/inspection/results", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("list results = %d, body=%s", w.Code, w.Body.String())
	}
}

func TestInspection_GetResultNotFound(t *testing.T) {
	svc := mustNewInspectionService(t)
	r := newInspectionTestEngine(t, svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/aiops/inspection/results/999", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("get result missing = %d", w.Code)
	}
}

func TestInspection_GetResultInvalidID(t *testing.T) {
	svc := mustNewInspectionService(t)
	r := newInspectionTestEngine(t, svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/aiops/inspection/results/abc", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("get result invalid = %d", w.Code)
	}
}

func TestInspection_EffectiveRulesHappyPath(t *testing.T) {
	svc := mustNewInspectionService(t)
	h := inspectionHandler{service: svc}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", int64(1))
		c.Set("workspace_id", int64(1))
		c.Set("workspace_roles", map[int64][]string{1: {"admin"}})
		c.Next()
	})
	r.GET("/api/v1/clusters/:cluster_id/aiops/inspection/rules", h.effectiveRules)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/aiops/inspection/rules", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("effective rules = %d, body=%s", w.Code, w.Body.String())
	}
}
