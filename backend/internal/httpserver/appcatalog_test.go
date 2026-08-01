package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/appcatalog"
	"k8s-aiops.local/backend/internal/requestctx"
)

// --- appcatalog handler test helpers ---

func newAppCatalogTestEngine(t *testing.T, svc *appcatalog.Service) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// Inject fake user metadata for audit.
	r.Use(func(c *gin.Context) {
		ctx := requestctx.WithMetadata(c.Request.Context(), requestctx.Metadata{
			ActorID:          1,
			ActorDisplayName: "test-admin",
			RequestID:        "test-req-id",
		})
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	h := appCatalogHandler{service: svc}
	api := r.Group("/api/v1")
	{
		api.GET("/app-catalog/repositories", h.listRepositories)
		api.POST("/app-catalog/repositories", h.createRepository)
		api.GET("/app-catalog/repositories/:repo_id", h.getRepository)
		api.DELETE("/app-catalog/repositories/:repo_id", h.deleteRepository)
		api.GET("/app-catalog/repositories/:repo_id/charts", h.listCharts)
		api.GET("/app-catalog/repositories/:repo_id/charts/:chart_name", h.getChart)
		api.GET("/app-catalog/plans", h.listPlans)
		api.POST("/app-catalog/plans/preview", h.previewDeploy)
		api.GET("/app-catalog/plans/:plan_id", h.getPlan)
		api.POST("/app-catalog/plans/:plan_id/execute", h.executeDeploy)
	}
	return r
}

// --- Repository CRUD handler tests ---

func TestAppCatalog_CreateRepository_201(t *testing.T) {
	svc := newAppCatalogTestService(t)
	r := newAppCatalogTestEngine(t, svc)
	body := `{"name":"test-repo","display_name":"Test","url":"https://charts.example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/app-catalog/repositories", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["name"] != "test-repo" {
		t.Errorf("name = %v, want %q", resp["name"], "test-repo")
	}
	// Verify credentials are not in response.
	if _, hasCreds := resp["credentials_json"]; hasCreds {
		t.Error("credentials_json should not be in response")
	}
}

func TestAppCatalog_CreateRepository_400InvalidJSON(t *testing.T) {
	svc := newAppCatalogTestService(t)
	r := newAppCatalogTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/app-catalog/repositories", strings.NewReader("{invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestAppCatalog_CreateRepository_400InvalidName(t *testing.T) {
	svc := newAppCatalogTestService(t)
	r := newAppCatalogTestEngine(t, svc)
	body := `{"name":"INVALID!","url":"https://charts.example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/app-catalog/repositories", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAppCatalog_ListRepositories_200(t *testing.T) {
	svc := newAppCatalogTestService(t)
	_, _ = svc.CreateRepository(context.Background(), appcatalog.CreateRepositoryRequest{
		Name: "repo1", URL: "https://example.com",
	}, appcatalog.ActorRef{ID: 1, Name: "admin"})
	_, _ = svc.CreateRepository(context.Background(), appcatalog.CreateRepositoryRequest{
		Name: "repo2", URL: "https://example.com",
	}, appcatalog.ActorRef{ID: 1, Name: "admin"})

	r := newAppCatalogTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/app-catalog/repositories", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Items []map[string]interface{} `json:"items"`
		Total int                      `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Total != 2 {
		t.Errorf("total = %d, want 2", resp.Total)
	}
}

func TestAppCatalog_GetRepository_200(t *testing.T) {
	svc := newAppCatalogTestService(t)
	repo, _ := svc.CreateRepository(context.Background(), appcatalog.CreateRepositoryRequest{
		Name: "test-repo", URL: "https://example.com",
	}, appcatalog.ActorRef{ID: 1, Name: "admin"})

	r := newAppCatalogTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/app-catalog/repositories/"+itoaInt64(repo.ID), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAppCatalog_GetRepository_404NotFound(t *testing.T) {
	svc := newAppCatalogTestService(t)
	r := newAppCatalogTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/app-catalog/repositories/999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAppCatalog_GetRepository_400InvalidID(t *testing.T) {
	svc := newAppCatalogTestService(t)
	r := newAppCatalogTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/app-catalog/repositories/abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestAppCatalog_DeleteRepository_204(t *testing.T) {
	svc := newAppCatalogTestService(t)
	repo, _ := svc.CreateRepository(context.Background(), appcatalog.CreateRepositoryRequest{
		Name: "test-repo", URL: "https://example.com",
	}, appcatalog.ActorRef{ID: 1, Name: "admin"})

	r := newAppCatalogTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/app-catalog/repositories/"+itoaInt64(repo.ID), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAppCatalog_DeleteRepository_404NotFound(t *testing.T) {
	svc := newAppCatalogTestService(t)
	r := newAppCatalogTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/app-catalog/repositories/999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Chart listing handler tests ---

func TestAppCatalog_ListCharts_200(t *testing.T) {
	svc := newAppCatalogTestService(t)
	repo, _ := svc.CreateRepository(context.Background(), appcatalog.CreateRepositoryRequest{
		Name: "test-repo", URL: "https://example.com",
	}, appcatalog.ActorRef{ID: 1, Name: "admin"})

	r := newAppCatalogTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/app-catalog/repositories/"+itoaInt64(repo.ID)+"/charts", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Items []map[string]interface{} `json:"items"`
		Total int                      `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Total != 2 {
		t.Errorf("total = %d, want 2", resp.Total)
	}
}

func TestAppCatalog_GetChart_200(t *testing.T) {
	svc := newAppCatalogTestService(t)
	repo, _ := svc.CreateRepository(context.Background(), appcatalog.CreateRepositoryRequest{
		Name: "test-repo", URL: "https://example.com",
	}, appcatalog.ActorRef{ID: 1, Name: "admin"})

	r := newAppCatalogTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/app-catalog/repositories/"+itoaInt64(repo.ID)+"/charts/nginx", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["name"] != "nginx" {
		t.Errorf("name = %v, want %q", resp["name"], "nginx")
	}
}

func TestAppCatalog_GetChart_404NotFound(t *testing.T) {
	svc := newAppCatalogTestService(t)
	repo, _ := svc.CreateRepository(context.Background(), appcatalog.CreateRepositoryRequest{
		Name: "test-repo", URL: "https://example.com",
	}, appcatalog.ActorRef{ID: 1, Name: "admin"})

	r := newAppCatalogTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/app-catalog/repositories/"+itoaInt64(repo.ID)+"/charts/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Deploy preview/execute handler tests ---

func TestAppCatalog_PreviewDeploy_201(t *testing.T) {
	svc := newAppCatalogTestService(t)
	repo, _ := svc.CreateRepository(context.Background(), appcatalog.CreateRepositoryRequest{
		Name: "test-repo", URL: "https://example.com",
	}, appcatalog.ActorRef{ID: 1, Name: "admin"})

	r := newAppCatalogTestEngine(t, svc)
	body := `{"repo_id":` + itoaInt64(repo.ID) + `,"chart_name":"nginx","chart_version":"1.2.3","target_cluster_id":1,"target_namespace":"default","release_name":"my-nginx"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/app-catalog/plans/preview", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["status"] != "awaiting_confirmation" {
		t.Errorf("status = %v, want %q", resp["status"], "awaiting_confirmation")
	}
	if resp["confirmation_token"] == nil || resp["confirmation_token"] == "" {
		t.Error("expected non-empty confirmation_token")
	}
}

func TestAppCatalog_PreviewDeploy_400InvalidRequest(t *testing.T) {
	svc := newAppCatalogTestService(t)
	r := newAppCatalogTestEngine(t, svc)
	body := `{"chart_name":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/app-catalog/plans/preview", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAppCatalog_PreviewDeploy_400NamespaceMissing(t *testing.T) {
	svc := newAppCatalogTestService(t)
	repo, _ := svc.CreateRepository(context.Background(), appcatalog.CreateRepositoryRequest{
		Name: "test-repo", URL: "https://example.com",
	}, appcatalog.ActorRef{ID: 1, Name: "admin"})

	r := newAppCatalogTestEngine(t, svc)
	body := `{"repo_id":` + itoaInt64(repo.ID) + `,"chart_name":"nginx","chart_version":"1.2.3","target_cluster_id":1,"target_namespace":"nonexistent","release_name":"my-nginx"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/app-catalog/plans/preview", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAppCatalog_ExecuteDeploy_200(t *testing.T) {
	svc := newAppCatalogTestService(t)
	repo, _ := svc.CreateRepository(context.Background(), appcatalog.CreateRepositoryRequest{
		Name: "test-repo", URL: "https://example.com",
	}, appcatalog.ActorRef{ID: 1, Name: "admin"})
	plan, _ := svc.Preview(context.Background(), appcatalog.DeployPreviewRequest{
		RepoID: repo.ID, ChartName: "nginx", ChartVersion: "1.2.3",
		TargetClusterID: 1, TargetNamespace: "default", ReleaseName: "my-nginx",
	}, appcatalog.ActorRef{ID: 1, Name: "admin"})

	r := newAppCatalogTestEngine(t, svc)
	body := `{"confirmation_token":"` + plan.ConfirmationToken + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/app-catalog/plans/"+plan.ID+"/execute", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "test-idempotency-key-1234")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["status"] != "succeeded" {
		t.Errorf("status = %v, want %q", resp["status"], "succeeded")
	}
}

func TestAppCatalog_ExecuteDeploy_400InvalidPlanID(t *testing.T) {
	svc := newAppCatalogTestService(t)
	r := newAppCatalogTestEngine(t, svc)
	body := `{"confirmation_token":"token"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/app-catalog/plans/short/execute", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAppCatalog_ExecuteDeploy_400MissingToken(t *testing.T) {
	svc := newAppCatalogTestService(t)
	r := newAppCatalogTestEngine(t, svc)
	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/app-catalog/plans/aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee/execute", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAppCatalog_GetPlan_200(t *testing.T) {
	svc := newAppCatalogTestService(t)
	repo, _ := svc.CreateRepository(context.Background(), appcatalog.CreateRepositoryRequest{
		Name: "test-repo", URL: "https://example.com",
	}, appcatalog.ActorRef{ID: 1, Name: "admin"})
	plan, _ := svc.Preview(context.Background(), appcatalog.DeployPreviewRequest{
		RepoID: repo.ID, ChartName: "nginx", ChartVersion: "1.2.3",
		TargetClusterID: 1, TargetNamespace: "default", ReleaseName: "my-nginx",
	}, appcatalog.ActorRef{ID: 1, Name: "admin"})

	r := newAppCatalogTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/app-catalog/plans/"+plan.ID, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAppCatalog_GetPlan_404NotFound(t *testing.T) {
	svc := newAppCatalogTestService(t)
	r := newAppCatalogTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/app-catalog/plans/aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAppCatalog_ListPlans_200(t *testing.T) {
	svc := newAppCatalogTestService(t)
	repo, _ := svc.CreateRepository(context.Background(), appcatalog.CreateRepositoryRequest{
		Name: "test-repo", URL: "https://example.com",
	}, appcatalog.ActorRef{ID: 1, Name: "admin"})
	_, _ = svc.Preview(context.Background(), appcatalog.DeployPreviewRequest{
		RepoID: repo.ID, ChartName: "nginx", ChartVersion: "1.2.3",
		TargetClusterID: 1, TargetNamespace: "default", ReleaseName: "my-nginx",
	}, appcatalog.ActorRef{ID: 1, Name: "admin"})

	r := newAppCatalogTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/app-catalog/plans?cluster_id=1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAppCatalog_ListPlans_400MissingClusterID(t *testing.T) {
	svc := newAppCatalogTestService(t)
	r := newAppCatalogTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/app-catalog/plans", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestAppCatalog_ListPlans_400InvalidClusterID(t *testing.T) {
	svc := newAppCatalogTestService(t)
	r := newAppCatalogTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/app-catalog/plans?cluster_id=abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// --- error mapping test ---

func TestAppCatalog_WriteError_AllSentinels(t *testing.T) {
	// Verify each sentinel error maps to the expected status code.
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"repo not found", appcatalog.ErrRepoNotFound, http.StatusNotFound},
		{"repo name exists", appcatalog.ErrRepoNameExists, http.StatusConflict},
		{"repo URL invalid", appcatalog.ErrRepoURLInvalid, http.StatusBadRequest},
		{"repo unreachable", appcatalog.ErrRepoUnreachable, http.StatusBadGateway},
		{"chart not found", appcatalog.ErrChartNotFound, http.StatusNotFound},
		{"plan not found", appcatalog.ErrPlanNotFound, http.StatusNotFound},
		{"invalid request", appcatalog.ErrInvalidRequest, http.StatusBadRequest},
		{"namespace missing", appcatalog.ErrNamespaceMissing, http.StatusConflict},
		{"cluster unavailable", appcatalog.ErrClusterUnavailable, http.StatusConflict},
		{"preview failed", appcatalog.ErrPreviewFailed, http.StatusBadRequest},
		{"confirmation invalid", appcatalog.ErrConfirmationInvalid, http.StatusForbidden},
		{"invalid idempotency", appcatalog.ErrInvalidIdempotency, http.StatusBadRequest},
		{"expired", appcatalog.ErrExpired, http.StatusGone},
		{"in progress", appcatalog.ErrInProgress, http.StatusConflict},
		{"already executed", appcatalog.ErrAlreadyExecuted, http.StatusConflict},
		{"execution failed", appcatalog.ErrExecutionFailed, http.StatusBadGateway},
		{"unknown error", errors.New("unknown"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			appCatalogHandler{}.writeError(c, tt.err, "fallback")
			if w.Code != tt.wantStatus {
				t.Errorf("got %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

// --- helpers ---

func newAppCatalogTestService(t *testing.T) *appcatalog.Service {
	t.Helper()
	k8s := newFakeAppCatalogKubernetes()
	store := newFakeAppCatalogDataStore()
	idx := &fakeAppCatalogIndexSource{body: []byte(validAppCatalogIndexYAML)}
	return appcatalog.NewTestService(k8s, store, idx)
}

func itoaInt64(n int64) string {
	return strings.TrimSpace(string(bytesFromInt(n)))
}

func bytesFromInt(n int64) []byte {
	if n == 0 {
		return []byte("0")
	}
	var buf []byte
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	return buf
}

// Test fakes for the handler tests — mirror the service_test.go fakes but in
// the httpserver package (so they can be reused across handler tests).

type fakeAppCatalogKubernetes struct {
	namespaces map[string]bool
}

func newFakeAppCatalogKubernetes() *fakeAppCatalogKubernetes {
	return &fakeAppCatalogKubernetes{namespaces: map[string]bool{"1:default": true}}
}

func (f *fakeAppCatalogKubernetes) NamespaceExists(_ context.Context, clusterID int64, namespace string) (bool, error) {
	key := itoaInt64(clusterID) + ":" + namespace
	return f.namespaces[key], nil
}

func (f *fakeAppCatalogKubernetes) ResourceExists(_ context.Context, _ int64, _ string) (bool, error) {
	return false, nil
}

func (f *fakeAppCatalogKubernetes) CreateResource(_ context.Context, _ int64, _ string, body []byte, _ bool) ([]byte, error) {
	return body, nil
}

type fakeAppCatalogDataStore struct {
	repos  map[int64]*appcatalog.Repository
	plans  map[string]*appcatalog.Plan
	nextID int64
}

func newFakeAppCatalogDataStore() *fakeAppCatalogDataStore {
	return &fakeAppCatalogDataStore{repos: map[int64]*appcatalog.Repository{}, plans: map[string]*appcatalog.Plan{}, nextID: 1}
}

func (f *fakeAppCatalogDataStore) SaveRepo(_ context.Context, repo *appcatalog.Repository) error {
	if repo.ID == 0 {
		repo.ID = f.nextID
		f.nextID++
	}
	stored := *repo
	f.repos[repo.ID] = &stored
	return nil
}

func (f *fakeAppCatalogDataStore) GetRepo(_ context.Context, id int64) (appcatalog.Repository, error) {
	repo, ok := f.repos[id]
	if !ok {
		return appcatalog.Repository{}, appcatalog.ErrRepoNotFound
	}
	return *repo, nil
}

func (f *fakeAppCatalogDataStore) GetRepoByName(_ context.Context, name string) (appcatalog.Repository, error) {
	for _, repo := range f.repos {
		if repo.Name == name {
			return *repo, nil
		}
	}
	return appcatalog.Repository{}, appcatalog.ErrRepoNotFound
}

func (f *fakeAppCatalogDataStore) ListRepos(_ context.Context) ([]appcatalog.Repository, error) {
	result := make([]appcatalog.Repository, 0, len(f.repos))
	for _, repo := range f.repos {
		result = append(result, *repo)
	}
	return result, nil
}

func (f *fakeAppCatalogDataStore) DeleteRepo(_ context.Context, id int64) error {
	if _, ok := f.repos[id]; !ok {
		return appcatalog.ErrRepoNotFound
	}
	delete(f.repos, id)
	return nil
}

func (f *fakeAppCatalogDataStore) SavePlan(_ context.Context, plan *appcatalog.Plan) error {
	stored := *plan
	f.plans[plan.ID] = &stored
	return nil
}

func (f *fakeAppCatalogDataStore) GetPlan(_ context.Context, id string) (appcatalog.Plan, error) {
	plan, ok := f.plans[id]
	if !ok {
		return appcatalog.Plan{}, appcatalog.ErrPlanNotFound
	}
	return *plan, nil
}

func (f *fakeAppCatalogDataStore) ListPlans(_ context.Context, clusterID int64, namespace string) ([]appcatalog.Plan, error) {
	result := make([]appcatalog.Plan, 0)
	for _, plan := range f.plans {
		if plan.TargetClusterID == clusterID {
			if namespace == "" || plan.TargetNamespace == namespace {
				result = append(result, *plan)
			}
		}
	}
	return result, nil
}

func (f *fakeAppCatalogDataStore) ClaimPlan(_ context.Context, id string, tokenHash []byte, idempotencyKey string, now, _ time.Time) (appcatalog.Plan, bool, error) {
	plan, ok := f.plans[id]
	if !ok {
		return appcatalog.Plan{}, false, appcatalog.ErrPlanNotFound
	}
	if len(plan.ConfirmationTokenHash) != len(tokenHash) {
		return *plan, false, appcatalog.ErrConfirmationInvalid
	}
	for i := range tokenHash {
		if plan.ConfirmationTokenHash[i] != tokenHash[i] {
			return *plan, false, appcatalog.ErrConfirmationInvalid
		}
	}
	if plan.Status == appcatalog.StatusAwaitingConfirmation {
		plan.Status = appcatalog.StatusExecuting
		plan.IdempotencyKey = idempotencyKey
		plan.LockedAt = &now
		return *plan, true, nil
	}
	if plan.Status == appcatalog.StatusExecuting {
		if plan.IdempotencyKey != idempotencyKey {
			return *plan, false, appcatalog.ErrAlreadyExecuted
		}
		return *plan, true, nil
	}
	return *plan, false, nil
}

func (f *fakeAppCatalogDataStore) CompletePlan(_ context.Context, id, _ string, executedAt time.Time) (appcatalog.Plan, error) {
	plan, ok := f.plans[id]
	if !ok {
		return appcatalog.Plan{}, appcatalog.ErrPlanNotFound
	}
	plan.Status = appcatalog.StatusSucceeded
	plan.ExecutedAt = &executedAt
	plan.LockedAt = nil
	plan.LastError = ""
	return *plan, nil
}

func (f *fakeAppCatalogDataStore) FailPlan(_ context.Context, id, _, message string) (appcatalog.Plan, error) {
	plan, ok := f.plans[id]
	if !ok {
		return appcatalog.Plan{}, appcatalog.ErrPlanNotFound
	}
	plan.Status = appcatalog.StatusFailed
	plan.LastError = message
	plan.LockedAt = nil
	return *plan, nil
}

func (f *fakeAppCatalogDataStore) ExpireStalePlans(_ context.Context, _ time.Time) error {
	return nil
}

type fakeAppCatalogIndexSource struct {
	body []byte
	err  error
}

func (f *fakeAppCatalogIndexSource) FetchIndex(_ context.Context, _, _, _ string) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.body, nil
}

const validAppCatalogIndexYAML = `apiVersion: v1
entries:
  nginx:
    - name: nginx
      version: 1.2.3
      appVersion: "1.25"
      description: A test nginx chart
      home: https://example.com
      icon: https://example.com/icon.png
      digest: abc123
      created: "2026-07-01T00:00:00Z"
      maintainers:
        - name: alice
          email: alice@example.com
    - name: nginx
      version: 1.1.0
      appVersion: "1.24"
      description: Older nginx chart
      digest: def456
      created: "2026-06-01T00:00:00Z"
  redis:
    - name: redis
      version: 0.9.0
      appVersion: "7.0"
      description: A redis chart
`
