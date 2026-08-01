package httpserver

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/apiquery"
	"k8s-aiops.local/backend/internal/backup"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
	"k8s-aiops.local/backend/internal/requestctx"
)

type backupHandler struct {
	service    *backup.Service
	kubernetes *k8sgateway.Service
}

type previewBackupRequest struct {
	SourceNamespace string `json:"source_namespace" binding:"required"`
	StorageLocation string `json:"storage_location" binding:"required"`
	TTL             string `json:"ttl,omitempty"`
}

type executeBackupRequest struct {
	ConfirmationToken string `json:"confirmation_token" binding:"required"`
}

func (h backupHandler) preview(c *gin.Context) {
	clusterID, ok := clusterID(c)
	if !ok {
		return
	}
	var request previewBackupRequest
	if err := decodeStrictJSON(c, &request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	setAuditTarget(c, "BackupPlan", request.SourceNamespace, "")
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	plan, err := h.service.Preview(c.Request.Context(), clusterID, backup.Request{
		SourceNamespace: strings.TrimSpace(request.SourceNamespace),
		StorageLocation: strings.TrimSpace(request.StorageLocation),
		TTL:             strings.TrimSpace(request.TTL),
	}, backup.ActorRef{ID: metadata.ActorID, Name: metadata.ActorDisplayName})
	if err == nil {
		setAuditClusterID(c, clusterID)
		setAuditTarget(c, "BackupPlan", plan.BackupNamespace, plan.ID)
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusCreated, backup.Response(plan))
		return
	}
	setAuditClusterID(c, clusterID)
	h.writeError(c, err, "unable to preview backup")
}

func (h backupHandler) execute(c *gin.Context) {
	planID := strings.TrimSpace(c.Param("plan_id"))
	var request executeBackupRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "confirmation_token is required")
		return
	}
	idempotencyKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if idempotencyKey == "" {
		writeError(c, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key header is required")
		return
	}
	setAuditTarget(c, "BackupPlan", "", planID)
	plan, err := h.service.Execute(c.Request.Context(), planID, request.ConfirmationToken, idempotencyKey)
	if err == nil {
		setAuditClusterID(c, plan.ClusterID)
		setAuditTarget(c, "BackupPlan", plan.BackupNamespace, plan.ID)
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, backup.Response(plan))
		return
	}
	if plan.ClusterID > 0 {
		setAuditClusterID(c, plan.ClusterID)
	}
	h.writeError(c, err, "unable to execute backup")
}

func (h backupHandler) list(c *gin.Context) {
	clusterID, ok := clusterID(c)
	if !ok {
		return
	}
	plans, err := h.service.List(c.Request.Context(), clusterID)
	if err != nil {
		h.writeError(c, err, "unable to list backup plans")
		return
	}
	items := make([]backup.PlanResponse, 0, len(plans))
	for _, plan := range plans {
		items = append(items, backup.Response(plan))
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": len(items)})
}

func (h backupHandler) writeError(c *gin.Context, err error, fallback string) {
	switch {
	case errors.Is(err, backup.ErrInvalidRequest):
		writeError(c, http.StatusBadRequest, "INVALID_BACKUP_REQUEST", "backup request parameters are invalid")
	case errors.Is(err, backup.ErrVeleroNotInstalled):
		writeError(c, http.StatusUnprocessableEntity, "VELERO_UNAVAILABLE", "Velero is not installed on the target cluster")
	case errors.Is(err, backup.ErrStorageLocationNotFound):
		writeError(c, http.StatusBadRequest, "STORAGE_LOCATION_NOT_FOUND", "backup storage location not found")
	case errors.Is(err, backup.ErrStorageLocationUnavailable):
		writeError(c, http.StatusUnprocessableEntity, "STORAGE_LOCATION_UNAVAILABLE", "backup storage location is not Available")
	case errors.Is(err, backup.ErrSourceNamespaceNotFound):
		writeError(c, http.StatusNotFound, "SOURCE_NAMESPACE_NOT_FOUND", "source namespace not found")
	case errors.Is(err, backup.ErrStaleSourceNamespace):
		writeError(c, http.StatusConflict, "STALE_SOURCE_NAMESPACE", "source namespace changed since preview")
	case errors.Is(err, backup.ErrBackupNameConflict):
		writeError(c, http.StatusConflict, "BACKUP_NAME_CONFLICT", "backup name already exists on the target cluster")
	case errors.Is(err, backup.ErrConfirmationInvalid):
		writeError(c, http.StatusForbidden, "BACKUP_CONFIRMATION_INVALID", "backup confirmation is invalid")
	case errors.Is(err, backup.ErrInvalidIdempotency):
		writeError(c, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "idempotency key is invalid")
	case errors.Is(err, backup.ErrExpired):
		writeError(c, http.StatusGone, "BACKUP_EXPIRED", "backup plan has expired")
	case errors.Is(err, backup.ErrInProgress):
		writeError(c, http.StatusConflict, "BACKUP_IN_PROGRESS", "backup execution is in progress")
	case errors.Is(err, backup.ErrAlreadyExecuted):
		writeError(c, http.StatusConflict, "BACKUP_ALREADY_USED", "backup plan already used with another idempotency key")
	case errors.Is(err, backup.ErrExecutionFailed):
		writeError(c, http.StatusBadGateway, "BACKUP_FAILED", "backup execution failed")
	case errors.Is(err, backup.ErrNotFound):
		writeError(c, http.StatusNotFound, "BACKUP_PLAN_NOT_FOUND", "backup plan not found")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", fallback)
	}
}

// --- M58 live Velero Backup CR list/detail -----------------------------------

func (h backupHandler) listBackups(c *gin.Context) {
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
	resp, err := h.kubernetes.Backups(c.Request.Context(), clusterID, namespace, query)
	if err != nil {
		switch {
		case errors.Is(err, k8sgateway.ErrVeleroUnavailable):
			writeError(c, http.StatusServiceUnavailable, "VELERO_UNAVAILABLE", "Velero is not installed on the target cluster")
		default:
			writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to list Velero backups")
		}
		return
	}
	setAuditClusterID(c, clusterID)
	c.JSON(http.StatusOK, gin.H{"items": resp.Items, "total": resp.Total, "remaining": resp.Remaining})
}

func (h backupHandler) getBackup(c *gin.Context) {
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
	b, err := h.kubernetes.Backup(c.Request.Context(), clusterID, namespace, name)
	if err != nil {
		switch {
		case errors.Is(err, k8sgateway.ErrVeleroUnavailable):
			writeError(c, http.StatusServiceUnavailable, "VELERO_UNAVAILABLE", "Velero is not installed on the target cluster")
		case errors.Is(err, k8sgateway.ErrResourceNotFound):
			writeError(c, http.StatusNotFound, "VELERO_BACKUP_NOT_FOUND", "Velero backup not found")
		default:
			writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to fetch Velero backup")
		}
		return
	}
	setAuditClusterID(c, clusterID)
	setAuditTarget(c, "VeleroBackup", namespace, name)
	c.JSON(http.StatusOK, b)
}
