package httpserver

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/apiquery"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
	"k8s-aiops.local/backend/internal/maintenance"
	"k8s-aiops.local/backend/internal/requestctx"
)

type maintenanceKubernetesStub struct {
	nodeErr error
}

func (s maintenanceKubernetesStub) Node(context.Context, int64, string) (k8sgateway.Node, error) {
	return k8sgateway.Node{}, s.nodeErr
}
func (s maintenanceKubernetesStub) Pods(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Pod], error) {
	return apiquery.ListResponse[k8sgateway.Pod]{}, nil
}
func (s maintenanceKubernetesStub) PodDisruptionBudgets(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.PodDisruptionBudget], error) {
	return apiquery.ListResponse[k8sgateway.PodDisruptionBudget]{}, nil
}
func (s maintenanceKubernetesStub) PatchNode(context.Context, int64, string, []byte, bool) (k8sgateway.Node, error) {
	return k8sgateway.Node{}, nil
}
func (s maintenanceKubernetesStub) CreateResource(context.Context, int64, string, []byte, bool) ([]byte, error) {
	return nil, nil
}

type maintenanceRepoStub struct {
	listErr  error
	claimErr error
}

func (s maintenanceRepoStub) Save(context.Context, *maintenance.Plan) error { return nil }
func (s maintenanceRepoStub) List(context.Context, int64) ([]maintenance.Plan, error) {
	return nil, s.listErr
}
func (s maintenanceRepoStub) Claim(context.Context, string, []byte, string, time.Time, time.Time) (maintenance.Plan, bool, error) {
	return maintenance.Plan{}, false, s.claimErr
}
func (s maintenanceRepoStub) Complete(context.Context, string, string, time.Time, maintenance.Plan, *maintenance.ExecutionResultJSON) (maintenance.Plan, error) {
	return maintenance.Plan{}, nil
}
func (s maintenanceRepoStub) Fail(context.Context, string, string, string, *maintenance.ExecutionResultJSON) (maintenance.Plan, error) {
	return maintenance.Plan{}, nil
}

func newMaintenanceRouter(kube maintenanceKubernetesStub, repo maintenanceRepoStub) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := maintenanceHandler{service: maintenance.NewService(kube, repo)}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), requestctx.Metadata{
			ActorID: 1, ActorDisplayName: "Admin", Roles: []string{"system_admin"}, RequestID: "maintenance-test",
		}))
		c.Next()
	})
	router.POST("/api/v1/clusters/:cluster_id/maintenance-plans/preview", h.preview)
	router.POST("/api/v1/maintenance-plans/:plan_id/execute", h.execute)
	router.GET("/api/v1/clusters/:cluster_id/maintenance-plans", h.list)
	return router
}

func TestMaintenancePreviewValidation(t *testing.T) {
	router := newMaintenanceRouter(maintenanceKubernetesStub{}, maintenanceRepoStub{})

	// invalid cluster id
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/clusters/0/maintenance-plans/preview", strings.NewReader(`{"action":"cordon","node_name":"node-1"}`)))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_CLUSTER_ID") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// missing action/node_name: decodeStrictJSON does not run binding
	// validation, so the service rejects with ErrInvalidRequest.
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/clusters/1/maintenance-plans/preview", strings.NewReader(`{}`)))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_MAINTENANCE_REQUEST") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// node lookup failure maps through writeError
	router = newMaintenanceRouter(maintenanceKubernetesStub{nodeErr: maintenance.ErrNodeNotFound}, maintenanceRepoStub{})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/clusters/1/maintenance-plans/preview", strings.NewReader(`{"action":"cordon","node_name":"node-1"}`)))
	if rec.Code != http.StatusNotFound || !contains(rec.Body.String(), "NODE_NOT_FOUND") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestMaintenanceExecuteValidation(t *testing.T) {
	router := newMaintenanceRouter(maintenanceKubernetesStub{}, maintenanceRepoStub{})

	// missing confirmation token
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/maintenance-plans/p-1/execute", strings.NewReader(`{}`)))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_REQUEST") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// missing idempotency key
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/maintenance-plans/p-1/execute", strings.NewReader(`{"confirmation_token":"tok"}`))
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_IDEMPOTENCY_KEY") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// expired plan surfaces 410 through writeError
	router = newMaintenanceRouter(maintenanceKubernetesStub{}, maintenanceRepoStub{claimErr: maintenance.ErrExpired})
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/maintenance-plans/p-1/execute", strings.NewReader(`{"confirmation_token":"tok"}`))
	req.Header.Set("Idempotency-Key", "idem-12345678")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusGone || !contains(rec.Body.String(), "MAINTENANCE_EXPIRED") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestMaintenanceList(t *testing.T) {
	router := newMaintenanceRouter(maintenanceKubernetesStub{}, maintenanceRepoStub{})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/maintenance-plans", nil))
	if rec.Code != http.StatusOK || !contains(rec.Body.String(), `"items":[]`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	router = newMaintenanceRouter(maintenanceKubernetesStub{}, maintenanceRepoStub{listErr: errors.New("db down")})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/maintenance-plans", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestMaintenanceWriteErrorMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := maintenanceHandler{}
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{err: maintenance.ErrInvalidRequest, status: http.StatusBadRequest, code: "INVALID_MAINTENANCE_REQUEST"},
		{err: maintenance.ErrNodeNotFound, status: http.StatusNotFound, code: "NODE_NOT_FOUND"},
		{err: maintenance.ErrControlPlaneNode, status: http.StatusUnprocessableEntity, code: "CONTROL_PLANE_NODE"},
		{err: maintenance.ErrAlreadyCordoned, status: http.StatusConflict, code: "ALREADY_CORDONED"},
		{err: maintenance.ErrAlreadyUncordoned, status: http.StatusConflict, code: "ALREADY_UNCORDONED"},
		{err: maintenance.ErrNotCordoned, status: http.StatusConflict, code: "NOT_CORDONED"},
		{err: maintenance.ErrTooManyPods, status: http.StatusUnprocessableEntity, code: "TOO_MANY_PODS"},
		{err: maintenance.ErrUnmanagedPod, status: http.StatusUnprocessableEntity, code: "UNMANAGED_POD"},
		{err: maintenance.ErrEmptyDirPod, status: http.StatusUnprocessableEntity, code: "EMPTYDIR_POD"},
		{err: maintenance.ErrPDBUnavailable, status: http.StatusUnprocessableEntity, code: "PDB_UNAVAILABLE"},
		{err: maintenance.ErrStaleTarget, status: http.StatusConflict, code: "STALE_TARGET"},
		{err: maintenance.ErrConfirmationInvalid, status: http.StatusForbidden, code: "MAINTENANCE_CONFIRMATION_INVALID"},
		{err: maintenance.ErrInvalidIdempotency, status: http.StatusBadRequest, code: "INVALID_IDEMPOTENCY_KEY"},
		{err: maintenance.ErrExpired, status: http.StatusGone, code: "MAINTENANCE_EXPIRED"},
		{err: maintenance.ErrInProgress, status: http.StatusConflict, code: "MAINTENANCE_IN_PROGRESS"},
		{err: maintenance.ErrAlreadyExecuted, status: http.StatusConflict, code: "MAINTENANCE_ALREADY_USED"},
		{err: maintenance.ErrExecutionFailed, status: http.StatusBadGateway, code: "MAINTENANCE_FAILED"},
		{err: maintenance.ErrPartialDrain, status: http.StatusMultiStatus, code: "PARTIAL_DRAIN"},
		{err: maintenance.ErrNotFound, status: http.StatusNotFound, code: "MAINTENANCE_PLAN_NOT_FOUND"},
		{err: errors.New("boom"), status: http.StatusInternalServerError, code: "INTERNAL_ERROR"},
	}
	for _, tt := range cases {
		router := gin.New()
		router.GET("/x", func(c *gin.Context) { h.writeError(c, tt.err, "fallback") })
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
		if rec.Code != tt.status || !contains(rec.Body.String(), tt.code) {
			t.Fatalf("err=%v status=%d body=%s want status=%d code=%s", tt.err, rec.Code, rec.Body.String(), tt.status, tt.code)
		}
	}
}
