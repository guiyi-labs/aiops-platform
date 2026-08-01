package httpserver

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/apiquery"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
	"k8s-aiops.local/backend/internal/requestctx"
	"k8s-aiops.local/backend/internal/restore"
)

type restoreHandler struct {
	service    *restore.Service
	kubernetes *k8sgateway.Service
}

type previewRestoreRequest struct {
	SourceBackupName      string `json:"source_backup_name" binding:"required"`
	SourceBackupNamespace string `json:"source_backup_namespace" binding:"required"`
}

type executeRestoreRequest struct {
	ConfirmationToken string `json:"confirmation_token" binding:"required"`
}

func (h restoreHandler) preview(c *gin.Context) {
	clusterID, ok := clusterID(c)
	if !ok {
		return
	}
	var request previewRestoreRequest
	if err := decodeStrictJSON(c, &request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	setAuditTarget(c, "RestorePlan", request.SourceBackupNamespace, request.SourceBackupName)
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	plan, err := h.service.Preview(c.Request.Context(), clusterID, restore.Request{
		SourceBackupName:      strings.TrimSpace(request.SourceBackupName),
		SourceBackupNamespace: strings.TrimSpace(request.SourceBackupNamespace),
	}, restore.ActorRef{ID: metadata.ActorID, Name: metadata.ActorDisplayName})
	if err == nil {
		setAuditClusterID(c, clusterID)
		setAuditTarget(c, "RestorePlan", plan.SourceBackupNamespace, plan.ID)
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusCreated, restore.Response(plan))
		return
	}
	setAuditClusterID(c, clusterID)
	h.writeError(c, err, "unable to preview restore rehearsal")
}

func (h restoreHandler) execute(c *gin.Context) {
	planID := strings.TrimSpace(c.Param("plan_id"))
	var request executeRestoreRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "confirmation_token is required")
		return
	}
	idempotencyKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if idempotencyKey == "" {
		writeError(c, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key header is required")
		return
	}
	setAuditTarget(c, "RestorePlan", "", planID)
	plan, err := h.service.Execute(c.Request.Context(), planID, request.ConfirmationToken, idempotencyKey)
	if err == nil {
		setAuditClusterID(c, plan.ClusterID)
		setAuditTarget(c, "RestorePlan", plan.SourceBackupNamespace, plan.ID)
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, restore.Response(plan))
		return
	}
	if plan.ClusterID > 0 {
		setAuditClusterID(c, plan.ClusterID)
	}
	h.writeError(c, err, "unable to execute restore rehearsal")
}

func (h restoreHandler) list(c *gin.Context) {
	clusterID, ok := clusterID(c)
	if !ok {
		return
	}
	plans, err := h.service.List(c.Request.Context(), clusterID)
	if err != nil {
		h.writeError(c, err, "unable to list restore plans")
		return
	}
	items := make([]restore.PlanResponse, 0, len(plans))
	for _, plan := range plans {
		items = append(items, restore.Response(plan))
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": len(items)})
}

func (h restoreHandler) writeError(c *gin.Context, err error, fallback string) {
	switch {
	case errors.Is(err, restore.ErrInvalidRequest):
		writeError(c, http.StatusBadRequest, "INVALID_RESTORE_REQUEST", "restore request parameters are invalid")
	case errors.Is(err, restore.ErrVeleroNotInstalled):
		writeError(c, http.StatusUnprocessableEntity, "VELERO_UNAVAILABLE", "Velero is not installed on the target cluster")
	case errors.Is(err, restore.ErrSourceBackupNotFound):
		writeError(c, http.StatusNotFound, "SOURCE_BACKUP_NOT_FOUND", "source backup not found")
	case errors.Is(err, restore.ErrSourceBackupIncomplete):
		writeError(c, http.StatusUnprocessableEntity, "SOURCE_BACKUP_INCOMPLETE", "source backup is not in Completed phase")
	case errors.Is(err, restore.ErrSourceBackupScope):
		writeError(c, http.StatusUnprocessableEntity, "SOURCE_BACKUP_SCOPE", "source backup scope is not M28-compatible")
	case errors.Is(err, restore.ErrDestinationExists):
		writeError(c, http.StatusConflict, "DESTINATION_EXISTS", "destination namespace already exists")
	case errors.Is(err, restore.ErrDestinationCollision):
		writeError(c, http.StatusConflict, "DESTINATION_COLLISION", "destination namespace collides with an active restore plan")
	case errors.Is(err, restore.ErrRestoreNameConflict):
		writeError(c, http.StatusConflict, "RESTORE_NAME_CONFLICT", "velero restore name already exists on the target cluster")
	case errors.Is(err, restore.ErrQuarantineDryRunFailed):
		writeError(c, http.StatusBadRequest, "QUARANTINE_DRY_RUN_FAILED", "quarantine resource dry-run failed")
	case errors.Is(err, restore.ErrRestoreDryRunFailed):
		writeError(c, http.StatusBadRequest, "RESTORE_DRY_RUN_FAILED", "velero restore dry-run failed")
	case errors.Is(err, restore.ErrConfirmationInvalid):
		writeError(c, http.StatusForbidden, "RESTORE_CONFIRMATION_INVALID", "restore confirmation is invalid")
	case errors.Is(err, restore.ErrInvalidIdempotency):
		writeError(c, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "idempotency key is invalid")
	case errors.Is(err, restore.ErrExpired):
		writeError(c, http.StatusGone, "RESTORE_EXPIRED", "restore plan has expired")
	case errors.Is(err, restore.ErrInProgress):
		writeError(c, http.StatusConflict, "RESTORE_IN_PROGRESS", "restore execution is in progress")
	case errors.Is(err, restore.ErrAlreadyExecuted):
		writeError(c, http.StatusConflict, "RESTORE_ALREADY_USED", "restore plan already used with another idempotency key")
	case errors.Is(err, restore.ErrStaleSource):
		writeError(c, http.StatusConflict, "STALE_SOURCE", "source backup has changed since preview")
	case errors.Is(err, restore.ErrQuarantineFailed):
		writeError(c, http.StatusBadGateway, "QUARANTINE_FAILED", "quarantine controls were not established before restore")
	case errors.Is(err, restore.ErrExecutionFailed):
		writeError(c, http.StatusBadGateway, "RESTORE_FAILED", "restore execution failed")
	case errors.Is(err, restore.ErrRestorePollTimeout):
		writeError(c, http.StatusGatewayTimeout, "RESTORE_POLL_TIMEOUT", "velero restore did not reach a terminal phase within the bounded wait")
	case errors.Is(err, restore.ErrPartialRestore):
		writeError(c, http.StatusMultiStatus, "PARTIAL_RESTORE", "velero restore completed with partial failures; quarantine target retained")
	case errors.Is(err, restore.ErrNotFound):
		writeError(c, http.StatusNotFound, "RESTORE_PLAN_NOT_FOUND", "restore plan not found")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", fallback)
	}
}

// --- M58 live Velero Restore CR list/detail ----------------------------------

func (h restoreHandler) listRestores(c *gin.Context) {
	clusterID, ok := clusterID(c)
	if !ok {
		return
	}
	if h.kubernetes == nil {
		writeError(c, http.StatusServiceUnavailable, "VELERO_UNAVAILABLE", "Velero live CR provider is not configured")
		return
	}
	query, err := apiquery.Parse(c.Request, "name")
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", err.Error())
		return
	}
	namespace := strings.TrimSpace(c.Query("namespace"))
	resp, err := h.kubernetes.Restores(c.Request.Context(), clusterID, namespace, query)
	if err != nil {
		switch {
		case errors.Is(err, k8sgateway.ErrVeleroUnavailable):
			writeError(c, http.StatusServiceUnavailable, "VELERO_UNAVAILABLE", "Velero is not installed on the target cluster")
		default:
			writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to list Velero restores")
		}
		return
	}
	setAuditClusterID(c, clusterID)
	c.JSON(http.StatusOK, gin.H{"items": resp.Items, "total": resp.Total, "remaining": resp.Remaining})
}

func (h restoreHandler) getRestore(c *gin.Context) {
	clusterID, ok := clusterID(c)
	if !ok {
		return
	}
	if h.kubernetes == nil {
		writeError(c, http.StatusServiceUnavailable, "VELERO_UNAVAILABLE", "Velero live CR provider is not configured")
		return
	}
	namespace := strings.TrimSpace(c.Param("namespace"))
	name := strings.TrimSpace(c.Param("name"))
	if namespace == "" || name == "" {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "namespace and name are required")
		return
	}
	r, err := h.kubernetes.VeleroRestore(c.Request.Context(), clusterID, namespace, name)
	if err != nil {
		switch {
		case errors.Is(err, k8sgateway.ErrVeleroUnavailable):
			writeError(c, http.StatusServiceUnavailable, "VELERO_UNAVAILABLE", "Velero is not installed on the target cluster")
		case errors.Is(err, k8sgateway.ErrResourceNotFound):
			writeError(c, http.StatusNotFound, "VELERO_RESTORE_NOT_FOUND", "Velero restore not found")
		default:
			writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to fetch Velero restore")
		}
		return
	}
	setAuditClusterID(c, clusterID)
	setAuditTarget(c, "VeleroRestore", namespace, name)
	c.JSON(http.StatusOK, r)
}
