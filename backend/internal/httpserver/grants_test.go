package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/authz"
	"k8s-aiops.local/backend/internal/requestctx"
)

// grantsTestRepo is an in-memory authz.Repository for grants handler tests.
type grantsTestRepo struct {
	clusterGrants    map[int64]map[int64]authz.ClusterGrant
	namespaceGrants  map[int64]map[string]authz.NamespaceGrant
	createErr        error
	deleteErr        error
	listClusterErr   error
	listNamespaceErr error
}

func newGrantsTestRepo() *grantsTestRepo {
	return &grantsTestRepo{
		clusterGrants:   make(map[int64]map[int64]authz.ClusterGrant),
		namespaceGrants: make(map[int64]map[string]authz.NamespaceGrant),
	}
}

func (r *grantsTestRepo) CreateClusterGrant(_ context.Context, userID, clusterID int64) (authz.ClusterGrant, error) {
	if r.createErr != nil {
		return authz.ClusterGrant{}, r.createErr
	}
	if r.clusterGrants[userID] != nil {
		if _, ok := r.clusterGrants[userID][clusterID]; ok {
			return authz.ClusterGrant{}, authz.ErrGrantAlreadyExists
		}
	}
	if r.clusterGrants[userID] == nil {
		r.clusterGrants[userID] = make(map[int64]authz.ClusterGrant)
	}
	grant := authz.ClusterGrant{ID: int64(len(r.clusterGrants[userID]) + 1), UserID: userID, ClusterID: clusterID}
	r.clusterGrants[userID][clusterID] = grant
	return grant, nil
}

func (r *grantsTestRepo) DeleteClusterGrant(_ context.Context, userID, clusterID int64) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	if r.clusterGrants[userID] == nil {
		return authz.ErrGrantNotFound
	}
	if _, ok := r.clusterGrants[userID][clusterID]; !ok {
		return authz.ErrGrantNotFound
	}
	delete(r.clusterGrants[userID], clusterID)
	return nil
}

func (r *grantsTestRepo) ListClusterGrants(_ context.Context, userID int64) ([]authz.ClusterGrant, error) {
	if r.listClusterErr != nil {
		return nil, r.listClusterErr
	}
	if r.clusterGrants[userID] == nil {
		return []authz.ClusterGrant{}, nil
	}
	grants := make([]authz.ClusterGrant, 0, len(r.clusterGrants[userID]))
	for _, g := range r.clusterGrants[userID] {
		grants = append(grants, g)
	}
	return grants, nil
}

func (r *grantsTestRepo) CreateNamespaceGrant(_ context.Context, userID, clusterID int64, namespace string) (authz.NamespaceGrant, error) {
	if r.createErr != nil {
		return authz.NamespaceGrant{}, r.createErr
	}
	key := grantsKey(clusterID, namespace)
	if r.namespaceGrants[userID] != nil {
		if _, ok := r.namespaceGrants[userID][key]; ok {
			return authz.NamespaceGrant{}, authz.ErrGrantAlreadyExists
		}
	}
	if r.namespaceGrants[userID] == nil {
		r.namespaceGrants[userID] = make(map[string]authz.NamespaceGrant)
	}
	grant := authz.NamespaceGrant{ID: int64(len(r.namespaceGrants[userID]) + 1), UserID: userID, ClusterID: clusterID, Namespace: namespace}
	r.namespaceGrants[userID][key] = grant
	return grant, nil
}

func (r *grantsTestRepo) DeleteNamespaceGrant(_ context.Context, userID, clusterID int64, namespace string) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	key := grantsKey(clusterID, namespace)
	if r.namespaceGrants[userID] == nil {
		return authz.ErrGrantNotFound
	}
	if _, ok := r.namespaceGrants[userID][key]; !ok {
		return authz.ErrGrantNotFound
	}
	delete(r.namespaceGrants[userID], key)
	return nil
}

func (r *grantsTestRepo) ListNamespaceGrants(_ context.Context, userID int64) ([]authz.NamespaceGrant, error) {
	if r.listNamespaceErr != nil {
		return nil, r.listNamespaceErr
	}
	if r.namespaceGrants[userID] == nil {
		return []authz.NamespaceGrant{}, nil
	}
	grants := make([]authz.NamespaceGrant, 0, len(r.namespaceGrants[userID]))
	for _, g := range r.namespaceGrants[userID] {
		grants = append(grants, g)
	}
	return grants, nil
}

func (r *grantsTestRepo) ClusterScope(_ context.Context, userID, clusterID int64) (authz.ClusterScope, error) {
	return authz.ClusterScope{ClusterID: clusterID}, nil
}
func (r *grantsTestRepo) VisibleClusters(context.Context, int64) ([]int64, error) {
	return nil, nil
}
func (r *grantsTestRepo) HasClusterGrant(context.Context, int64, int64) (bool, error) {
	return false, nil
}

func grantsKey(clusterID int64, namespace string) string {
	return jsonKey(clusterID) + "/" + namespace
}

func jsonKey(n int64) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	digits := make([]byte, 0, 20)
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if negative {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}

func newGrantsRouter(repo *grantsTestRepo) (*gin.Engine, *grantHandler) {
	gin.SetMode(gin.TestMode)
	manager := authz.NewGrantManager(repo)
	handler := grantHandler{manager: manager}
	router := gin.New()
	return router, &handler
}

func TestGrantsListClusterGrantsEmptyByDefault(t *testing.T) {
	router, handler := newGrantsRouter(newGrantsTestRepo())
	router.GET("/users/:user_id/cluster-grants", handler.listClusterGrants)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/users/7/cluster-grants", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Items []authz.ClusterGrant `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Items) != 0 {
		t.Fatalf("items = %#v", response.Items)
	}
}

func TestGrantsCreateClusterGrantReturnsCreated(t *testing.T) {
	router, handler := newGrantsRouter(newGrantsTestRepo())
	router.POST("/users/:user_id/cluster-grants", handler.createClusterGrant)
	body, _ := json.Marshal(map[string]int64{"cluster_id": 42})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/users/7/cluster-grants", bytes.NewReader(body)))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var grant authz.ClusterGrant
	if err := json.Unmarshal(recorder.Body.Bytes(), &grant); err != nil {
		t.Fatal(err)
	}
	if grant.UserID != 7 || grant.ClusterID != 42 {
		t.Fatalf("grant = %#v", grant)
	}
}

func TestGrantsCreateClusterGrantDuplicateReturns409(t *testing.T) {
	router, handler := newGrantsRouter(newGrantsTestRepo())
	router.POST("/users/:user_id/cluster-grants", handler.createClusterGrant)
	body, _ := json.Marshal(map[string]int64{"cluster_id": 42})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/users/7/cluster-grants", bytes.NewReader(body)))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("setup status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/users/7/cluster-grants", bytes.NewReader(body)))
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestGrantsDeleteClusterGrantMissingReturns404(t *testing.T) {
	router, handler := newGrantsRouter(newGrantsTestRepo())
	router.DELETE("/users/:user_id/cluster-grants/:cluster_id", handler.deleteClusterGrant)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/users/7/cluster-grants/42", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestGrantsDeleteClusterGrantSuccessReturns204(t *testing.T) {
	repo := newGrantsTestRepo()
	if _, err := repo.CreateClusterGrant(context.Background(), 7, 42); err != nil {
		t.Fatal(err)
	}
	router, handler := newGrantsRouter(repo)
	router.DELETE("/users/:user_id/cluster-grants/:cluster_id", handler.deleteClusterGrant)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/users/7/cluster-grants/42", nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestGrantsCreateClusterGrantRejectsInvalidUserID(t *testing.T) {
	router, handler := newGrantsRouter(newGrantsTestRepo())
	router.POST("/users/:user_id/cluster-grants", handler.createClusterGrant)
	body, _ := json.Marshal(map[string]int64{"cluster_id": 42})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/users/abc/cluster-grants", bytes.NewReader(body)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestGrantsCreateClusterGrantRejectsInvalidBody(t *testing.T) {
	router, handler := newGrantsRouter(newGrantsTestRepo())
	router.POST("/users/:user_id/cluster-grants", handler.createClusterGrant)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/users/7/cluster-grants", bytes.NewReader([]byte("not-json"))))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestGrantsCreateNamespaceGrantReturnsCreated(t *testing.T) {
	router, handler := newGrantsRouter(newGrantsTestRepo())
	router.POST("/users/:user_id/namespace-grants", handler.createNamespaceGrant)
	body, _ := json.Marshal(map[string]any{"cluster_id": 42, "namespace": "prod"})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/users/7/namespace-grants", bytes.NewReader(body)))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var grant authz.NamespaceGrant
	if err := json.Unmarshal(recorder.Body.Bytes(), &grant); err != nil {
		t.Fatal(err)
	}
	if grant.UserID != 7 || grant.ClusterID != 42 || grant.Namespace != "prod" {
		t.Fatalf("grant = %#v", grant)
	}
}

func TestGrantsDeleteNamespaceGrantSuccessReturns204(t *testing.T) {
	repo := newGrantsTestRepo()
	if _, err := repo.CreateNamespaceGrant(context.Background(), 7, 42, "prod"); err != nil {
		t.Fatal(err)
	}
	router, handler := newGrantsRouter(repo)
	router.DELETE("/users/:user_id/namespace-grants/:cluster_id/:namespace", handler.deleteNamespaceGrant)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/users/7/namespace-grants/42/prod", nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestGrantsDeleteNamespaceGrantMissingReturns404(t *testing.T) {
	router, handler := newGrantsRouter(newGrantsTestRepo())
	router.DELETE("/users/:user_id/namespace-grants/:cluster_id/:namespace", handler.deleteNamespaceGrant)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/users/7/namespace-grants/42/prod", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestGrantsMyGrantsReturnsCurrentUserGrants(t *testing.T) {
	repo := newGrantsTestRepo()
	if _, err := repo.CreateClusterGrant(context.Background(), 7, 42); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateNamespaceGrant(context.Background(), 7, 11, "prod"); err != nil {
		t.Fatal(err)
	}
	router, handler := newGrantsRouter(repo)
	router.GET("/auth/me/grants", func(c *gin.Context) {
		metadata := requestctx.Metadata{RequestID: "test", ActorID: 7}
		c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), metadata))
		handler.myGrants(c)
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/auth/me/grants", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		ClusterGrants   []authz.ClusterGrant   `json:"cluster_grants"`
		NamespaceGrants []authz.NamespaceGrant `json:"namespace_grants"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.ClusterGrants) != 1 || len(response.NamespaceGrants) != 1 {
		t.Fatalf("response = %#v", response)
	}
}

func TestGrantsWriteGrantErrorMapsKnownErrors(t *testing.T) {
	handler := grantHandler{}
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "already exists", err: authz.ErrGrantAlreadyExists, want: http.StatusConflict},
		{name: "not found", err: authz.ErrGrantNotFound, want: http.StatusNotFound},
		{name: "internal", err: errors.New("boom"), want: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			if !handler.writeGrantError(c, tt.err) {
				t.Fatalf("writeGrantError returned false for err=%v", tt.err)
			}
			if recorder.Code != tt.want {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.want)
			}
		})
	}
}

func TestGrantsWriteGrantErrorReturnsFalseOnNil(t *testing.T) {
	handler := grantHandler{}
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	if handler.writeGrantError(c, nil) {
		t.Fatal("writeGrantError returned true for nil error")
	}
}

// --- M109: listNamespaceGrants + myGrants error + listClusterGrants error ---

func TestGrantsListNamespaceGrantsEmptyByDefault(t *testing.T) {
	router, handler := newGrantsRouter(newGrantsTestRepo())
	router.GET("/users/:user_id/namespace-grants", handler.listNamespaceGrants)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/users/1/namespace-grants", nil))
	if rec.Code != http.StatusOK || !contains(rec.Body.String(), `"items":[]`) {
		t.Fatalf("expected 200 empty items, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGrantsListNamespaceGrantsWithGrants(t *testing.T) {
	repo := newGrantsTestRepo()
	_, _ = repo.CreateNamespaceGrant(context.Background(), 1, 10, "prod")
	_, _ = repo.CreateNamespaceGrant(context.Background(), 1, 10, "staging")
	router, handler := newGrantsRouter(repo)
	router.GET("/users/:user_id/namespace-grants", handler.listNamespaceGrants)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/users/1/namespace-grants", nil))
	if rec.Code != http.StatusOK || !contains(rec.Body.String(), "prod") {
		t.Fatalf("expected 200 with grants, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGrantsListNamespaceGrantsError(t *testing.T) {
	repo := newGrantsTestRepo()
	repo.listNamespaceErr = errors.New("db fail")
	router, handler := newGrantsRouter(repo)
	router.GET("/users/:user_id/namespace-grants", handler.listNamespaceGrants)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/users/1/namespace-grants", nil))
	if rec.Code != http.StatusInternalServerError || !contains(rec.Body.String(), "INTERNAL_ERROR") {
		t.Fatalf("expected 500 INTERNAL_ERROR, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGrantsListClusterGrantsError(t *testing.T) {
	repo := newGrantsTestRepo()
	repo.listClusterErr = errors.New("db fail")
	router, handler := newGrantsRouter(repo)
	router.GET("/users/:user_id/cluster-grants", handler.listClusterGrants)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/users/1/cluster-grants", nil))
	if rec.Code != http.StatusInternalServerError || !contains(rec.Body.String(), "INTERNAL_ERROR") {
		t.Fatalf("expected 500 INTERNAL_ERROR, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGrantsMyGrantsClusterGrantsError(t *testing.T) {
	repo := newGrantsTestRepo()
	repo.listClusterErr = errors.New("db down")
	router, handler := newGrantsRouter(repo)
	router.GET("/my-grants", func(c *gin.Context) {
		c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), requestctx.Metadata{ActorID: 1}))
		handler.myGrants(c)
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/my-grants", nil))
	if rec.Code != http.StatusInternalServerError || !contains(rec.Body.String(), "internal_error") {
		t.Fatalf("expected 500 internal_error, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGrantsMyGrantsNamespaceGrantsError(t *testing.T) {
	repo := newGrantsTestRepo()
	repo.listNamespaceErr = errors.New("db down")
	router, handler := newGrantsRouter(repo)
	router.GET("/my-grants", func(c *gin.Context) {
		c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), requestctx.Metadata{ActorID: 1}))
		handler.myGrants(c)
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/my-grants", nil))
	if rec.Code != http.StatusInternalServerError || !contains(rec.Body.String(), "internal_error") {
		t.Fatalf("expected 500 internal_error, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGrantsDeleteNamespaceGrantInvalidClusterID(t *testing.T) {
	router, handler := newGrantsRouter(newGrantsTestRepo())
	router.DELETE("/users/:user_id/namespace-grants/:cluster_id/:namespace", handler.deleteNamespaceGrant)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/users/1/namespace-grants/abc/prod", nil))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_CLUSTER_ID") {
		t.Fatalf("expected 400 INVALID_CLUSTER_ID, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGrantsCreateNamespaceGrantEmptyNamespace(t *testing.T) {
	router, handler := newGrantsRouter(newGrantsTestRepo())
	router.POST("/users/:user_id/namespace-grants", handler.createNamespaceGrant)
	body, _ := json.Marshal(map[string]int64{"cluster_id": 10})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/users/1/namespace-grants", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	// binding:"required" on Namespace catches empty namespace before handler check
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_REQUEST") {
		t.Fatalf("expected 400 INVALID_REQUEST, got %d: %s", rec.Code, rec.Body.String())
	}
}
