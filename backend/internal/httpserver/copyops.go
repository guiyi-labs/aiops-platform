package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/copyops"
	"k8s-aiops.local/backend/internal/requestctx"
)

type copyopsHandler struct {
	service *copyops.Service
}

type previewCopyRequest struct {
	SourceClusterID         *int64                      `json:"source_cluster_id"`
	SourceNamespace         string                      `json:"source_namespace" binding:"required"`
	TargetClusterID         *int64                      `json:"target_cluster_id"`
	TargetNamespace         string                      `json:"target_namespace" binding:"required"`
	Bundle                  []copyops.BundleItemRequest `json:"bundle" binding:"required,min=1,max=20"`
	StripSecrets            bool                        `json:"strip_secrets,omitempty"`
	StripLabelPrefixes      []string                    `json:"strip_label_prefixes,omitempty"`
	StripAnnotationPrefixes []string                    `json:"strip_annotation_prefixes,omitempty"`
}

type executeCopyRequest struct {
	ConfirmationToken string `json:"confirmation_token" binding:"required"`
}

func (h copyopsHandler) preview(c *gin.Context) {
	var body previewCopyRequest
	if err := decodeStrictJSON(c, &body); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	var sourceClusterID int64
	if body.SourceClusterID != nil {
		sourceClusterID = *body.SourceClusterID
	} else {
		id, ok := clusterID(c)
		if !ok {
			return
		}
		sourceClusterID = id
	}
	var targetClusterID int64
	if body.TargetClusterID != nil {
		targetClusterID = *body.TargetClusterID
	} else {
		// Default target cluster is a different cluster; since this is
		// interactive the operator must provide it explicitly.
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "target_cluster_id is required")
		return
	}
	setAuditClusterID(c, sourceClusterID)
	setAuditTarget(c, "CopyPlan", body.SourceNamespace, "")
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	plan, err := h.service.Preview(c.Request.Context(), copyops.PreviewRequest{
		SourceClusterID:         sourceClusterID,
		SourceNamespace:         strings.TrimSpace(body.SourceNamespace),
		TargetClusterID:         targetClusterID,
		TargetNamespace:         strings.TrimSpace(body.TargetNamespace),
		Bundle:                  body.Bundle,
		StripSecrets:            body.StripSecrets,
		StripLabelPrefixes:      body.StripLabelPrefixes,
		StripAnnotationPrefixes: body.StripAnnotationPrefixes,
	}, copyops.ActorRef{ID: metadata.ActorID, Name: metadata.ActorDisplayName})
	if err == nil {
		setAuditTarget(c, "CopyPlan", plan.SourceNamespace, plan.ID)
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusCreated, plan)
		return
	}
	h.writeError(c, err, "unable to preview copy plan")
}

func (h copyopsHandler) execute(c *gin.Context) {
	planID := strings.TrimSpace(c.Param("plan_id"))
	var body executeCopyRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "confirmation_token is required")
		return
	}
	idempotencyKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if idempotencyKey == "" {
		writeError(c, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key header is required")
		return
	}
	setAuditTarget(c, "CopyPlan", "", planID)
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	plan, err := h.service.Execute(c.Request.Context(), copyops.ExecuteRequest{
		PlanID:            planID,
		ConfirmationToken: body.ConfirmationToken,
		IdempotencyKey:    idempotencyKey,
	}, copyops.ActorRef{ID: metadata.ActorID, Name: metadata.ActorDisplayName})
	if err == nil {
		setAuditClusterID(c, plan.SourceClusterID)
		setAuditTarget(c, "CopyPlan", plan.SourceNamespace, plan.ID)
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, plan)
		return
	}
	if plan.SourceClusterID > 0 {
		setAuditClusterID(c, plan.SourceClusterID)
	}
	h.writeError(c, err, "unable to execute copy plan")
}

func (h copyopsHandler) get(c *gin.Context) {
	id := strings.TrimSpace(c.Param("plan_id"))
	plan, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		h.writeError(c, err, "unable to fetch copy plan")
		return
	}
	setAuditClusterID(c, plan.SourceClusterID)
	setAuditTarget(c, "CopyPlan", plan.SourceNamespace, plan.ID)
	c.JSON(http.StatusOK, plan)
}

func (h copyopsHandler) listCurrentUser(c *gin.Context) {
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	offset, limit := parsePaging(c, 0, 20)
	plans, total, err := h.service.ListByUser(c.Request.Context(), metadata.ActorID, offset, limit)
	if err != nil {
		h.writeError(c, err, "unable to list copy plans")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": plans, "total": total, "offset": offset, "limit": limit})
}

func (h copyopsHandler) listByCluster(c *gin.Context) {
	id, ok := clusterID(c)
	if !ok {
		return
	}
	offset, limit := parsePaging(c, 0, 20)
	plans, total, err := h.service.ListByCluster(c.Request.Context(), id, offset, limit)
	if err != nil {
		h.writeError(c, err, "unable to list copy plans for cluster")
		return
	}
	setAuditClusterID(c, id)
	c.JSON(http.StatusOK, gin.H{"items": plans, "total": total, "offset": offset, "limit": limit})
}

func parsePaging(c *gin.Context, defaultOffset, defaultLimit int) (int, int) {
	offset := defaultOffset
	if v := strings.TrimSpace(c.Query("offset")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	limit := defaultLimit
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	return offset, limit
}

func (h copyopsHandler) writeError(c *gin.Context, err error, fallback string) {
	switch {
	case errors.Is(err, copyops.ErrInvalidRequest):
		writeError(c, http.StatusBadRequest, "INVALID_COPY_REQUEST", "copy request parameters are invalid")
	case errors.Is(err, copyops.ErrBundleEmpty):
		writeError(c, http.StatusBadRequest, "COPY_BUNDLE_EMPTY", "copy bundle must include at least one resource")
	case errors.Is(err, copyops.ErrBundleTooLarge):
		writeError(c, http.StatusBadRequest, "COPY_BUNDLE_TOO_LARGE", "copy bundle exceeds resource limit")
	case errors.Is(err, copyops.ErrKindDisallowed):
		writeError(c, http.StatusBadRequest, "COPY_KIND_DISALLOWED", err.Error())
	case errors.Is(err, copyops.ErrCrossClusterSame):
		writeError(c, http.StatusBadRequest, "COPY_SAME_CLUSTER", "source and destination clusters must be different")
	case errors.Is(err, copyops.ErrSourceUnavailable):
		writeError(c, http.StatusBadGateway, "COPY_SOURCE_UNAVAILABLE", "source cluster is unavailable")
	case errors.Is(err, copyops.ErrSourceNotFound):
		writeError(c, http.StatusNotFound, "COPY_SOURCE_NOT_FOUND", err.Error())
	case errors.Is(err, copyops.ErrDestinationUnavailable):
		writeError(c, http.StatusBadGateway, "COPY_DESTINATION_UNAVAILABLE", "destination cluster is unavailable")
	case errors.Is(err, copyops.ErrNamespaceMissing):
		writeError(c, http.StatusBadRequest, "COPY_NAMESPACE_MISSING", "destination namespace does not exist")
	case errors.Is(err, copyops.ErrConflict):
		writeError(c, http.StatusConflict, "COPY_CONFLICT", "destination resource already exists")
	case errors.Is(err, copyops.ErrPreviewFailed):
		writeError(c, http.StatusUnprocessableEntity, "COPY_PREVIEW_FAILED", err.Error())
	case errors.Is(err, copyops.ErrConfirmationInvalid):
		writeError(c, http.StatusForbidden, "COPY_CONFIRMATION_INVALID", "copy confirmation token is invalid")
	case errors.Is(err, copyops.ErrInvalidIdempotency):
		writeError(c, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "idempotency key is invalid or mismatched")
	case errors.Is(err, copyops.ErrExpired):
		writeError(c, http.StatusGone, "COPY_EXPIRED", "copy plan has expired")
	case errors.Is(err, copyops.ErrInProgress):
		writeError(c, http.StatusConflict, "COPY_IN_PROGRESS", "copy execution is already in progress")
	case errors.Is(err, copyops.ErrAlreadyExecuted):
		writeError(c, http.StatusConflict, "COPY_ALREADY_USED", "copy plan already used with another idempotency key")
	case errors.Is(err, copyops.ErrExecutionFailed):
		writeError(c, http.StatusBadGateway, "COPY_EXECUTION_FAILED", err.Error())
	case errors.Is(err, copyops.ErrNotFound):
		writeError(c, http.StatusNotFound, "COPY_PLAN_NOT_FOUND", "copy plan not found")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", fallback)
	}
}
