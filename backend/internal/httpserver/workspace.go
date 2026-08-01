package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/requestctx"
	"k8s-aiops.local/backend/internal/workspace"
)

// workspaceHandler exposes the M46 workspace multi-tenancy service.
//
// Routes (all under /api/v1/workspaces):
//
//	GET    /workspaces                                — list visible workspaces
//	POST   /workspaces                                — create workspace (SystemAdmin)
//	GET    /workspaces/:workspace_id                  — get workspace
//	PATCH  /workspaces/:workspace_id                  — update metadata/display_name
//	DELETE /workspaces/:workspace_id                  — delete workspace (SystemAdmin)
//	GET    /workspaces/:workspace_id/memberships      — list memberships
//	POST   /workspaces/:workspace_id/memberships      — add membership
//	DELETE /workspaces/:workspace_id/memberships      — remove membership (query: cluster_id, namespace)
//	GET    /workspaces/:workspace_id/quota            — get quota
//	PUT    /workspaces/:workspace_id/quota            — set quota
//	GET    /workspaces/:workspace_id/role-bindings    — list role bindings
//	POST   /workspaces/:workspace_id/role-bindings    — grant or replace role
//	DELETE /workspaces/:workspace_id/role-bindings/:user_id — revoke role
//	GET    /workspaces/:workspace_id/role-bindings/audit   — list audit trail
//
// Authorization is enforced inside the service layer (404 > 403 anti-leakage);
// the handler only parses inputs and maps errors.
type workspaceHandler struct {
	service *workspace.Service
}

// actorFromContext returns the authenticated actor's user ID and roles. The
// metadata is populated by withAuthentication.
func actorFromContext(c *gin.Context) (int64, []string) {
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	return metadata.ActorID, append([]string(nil), metadata.Roles...)
}

// listWorkspaces handles GET /api/v1/workspaces.
func (h workspaceHandler) listWorkspaces(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "WORKSPACE_UNAVAILABLE", "workspace service is not configured")
		return
	}
	actorID, actorRoles := actorFromContext(c)
	items, err := h.service.ListWorkspaces(c.Request.Context(), actorID, actorRoles)
	if err != nil {
		writeWorkspaceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

type createWorkspaceRequest struct {
	Name        string          `json:"name"`
	DisplayName string          `json:"display_name"`
	Metadata    json.RawMessage `json:"metadata"`
}

// createWorkspace handles POST /api/v1/workspaces.
func (h workspaceHandler) createWorkspace(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "WORKSPACE_UNAVAILABLE", "workspace service is not configured")
		return
	}
	var req createWorkspaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_BODY", "request body is not valid JSON")
		return
	}
	actorID, actorRoles := actorFromContext(c)
	ws, err := h.service.CreateWorkspace(c.Request.Context(), actorID, actorRoles, workspace.CreateWorkspaceInput{
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Metadata:    req.Metadata,
	})
	if err != nil {
		writeWorkspaceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, ws)
}

// getWorkspace handles GET /api/v1/workspaces/:workspace_id.
func (h workspaceHandler) getWorkspace(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "WORKSPACE_UNAVAILABLE", "workspace service is not configured")
		return
	}
	workspaceID, ok := parseWorkspaceID(c)
	if !ok {
		return
	}
	actorID, actorRoles := actorFromContext(c)
	ws, err := h.service.GetWorkspace(c.Request.Context(), actorID, actorRoles, workspaceID)
	if err != nil {
		writeWorkspaceError(c, err)
		return
	}
	c.JSON(http.StatusOK, ws)
}

type updateWorkspaceRequest struct {
	DisplayName string          `json:"display_name"`
	Metadata    json.RawMessage `json:"metadata"`
}

// updateWorkspace handles PATCH /api/v1/workspaces/:workspace_id.
func (h workspaceHandler) updateWorkspace(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "WORKSPACE_UNAVAILABLE", "workspace service is not configured")
		return
	}
	workspaceID, ok := parseWorkspaceID(c)
	if !ok {
		return
	}
	var req updateWorkspaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_BODY", "request body is not valid JSON")
		return
	}
	actorID, actorRoles := actorFromContext(c)
	ws, err := h.service.UpdateWorkspace(c.Request.Context(), actorID, actorRoles, workspaceID, workspace.UpdateWorkspaceInput{
		DisplayName: req.DisplayName,
		Metadata:    req.Metadata,
	})
	if err != nil {
		writeWorkspaceError(c, err)
		return
	}
	c.JSON(http.StatusOK, ws)
}

// deleteWorkspace handles DELETE /api/v1/workspaces/:workspace_id.
func (h workspaceHandler) deleteWorkspace(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "WORKSPACE_UNAVAILABLE", "workspace service is not configured")
		return
	}
	workspaceID, ok := parseWorkspaceID(c)
	if !ok {
		return
	}
	actorID, actorRoles := actorFromContext(c)
	if err := h.service.DeleteWorkspace(c.Request.Context(), actorID, actorRoles, workspaceID); err != nil {
		writeWorkspaceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// listMemberships handles GET /api/v1/workspaces/:workspace_id/memberships.
func (h workspaceHandler) listMemberships(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "WORKSPACE_UNAVAILABLE", "workspace service is not configured")
		return
	}
	workspaceID, ok := parseWorkspaceID(c)
	if !ok {
		return
	}
	actorID, actorRoles := actorFromContext(c)
	items, err := h.service.ListMemberships(c.Request.Context(), actorID, actorRoles, workspaceID)
	if err != nil {
		writeWorkspaceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

type addMembershipRequest struct {
	ClusterID int64  `json:"cluster_id"`
	Namespace string `json:"namespace"`
}

// addMembership handles POST /api/v1/workspaces/:workspace_id/memberships.
func (h workspaceHandler) addMembership(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "WORKSPACE_UNAVAILABLE", "workspace service is not configured")
		return
	}
	workspaceID, ok := parseWorkspaceID(c)
	if !ok {
		return
	}
	var req addMembershipRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_BODY", "request body is not valid JSON")
		return
	}
	actorID, actorRoles := actorFromContext(c)
	membership, err := h.service.AddMembership(c.Request.Context(), actorID, actorRoles, workspaceID, req.ClusterID, req.Namespace)
	if err != nil {
		writeWorkspaceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, membership)
}

// removeMembership handles DELETE /api/v1/workspaces/:workspace_id/memberships.
// Required query params: cluster_id, namespace.
func (h workspaceHandler) removeMembership(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "WORKSPACE_UNAVAILABLE", "workspace service is not configured")
		return
	}
	workspaceID, ok := parseWorkspaceID(c)
	if !ok {
		return
	}
	clusterID, err := strconv.ParseInt(c.Query("cluster_id"), 10, 64)
	if err != nil || clusterID <= 0 {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "cluster_id query parameter is required and must be a positive integer")
		return
	}
	namespace := c.Query("namespace")
	if namespace == "" {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "namespace query parameter is required")
		return
	}
	actorID, actorRoles := actorFromContext(c)
	if err := h.service.RemoveMembership(c.Request.Context(), actorID, actorRoles, workspaceID, clusterID, namespace); err != nil {
		writeWorkspaceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// getQuota handles GET /api/v1/workspaces/:workspace_id/quota.
func (h workspaceHandler) getQuota(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "WORKSPACE_UNAVAILABLE", "workspace service is not configured")
		return
	}
	workspaceID, ok := parseWorkspaceID(c)
	if !ok {
		return
	}
	actorID, actorRoles := actorFromContext(c)
	quota, err := h.service.GetQuota(c.Request.Context(), actorID, actorRoles, workspaceID)
	if err != nil {
		writeWorkspaceError(c, err)
		return
	}
	c.JSON(http.StatusOK, quota)
}

type setQuotaRequest struct {
	HardCPUCores       *float64 `json:"hard_cpu_cores,omitempty"`
	HardMemoryMiB      *int64   `json:"hard_memory_mib,omitempty"`
	HardPodCount       *int64   `json:"hard_pod_count,omitempty"`
	HardNamespaceCount *int64   `json:"hard_namespace_count,omitempty"`
}

// setQuota handles PUT /api/v1/workspaces/:workspace_id/quota.
func (h workspaceHandler) setQuota(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "WORKSPACE_UNAVAILABLE", "workspace service is not configured")
		return
	}
	workspaceID, ok := parseWorkspaceID(c)
	if !ok {
		return
	}
	var req setQuotaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_BODY", "request body is not valid JSON")
		return
	}
	actorID, actorRoles := actorFromContext(c)
	quota, err := h.service.SetQuota(c.Request.Context(), actorID, actorRoles, workspaceID, workspace.SetQuotaInput{
		HardCPUCores:       req.HardCPUCores,
		HardMemoryMiB:      req.HardMemoryMiB,
		HardPodCount:       req.HardPodCount,
		HardNamespaceCount: req.HardNamespaceCount,
	})
	if err != nil {
		writeWorkspaceError(c, err)
		return
	}
	c.JSON(http.StatusOK, quota)
}

// listRoleBindings handles GET /api/v1/workspaces/:workspace_id/role-bindings.
func (h workspaceHandler) listRoleBindings(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "WORKSPACE_UNAVAILABLE", "workspace service is not configured")
		return
	}
	workspaceID, ok := parseWorkspaceID(c)
	if !ok {
		return
	}
	actorID, actorRoles := actorFromContext(c)
	items, err := h.service.ListGrants(c.Request.Context(), actorID, actorRoles, workspaceID)
	if err != nil {
		writeWorkspaceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

type grantRoleRequest struct {
	UserID int64  `json:"user_id"`
	Role   string `json:"role"`
}

// grantRole handles POST /api/v1/workspaces/:workspace_id/role-bindings.
// Creates or replaces a workspace role binding.
func (h workspaceHandler) grantRole(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "WORKSPACE_UNAVAILABLE", "workspace service is not configured")
		return
	}
	workspaceID, ok := parseWorkspaceID(c)
	if !ok {
		return
	}
	var req grantRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_BODY", "request body is not valid JSON")
		return
	}
	actorID, actorRoles := actorFromContext(c)
	grant, err := h.service.GrantRole(c.Request.Context(), actorID, actorRoles, workspace.GrantRoleInput{
		UserID:      req.UserID,
		WorkspaceID: workspaceID,
		Role:        req.Role,
	})
	if err != nil {
		writeWorkspaceError(c, err)
		return
	}
	c.JSON(http.StatusOK, grant)
}

// revokeRole handles DELETE /api/v1/workspaces/:workspace_id/role-bindings/:user_id.
func (h workspaceHandler) revokeRole(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "WORKSPACE_UNAVAILABLE", "workspace service is not configured")
		return
	}
	workspaceID, ok := parseWorkspaceID(c)
	if !ok {
		return
	}
	targetUserID, ok := parseWorkspaceUserID(c)
	if !ok {
		return
	}
	actorID, actorRoles := actorFromContext(c)
	if err := h.service.RevokeRole(c.Request.Context(), actorID, actorRoles, workspaceID, targetUserID); err != nil {
		writeWorkspaceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// listRoleBindingsAudit handles GET /api/v1/workspaces/:workspace_id/role-bindings/audit.
func (h workspaceHandler) listRoleBindingsAudit(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "WORKSPACE_UNAVAILABLE", "workspace service is not configured")
		return
	}
	workspaceID, ok := parseWorkspaceID(c)
	if !ok {
		return
	}
	limit := 100
	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "limit must be a positive integer")
			return
		}
		limit = n
	}
	actorID, actorRoles := actorFromContext(c)
	items, err := h.service.ListAudit(c.Request.Context(), actorID, actorRoles, workspaceID, limit)
	if err != nil {
		writeWorkspaceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// parseWorkspaceID extracts and validates the :workspace_id path parameter.
func parseWorkspaceID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("workspace_id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(c, http.StatusBadRequest, "INVALID_PATH", "workspace_id must be a positive integer")
		return 0, false
	}
	return id, true
}

// parseWorkspaceUserID extracts and validates the :user_id path parameter.
func parseWorkspaceUserID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("user_id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(c, http.StatusBadRequest, "INVALID_PATH", "user_id must be a positive integer")
		return 0, false
	}
	return id, true
}

// writeWorkspaceError maps workspace service errors to stable HTTP responses.
// Anti-leakage: ErrWorkspaceNotFound and ErrAccessDenied both surface as 404
// so unauthorized workspaces cannot be distinguished from missing ones.
func writeWorkspaceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, workspace.ErrWorkspaceNotFound), errors.Is(err, workspace.ErrAccessDenied):
		writeError(c, http.StatusNotFound, "WORKSPACE_NOT_FOUND", "workspace not found")
	case errors.Is(err, workspace.ErrWorkspaceAlreadyExists):
		writeError(c, http.StatusConflict, "WORKSPACE_ALREADY_EXISTS", "workspace name already in use")
	case errors.Is(err, workspace.ErrMembershipNotFound):
		writeError(c, http.StatusNotFound, "MEMBERSHIP_NOT_FOUND", "workspace membership not found")
	case errors.Is(err, workspace.ErrMembershipAlreadyExists):
		writeError(c, http.StatusConflict, "MEMBERSHIP_ALREADY_EXISTS", "namespace already bound to a workspace")
	case errors.Is(err, workspace.ErrWorkspaceGrantNotFound):
		writeError(c, http.StatusNotFound, "ROLE_BINDING_NOT_FOUND", "workspace role binding not found")
	case errors.Is(err, workspace.ErrWorkspaceGrantAlreadyExists):
		writeError(c, http.StatusConflict, "ROLE_BINDING_ALREADY_EXISTS", "user already holds a role on this workspace")
	case errors.Is(err, workspace.ErrInvalidRole):
		writeError(c, http.StatusBadRequest, "INVALID_ROLE", "role must be one of workspace_admin, workspace_editor, workspace_viewer")
	default:
		writeError(c, http.StatusBadRequest, "WORKSPACE_INVALID_INPUT", err.Error())
	}
}
