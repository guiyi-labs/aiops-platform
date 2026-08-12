package httpserver

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/authz"
	"k8s-aiops.local/backend/internal/requestctx"
)

// namespaceScopeKey is the key for the resolved authz.ClusterScope stored in
// the Gin context by requireNamespaceQueryAccess.
const namespaceScopeKey = "authz_namespace_scope"

// requireClusterAccess returns a middleware that checks whether the authenticated
// user may access the cluster in the request path. It must run after
// withClusterContext (which sets metadata.ClusterID) and after withAuthentication.
//
// On denied access the middleware returns 404, not 403, so that the existence of
// an unauthorized cluster cannot be distinguished from a genuinely missing one.
// This satisfies the M35 acceptance standard: "An unauthorized target is absent
// from lists/fan-out and cannot be distinguished through direct IDs or error
// details."
func requireClusterAccess(service *authz.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if service == nil {
			c.Next()
			return
		}
		metadata, _ := requestctx.MetadataFrom(c.Request.Context())
		decision, err := service.CanAccessCluster(c.Request.Context(), metadata.ActorID, metadata.Roles, metadata.ClusterID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
			return
		}
		if !decision.Allowed {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "resource_not_found"})
			return
		}
		c.Next()
	}
}

// requireNamespaceAccess returns a middleware that checks whether the
// authenticated user may access the namespace path parameter. It must run after
// withClusterContext. The namespaceParam is the Gin route parameter name
// (typically "namespace").
func requireNamespaceAccess(service *authz.Service, namespaceParam string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if service == nil {
			c.Next()
			return
		}
		namespace := c.Param(namespaceParam)
		if namespace == "" {
			c.Next()
			return
		}
		metadata, _ := requestctx.MetadataFrom(c.Request.Context())
		decision, err := service.CanAccessNamespace(c.Request.Context(), metadata.ActorID, metadata.Roles, metadata.ClusterID, namespace)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
			return
		}
		if !decision.Allowed {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "resource_not_found"})
			return
		}
		c.Next()
	}
}

// requireNamespaceQueryAccess validates the `namespace` query parameter for
// list-style routes and stores the resolved authz.ClusterScope in the Gin
// context under `namespaceScopeKey`. Handlers can read the scope via
// ResolvedNamespaceScope(c).
//
// Semantics:
//   - If service is nil: pass-through (development disabled authz).
//   - If ?namespace= is specific: validate the user has access to that exact
//     namespace; deny with 404 if unauthorized.
//   - If ?namespace= is empty or missing: resolve the caller's full scope for
//     the cluster. If they have no cluster access (neither AllNamespaces nor
//     any NamespaceGrants) → pass the empty scope through. Callers should
//     render an empty collection rather than 404 to avoid disclosing hidden
//     grants via error-differentiation.
//
// This is the query-param sibling of requireNamespaceAccess which only handles
// path parameters.
func requireNamespaceQueryAccess(service *authz.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if service == nil {
			c.Next()
			return
		}
		metadata, _ := requestctx.MetadataFrom(c.Request.Context())
		requestedNamespace := c.Query("namespace")
		scope, err := service.AuthorizedNamespaces(c.Request.Context(), metadata.ActorID, metadata.Roles, metadata.ClusterID, requestedNamespace)
		if err != nil {
			if err == authz.ErrAccessDenied {
				c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "resource_not_found"})
				return
			}
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
			return
		}
		c.Set(namespaceScopeKey, scope)
		c.Next()
	}
}

// ResolvedNamespaceScope returns the namespace scope resolved by
// requireNamespaceQueryAccess. If no scope was attached (e.g. middleware is
// skipped), it returns an all-namespaces scope so existing handlers without
// authz continue to behave consistently.
func ResolvedNamespaceScope(c *gin.Context) authz.ClusterScope {
	if v, ok := c.Get(namespaceScopeKey); ok {
		if s, ok := v.(authz.ClusterScope); ok {
			return s
		}
	}
	return authz.ClusterScope{AllNamespaces: true}
}

// requireClusterQueryAccess validates the `cluster_id` query parameter on
// routes that are not nested under /clusters/:cluster_id (e.g. the /aiops
// group). When cluster_id is present the caller must hold a grant for that
// cluster; when a namespace query parameter is also present, the caller must
// additionally hold a namespace grant for it. Denials return 404, not 403, so
// an unauthorized target cannot be distinguished from a missing one (M35
// anti-leakage). When cluster_id is absent the request passes through: the
// route's handler decides whether the query is valid (M100 follow-up keeps
// grant-scoped filtering for unscoped reads layered in the service).
func requireClusterQueryAccess(service *authz.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if service == nil {
			c.Next()
			return
		}
		raw := c.Query("cluster_id")
		if raw == "" {
			c.Next()
			return
		}
		clusterID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || clusterID <= 0 {
			// Shape validation is the handler's job (400); scope checks only
			// apply to well-formed cluster references.
			c.Next()
			return
		}
		metadata, _ := requestctx.MetadataFrom(c.Request.Context())
		decision, err := service.CanAccessCluster(c.Request.Context(), metadata.ActorID, metadata.Roles, clusterID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
			return
		}
		if !decision.Allowed {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "resource_not_found"})
			return
		}
		if namespace := c.Query("namespace"); namespace != "" {
			nsDecision, err := service.CanAccessNamespace(c.Request.Context(), metadata.ActorID, metadata.Roles, clusterID, namespace)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
				return
			}
			if !nsDecision.Allowed {
				c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "resource_not_found"})
				return
			}
		}
		c.Next()
	}
}

// authorizedClusterFilter returns the visible cluster IDs for the authenticated
// user, or nil if the user is SystemAdmin (meaning all enabled clusters). Used
// by fleet fan-out and global search to scope their cluster enumeration.
func authorizedClusterFilter(service *authz.Service, c *gin.Context) ([]int64, bool, error) {
	if service == nil {
		return nil, true, nil
	}
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	visible, err := service.VisibleClusters(c.Request.Context(), metadata.ActorID, metadata.Roles)
	if err != nil {
		return nil, false, err
	}
	return visible, visible == nil, nil
}
