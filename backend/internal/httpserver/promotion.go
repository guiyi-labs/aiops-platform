package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/cluster"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
	"k8s-aiops.local/backend/internal/promotion"
	"k8s-aiops.local/backend/internal/requestctx"
)

type promotionHandler struct{ service *promotion.Service }

type promotionBundleItemRequest struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

type promotionDependencyMappingRequest struct {
	Kind                 string `json:"kind"`
	SourceNamespace      string `json:"source_namespace"`
	SourceName           string `json:"source_name"`
	DestinationNamespace string `json:"destination_namespace"`
	DestinationName      string `json:"destination_name"`
}

type previewPromotionRequest struct {
	SourceClusterID      int64                               `json:"source_cluster_id"`
	DestinationClusterID int64                               `json:"destination_cluster_id"`
	SourceNamespace      string                              `json:"source_namespace"`
	DestinationNamespace string                              `json:"destination_namespace"`
	Bundle               []promotionBundleItemRequest        `json:"bundle"`
	DependencyMappings   []promotionDependencyMappingRequest `json:"dependency_mappings,omitempty"`
}

type executePromotionRequest struct {
	ConfirmationToken string `json:"confirmation_token" binding:"required"`
}

func (h promotionHandler) preview(c *gin.Context) {
	var request previewPromotionRequest
	if err := decodeStrictJSON(c, &request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "request must contain only the promotion preview fields")
		return
	}
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	setAuditTarget(c, "PromotionPlan", strings.TrimSpace(request.SourceNamespace), strings.TrimSpace(request.DestinationNamespace))
	previewRequest := promotion.PreviewRequest{
		SourceClusterID:      request.SourceClusterID,
		DestinationClusterID: request.DestinationClusterID,
		SourceNamespace:      strings.TrimSpace(request.SourceNamespace),
		DestinationNamespace: strings.TrimSpace(request.DestinationNamespace),
		Bundle:               make([]promotion.BundleItemRequest, 0, len(request.Bundle)),
		DependencyMappings:   make([]promotion.DependencyMapping, 0, len(request.DependencyMappings)),
	}
	for _, item := range request.Bundle {
		previewRequest.Bundle = append(previewRequest.Bundle, promotion.BundleItemRequest{
			Kind:      strings.TrimSpace(item.Kind),
			Namespace: strings.TrimSpace(item.Namespace),
			Name:      strings.TrimSpace(item.Name),
		})
	}
	for _, mapping := range request.DependencyMappings {
		previewRequest.DependencyMappings = append(previewRequest.DependencyMappings, promotion.DependencyMapping{
			Kind:                 strings.TrimSpace(mapping.Kind),
			SourceNamespace:      strings.TrimSpace(mapping.SourceNamespace),
			SourceName:           strings.TrimSpace(mapping.SourceName),
			DestinationNamespace: strings.TrimSpace(mapping.DestinationNamespace),
			DestinationName:      strings.TrimSpace(mapping.DestinationName),
		})
	}
	plan, err := h.service.Preview(c.Request.Context(), previewRequest, promotion.ActorRef{ID: metadata.ActorID, Name: metadata.ActorDisplayName})
	if plan.ID != "" {
		setAuditTarget(c, "PromotionPlan", plan.SourceNamespace, plan.ID)
		setAuditClusterID(c, plan.DestinationClusterID)
	}
	if err == nil {
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusCreated, plan)
		return
	}
	h.writeError(c, err, "unable to preview promotion")
}

func (h promotionHandler) execute(c *gin.Context) {
	id := strings.TrimSpace(c.Param("promotion_id"))
	if len(id) != 36 {
		writeError(c, http.StatusBadRequest, "INVALID_PROMOTION_ID", "promotion_id must be a valid plan identifier")
		return
	}
	var request executePromotionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "confirmation_token is required")
		return
	}
	idempotencyKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	setAuditTarget(c, "PromotionPlan", "", id)
	plan, err := h.service.Execute(c.Request.Context(), id, request.ConfirmationToken, idempotencyKey)
	if plan.DestinationClusterID > 0 {
		setAuditClusterID(c, plan.DestinationClusterID)
	}
	if err == nil {
		c.JSON(http.StatusOK, plan)
		return
	}
	h.writeError(c, err, "unable to execute promotion")
}

func (h promotionHandler) get(c *gin.Context) {
	id := strings.TrimSpace(c.Param("promotion_id"))
	if len(id) != 36 {
		writeError(c, http.StatusBadRequest, "INVALID_PROMOTION_ID", "promotion_id must be a valid plan identifier")
		return
	}
	plan, err := h.service.Get(c.Request.Context(), id)
	if err == nil {
		c.JSON(http.StatusOK, plan)
		return
	}
	h.writeError(c, err, "unable to read promotion plan")
}

func (h promotionHandler) list(c *gin.Context) {
	sourceClusterID, ok := parseClusterQuery(c, "source_cluster_id")
	if !ok {
		return
	}
	namespace := strings.TrimSpace(c.Query("namespace"))
	plans, err := h.service.List(c.Request.Context(), sourceClusterID, namespace)
	if err != nil {
		h.writeError(c, err, "unable to list promotion plans")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": plans, "total": len(plans)})
}

func (promotionHandler) writeError(c *gin.Context, err error, fallback string) {
	switch {
	case errors.Is(err, promotion.ErrNotFound):
		writeError(c, http.StatusNotFound, "PROMOTION_NOT_FOUND", "promotion plan does not exist")
	case errors.Is(err, promotion.ErrInvalidRequest):
		writeError(c, http.StatusBadRequest, "INVALID_PROMOTION", "promotion request is invalid")
	case errors.Is(err, promotion.ErrBundleEmpty):
		writeError(c, http.StatusBadRequest, "PROMOTION_BUNDLE_EMPTY", "promotion bundle must include at least one resource")
	case errors.Is(err, promotion.ErrSourceUnavailable):
		writeError(c, http.StatusConflict, "PROMOTION_SOURCE_UNAVAILABLE", "promotion source cluster or resource is unavailable")
	case errors.Is(err, promotion.ErrDestinationUnavailable):
		writeError(c, http.StatusConflict, "PROMOTION_DESTINATION_UNAVAILABLE", "promotion destination cluster is unavailable")
	case errors.Is(err, promotion.ErrNamespaceMissing):
		writeError(c, http.StatusConflict, "PROMOTION_NAMESPACE_MISSING", "destination namespace does not exist")
	case errors.Is(err, promotion.ErrDependencyUnresolved):
		writeError(c, http.StatusConflict, "PROMOTION_DEPENDENCY_UNRESOLVED", "a referenced ConfigMap or Secret has no valid destination mapping")
	case errors.Is(err, promotion.ErrConflict):
		writeError(c, http.StatusConflict, "PROMOTION_CONFLICT", "destination resource already exists")
	case errors.Is(err, promotion.ErrPreviewFailed):
		writeError(c, http.StatusBadRequest, "PROMOTION_PREVIEW_FAILED", "server-side dry-run rejected the promotion manifest")
	case errors.Is(err, promotion.ErrConfirmationInvalid):
		writeError(c, http.StatusForbidden, "PROMOTION_CONFIRMATION_INVALID", "confirmation token is invalid")
	case errors.Is(err, promotion.ErrInvalidIdempotency):
		writeError(c, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key must contain 8 to 128 characters")
	case errors.Is(err, promotion.ErrExpired):
		writeError(c, http.StatusGone, "PROMOTION_EXPIRED", "promotion plan has expired")
	case errors.Is(err, promotion.ErrInProgress):
		writeError(c, http.StatusConflict, "PROMOTION_IN_PROGRESS", "promotion is already executing")
	case errors.Is(err, promotion.ErrAlreadyExecuted):
		writeError(c, http.StatusConflict, "PROMOTION_ALREADY_USED", "promotion plan was already used with another idempotency key")
	case errors.Is(err, promotion.ErrExecutionFailed):
		writeError(c, http.StatusBadGateway, "PROMOTION_FAILED", "Kubernetes API rejected or failed the promotion")
	case errors.Is(err, cluster.ErrDisabled):
		writeError(c, http.StatusConflict, "CLUSTER_DISABLED", "cluster must be enabled before promotion")
	case errors.Is(err, cluster.ErrNotFound):
		writeError(c, http.StatusNotFound, "CLUSTER_NOT_FOUND", "cluster does not exist")
	case errors.Is(err, k8sgateway.ErrResourceNotFound):
		writeError(c, http.StatusNotFound, "RESOURCE_NOT_FOUND", "Kubernetes promotion source does not exist")
	case errors.Is(err, k8sgateway.ErrResourceConflict):
		writeError(c, http.StatusConflict, "PROMOTION_CONFLICT", "destination resource already exists")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", fallback)
	}
}

func parseClusterQuery(c *gin.Context, key string) (int64, bool) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", key+" query parameter is required")
		return 0, false
	}
	value, err := parseJSONInt(raw)
	if err != nil || value <= 0 {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", key+" must be a positive integer")
		return 0, false
	}
	return value, true
}

func parseJSONInt(raw string) (int64, error) {
	decoder := json.NewDecoder(io.LimitReader(strings.NewReader(raw), 32))
	var value int64
	if err := decoder.Decode(&value); err != nil {
		return 0, err
	}
	return value, nil
}
