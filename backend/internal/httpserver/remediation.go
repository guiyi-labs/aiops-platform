package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/cluster"
	"k8s-aiops.local/backend/internal/diagnosis"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
	"k8s-aiops.local/backend/internal/remediation"
	"k8s-aiops.local/backend/internal/requestctx"
)

type remediationHandler struct{ service *remediation.Service }

type previewRemediationRequest struct {
	Action     string `json:"action" binding:"required"`
	TargetName string `json:"target_name" binding:"required"`
}

type previewOperationRequest struct {
	Action           string `json:"action"`
	Namespace        string `json:"namespace"`
	TargetName       string `json:"target_name"`
	DesiredReplicas  *int32 `json:"desired_replicas,omitempty"`
	ContainerName    string `json:"container_name,omitempty"`
	DesiredImage     string `json:"desired_image,omitempty"`
	RollbackRevision *int32 `json:"rollback_revision,omitempty"`
}

type executeRemediationRequest struct {
	ConfirmationToken string `json:"confirmation_token" binding:"required"`
}

func (h remediationHandler) preview(c *gin.Context) {
	diagnosisID, ok := diagnosisID(c)
	if !ok {
		return
	}
	var request previewRemediationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "action and target_name are required")
		return
	}
	setAuditTarget(c, "RemediationPlan", "", strconv.FormatInt(diagnosisID, 10))
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	plan, err := h.service.Preview(c.Request.Context(), diagnosisID, strings.TrimSpace(request.Action), strings.TrimSpace(request.TargetName), remediation.ActorRef{ID: metadata.ActorID, Name: metadata.ActorDisplayName})
	if plan.ClusterID > 0 {
		setAuditClusterID(c, plan.ClusterID)
	}
	if plan.ID != "" {
		setAuditTarget(c, "RemediationPlan", plan.TargetNamespace, plan.ID)
	}
	if err == nil {
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusCreated, remediation.Response(plan))
		return
	}
	h.writeError(c, err, "unable to preview remediation")
}

func (h remediationHandler) list(c *gin.Context) {
	diagnosisID, ok := diagnosisID(c)
	if !ok {
		return
	}
	plans, err := h.service.List(c.Request.Context(), diagnosisID)
	if err != nil {
		h.writeError(c, err, "unable to list remediations")
		return
	}
	items := make([]remediation.PlanResponse, 0, len(plans))
	for _, plan := range plans {
		items = append(items, remediation.Response(plan))
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": len(items), "remaining": 0})
}

func (h remediationHandler) previewOperation(c *gin.Context) {
	var request previewOperationRequest
	if err := decodeStrictJSON(c, &request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "request must contain only the fixed operation fields")
		return
	}
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	setAuditTarget(c, "ControlledOperation", strings.TrimSpace(request.Namespace), strings.TrimSpace(request.TargetName))
	plan, err := h.service.PreviewOperation(c.Request.Context(), currentClusterID(c), remediation.OperationRequest{
		Action: strings.TrimSpace(request.Action), Namespace: strings.TrimSpace(request.Namespace),
		TargetName: strings.TrimSpace(request.TargetName), DesiredReplicas: request.DesiredReplicas,
		ContainerName: strings.TrimSpace(request.ContainerName), DesiredImage: strings.TrimSpace(request.DesiredImage),
		RollbackRevision: request.RollbackRevision,
	}, remediation.ActorRef{ID: metadata.ActorID, Name: metadata.ActorDisplayName})
	if plan.ID != "" {
		setAuditTarget(c, "RemediationPlan", plan.TargetNamespace, plan.ID)
	}
	if err == nil {
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusCreated, remediation.Response(plan))
		return
	}
	h.writeError(c, err, "unable to preview controlled operation")
}

func (h remediationHandler) rolloutHistory(c *gin.Context) {
	namespace := strings.TrimSpace(c.Param("namespace"))
	name := strings.TrimSpace(c.Param("name"))
	history, err := h.service.RolloutHistory(c.Request.Context(), currentClusterID(c), namespace, name)
	if err == nil {
		c.JSON(http.StatusOK, history)
		return
	}
	h.writeError(c, err, "unable to read rollout history")
}

func (h remediationHandler) rolloutStatus(c *gin.Context) {
	namespace := strings.TrimSpace(c.Param("namespace"))
	name := strings.TrimSpace(c.Param("name"))
	status, err := h.service.RolloutStatus(c.Request.Context(), currentClusterID(c), namespace, name)
	if err == nil {
		c.JSON(http.StatusOK, status)
		return
	}
	h.writeError(c, err, "unable to read rollout status")
}

func (h remediationHandler) listOperations(c *gin.Context) {
	plans, err := h.service.ListOperations(c.Request.Context(), currentClusterID(c), c.Query("namespace"), c.Query("target_kind"), c.Query("target_name"))
	if err != nil {
		h.writeError(c, err, "unable to list controlled operations")
		return
	}
	items := make([]remediation.PlanResponse, 0, len(plans))
	for _, plan := range plans {
		items = append(items, remediation.Response(plan))
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": len(items), "remaining": 0})
}

func (h remediationHandler) execute(c *gin.Context) {
	id := strings.TrimSpace(c.Param("remediation_id"))
	if len(id) != 36 {
		writeError(c, http.StatusBadRequest, "INVALID_REMEDIATION_ID", "remediation_id must be a valid plan identifier")
		return
	}
	var request executeRemediationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "confirmation_token is required")
		return
	}
	idempotencyKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	setAuditTarget(c, "RemediationPlan", "", id)
	plan, err := h.service.Execute(c.Request.Context(), id, request.ConfirmationToken, idempotencyKey)
	if plan.ClusterID > 0 {
		setAuditClusterID(c, plan.ClusterID)
	}
	if err == nil {
		c.JSON(http.StatusOK, remediation.Response(plan))
		return
	}
	h.writeError(c, err, "unable to execute remediation")
}

func (remediationHandler) writeError(c *gin.Context, err error, fallback string) {
	switch {
	case errors.Is(err, diagnosis.ErrRecordNotFound):
		writeError(c, http.StatusNotFound, "DIAGNOSIS_NOT_FOUND", "diagnosis record does not exist")
	case errors.Is(err, remediation.ErrNotFound):
		writeError(c, http.StatusNotFound, "REMEDIATION_NOT_FOUND", "diagnosis or remediation plan does not exist")
	case errors.Is(err, remediation.ErrUnsupportedAction):
		writeError(c, http.StatusBadRequest, "UNSUPPORTED_REMEDIATION", "the requested action is not in the controlled operations catalog")
	case errors.Is(err, remediation.ErrInvalidOperation):
		writeError(c, http.StatusBadRequest, "INVALID_OPERATION", "controlled operation parameters are invalid")
	case errors.Is(err, remediation.ErrOperationNoChange):
		writeError(c, http.StatusConflict, "OPERATION_NO_CHANGE", "the target already has the requested value")
	case errors.Is(err, remediation.ErrRevisionNotFound):
		writeError(c, http.StatusNotFound, "REVISION_NOT_FOUND", "the requested rollout revision does not exist")
	case errors.Is(err, remediation.ErrDiagnosisNotEligible):
		writeError(c, http.StatusConflict, "DIAGNOSIS_NOT_ELIGIBLE", "only confirmed Pod diagnoses can be remediated")
	case errors.Is(err, remediation.ErrTargetMismatch):
		writeError(c, http.StatusConflict, "REMEDIATION_TARGET_MISMATCH", "deployment selector does not match the diagnosed Pod")
	case errors.Is(err, remediation.ErrTargetChanged):
		writeError(c, http.StatusConflict, "REMEDIATION_TARGET_CHANGED", "the diagnosed or target resource changed")
	case errors.Is(err, remediation.ErrConfirmationInvalid):
		writeError(c, http.StatusForbidden, "REMEDIATION_CONFIRMATION_INVALID", "confirmation token is invalid")
	case errors.Is(err, remediation.ErrInvalidIdempotency):
		writeError(c, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key must contain 8 to 128 characters")
	case errors.Is(err, remediation.ErrExpired):
		writeError(c, http.StatusGone, "REMEDIATION_EXPIRED", "remediation plan has expired")
	case errors.Is(err, remediation.ErrInProgress):
		writeError(c, http.StatusConflict, "REMEDIATION_IN_PROGRESS", "remediation is already executing")
	case errors.Is(err, remediation.ErrAlreadyExecuted):
		writeError(c, http.StatusConflict, "REMEDIATION_ALREADY_USED", "remediation plan was already used with another idempotency key")
	case errors.Is(err, cluster.ErrDisabled):
		writeError(c, http.StatusConflict, "CLUSTER_DISABLED", "cluster must be enabled before remediation")
	case errors.Is(err, cluster.ErrNotFound):
		writeError(c, http.StatusNotFound, "CLUSTER_NOT_FOUND", "cluster does not exist")
	case errors.Is(err, k8sgateway.ErrResourceNotFound):
		writeError(c, http.StatusNotFound, "RESOURCE_NOT_FOUND", "Kubernetes remediation target does not exist")
	case errors.Is(err, remediation.ErrExecutionFailed):
		writeError(c, http.StatusBadGateway, "REMEDIATION_FAILED", "Kubernetes API rejected or failed the remediation")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", fallback)
	}
}

func decodeStrictJSON(c *gin.Context, target any) error {
	decoder := json.NewDecoder(io.LimitReader(c.Request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("request must contain one JSON object")
	}
	return nil
}
