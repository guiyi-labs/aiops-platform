package httpserver

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/capability"
	"k8s-aiops.local/backend/internal/monitoring"
	"k8s-aiops.local/backend/internal/requestctx"
)

// monitoringHandler exposes the M50 monitoring dashboard and log explorer
// endpoints.
//
// Routes:
//
//	GET  /clusters/:cluster_id/monitoring/dashboard/:template — single-cluster fixed dashboard
//	GET  /workspaces/:workspace_id/monitoring/dashboard       — workspace cross-cluster dashboard
//	POST /clusters/:cluster_id/logs/query                     — bounded Loki log query
//
// Authorization:
//   - Cluster dashboard + logs/query are registered under resourceRoutes,
//     which applies requireClusterAccess + requireNamespaceQueryAccess (M35
//     scope). The logs/query handler additionally re-checks the body namespace
//     against the resolved scope because the namespace arrives in the JSON
//     body, not as a query parameter (anti-leakage 404, ADR 0065 §4).
//   - Workspace dashboard is registered under workspaceRoutes; the monitoring
//     service enforces workspace_viewer via workspace.Service.ListMemberships.
type monitoringHandler struct {
	service     *monitoring.Service
	logProvider capability.LogProvider
}

// clusterDashboard handles GET /api/v1/clusters/:cluster_id/monitoring/dashboard/:template.
//
// Query params: from, to (RFC3339), namespace (optional). The handler returns
// the fixed template panels for the cluster; it does NOT pre-fetch time series
// — the frontend uses the panel descriptors to drive /metrics/history calls
// (ADR 0065 §1).
func (h monitoringHandler) clusterDashboard(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "MONITORING_UNAVAILABLE", "monitoring service is not configured")
		return
	}
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	template := c.Param("template")
	from, ok := parseMonitoringTime(c, "from")
	if !ok {
		return
	}
	to, ok := parseMonitoringTime(c, "to")
	if !ok {
		return
	}
	resp, err := h.service.ClusterDashboard(c.Request.Context(), monitoring.ClusterDashboardRequest{
		ClusterID: metadata.ClusterID,
		Template:  template,
		Namespace: c.Query("namespace"),
		From:      from,
		To:        to,
	})
	if err != nil {
		writeMonitoringError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

// workspaceDashboard handles GET /api/v1/workspaces/:workspace_id/monitoring/dashboard.
//
// Query params: from, to (RFC3339). The handler returns the fixed
// workspace_overview template plus the workspace's cross-cluster (cluster,
// namespaces) topology; the frontend fans out per-cluster /metrics/history
// calls using the topology (ADR 0065 §2).
func (h monitoringHandler) workspaceDashboard(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "MONITORING_UNAVAILABLE", "monitoring service is not configured")
		return
	}
	workspaceID, ok := parseWorkspaceID(c)
	if !ok {
		return
	}
	from, ok := parseMonitoringTime(c, "from")
	if !ok {
		return
	}
	to, ok := parseMonitoringTime(c, "to")
	if !ok {
		return
	}
	actorID, actorRoles := actorFromContext(c)
	resp, err := h.service.WorkspaceDashboard(c.Request.Context(), monitoring.WorkspaceDashboardRequest{
		WorkspaceID: workspaceID,
		ActorUserID: actorID,
		ActorRoles:  actorRoles,
		From:        from,
		To:          to,
	})
	if err != nil {
		writeMonitoringError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

// monitoringLogsQueryRequest is the JSON body for POST /logs/query. Start and
// End are RFC3339 strings so the body is human-readable; the handler parses
// them into time.Time before calling the provider.
type monitoringLogsQueryRequest struct {
	Namespace  string `json:"namespace" binding:"required"`
	Pod        string `json:"pod"`
	Container  string `json:"container"`
	TextFilter string `json:"text_filter"`
	Start      string `json:"start" binding:"required"`
	End        string `json:"end" binding:"required"`
	Direction  string `json:"direction"`
	Limit      int    `json:"limit"`
}

// queryLogs handles POST /api/v1/clusters/:cluster_id/logs/query.
//
// The handler reuses the M37A capability.LogProvider (Loki) with a bounded
// query shape — no LogQL is accepted from the client. The namespace arrives in
// the JSON body, so the handler re-validates it against the M35 resolved
// namespace scope (set by requireNamespaceQueryAccess middleware). An
// unauthorized namespace surfaces as 404 for anti-leakage (ADR 0065 §4).
func (h monitoringHandler) queryLogs(c *gin.Context) {
	if h.logProvider == nil {
		writeError(c, http.StatusServiceUnavailable, "LOG_PROVIDER_UNAVAILABLE", "log provider is not configured")
		return
	}
	var req monitoringLogsQueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	start, err := time.Parse(time.RFC3339Nano, req.Start)
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "start must be an RFC3339 timestamp")
		return
	}
	end, err := time.Parse(time.RFC3339Nano, req.End)
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "end must be an RFC3339 timestamp")
		return
	}
	direction := req.Direction
	if direction == "" {
		direction = capability.DirectionForward
	}
	// M35 namespace scope re-check. The resourceRoutes middleware already
	// resolved the caller's scope for the cluster via
	// requireNamespaceQueryAccess. Since the namespace arrives in the body
	// (not a query param), the middleware resolved the caller's FULL scope
	// rather than validating a specific namespace. We re-check the body
	// namespace here. Anti-leakage: unauthorized namespace → 404.
	scope := ResolvedNamespaceScope(c)
	if !scope.AllNamespaces {
		allowed := false
		for _, ns := range scope.NamespaceGrants {
			if ns == req.Namespace {
				allowed = true
				break
			}
		}
		if !allowed {
			writeError(c, http.StatusNotFound, "RESOURCE_NOT_FOUND", "resource not found")
			return
		}
	}
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	query := capability.LogQuery{
		ClusterID:  metadata.ClusterID,
		Namespace:  req.Namespace,
		PodName:    req.Pod,
		Container:  req.Container,
		TextFilter: req.TextFilter,
		Start:      start,
		End:        end,
		Direction:  direction,
		Limit:      req.Limit,
	}
	result, err := h.logProvider.QueryLogs(c.Request.Context(), query)
	if err != nil {
		if errors.Is(err, capability.ErrInvalidLogQuery) {
			writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
			return
		}
		writeError(c, http.StatusInternalServerError, "LOG_QUERY_FAILED", "unable to query logs")
		return
	}
	c.JSON(http.StatusOK, result)
}

// parseMonitoringTime parses an RFC3339 timestamp query parameter. It writes
// a 400 error and returns ok=false on failure.
func parseMonitoringTime(c *gin.Context, name string) (time.Time, bool) {
	value, err := time.Parse(time.RFC3339Nano, c.Query(name))
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", name+" must be an RFC3339 timestamp")
		return time.Time{}, false
	}
	return value, true
}

// writeMonitoringError maps monitoring service errors to stable HTTP
// responses. Anti-leakage: ErrWorkspaceNotFound surfaces as 404 so a missing
// or unauthorized workspace cannot be distinguished.
func writeMonitoringError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, monitoring.ErrInvalidTemplate):
		writeError(c, http.StatusBadRequest, "INVALID_TEMPLATE", "dashboard template is not supported")
	case errors.Is(err, monitoring.ErrInvalidWindow):
		writeError(c, http.StatusBadRequest, "INVALID_WINDOW", "time window is invalid or exceeds 24h")
	case errors.Is(err, monitoring.ErrWorkspaceNotFound):
		writeError(c, http.StatusNotFound, "WORKSPACE_NOT_FOUND", "workspace not found")
	default:
		writeError(c, http.StatusInternalServerError, "MONITORING_QUERY_FAILED", "unable to load dashboard")
	}
}
