package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/auth"
	"k8s-aiops.local/backend/internal/authz"
	"k8s-aiops.local/backend/internal/requestctx"
)

// stubAuthzRepo is a minimal Repository for httpserver middleware tests. It
// only implements ClusterScope and VisibleClusters, which are the methods used
// by the policy evaluator in the middleware path.
type stubAuthzRepo struct {
	clusterScopes map[int64]authz.ClusterScope // userID -> scope (keyed by clusterID encoded into ClusterID field)
	visible       []int64
	err           error
}

func (r *stubAuthzRepo) CreateClusterGrant(context.Context, int64, int64) (authz.ClusterGrant, error) {
	return authz.ClusterGrant{}, nil
}
func (r *stubAuthzRepo) DeleteClusterGrant(context.Context, int64, int64) error { return nil }
func (r *stubAuthzRepo) ListClusterGrants(context.Context, int64) ([]authz.ClusterGrant, error) {
	return nil, nil
}
func (r *stubAuthzRepo) CreateNamespaceGrant(context.Context, int64, int64, string) (authz.NamespaceGrant, error) {
	return authz.NamespaceGrant{}, nil
}
func (r *stubAuthzRepo) DeleteNamespaceGrant(context.Context, int64, int64, string) error {
	return nil
}
func (r *stubAuthzRepo) ListNamespaceGrants(context.Context, int64) ([]authz.NamespaceGrant, error) {
	return nil, nil
}
func (r *stubAuthzRepo) ClusterScope(_ context.Context, userID, clusterID int64) (authz.ClusterScope, error) {
	if r.err != nil {
		return authz.ClusterScope{}, r.err
	}
	scope, ok := r.clusterScopes[scopeKey(userID, clusterID)]
	if !ok {
		return authz.ClusterScope{ClusterID: clusterID, AllNamespaces: false}, nil
	}
	return scope, nil
}
func (r *stubAuthzRepo) VisibleClusters(context.Context, int64) ([]int64, error) {
	return r.visible, r.err
}
func (r *stubAuthzRepo) HasClusterGrant(context.Context, int64, int64) (bool, error) {
	return false, nil
}

func scopeKey(userID, clusterID int64) int64 {
	return userID*100_000 + clusterID
}

func TestRequireClusterAccessAllowsSystemAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := authz.NewService(&stubAuthzRepo{})
	router := gin.New()
	router.Use(func(c *gin.Context) {
		metadata := requestctx.Metadata{RequestID: "test", ActorID: 1, Roles: []string{auth.SystemAdmin}, ClusterID: 42}
		c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), metadata))
		c.Next()
	})
	router.GET("/clusters/:cluster_id/pods", requireClusterAccess(service), func(c *gin.Context) { c.Status(http.StatusOK) })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/clusters/42/pods", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestRequireClusterAccessAllowsClusterGrant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &stubAuthzRepo{clusterScopes: map[int64]authz.ClusterScope{scopeKey(7, 42): {ClusterID: 42, AllNamespaces: true}}}
	service := authz.NewService(repo)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		metadata := requestctx.Metadata{RequestID: "test", ActorID: 7, Roles: []string{auth.Viewer}, ClusterID: 42}
		c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), metadata))
		c.Next()
	})
	router.GET("/clusters/:cluster_id/pods", requireClusterAccess(service), func(c *gin.Context) { c.Status(http.StatusOK) })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/clusters/42/pods", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestRequireClusterAccessDeniesWithoutGrant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := authz.NewService(&stubAuthzRepo{})
	router := gin.New()
	router.Use(func(c *gin.Context) {
		metadata := requestctx.Metadata{RequestID: "test", ActorID: 7, Roles: []string{auth.Viewer}, ClusterID: 42}
		c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), metadata))
		c.Next()
	})
	router.GET("/clusters/:cluster_id/pods", requireClusterAccess(service), func(c *gin.Context) { c.Status(http.StatusOK) })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/clusters/42/pods", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (404 hides cluster existence)", recorder.Code, http.StatusNotFound)
	}
}

func TestRequireClusterAccessNilServiceAllowsRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		metadata := requestctx.Metadata{RequestID: "test", ActorID: 7, Roles: []string{auth.Viewer}, ClusterID: 42}
		c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), metadata))
		c.Next()
	})
	router.GET("/clusters/:cluster_id/pods", requireClusterAccess(nil), func(c *gin.Context) { c.Status(http.StatusOK) })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/clusters/42/pods", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestRequireNamespaceAccessAllowsClusterGrant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &stubAuthzRepo{clusterScopes: map[int64]authz.ClusterScope{scopeKey(7, 42): {ClusterID: 42, AllNamespaces: true}}}
	service := authz.NewService(repo)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		metadata := requestctx.Metadata{RequestID: "test", ActorID: 7, Roles: []string{auth.Viewer}, ClusterID: 42}
		c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), metadata))
		c.Next()
	})
	router.GET("/clusters/:cluster_id/pods/:namespace/:name", requireNamespaceAccess(service, "namespace"), func(c *gin.Context) { c.Status(http.StatusOK) })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/clusters/42/pods/prod/api", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestRequireNamespaceAccessAllowsExactNamespaceGrant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &stubAuthzRepo{clusterScopes: map[int64]authz.ClusterScope{scopeKey(7, 42): {ClusterID: 42, NamespaceGrants: []string{"prod"}}}}
	service := authz.NewService(repo)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		metadata := requestctx.Metadata{RequestID: "test", ActorID: 7, Roles: []string{auth.Viewer}, ClusterID: 42}
		c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), metadata))
		c.Next()
	})
	router.GET("/clusters/:cluster_id/pods/:namespace/:name", requireNamespaceAccess(service, "namespace"), func(c *gin.Context) { c.Status(http.StatusOK) })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/clusters/42/pods/prod/api", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestRequireNamespaceAccessDeniesUnauthorizedNamespace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &stubAuthzRepo{clusterScopes: map[int64]authz.ClusterScope{scopeKey(7, 42): {ClusterID: 42, NamespaceGrants: []string{"prod"}}}}
	service := authz.NewService(repo)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		metadata := requestctx.Metadata{RequestID: "test", ActorID: 7, Roles: []string{auth.Viewer}, ClusterID: 42}
		c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), metadata))
		c.Next()
	})
	router.GET("/clusters/:cluster_id/pods/:namespace/:name", requireNamespaceAccess(service, "namespace"), func(c *gin.Context) { c.Status(http.StatusOK) })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/clusters/42/pods/staging/api", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (404 hides namespace existence)", recorder.Code, http.StatusNotFound)
	}
}

func TestRequireNamespaceAccessSkipsWhenNamespaceParamAbsent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := authz.NewService(&stubAuthzRepo{})
	router := gin.New()
	router.Use(func(c *gin.Context) {
		metadata := requestctx.Metadata{RequestID: "test", ActorID: 7, Roles: []string{auth.Viewer}, ClusterID: 42}
		c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), metadata))
		c.Next()
	})
	router.GET("/clusters/:cluster_id/pods", requireNamespaceAccess(service, "namespace"), func(c *gin.Context) { c.Status(http.StatusOK) })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/clusters/42/pods", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestAuthorizedClusterFilterSystemAdminReturnsNil(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := authz.NewService(&stubAuthzRepo{})
	router := gin.New()
	router.Use(func(c *gin.Context) {
		metadata := requestctx.Metadata{RequestID: "test", ActorID: 1, Roles: []string{auth.SystemAdmin}}
		c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), metadata))
		c.Next()
	})
	router.GET("/fleet/health", func(c *gin.Context) {
		visible, all, err := authorizedClusterFilter(service, c)
		if err != nil || !all || visible != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusOK)
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/fleet/health", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestAuthorizedClusterFilterViewerReturnsVisibleSet(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &stubAuthzRepo{visible: []int64{1, 3, 5}}
	service := authz.NewService(repo)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		metadata := requestctx.Metadata{RequestID: "test", ActorID: 7, Roles: []string{auth.Viewer}}
		c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), metadata))
		c.Next()
	})
	router.GET("/fleet/health", func(c *gin.Context) {
		visible, all, err := authorizedClusterFilter(service, c)
		if err != nil || all || len(visible) != 3 {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusOK)
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/fleet/health", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestAuthorizedClusterFilterNilServiceReturnsAll(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		metadata := requestctx.Metadata{RequestID: "test", ActorID: 7, Roles: []string{auth.Viewer}}
		c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), metadata))
		c.Next()
	})
	router.GET("/fleet/health", func(c *gin.Context) {
		visible, all, err := authorizedClusterFilter(nil, c)
		if err != nil || !all || visible != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusOK)
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/fleet/health", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}
