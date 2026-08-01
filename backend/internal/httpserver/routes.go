package httpserver

import (
	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/auth"
)

// RouteDescriptor is the single source of truth for a route's metadata. The
// same descriptor drives route registration, role checks and audit
// classification.
type RouteDescriptor struct {
	Method        string
	Path          string // path relative to the registering group
	Handler       gin.HandlerFunc
	AuthRequired  bool     // true = add withAuthentication middleware (for routes not on an authed group)
	RequiredRoles []string // nil/empty = any authenticated user
	AuditAction   string   // empty = not audited
	AuditResource string   // fallback resource type for audit
}

// registeredRoute is the internal record after a descriptor is registered
// against a group. FullPath is the absolute path template used for audit
// lookup and OpenAPI parity.
type registeredRoute struct {
	Method        string
	FullPath      string
	RequiredRoles []string
	AuditAction   string
	AuditResource string
}

// routeTable holds every registered route's metadata. Populated by
// registerRoute, consumed by findAuditedRoute and route parity tests.
var routeTable []registeredRoute

// routeRegistrar binds the auth middleware to a closure so that descriptors
// can declare AuthRequired without the caller repeating the auth service.
type routeRegistrar struct {
	authMiddleware gin.HandlerFunc
}

func newRouteRegistrar(authService *auth.Service) routeRegistrar {
	if authService != nil {
		return routeRegistrar{authMiddleware: withAuthentication(authService)}
	}
	return routeRegistrar{}
}

// register registers a descriptor against a gin RouterGroup. The group may
// already have authentication middleware applied (group-level); in that case
// the descriptor's AuthRequired should be false. For routes on groups
// without group-level auth, set AuthRequired=true and the registrar adds the
// authentication middleware before the handler.
func (r routeRegistrar) register(group *gin.RouterGroup, desc RouteDescriptor) {
	handlers := make([]gin.HandlerFunc, 0, 3)
	if desc.AuthRequired && r.authMiddleware != nil {
		handlers = append(handlers, r.authMiddleware)
	}
	if len(desc.RequiredRoles) > 0 {
		handlers = append(handlers, requireRoles(desc.RequiredRoles...))
	}
	handlers = append(handlers, desc.Handler)
	group.Handle(desc.Method, desc.Path, handlers...)

	fullPath := normalizeFullPath(group.BasePath(), desc.Path)
	routeTable = append(routeTable, registeredRoute{
		Method:        desc.Method,
		FullPath:      fullPath,
		RequiredRoles: append([]string(nil), desc.RequiredRoles...),
		AuditAction:   desc.AuditAction,
		AuditResource: desc.AuditResource,
	})
}

// normalizeFullPath joins a group base path and a relative path into the
// absolute path template used by Gin's c.FullPath().
func normalizeFullPath(base, relative string) string {
	if relative == "" {
		return base
	}
	if base == "" || base == "/" {
		return relative
	}
	return base + relative
}

// resetRouteTable clears the global route table. Used by tests and New().
func resetRouteTable() {
	routeTable = nil
}

// findAuditedRoute looks up the audit metadata for a given method and full
// path template. Returns (action, resource, true) if the route is audited.
func findAuditedRoute(method, fullPath string) (string, string, bool) {
	for _, r := range routeTable {
		if r.Method == method && r.FullPath == fullPath && r.AuditAction != "" {
			return r.AuditAction, r.AuditResource, true
		}
	}
	return "", "", false
}

// Convenience role slices to avoid repeating literal slices.
var (
	rolesSystemAdmin         = []string{auth.SystemAdmin}
	rolesSystemOpsAdmin      = []string{auth.SystemAdmin, auth.OperationsAdmin}
	rolesSystemSecurityAudit = []string{auth.SystemAdmin, auth.SecurityAuditor}
)
