package httpserver

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/authz"
	"k8s-aiops.local/backend/internal/requestctx"
	"k8s-aiops.local/backend/internal/workspace"
)

// ============================================================================
// narrowScopeByWorkspace (pure function)
// ============================================================================

// TestNarrowScopeByWorkspace_AllNamespacesNarrowsToWorkspace verifies that a
// SystemAdmin / cluster-grant scope (AllNamespaces=true) is narrowed to the
// workspace's member namespaces when a workspace_id filter is applied.
func TestNarrowScopeByWorkspace_AllNamespacesNarrowsToWorkspace(t *testing.T) {
	scope := authz.ClusterScope{AllNamespaces: true}
	wsNamespaces := []string{"prod", "staging"}

	out := narrowScopeByWorkspace(scope, wsNamespaces)

	if out.AllNamespaces {
		t.Fatal("AllNamespaces should be false after narrowing")
	}
	if len(out.NamespaceGrants) != 2 {
		t.Fatalf("NamespaceGrants len = %d, want 2; out=%v", len(out.NamespaceGrants), out.NamespaceGrants)
	}
	got := make(map[string]bool, len(out.NamespaceGrants))
	for _, ns := range out.NamespaceGrants {
		got[ns] = true
	}
	if !got["prod"] || !got["staging"] {
		t.Fatalf("out = %v, want {prod, staging}", out.NamespaceGrants)
	}
}

// TestNarrowScopeByWorkspace_NamespaceGrantsIntersected verifies that a
// namespace-grant user's scope is intersected with the workspace's
// namespaces: namespaces the user cannot see are dropped, and namespaces
// outside the workspace are dropped.
func TestNarrowScopeByWorkspace_NamespaceGrantsIntersected(t *testing.T) {
	// User can see: prod, dev, sandbox. Workspace owns: prod, staging.
	// Intersection must be: prod only.
	scope := authz.ClusterScope{
		AllNamespaces:   false,
		NamespaceGrants: []string{"prod", "dev", "sandbox"},
	}
	wsNamespaces := []string{"prod", "staging"}

	out := narrowScopeByWorkspace(scope, wsNamespaces)

	if out.AllNamespaces {
		t.Fatal("AllNamespaces should be false")
	}
	if len(out.NamespaceGrants) != 1 || out.NamespaceGrants[0] != "prod" {
		t.Fatalf("out = %v, want [prod] (intersection)", out.NamespaceGrants)
	}
}

// TestNarrowScopeByWorkspace_EmptyWorkspaceReturnsEmptyScope verifies
// anti-leakage: when the workspace has no memberships on the cluster (or does
// not exist), the scope becomes empty so list handlers render an empty
// collection rather than leaking the workspace's existence via 404.
func TestNarrowScopeByWorkspace_EmptyWorkspaceReturnsEmptyScope(t *testing.T) {
	// Even an AllNamespaces scope is collapsed to empty.
	scope := authz.ClusterScope{AllNamespaces: true}
	out := narrowScopeByWorkspace(scope, nil)
	if out.AllNamespaces {
		t.Fatal("AllNamespaces should be false (anti-leakage collapse)")
	}
	if len(out.NamespaceGrants) != 0 {
		t.Fatalf("NamespaceGrants = %v, want empty", out.NamespaceGrants)
	}

	// Same for an explicit empty slice.
	scope2 := authz.ClusterScope{AllNamespaces: false, NamespaceGrants: []string{"prod"}}
	out2 := narrowScopeByWorkspace(scope2, []string{})
	if out2.AllNamespaces || len(out2.NamespaceGrants) != 0 {
		t.Fatalf("out2 = %+v, want empty scope (anti-leakage)", out2)
	}
}

// TestNarrowScopeByWorkspace_NoOverlapYieldsEmpty verifies that when the
// user's namespace grants and the workspace's namespaces are disjoint, the
// result is an empty (not nil) scope.
func TestNarrowScopeByWorkspace_NoOverlapYieldsEmpty(t *testing.T) {
	scope := authz.ClusterScope{AllNamespaces: false, NamespaceGrants: []string{"dev"}}
	wsNamespaces := []string{"prod", "staging"}

	out := narrowScopeByWorkspace(scope, wsNamespaces)

	if out.AllNamespaces {
		t.Fatal("AllNamespaces should be false")
	}
	if len(out.NamespaceGrants) != 0 {
		t.Fatalf("NamespaceGrants = %v, want empty (no overlap)", out.NamespaceGrants)
	}
}

// ============================================================================
// withWorkspaceNamespaceFilter (middleware)
// ============================================================================

// filterTestRepo is a minimal workspace.Repository whose
// ListMembershipsByCluster returns a fixed slice (or error). The other
// methods are unused by the filter middleware path.
type filterTestRepo struct {
	memberships []workspace.WorkspaceMembership
	err         error
}

func (r *filterTestRepo) CreateWorkspace(context.Context, workspace.Workspace) (workspace.Workspace, error) {
	return workspace.Workspace{}, nil
}
func (r *filterTestRepo) GetWorkspace(context.Context, int64) (workspace.Workspace, error) {
	return workspace.Workspace{}, nil
}
func (r *filterTestRepo) GetWorkspaceByName(context.Context, string) (workspace.Workspace, error) {
	return workspace.Workspace{}, nil
}
func (r *filterTestRepo) ListWorkspaces(context.Context, int64) ([]workspace.Workspace, error) {
	return nil, nil
}
func (r *filterTestRepo) UpdateWorkspace(context.Context, workspace.Workspace) (workspace.Workspace, error) {
	return workspace.Workspace{}, nil
}
func (r *filterTestRepo) DeleteWorkspace(context.Context, int64) error { return nil }
func (r *filterTestRepo) AddMembership(context.Context, workspace.WorkspaceMembership) (workspace.WorkspaceMembership, error) {
	return workspace.WorkspaceMembership{}, nil
}
func (r *filterTestRepo) ListMemberships(context.Context, int64) ([]workspace.WorkspaceMembership, error) {
	return nil, nil
}
func (r *filterTestRepo) ListMembershipsByCluster(_ context.Context, clusterID int64) ([]workspace.WorkspaceMembership, error) {
	if r.err != nil {
		return nil, r.err
	}
	out := make([]workspace.WorkspaceMembership, 0)
	for _, m := range r.memberships {
		if m.ClusterID == clusterID {
			out = append(out, m)
		}
	}
	return out, nil
}
func (r *filterTestRepo) RemoveMembership(context.Context, int64, int64, string) error { return nil }
func (r *filterTestRepo) GetQuota(context.Context, int64) (workspace.WorkspaceQuota, error) {
	return workspace.WorkspaceQuota{}, nil
}
func (r *filterTestRepo) UpsertQuota(context.Context, workspace.WorkspaceQuota) (workspace.WorkspaceQuota, error) {
	return workspace.WorkspaceQuota{}, nil
}
func (r *filterTestRepo) CreateGrant(context.Context, workspace.UserWorkspaceGrant) (workspace.UserWorkspaceGrant, error) {
	return workspace.UserWorkspaceGrant{}, nil
}
func (r *filterTestRepo) GetGrant(context.Context, int64, int64) (workspace.UserWorkspaceGrant, error) {
	return workspace.UserWorkspaceGrant{}, nil
}
func (r *filterTestRepo) ListGrants(context.Context, int64) ([]workspace.UserWorkspaceGrant, error) {
	return nil, nil
}
func (r *filterTestRepo) UpdateGrantRole(context.Context, int64, int64, string) (workspace.UserWorkspaceGrant, error) {
	return workspace.UserWorkspaceGrant{}, nil
}
func (r *filterTestRepo) DeleteGrant(context.Context, int64, int64) error { return nil }
func (r *filterTestRepo) ListUserWorkspaces(context.Context, int64) ([]int64, error) {
	return nil, nil
}
func (r *filterTestRepo) AppendAudit(context.Context, workspace.WorkspaceRoleBindingAudit) error {
	return nil
}
func (r *filterTestRepo) ListAudit(context.Context, int64, int) ([]workspace.WorkspaceRoleBindingAudit, error) {
	return nil, nil
}

// newFilterTestEngine builds a gin engine with a chain that mirrors the
// production order: a pre-middleware sets the authz scope (simulating
// requireNamespaceQueryAccess), then withWorkspaceNamespaceFilter runs, and a
// terminal handler echoes the resolved scope back as JSON for assertions.
func newFilterTestEngine(t *testing.T, svc *workspace.Service, initialScope authz.ClusterScope, clusterID int64) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		// Simulate requireNamespaceQueryAccess setting the scope.
		c.Set(namespaceScopeKey, initialScope)
		// Inject clusterID into request metadata so the filter can read it.
		md := requestctx.Metadata{ClusterID: clusterID, RequestID: "filter-test"}
		c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), md))
		c.Next()
	})
	r.GET("/pods", withWorkspaceNamespaceFilter(svc), func(c *gin.Context) {
		scope := ResolvedNamespaceScope(c)
		c.JSON(http.StatusOK, gin.H{
			"all_namespaces":   scope.AllNamespaces,
			"namespace_grants": scope.NamespaceGrants,
		})
	})
	return r
}

// TestWithWorkspaceNamespaceFilter_NoWorkspaceIDParamPassThrough verifies
// that when workspace_id is absent, the scope is untouched (the filter is a
// no-op).
func TestWithWorkspaceNamespaceFilter_NoWorkspaceIDParamPassThrough(t *testing.T) {
	repo := &filterTestRepo{memberships: []workspace.WorkspaceMembership{
		{WorkspaceID: 1, ClusterID: 5, Namespace: "prod"},
	}}
	svc := workspace.NewService(repo)
	initial := authz.ClusterScope{AllNamespaces: true}
	engine := newFilterTestEngine(t, svc, initial, 5)

	req := httptest.NewRequest(http.MethodGet, "/pods", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	// Scope should be unchanged.
	body := w.Body.String()
	if !containsStr(body, `"all_namespaces":true`) {
		t.Fatalf("body = %s, want all_namespaces=true (untouched)", body)
	}
}

// TestWithWorkspaceNamespaceFilter_InvalidWorkspaceIDReturns400 verifies that
// a non-numeric or non-positive workspace_id is rejected with 400.
func TestWithWorkspaceNamespaceFilter_InvalidWorkspaceIDReturns400(t *testing.T) {
	svc := workspace.NewService(&filterTestRepo{})
	initial := authz.ClusterScope{AllNamespaces: true}
	engine := newFilterTestEngine(t, svc, initial, 5)

	cases := []string{"workspace_id=abc", "workspace_id=0", "workspace_id=-3", "workspace_id=1.5"}
	for _, q := range cases {
		t.Run(q, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/pods?"+q, nil)
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 for %q; body=%s", w.Code, q, w.Body.String())
			}
		})
	}
}

// TestWithWorkspaceNamespaceFilter_NarrowsAllNamespacesScope verifies the
// middleware narrows an AllNamespaces scope to the workspace's namespaces.
func TestWithWorkspaceNamespaceFilter_NarrowsAllNamespacesScope(t *testing.T) {
	repo := &filterTestRepo{memberships: []workspace.WorkspaceMembership{
		{WorkspaceID: 1, ClusterID: 5, Namespace: "prod"},
		{WorkspaceID: 1, ClusterID: 5, Namespace: "staging"},
		// Different workspace on same cluster — must be excluded.
		{WorkspaceID: 2, ClusterID: 5, Namespace: "dev"},
		// Same workspace on different cluster — must be excluded.
		{WorkspaceID: 1, ClusterID: 6, Namespace: "other"},
	}}
	svc := workspace.NewService(repo)
	initial := authz.ClusterScope{AllNamespaces: true}
	engine := newFilterTestEngine(t, svc, initial, 5)

	req := httptest.NewRequest(http.MethodGet, "/pods?workspace_id=1", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if containsStr(body, `"all_namespaces":true`) {
		t.Fatalf("body = %s, all_namespaces should be false after narrowing", body)
	}
	if !containsStr(body, `"prod"`) || !containsStr(body, `"staging"`) {
		t.Fatalf("body = %s, want prod and staging in namespace_grants", body)
	}
	if containsStr(body, `"dev"`) {
		t.Fatalf("body = %s, dev (ws2) must not appear", body)
	}
	if containsStr(body, `"other"`) {
		t.Fatalf("body = %s, other (cluster 6) must not appear", body)
	}
}

// TestWithWorkspaceNamespaceFilter_NarrowsNamespaceGrantsScope verifies the
// middleware intersects a namespace-grant scope with the workspace's
// namespaces.
func TestWithWorkspaceNamespaceFilter_NarrowsNamespaceGrantsScope(t *testing.T) {
	repo := &filterTestRepo{memberships: []workspace.WorkspaceMembership{
		{WorkspaceID: 1, ClusterID: 5, Namespace: "prod"},
		{WorkspaceID: 1, ClusterID: 5, Namespace: "staging"},
	}}
	svc := workspace.NewService(repo)
	// User can see prod and sandbox; workspace owns prod and staging.
	// Intersection must be prod only.
	initial := authz.ClusterScope{AllNamespaces: false, NamespaceGrants: []string{"prod", "sandbox"}}
	engine := newFilterTestEngine(t, svc, initial, 5)

	req := httptest.NewRequest(http.MethodGet, "/pods?workspace_id=1", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !containsStr(body, `"prod"`) {
		t.Fatalf("body = %s, want prod in intersection", body)
	}
	if containsStr(body, `"sandbox"`) {
		t.Fatalf("body = %s, sandbox (outside workspace) must be dropped", body)
	}
	if containsStr(body, `"staging"`) {
		t.Fatalf("body = %s, staging (outside user grants) must be dropped", body)
	}
}

// TestWithWorkspaceNamespaceFilter_EmptyWorkspaceReturnsEmptyScope verifies
// anti-leakage: when the workspace has no memberships on the cluster (or does
// not exist), the middleware sets an empty scope so the list handler returns
// 200 with items:[] rather than 404.
func TestWithWorkspaceNamespaceFilter_EmptyWorkspaceReturnsEmptyScope(t *testing.T) {
	repo := &filterTestRepo{memberships: nil} // workspace 999 has no memberships
	svc := workspace.NewService(repo)
	initial := authz.ClusterScope{AllNamespaces: true}
	engine := newFilterTestEngine(t, svc, initial, 5)

	req := httptest.NewRequest(http.MethodGet, "/pods?workspace_id=999", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (anti-leakage: empty list, not 404); body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if containsStr(body, `"all_namespaces":true`) {
		t.Fatalf("body = %s, all_namespaces must be false (collapsed)", body)
	}
	if !containsStr(body, `"namespace_grants":[]`) && !containsStr(body, `"namespace_grants":null`) {
		t.Fatalf("body = %s, want empty namespace_grants", body)
	}
}

// TestWithWorkspaceNamespaceFilter_RepositoryErrorReturns500 verifies that a
// repository failure surfaces as 500 rather than being swallowed into an
// empty scope (which would mask a real outage).
func TestWithWorkspaceNamespaceFilter_RepositoryErrorReturns500(t *testing.T) {
	repo := &filterTestRepo{err: errors.New("db unavailable")}
	svc := workspace.NewService(repo)
	initial := authz.ClusterScope{AllNamespaces: true}
	engine := newFilterTestEngine(t, svc, initial, 5)

	req := httptest.NewRequest(http.MethodGet, "/pods?workspace_id=1", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", w.Code, w.Body.String())
	}
}

// TestWithWorkspaceNamespaceFilter_NilServicePassThrough verifies that when
// the workspace service is nil (workspaces disabled), the middleware is a
// no-op and the scope is untouched.
func TestWithWorkspaceNamespaceFilter_NilServicePassThrough(t *testing.T) {
	initial := authz.ClusterScope{AllNamespaces: true}
	engine := newFilterTestEngine(t, nil, initial, 5)

	req := httptest.NewRequest(http.MethodGet, "/pods?workspace_id=1", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (nil service = no-op); body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !containsStr(body, `"all_namespaces":true`) {
		t.Fatalf("body = %s, want all_namespaces=true (untouched when service is nil)", body)
	}
}

// containsStr is a substring helper scoped to this test file.
func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || strIndexOf(s, substr) >= 0)
}

func strIndexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
