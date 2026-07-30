package httpserver

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/maintenance"
	"k8s-aiops.local/backend/internal/requestctx"
)

type maintenanceHandler struct {
	service *maintenance.Service
}

type previewMaintenanceRequest struct {
	Action   string `json:"action" binding:"required"`
	NodeName string `json:"node_name" binding:"required"`
}

type executeMaintenanceRequest struct {
	ConfirmationToken string `json:"confirmation_token" binding:"required"`
}

func (h maintenanceHandler) preview(c *gin.Context) {
	clusterID, ok := clusterID(c)
	if !ok {
		return
	}
	var request previewMaintenanceRequest
	if err := decodeStrictJSON(c, &request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	setAuditTarget(c, "MaintenancePlan", "", request.NodeName)
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	plan, err := h.service.Preview(c.Request.Context(), clusterID, maintenance.Request{
		Action:   strings.TrimSpace(request.Action),
		NodeName: strings.TrimSpace(request.NodeName),
	}, maintenance.ActorRef{ID: metadata.ActorID, Name: metadata.ActorDisplayName})
	if err == nil {
		setAuditClusterID(c, clusterID)
		setAuditTarget(c, "MaintenancePlan", "", plan.ID)
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusCreated, maintenance.Response(plan))
		return
	}
	setAuditClusterID(c, clusterID)
	h.writeError(c, err, "unable to preview maintenance")
}

func (h maintenanceHandler) execute(c *gin.Context) {
	planID := strings.TrimSpace(c.Param("plan_id"))
	var request executeMaintenanceRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "confirmation_token is required")
		return
	}
	idempotencyKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if idempotencyKey == "" {
		writeError(c, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key header is required")
		return
	}
	setAuditTarget(c, "MaintenancePlan", "", planID)
	plan, err := h.service.Execute(c.Request.Context(), planID, request.ConfirmationToken, idempotencyKey)
	if err == nil {
		setAuditClusterID(c, plan.ClusterID)
		setAuditTarget(c, "MaintenancePlan", "", plan.ID)
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, maintenance.Response(plan))
		return
	}
	if plan.ClusterID > 0 {
		setAuditClusterID(c, plan.ClusterID)
	}
	h.writeError(c, err, "unable to execute maintenance")
}

func (h maintenanceHandler) list(c *gin.Context) {
	clusterID, ok := clusterID(c)
	if !ok {
		return
	}
	plans, err := h.service.List(c.Request.Context(), clusterID)
	if err != nil {
		h.writeError(c, err, "unable to list maintenance plans")
		return
	}
	items := make([]maintenance.PlanResponse, 0, len(plans))
	for _, plan := range plans {
		items = append(items, maintenance.Response(plan))
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": len(items)})
}

func (h maintenanceHandler) writeError(c *gin.Context, err error, fallback string) {
	switch {
	case errors.Is(err, maintenance.ErrInvalidRequest):
		writeError(c, http.StatusBadRequest, "INVALID_MAINTENANCE_REQUEST", "maintenance request parameters are invalid")
	case errors.Is(err, maintenance.ErrNodeNotFound):
		writeError(c, http.StatusNotFound, "NODE_NOT_FOUND", "target node not found")
	case errors.Is(err, maintenance.ErrControlPlaneNode):
		writeError(c, http.StatusUnprocessableEntity, "CONTROL_PLANE_NODE", "control-plane nodes cannot be maintained")
	case errors.Is(err, maintenance.ErrAlreadyCordoned):
		writeError(c, http.StatusConflict, "ALREADY_CORDONED", "node is already cordoned")
	case errors.Is(err, maintenance.ErrAlreadyUncordoned):
		writeError(c, http.StatusConflict, "ALREADY_UNCORDONED", "node is already schedulable")
	case errors.Is(err, maintenance.ErrNotCordoned):
		writeError(c, http.StatusConflict, "NOT_CORDONED", "node must be cordoned before drain")
	case errors.Is(err, maintenance.ErrTooManyPods):
		writeError(c, http.StatusUnprocessableEntity, "TOO_MANY_PODS", "node has too many resident pods for bounded drain")
	case errors.Is(err, maintenance.ErrUnmanagedPod):
		writeError(c, http.StatusUnprocessableEntity, "UNMANAGED_POD", "node has unmanaged pods that block drain")
	case errors.Is(err, maintenance.ErrEmptyDirPod):
		writeError(c, http.StatusUnprocessableEntity, "EMPTYDIR_POD", "node has pods using emptyDir that block drain")
	case errors.Is(err, maintenance.ErrPDBUnavailable):
		writeError(c, http.StatusUnprocessableEntity, "PDB_UNAVAILABLE", "pdb evidence unavailable for drain")
	case errors.Is(err, maintenance.ErrStaleTarget):
		writeError(c, http.StatusConflict, "STALE_TARGET", "node or pod evidence has changed since preview")
	case errors.Is(err, maintenance.ErrConfirmationInvalid):
		writeError(c, http.StatusForbidden, "MAINTENANCE_CONFIRMATION_INVALID", "maintenance confirmation is invalid")
	case errors.Is(err, maintenance.ErrInvalidIdempotency):
		writeError(c, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "idempotency key is invalid")
	case errors.Is(err, maintenance.ErrExpired):
		writeError(c, http.StatusGone, "MAINTENANCE_EXPIRED", "maintenance plan has expired")
	case errors.Is(err, maintenance.ErrInProgress):
		writeError(c, http.StatusConflict, "MAINTENANCE_IN_PROGRESS", "maintenance execution is in progress")
	case errors.Is(err, maintenance.ErrAlreadyExecuted):
		writeError(c, http.StatusConflict, "MAINTENANCE_ALREADY_USED", "maintenance plan already used with another idempotency key")
	case errors.Is(err, maintenance.ErrExecutionFailed):
		writeError(c, http.StatusBadGateway, "MAINTENANCE_FAILED", "maintenance execution failed")
	case errors.Is(err, maintenance.ErrPartialDrain):
		writeError(c, http.StatusMultiStatus, "PARTIAL_DRAIN", "drain completed with partial failures; node remains cordoned")
	case errors.Is(err, maintenance.ErrNotFound):
		writeError(c, http.StatusNotFound, "MAINTENANCE_PLAN_NOT_FOUND", "maintenance plan not found")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", fallback)
	}
}
