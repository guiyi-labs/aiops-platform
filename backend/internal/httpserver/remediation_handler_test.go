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

	"k8s-aiops.local/backend/internal/cluster"
	"k8s-aiops.local/backend/internal/diagnosis"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
	"k8s-aiops.local/backend/internal/remediation"
	"k8s-aiops.local/backend/internal/requestctx"
)

type remediationDiagnosisStub struct{}

func (s remediationDiagnosisStub) Get(context.Context, int64) (diagnosis.Record, error) {
	return diagnosis.Record{ID: 7}, nil
}

type remediationKubernetesStub struct{}

func (s remediationKubernetesStub) Pod(context.Context, int64, string, string) (k8sgateway.Pod, error) {
	return k8sgateway.Pod{}, nil
}
func (s remediationKubernetesStub) Deployment(context.Context, int64, string, string) (k8sgateway.Deployment, error) {
	return k8sgateway.Deployment{}, nil
}
func (s remediationKubernetesStub) PatchDeployment(context.Context, int64, string, string, []byte, bool) (k8sgateway.Deployment, error) {
	return k8sgateway.Deployment{}, nil
}
func (s remediationKubernetesStub) CronJob(context.Context, int64, string, string) (k8sgateway.CronJob, error) {
	return k8sgateway.CronJob{}, nil
}
func (s remediationKubernetesStub) PatchCronJob(context.Context, int64, string, string, []byte, bool) (k8sgateway.CronJob, error) {
	return k8sgateway.CronJob{}, nil
}
func (s remediationKubernetesStub) ReplicaSet(context.Context, int64, string, string) (k8sgateway.ReplicaSet, error) {
	return k8sgateway.ReplicaSet{}, nil
}
func (s remediationKubernetesStub) ReplicaSetsByOwner(context.Context, int64, string, string) ([]k8sgateway.ReplicaSet, error) {
	return nil, nil
}
func (s remediationKubernetesStub) RolloutHistory(context.Context, int64, string, string) (k8sgateway.RolloutHistory, error) {
	return k8sgateway.RolloutHistory{}, nil
}
func (s remediationKubernetesStub) RolloutStatus(context.Context, int64, string, string) (k8sgateway.RolloutStatus, error) {
	return k8sgateway.RolloutStatus{}, nil
}

type remediationRepoStub struct {
	listErr    error
	claimErr   error
	listOpsErr error
}

func (s remediationRepoStub) Save(context.Context, *remediation.Plan) error { return nil }
func (s remediationRepoStub) List(context.Context, int64) ([]remediation.Plan, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return []remediation.Plan{}, nil
}
func (s remediationRepoStub) ListOperations(context.Context, int64, string, string, string) ([]remediation.Plan, error) {
	if s.listOpsErr != nil {
		return nil, s.listOpsErr
	}
	return []remediation.Plan{}, nil
}
func (s remediationRepoStub) Claim(context.Context, string, []byte, string, time.Time, time.Time) (remediation.Plan, bool, error) {
	return remediation.Plan{}, false, s.claimErr
}
func (s remediationRepoStub) Complete(context.Context, string, string, time.Time) (remediation.Plan, error) {
	return remediation.Plan{}, nil
}
func (s remediationRepoStub) Fail(context.Context, string, string, string) (remediation.Plan, error) {
	return remediation.Plan{}, nil
}

func newRemediationRouter(repo remediationRepoStub) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := remediationHandler{service: remediation.NewService(remediationDiagnosisStub{}, remediationKubernetesStub{}, repo)}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), requestctx.Metadata{
			ActorID: 1, ActorDisplayName: "Admin", Roles: []string{"system_admin"}, ClusterID: 1, RequestID: "remediation-test",
		}))
		c.Next()
	})
	router.POST("/api/v1/diagnoses/:diagnosis_id/remediations/preview", h.preview)
	router.GET("/api/v1/diagnoses/:diagnosis_id/remediations", h.list)
	router.POST("/api/v1/clusters/:cluster_id/operations/preview", h.previewOperation)
	router.GET("/api/v1/clusters/:cluster_id/operations", h.listOperations)
	router.POST("/api/v1/remediations/:remediation_id/execute", h.execute)
	router.GET("/api/v1/clusters/:cluster_id/rollouts/:namespace/:name/history", h.rolloutHistory)
	router.GET("/api/v1/clusters/:cluster_id/rollouts/:namespace/:name/status", h.rolloutStatus)
	return router
}

func TestRemediationValidationAndExecute(t *testing.T) {
	router := newRemediationRouter(remediationRepoStub{})

	// preview: missing body field (binding enforced)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/diagnoses/7/remediations/preview", strings.NewReader(`{"action":"x"}`)))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_REQUEST") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// preview: unsupported action
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/diagnoses/7/remediations/preview", strings.NewReader(`{"action":"explode","target_name":"web"}`)))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "UNSUPPORTED_REMEDIATION") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// list
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/diagnoses/7/remediations", nil))
	if rec.Code != http.StatusOK || !contains(rec.Body.String(), `"items":[]`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// execute: invalid id
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/remediations/short/execute", strings.NewReader(`{"confirmation_token":"tok"}`)))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_REMEDIATION_ID") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// execute: missing token
	const id = "12345678-1234-1234-1234-123456789012"
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/remediations/"+id+"/execute", strings.NewReader(`{}`)))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_REQUEST") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// execute: missing idempotency -> 400
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/remediations/"+id+"/execute", strings.NewReader(`{"confirmation_token":"tok"}`)))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_IDEMPOTENCY_KEY") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// execute: expired -> 410
	router = newRemediationRouter(remediationRepoStub{claimErr: remediation.ErrExpired})
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/remediations/"+id+"/execute", strings.NewReader(`{"confirmation_token":"tok"}`))
	req.Header.Set("Idempotency-Key", "idem-12345678")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusGone || !contains(rec.Body.String(), "REMEDIATION_EXPIRED") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// operations / rollout: invalid kind -> 400
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/operations?target_kind=Bogus", nil))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_OPERATION") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// operations success
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/operations?target_kind=Deployment", nil))
	if rec.Code != http.StatusOK || !contains(rec.Body.String(), `"items":[]`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRemediationWriteErrorMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := remediationHandler{}
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{err: diagnosis.ErrRecordNotFound, status: http.StatusNotFound, code: "DIAGNOSIS_NOT_FOUND"},
		{err: remediation.ErrNotFound, status: http.StatusNotFound, code: "REMEDIATION_NOT_FOUND"},
		{err: remediation.ErrUnsupportedAction, status: http.StatusBadRequest, code: "UNSUPPORTED_REMEDIATION"},
		{err: remediation.ErrInvalidOperation, status: http.StatusBadRequest, code: "INVALID_OPERATION"},
		{err: remediation.ErrOperationNoChange, status: http.StatusConflict, code: "OPERATION_NO_CHANGE"},
		{err: remediation.ErrRevisionNotFound, status: http.StatusNotFound, code: "REVISION_NOT_FOUND"},
		{err: remediation.ErrDiagnosisNotEligible, status: http.StatusConflict, code: "DIAGNOSIS_NOT_ELIGIBLE"},
		{err: remediation.ErrTargetMismatch, status: http.StatusConflict, code: "REMEDIATION_TARGET_MISMATCH"},
		{err: remediation.ErrTargetChanged, status: http.StatusConflict, code: "REMEDIATION_TARGET_CHANGED"},
		{err: remediation.ErrConfirmationInvalid, status: http.StatusForbidden, code: "REMEDIATION_CONFIRMATION_INVALID"},
		{err: remediation.ErrInvalidIdempotency, status: http.StatusBadRequest, code: "INVALID_IDEMPOTENCY_KEY"},
		{err: remediation.ErrExpired, status: http.StatusGone, code: "REMEDIATION_EXPIRED"},
		{err: remediation.ErrInProgress, status: http.StatusConflict, code: "REMEDIATION_IN_PROGRESS"},
		{err: remediation.ErrAlreadyExecuted, status: http.StatusConflict, code: "REMEDIATION_ALREADY_USED"},
		{err: cluster.ErrDisabled, status: http.StatusConflict, code: "CLUSTER_DISABLED"},
		{err: cluster.ErrNotFound, status: http.StatusNotFound, code: "CLUSTER_NOT_FOUND"},
		{err: k8sgateway.ErrResourceNotFound, status: http.StatusNotFound, code: "RESOURCE_NOT_FOUND"},
		{err: remediation.ErrExecutionFailed, status: http.StatusBadGateway, code: "REMEDIATION_FAILED"},
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
