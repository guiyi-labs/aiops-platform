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

// queryScopeRepo is an authz.Repository fake whose ClusterScope is controlled
// directly by the test (scope decisions, not grant CRUD, are under test).
type queryScopeRepo struct {
	scopeByCluster map[int64]authz.ClusterScope
}

func newQueryScopeRepo() *queryScopeRepo {
	return &queryScopeRepo{scopeByCluster: map[int64]authz.ClusterScope{}}
}

func (r *queryScopeRepo) ClusterScope(_ context.Context, _ int64, clusterID int64) (authz.ClusterScope, error) {
	if scope, ok := r.scopeByCluster[clusterID]; ok {
		return scope, nil
	}
	return authz.ClusterScope{ClusterID: clusterID}, nil
}

func (r *queryScopeRepo) CreateClusterGrant(context.Context, int64, int64) (authz.ClusterGrant, error) {
	return authz.ClusterGrant{}, nil
}
func (r *queryScopeRepo) DeleteClusterGrant(context.Context, int64, int64) error { return nil }
func (r *queryScopeRepo) ListClusterGrants(context.Context, int64) ([]authz.ClusterGrant, error) {
	return nil, nil
}
func (r *queryScopeRepo) CreateNamespaceGrant(context.Context, int64, int64, string) (authz.NamespaceGrant, error) {
	return authz.NamespaceGrant{}, nil
}
func (r *queryScopeRepo) DeleteNamespaceGrant(context.Context, int64, int64, string) error {
	return nil
}
func (r *queryScopeRepo) ListNamespaceGrants(context.Context, int64) ([]authz.NamespaceGrant, error) {
	return nil, nil
}
func (r *queryScopeRepo) VisibleClusters(context.Context, int64) ([]int64, error) { return nil, nil }
func (r *queryScopeRepo) HasClusterGrant(context.Context, int64, int64) (bool, error) {
	return false, nil
}

// queryScopeRouter builds a standalone router with requireClusterQueryAccess
// in front of a stub handler that answers 204.
func queryScopeRouter(repo *queryScopeRepo, service *authz.Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	if service == nil {
		service = authz.NewService(repo)
	}
	router := gin.New()
	router.GET("/api/v1/aiops/read", requireClusterQueryAccess(service), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	return router
}

func doQueryScopeRequest(router http.Handler, actorID int64, roles []string, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req = req.WithContext(requestctx.WithMetadata(req.Context(), requestctx.Metadata{RequestID: "t", ActorID: actorID, Roles: roles}))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func TestClusterQueryAccessRequiresGrant(t *testing.T) {
	repo := newQueryScopeRepo()
	repo.scopeByCluster[42] = authz.ClusterScope{ClusterID: 42, AllNamespaces: true}
	router := queryScopeRouter(repo, nil)

	t.Run("no cluster_id passes through", func(t *testing.T) {
		rec := doQueryScopeRequest(router, 7, []string{auth.Viewer}, "/api/v1/aiops/read")
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", rec.Code)
		}
	})
	t.Run("granted cluster passes", func(t *testing.T) {
		rec := doQueryScopeRequest(router, 7, []string{auth.Viewer}, "/api/v1/aiops/read?cluster_id=42")
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d body=%s, want 204", rec.Code, rec.Body.String())
		}
	})
	t.Run("unauthorized cluster is 404", func(t *testing.T) {
		rec := doQueryScopeRequest(router, 7, []string{auth.Viewer}, "/api/v1/aiops/read?cluster_id=43")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d body=%s, want 404", rec.Code, rec.Body.String())
		}
	})
	t.Run("system admin bypasses grants", func(t *testing.T) {
		rec := doQueryScopeRequest(router, 9, []string{auth.SystemAdmin}, "/api/v1/aiops/read?cluster_id=999")
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", rec.Code)
		}
	})
	t.Run("invalid cluster_id shape passes to handler", func(t *testing.T) {
		rec := doQueryScopeRequest(router, 7, []string{auth.Viewer}, "/api/v1/aiops/read?cluster_id=abc")
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204 (shape validation belongs to the handler)", rec.Code)
		}
	})
}

func TestClusterQueryAccessNamespaceEnforcement(t *testing.T) {
	repo := newQueryScopeRepo()
	// User holds only a namespace grant on prod: cluster-level reads denied
	// outside prod, namespace-level reads allowed inside prod.
	repo.scopeByCluster[42] = authz.ClusterScope{ClusterID: 42, NamespaceGrants: []string{"prod"}}
	router := queryScopeRouter(repo, nil)

	t.Run("namespace outside grant is 404", func(t *testing.T) {
		rec := doQueryScopeRequest(router, 7, []string{auth.Viewer}, "/api/v1/aiops/read?cluster_id=42&namespace=blocked")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
	})
	t.Run("granted namespace passes", func(t *testing.T) {
		rec := doQueryScopeRequest(router, 7, []string{auth.Viewer}, "/api/v1/aiops/read?cluster_id=42&namespace=prod")
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", rec.Code)
		}
	})
}

func TestClusterQueryAccessNilServicePassesThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/aiops/read", requireClusterQueryAccess(nil), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	rec := doQueryScopeRequest(router, 7, []string{auth.Viewer}, "/api/v1/aiops/read?cluster_id=42")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (dev mode without authz)", rec.Code)
	}
}
