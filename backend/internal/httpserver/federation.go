package httpserver

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/authz"
	"k8s-aiops.local/backend/internal/federation"
)

// federationHandler exposes the M48 multi-cluster federation service.
//
// Routes (all under /api/v1/federation):
//
//	GET    /federation/overview                        — federation topology + health summary
//	GET    /federation/events                          — recent federation events (bounded, newest first)
//	GET    /federation/resources/summary               — cross-cluster resource count by GVR (fixed whitelist)
//	POST   /federation/clusters/register               — register an existing cluster as host or member (SystemOpsAdmin)
//	DELETE /federation/clusters/:cluster_id            — deregister (soft-delete; host must demote first) (SystemOpsAdmin)
//	POST   /federation/clusters/:cluster_id/promote    — promote a member/standalone to host (SystemOpsAdmin)
//	POST   /federation/clusters/:cluster_id/demote     — demote the host to member or standalone (SystemOpsAdmin)
//	POST   /federation/clusters/:cluster_id/heartbeat  — record a heartbeat (SystemOpsAdmin)
//	PATCH  /federation/clusters/:cluster_id/status     — update federation_status (SystemOpsAdmin)
//	GET    /federation/clusters/:cluster_id/events      — recent federation events for one cluster
//
// Authorization is enforced at the route layer via RequiredRoles =
// rolesSystemOpsAdmin for write operations and authentication-only for reads.
// Anti-leakage (404 > 403) is preserved: a cluster that does not exist is
// reported as 404, never 403.
type federationHandler struct {
	service *federation.Service
	authz   *authz.Service
}

// overview handles GET /api/v1/federation/overview.
func (h federationHandler) overview(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "FEDERATION_UNAVAILABLE", "federation service is not configured")
		return
	}
	visible, _, err := authorizedClusterFilter(h.authz, c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "FEDERATION_QUERY_FAILED", "unable to evaluate access scope")
		return
	}
	overview, err := h.service.Overview(c.Request.Context(), visible)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "FEDERATION_QUERY_FAILED", "unable to load federation overview")
		return
	}
	c.JSON(http.StatusOK, overview)
}

// listEvents handles GET /api/v1/federation/events.
func (h federationHandler) listEvents(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "FEDERATION_UNAVAILABLE", "federation service is not configured")
		return
	}
	limit := federation.DefaultEventsLimit
	if raw := c.Query("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "limit must be a positive integer")
			return
		}
		limit = n
	}
	events, err := h.service.ListEvents(c.Request.Context(), limit)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "FEDERATION_QUERY_FAILED", "unable to load federation events")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": events})
}

// listClusterEvents handles GET /api/v1/federation/clusters/:cluster_id/events.
func (h federationHandler) listClusterEvents(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "FEDERATION_UNAVAILABLE", "federation service is not configured")
		return
	}
	clusterID, ok := parseFederationClusterID(c)
	if !ok {
		return
	}
	limit := federation.DefaultEventsLimit
	if raw := c.Query("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "limit must be a positive integer")
			return
		}
		limit = n
	}
	events, err := h.service.ListEventsByCluster(c.Request.Context(), clusterID, limit)
	if err != nil {
		writeFederationError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": events})
}

// resourceSummary handles GET /api/v1/federation/resources/summary.
func (h federationHandler) resourceSummary(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "FEDERATION_UNAVAILABLE", "federation service is not configured")
		return
	}
	visible, _, err := authorizedClusterFilter(h.authz, c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "FEDERATION_QUERY_FAILED", "unable to evaluate access scope")
		return
	}
	summary, err := h.service.ResourceSummary(c.Request.Context(), visible)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "FEDERATION_QUERY_FAILED", "unable to load resource summary")
		return
	}
	c.JSON(http.StatusOK, summary)
}

type registerClusterRequest struct {
	ClusterID int64  `json:"cluster_id"`
	Role      string `json:"role"`
	Status    string `json:"status"`
}

// registerCluster handles POST /api/v1/federation/clusters/register.
func (h federationHandler) registerCluster(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "FEDERATION_UNAVAILABLE", "federation service is not configured")
		return
	}
	var req registerClusterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_BODY", "request body is not valid JSON")
		return
	}
	item, err := h.service.RegisterCluster(c.Request.Context(), federation.RegisterClusterInput{
		ClusterID: req.ClusterID,
		Role:      req.Role,
		Status:    req.Status,
	})
	if err != nil {
		writeFederationError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

// deregisterCluster handles DELETE /api/v1/federation/clusters/:cluster_id.
func (h federationHandler) deregisterCluster(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "FEDERATION_UNAVAILABLE", "federation service is not configured")
		return
	}
	clusterID, ok := parseFederationClusterID(c)
	if !ok {
		return
	}
	item, err := h.service.DeregisterCluster(c.Request.Context(), clusterID)
	if err != nil {
		writeFederationError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

// promoteCluster handles POST /api/v1/federation/clusters/:cluster_id/promote.
func (h federationHandler) promoteCluster(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "FEDERATION_UNAVAILABLE", "federation service is not configured")
		return
	}
	clusterID, ok := parseFederationClusterID(c)
	if !ok {
		return
	}
	item, err := h.service.PromoteToHost(c.Request.Context(), clusterID)
	if err != nil {
		writeFederationError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

type demoteClusterRequest struct {
	TargetRole string `json:"target_role"`
}

// demoteCluster handles POST /api/v1/federation/clusters/:cluster_id/demote.
func (h federationHandler) demoteCluster(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "FEDERATION_UNAVAILABLE", "federation service is not configured")
		return
	}
	clusterID, ok := parseFederationClusterID(c)
	if !ok {
		return
	}
	var req demoteClusterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_BODY", "request body is not valid JSON")
		return
	}
	item, err := h.service.DemoteHost(c.Request.Context(), clusterID, req.TargetRole)
	if err != nil {
		writeFederationError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

type heartbeatRequest struct {
	Status string `json:"status"`
}

// heartbeat handles POST /api/v1/federation/clusters/:cluster_id/heartbeat.
func (h federationHandler) heartbeat(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "FEDERATION_UNAVAILABLE", "federation service is not configured")
		return
	}
	clusterID, ok := parseFederationClusterID(c)
	if !ok {
		return
	}
	var req heartbeatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Body is optional; an empty body is treated as a heartbeat without
		// an explicit status transition.
		req = heartbeatRequest{}
	}
	item, err := h.service.RecordHeartbeat(c.Request.Context(), clusterID, req.Status)
	if err != nil {
		writeFederationError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

type updateStatusRequest struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// updateStatus handles PATCH /api/v1/federation/clusters/:cluster_id/status.
func (h federationHandler) updateStatus(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "FEDERATION_UNAVAILABLE", "federation service is not configured")
		return
	}
	clusterID, ok := parseFederationClusterID(c)
	if !ok {
		return
	}
	var req updateStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_BODY", "request body is not valid JSON")
		return
	}
	item, err := h.service.UpdateFederationStatus(c.Request.Context(), clusterID, req.Status, req.Message)
	if err != nil {
		writeFederationError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

// parseFederationClusterID extracts and validates the :cluster_id path
// parameter used by the federation routes.
func parseFederationClusterID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("cluster_id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(c, http.StatusBadRequest, "INVALID_PATH", "cluster_id must be a positive integer")
		return 0, false
	}
	return id, true
}

// writeFederationError maps federation service errors to stable HTTP
// responses. Anti-leakage: ErrClusterNotFound surfaces as 404 so a missing
// cluster is indistinguishable from an unauthorized one.
func writeFederationError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, federation.ErrClusterNotFound):
		writeError(c, http.StatusNotFound, "CLUSTER_NOT_FOUND", "cluster not found")
	case errors.Is(err, federation.ErrClusterAlreadyRegistered):
		writeError(c, http.StatusConflict, "CLUSTER_ALREADY_REGISTERED", "cluster is already registered with the federation")
	case errors.Is(err, federation.ErrHostAlreadyExists):
		writeError(c, http.StatusConflict, "HOST_ALREADY_EXISTS", "a host cluster already exists; demote it first")
	case errors.Is(err, federation.ErrCannotDeregisterHost):
		writeError(c, http.StatusConflict, "CANNOT_DEREGISTER_HOST", "host cluster cannot be deregistered; demote it first")
	case errors.Is(err, federation.ErrInvalidClusterRole):
		writeError(c, http.StatusBadRequest, "INVALID_CLUSTER_ROLE", "cluster_role must be one of host, member, standalone")
	case errors.Is(err, federation.ErrInvalidFederationStatus):
		writeError(c, http.StatusBadRequest, "INVALID_FEDERATION_STATUS", "federation_status must be one of registered, healthy, degraded, disconnected")
	default:
		// Validation errors from the service (e.g. "cluster_id is required")
		// are surfaced as 400 with the raw message.
		writeError(c, http.StatusBadRequest, "FEDERATION_INVALID_INPUT", err.Error())
	}
}
