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
	"k8s-aiops.local/backend/internal/requestctx"
	"k8s-aiops.local/backend/internal/restore"
)

type restoreKubernetesStub struct {
	veleroErr error
}

func (s restoreKubernetesStub) VeleroCapability(context.Context, int64) (k8sgateway.VeleroCapability, error) {
	return k8sgateway.VeleroCapability{Installed: true}, s.veleroErr
}
func (s restoreKubernetesStub) Backup(context.Context, int64, string, string) (k8sgateway.VeleroBackup, error) {
	return k8sgateway.VeleroBackup{}, nil
}
func (s restoreKubernetesStub) NamespaceExists(context.Context, int64, string) (bool, error) {
	return false, nil
}
func (s restoreKubernetesStub) VeleroRestoreExists(context.Context, int64, string, string) (bool, error) {
	return false, nil
}
func (s restoreKubernetesStub) VeleroRestore(context.Context, int64, string, string) (k8sgateway.VeleroRestore, error) {
	return k8sgateway.VeleroRestore{}, nil
}
func (s restoreKubernetesStub) CreateResource(context.Context, int64, string, []byte, bool) ([]byte, error) {
	return nil, nil
}
func (s restoreKubernetesStub) Deployments(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Deployment], error) {
	return apiquery.ListResponse[k8sgateway.Deployment]{}, nil
}
func (s restoreKubernetesStub) StatefulSets(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.StatefulSet], error) {
	return apiquery.ListResponse[k8sgateway.StatefulSet]{}, nil
}
func (s restoreKubernetesStub) DaemonSets(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.DaemonSet], error) {
	return apiquery.ListResponse[k8sgateway.DaemonSet]{}, nil
}
func (s restoreKubernetesStub) CronJobs(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.CronJob], error) {
	return apiquery.ListResponse[k8sgateway.CronJob]{}, nil
}
func (s restoreKubernetesStub) ConfigMaps(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.ConfigMap], error) {
	return apiquery.ListResponse[k8sgateway.ConfigMap]{}, nil
}
func (s restoreKubernetesStub) Secrets(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Secret], error) {
	return apiquery.ListResponse[k8sgateway.Secret]{}, nil
}
func (s restoreKubernetesStub) ServiceAccounts(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.ServiceAccount], error) {
	return apiquery.ListResponse[k8sgateway.ServiceAccount]{}, nil
}

type restoreRepoStub struct {
	listErr  error
	claimErr error
}

func (s restoreRepoStub) Save(context.Context, *restore.Plan) error { return nil }
func (s restoreRepoStub) List(context.Context, int64) ([]restore.Plan, error) {
	return nil, s.listErr
}
func (s restoreRepoStub) Claim(context.Context, string, []byte, string, time.Time, time.Time) (restore.Plan, bool, error) {
	return restore.Plan{}, false, s.claimErr
}
func (s restoreRepoStub) Complete(context.Context, string, string, time.Time, restore.Plan, *restore.ExecutionResultJSON) (restore.Plan, error) {
	return restore.Plan{}, nil
}
func (s restoreRepoStub) Fail(context.Context, string, string, string, *restore.ExecutionResultJSON) (restore.Plan, error) {
	return restore.Plan{}, nil
}
func (s restoreRepoStub) ActiveBySource(context.Context, int64, string, string) (restore.Plan, bool, error) {
	return restore.Plan{}, false, nil
}

func newRestoreRouter(kube restoreKubernetesStub, repo restoreRepoStub, live *k8sgateway.Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := restoreHandler{service: restore.NewService(kube, repo), kubernetes: live}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), requestctx.Metadata{
			ActorID: 1, ActorDisplayName: "Admin", Roles: []string{"system_admin"}, RequestID: "restore-test",
		}))
		c.Next()
	})
	router.POST("/api/v1/clusters/:cluster_id/restore-plans/preview", h.preview)
	router.POST("/api/v1/restore-plans/:plan_id/execute", h.execute)
	router.GET("/api/v1/clusters/:cluster_id/restore-plans", h.list)
	router.GET("/api/v1/clusters/:cluster_id/restores", h.listRestores)
	router.GET("/api/v1/clusters/:cluster_id/restores/:namespace/:name", h.getRestore)
	return router
}

func TestRestorePreviewAndExecute(t *testing.T) {
	router := newRestoreRouter(restoreKubernetesStub{}, restoreRepoStub{}, nil)

	// invalid request (missing source_backup_name -> service validation)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/clusters/1/restore-plans/preview", strings.NewReader(`{}`)))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_RESTORE_REQUEST") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Velero not installed -> 503
	router = newRestoreRouter(restoreKubernetesStub{veleroErr: restore.ErrVeleroNotInstalled}, restoreRepoStub{}, nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/clusters/1/restore-plans/preview", strings.NewReader(`{"source_backup_name":"demo-backup","source_backup_namespace":"velero"}`)))
	if rec.Code != http.StatusServiceUnavailable || !contains(rec.Body.String(), "VELERO_UNAVAILABLE") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// execute: missing confirmation token (binding enforced)
	router = newRestoreRouter(restoreKubernetesStub{}, restoreRepoStub{}, nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/restore-plans/p-1/execute", strings.NewReader(`{}`)))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_REQUEST") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// execute: missing idempotency key
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/restore-plans/p-1/execute", strings.NewReader(`{"confirmation_token":"tok"}`))
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_IDEMPOTENCY_KEY") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// execute: expired plan -> 410
	router = newRestoreRouter(restoreKubernetesStub{}, restoreRepoStub{claimErr: restore.ErrExpired}, nil)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/restore-plans/p-1/execute", strings.NewReader(`{"confirmation_token":"tok"}`))
	req.Header.Set("Idempotency-Key", "idem-12345678")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusGone || !contains(rec.Body.String(), "RESTORE_EXPIRED") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRestoreListAndLiveCR(t *testing.T) {
	router := newRestoreRouter(restoreKubernetesStub{}, restoreRepoStub{}, nil)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/restore-plans", nil))
	if rec.Code != http.StatusOK || !contains(rec.Body.String(), `"items":[]`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	router = newRestoreRouter(restoreKubernetesStub{}, restoreRepoStub{listErr: errors.New("db down")}, nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/restore-plans", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// live CR list/detail with nil provider -> 503
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/restores", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/restores/velero/demo-restore", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// live CR detail with provider configured: empty name -> 400
	router = newRestoreRouter(restoreKubernetesStub{}, restoreRepoStub{}, k8sgateway.NewService(nil, nil, nil))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/restores/velero/%20", nil))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_REQUEST") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRestoreWriteErrorMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := restoreHandler{}
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{err: restore.ErrInvalidRequest, status: http.StatusBadRequest, code: "INVALID_RESTORE_REQUEST"},
		{err: restore.ErrVeleroNotInstalled, status: http.StatusServiceUnavailable, code: "VELERO_UNAVAILABLE"},
		{err: restore.ErrSourceBackupNotFound, status: http.StatusNotFound, code: "SOURCE_BACKUP_NOT_FOUND"},
		{err: restore.ErrSourceBackupIncomplete, status: http.StatusUnprocessableEntity, code: "SOURCE_BACKUP_INCOMPLETE"},
		{err: restore.ErrSourceBackupScope, status: http.StatusUnprocessableEntity, code: "SOURCE_BACKUP_SCOPE"},
		{err: restore.ErrDestinationExists, status: http.StatusConflict, code: "DESTINATION_EXISTS"},
		{err: restore.ErrDestinationCollision, status: http.StatusConflict, code: "DESTINATION_COLLISION"},
		{err: restore.ErrRestoreNameConflict, status: http.StatusConflict, code: "RESTORE_NAME_CONFLICT"},
		{err: restore.ErrQuarantineDryRunFailed, status: http.StatusBadRequest, code: "QUARANTINE_DRY_RUN_FAILED"},
		{err: restore.ErrRestoreDryRunFailed, status: http.StatusBadRequest, code: "RESTORE_DRY_RUN_FAILED"},
		{err: restore.ErrConfirmationInvalid, status: http.StatusForbidden, code: "RESTORE_CONFIRMATION_INVALID"},
		{err: restore.ErrInvalidIdempotency, status: http.StatusBadRequest, code: "INVALID_IDEMPOTENCY_KEY"},
		{err: restore.ErrExpired, status: http.StatusGone, code: "RESTORE_EXPIRED"},
		{err: restore.ErrInProgress, status: http.StatusConflict, code: "RESTORE_IN_PROGRESS"},
		{err: restore.ErrAlreadyExecuted, status: http.StatusConflict, code: "RESTORE_ALREADY_USED"},
		{err: restore.ErrStaleSource, status: http.StatusConflict, code: "STALE_SOURCE"},
		{err: restore.ErrQuarantineFailed, status: http.StatusBadGateway, code: "QUARANTINE_FAILED"},
		{err: restore.ErrExecutionFailed, status: http.StatusBadGateway, code: "RESTORE_FAILED"},
		{err: restore.ErrRestorePollTimeout, status: http.StatusGatewayTimeout, code: "RESTORE_POLL_TIMEOUT"},
		{err: restore.ErrPartialRestore, status: http.StatusMultiStatus, code: "PARTIAL_RESTORE"},
		{err: restore.ErrNotFound, status: http.StatusNotFound, code: "RESTORE_PLAN_NOT_FOUND"},
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
