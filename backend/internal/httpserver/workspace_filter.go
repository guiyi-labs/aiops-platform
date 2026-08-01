package httpserver

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/authz"
	"k8s-aiops.local/backend/internal/requestctx"
	"k8s-aiops.local/backend/internal/workspace"
)

// withWorkspaceNamespaceFilter narrows the authz namespace scope stored in
// the Gin context by the optional ?workspace_id query parameter.
//
// It MUST run after requireNamespaceQueryAccess (which sets the initial
// scope). When workspace_id is absent or zero, the scope is untouched. When
// present, the scope is intersected with the workspace's member namespaces
// on the current cluster:
//
//   - SystemAdmin / cluster-grant (AllNamespaces=true): the scope is
//     narrowed to the workspace's namespaces on this cluster.
//   - Namespace-grant user: the scope is intersected with the workspace's
//     namespaces; namespaces the user cannot see are dropped.
//   - Workspace does not exist or has no memberships on this cluster: the
//     scope becomes empty, so downstream list handlers return an empty
//     collection (200 with items:[]) — workspace existence is not leaked
//     (ADR 0062 §4).
//
// This middleware deliberately does NOT enforce workspace_viewer
// authorization. The workspace_id query parameter is a pure visibility
// narrowing filter, not an authorization decision; the caller has already
// passed requireClusterAccess + requireNamespaceQueryAccess for the cluster
// and namespace dimensions.
func withWorkspaceNamespaceFilter(svc *workspace.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil {
			c.Next()
			return
		}
		raw := c.Query("workspace_id")
		if raw == "" {
			c.Next()
			return
		}
		workspaceID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || workspaceID <= 0 {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid_query", "message": "workspace_id must be a positive integer"})
			return
		}
		metadata, _ := requestctx.MetadataFrom(c.Request.Context())
		wsNamespaces, err := svc.NamespacesForWorkspaceFilter(c.Request.Context(), metadata.ClusterID, workspaceID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
			return
		}
		scope := ResolvedNamespaceScope(c)
		narrowed := narrowScopeByWorkspace(scope, wsNamespaces)
		c.Set(namespaceScopeKey, narrowed)
		c.Next()
	}
}

// narrowScopeByWorkspace applies the workspace membership filter to an
// existing authz.ClusterScope. It is a pure function so it can be unit-tested
// without a gin context.
func narrowScopeByWorkspace(scope authz.ClusterScope, wsNamespaces []string) authz.ClusterScope {
	if len(wsNamespaces) == 0 {
		// Workspace has no memberships on this cluster (or does not exist).
		// Return an empty scope so list handlers produce an empty collection
		// without leaking the workspace's existence.
		return authz.ClusterScope{AllNamespaces: false, NamespaceGrants: nil}
	}
	wsSet := make(map[string]struct{}, len(wsNamespaces))
	for _, ns := range wsNamespaces {
		wsSet[ns] = struct{}{}
	}
	if scope.AllNamespaces {
		// SystemAdmin or cluster-grant: narrow to the workspace's namespaces.
		return authz.ClusterScope{AllNamespaces: false, NamespaceGrants: wsNamespaces}
	}
	// Namespace-grant user: intersect with the workspace's namespaces.
	narrowed := make([]string, 0, len(scope.NamespaceGrants))
	for _, ns := range scope.NamespaceGrants {
		if _, ok := wsSet[ns]; ok {
			narrowed = append(narrowed, ns)
		}
	}
	return authz.ClusterScope{AllNamespaces: false, NamespaceGrants: narrowed}
}
