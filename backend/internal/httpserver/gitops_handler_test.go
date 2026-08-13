package httpserver

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/apiquery"
	"k8s-aiops.local/backend/internal/gitops"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

func contains(body, substr string) bool { return strings.Contains(body, substr) }

type gitopsSourceStub struct {
	cap     k8sgateway.GitOpsCapability
	capErr  error
	list    apiquery.ListResponse[k8sgateway.GitOpsApplication]
	listErr error
	app     k8sgateway.GitOpsApplication
	appErr  error
}

func (s gitopsSourceStub) GitOpsCapability(context.Context, int64) (k8sgateway.GitOpsCapability, error) {
	return s.cap, s.capErr
}
func (s gitopsSourceStub) GitOpsApplications(context.Context, int64, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.GitOpsApplication], error) {
	return s.list, s.listErr
}
func (s gitopsSourceStub) GitOpsApplication(context.Context, int64, string) (k8sgateway.GitOpsApplication, error) {
	return s.app, s.appErr
}

func newGitopsRouter(stub gitopsSourceStub) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := gitopsHandler{service: gitops.NewService(stub)}
	router := gin.New()
	g := router.Group("/api/v1/clusters/:cluster_id/gitops")
	g.GET("/capability", h.capability)
	g.GET("/applications", h.list)
	g.GET("/applications/:name", h.get)
	return router
}

func gitopsApp() k8sgateway.GitOpsApplication {
	return k8sgateway.GitOpsApplication{Name: "web-app", Project: "default", SyncStatus: "Synced", HealthStatus: "Healthy"}
}

func TestGitOpsCapabilityHandler(t *testing.T) {
	router := newGitopsRouter(gitopsSourceStub{cap: k8sgateway.GitOpsCapability{Installed: true, Version: "v2.14"}})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/gitops/capability", nil))
	if rec.Code != http.StatusOK || !contains(rec.Body.String(), `"installed":true`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGitOpsInvalidClusterIDRejected(t *testing.T) {
	router := newGitopsRouter(gitopsSourceStub{})
	for _, path := range []string{"/api/v1/clusters/0/gitops/capability", "/api/v1/clusters/abc/gitops/applications", "/api/v1/clusters//gitops/applications/web-app"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_CLUSTER_ID") {
			t.Fatalf("path=%s status=%d body=%s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestGitOpsListHandler(t *testing.T) {
	app := gitopsApp()
	router := newGitopsRouter(gitopsSourceStub{list: apiquery.ListResponse[k8sgateway.GitOpsApplication]{Items: []k8sgateway.GitOpsApplication{app}, Total: 1, Remaining: 0}})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/gitops/applications?limit=1", nil))
	if rec.Code != http.StatusOK || !contains(rec.Body.String(), "web-app") || !contains(rec.Body.String(), `"total":1`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// ArgoCD not installed: service projects an empty list so the UI renders
	// the "not installed" empty state instead of a 503.
	router = newGitopsRouter(gitopsSourceStub{listErr: k8sgateway.ErrGitOpsUnavailable})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/gitops/applications", nil))
	if rec.Code != http.StatusOK || !contains(rec.Body.String(), `"items":[]`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Unexpected source failure surfaces as 500.
	router = newGitopsRouter(gitopsSourceStub{listErr: errors.New("boom")})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/gitops/applications", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGitOpsGetHandler(t *testing.T) {
	router := newGitopsRouter(gitopsSourceStub{app: gitopsApp()})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/gitops/applications/web-app", nil))
	if rec.Code != http.StatusOK || !contains(rec.Body.String(), "web-app") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Missing name is rejected before hitting the service.
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/gitops/applications/%20%20", nil))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_NAME") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Not-found maps to 404; ArgoCD missing maps to 503.
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{err: gitops.ErrNotFound, status: http.StatusNotFound, code: "GITOPS_APPLICATION_NOT_FOUND"},
		{err: gitops.ErrGitOpsUnavailable, status: http.StatusServiceUnavailable, code: "GITOPS_UNAVAILABLE"},
		{err: gitops.ErrInvalidRequest, status: http.StatusBadRequest, code: "INVALID_GITOPS_REQUEST"},
		{err: errors.New("boom"), status: http.StatusInternalServerError, code: "INTERNAL_ERROR"},
	}
	for _, tt := range cases {
		router := newGitopsRouter(gitopsSourceStub{appErr: tt.err})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/gitops/applications/web-app", nil))
		if rec.Code != tt.status || !contains(rec.Body.String(), tt.code) {
			t.Fatalf("err=%v status=%d body=%s want status=%d code=%s", tt.err, rec.Code, rec.Body.String(), tt.status, tt.code)
		}
	}
}

func TestGitOpsWriteErrorMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := gitopsHandler{}
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{err: gitops.ErrInvalidRequest, status: http.StatusBadRequest, code: "INVALID_GITOPS_REQUEST"},
		{err: gitops.ErrGitOpsUnavailable, status: http.StatusServiceUnavailable, code: "GITOPS_UNAVAILABLE"},
		{err: gitops.ErrNotFound, status: http.StatusNotFound, code: "GITOPS_APPLICATION_NOT_FOUND"},
		{err: errors.New("boom"), status: http.StatusInternalServerError, code: "INTERNAL_ERROR"},
	}
	for _, tt := range cases {
		router := gin.New()
		router.GET("/x", func(c *gin.Context) { h.writeError(c, tt.err, "fallback") })
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
		if rec.Code != tt.status || !contains(rec.Body.String(), tt.code) {
			t.Fatalf("err=%v status=%d body=%s", tt.err, rec.Code, rec.Body.String())
		}
	}
}
