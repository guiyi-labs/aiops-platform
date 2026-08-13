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
	"k8s-aiops.local/backend/internal/backup"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
	"k8s-aiops.local/backend/internal/requestctx"
)

type backupKubernetesStub struct {
	capErr error
}

func (s backupKubernetesStub) VeleroCapability(context.Context, int64) (k8sgateway.VeleroCapability, error) {
	return k8sgateway.VeleroCapability{Installed: true}, s.capErr
}
func (s backupKubernetesStub) Namespaces(context.Context, int64, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Namespace], error) {
	return apiquery.ListResponse[k8sgateway.Namespace]{Items: []k8sgateway.Namespace{{Metadata: k8sgateway.ObjectMeta{Name: "ns", UID: "ns-uid-1", ResourceVersion: "rv-1"}}}}, nil
}
func (s backupKubernetesStub) BackupStorageLocations(context.Context, int64, string) ([]k8sgateway.BackupStorageLocation, error) {
	return []k8sgateway.BackupStorageLocation{{Name: "default"}}, nil
}
func (s backupKubernetesStub) VeleroBackupExists(context.Context, int64, string, string) (bool, error) {
	return false, nil
}
func (s backupKubernetesStub) CreateResource(context.Context, int64, string, []byte, bool) ([]byte, error) {
	return nil, nil
}

type backupRepoStub struct {
	listErr  error
	claimErr error
}

func (s backupRepoStub) Save(context.Context, *backup.Plan) error { return nil }
func (s backupRepoStub) List(context.Context, int64) ([]backup.Plan, error) {
	return nil, s.listErr
}
func (s backupRepoStub) Claim(context.Context, string, []byte, string, time.Time, time.Time) (backup.Plan, bool, error) {
	return backup.Plan{}, false, s.claimErr
}
func (s backupRepoStub) Complete(context.Context, string, string, string, string, time.Time) (backup.Plan, error) {
	return backup.Plan{}, nil
}
func (s backupRepoStub) Fail(context.Context, string, string, string) (backup.Plan, error) {
	return backup.Plan{}, nil
}

func newBackupRouter(kube backupKubernetesStub, repo backupRepoStub, live *k8sgateway.Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := backupHandler{service: backup.NewService(kube, repo), kubernetes: live}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), requestctx.Metadata{
			ActorID: 1, ActorDisplayName: "Admin", Roles: []string{"system_admin"}, RequestID: "backup-test",
		}))
		c.Next()
	})
	router.POST("/api/v1/clusters/:cluster_id/backup-plans/preview", h.preview)
	router.POST("/api/v1/backup-plans/:plan_id/execute", h.execute)
	router.GET("/api/v1/clusters/:cluster_id/backup-plans", h.list)
	router.GET("/api/v1/clusters/:cluster_id/backups", h.listBackups)
	router.GET("/api/v1/clusters/:cluster_id/backups/:namespace/:name", h.getBackup)
	return router
}

func TestBackupPreviewAndExecute(t *testing.T) {
	router := newBackupRouter(backupKubernetesStub{}, backupRepoStub{}, nil)

	// invalid request: TTL not in allowlist -> service ErrInvalidRequest
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/clusters/1/backup-plans/preview", strings.NewReader(`{"source_namespace":"ns","storage_location":"default","ttl":"99h"}`)))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_BACKUP_REQUEST") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Velero not installed -> 503
	router = newBackupRouter(backupKubernetesStub{capErr: backup.ErrVeleroNotInstalled}, backupRepoStub{}, nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/clusters/1/backup-plans/preview", strings.NewReader(`{"source_namespace":"ns","storage_location":"default"}`)))
	if rec.Code != http.StatusServiceUnavailable || !contains(rec.Body.String(), "VELERO_UNAVAILABLE") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// execute: missing confirmation token (binding enforced)
	router = newBackupRouter(backupKubernetesStub{}, backupRepoStub{}, nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/backup-plans/p-1/execute", strings.NewReader(`{}`)))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_REQUEST") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// execute: missing idempotency key
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backup-plans/p-1/execute", strings.NewReader(`{"confirmation_token":"tok"}`))
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_IDEMPOTENCY_KEY") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// execute: expired plan -> 410
	router = newBackupRouter(backupKubernetesStub{}, backupRepoStub{claimErr: backup.ErrExpired}, nil)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/backup-plans/p-1/execute", strings.NewReader(`{"confirmation_token":"tok"}`))
	req.Header.Set("Idempotency-Key", "idem-12345678")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusGone || !contains(rec.Body.String(), "BACKUP_EXPIRED") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestBackupListAndLiveCR(t *testing.T) {
	router := newBackupRouter(backupKubernetesStub{}, backupRepoStub{}, nil)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/backup-plans", nil))
	if rec.Code != http.StatusOK || !contains(rec.Body.String(), `"items":[]`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	router = newBackupRouter(backupKubernetesStub{}, backupRepoStub{listErr: errors.New("db down")}, nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/backup-plans", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// live CR list/detail with nil provider -> 503
	router = newBackupRouter(backupKubernetesStub{}, backupRepoStub{}, nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/backups", nil))
	if rec.Code != http.StatusServiceUnavailable || !contains(rec.Body.String(), "VELERO_UNAVAILABLE") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/backups/velero/demo-backup", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// live CR detail with provider configured: empty name -> 400
	router = newBackupRouter(backupKubernetesStub{}, backupRepoStub{}, k8sgateway.NewService(nil, nil, nil))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/backups/velero/%20", nil))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_REQUEST") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestBackupWriteErrorMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := backupHandler{}
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{err: backup.ErrInvalidRequest, status: http.StatusBadRequest, code: "INVALID_BACKUP_REQUEST"},
		{err: backup.ErrVeleroNotInstalled, status: http.StatusServiceUnavailable, code: "VELERO_UNAVAILABLE"},
		{err: backup.ErrStorageLocationNotFound, status: http.StatusBadRequest, code: "STORAGE_LOCATION_NOT_FOUND"},
		{err: backup.ErrStorageLocationUnavailable, status: http.StatusUnprocessableEntity, code: "STORAGE_LOCATION_UNAVAILABLE"},
		{err: backup.ErrSourceNamespaceNotFound, status: http.StatusNotFound, code: "SOURCE_NAMESPACE_NOT_FOUND"},
		{err: backup.ErrStaleSourceNamespace, status: http.StatusConflict, code: "STALE_SOURCE_NAMESPACE"},
		{err: backup.ErrBackupNameConflict, status: http.StatusConflict, code: "BACKUP_NAME_CONFLICT"},
		{err: backup.ErrConfirmationInvalid, status: http.StatusForbidden, code: "BACKUP_CONFIRMATION_INVALID"},
		{err: backup.ErrInvalidIdempotency, status: http.StatusBadRequest, code: "INVALID_IDEMPOTENCY_KEY"},
		{err: backup.ErrExpired, status: http.StatusGone, code: "BACKUP_EXPIRED"},
		{err: backup.ErrInProgress, status: http.StatusConflict, code: "BACKUP_IN_PROGRESS"},
		{err: backup.ErrAlreadyExecuted, status: http.StatusConflict, code: "BACKUP_ALREADY_USED"},
		{err: backup.ErrExecutionFailed, status: http.StatusBadGateway, code: "BACKUP_FAILED"},
		{err: backup.ErrNotFound, status: http.StatusNotFound, code: "BACKUP_PLAN_NOT_FOUND"},
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
