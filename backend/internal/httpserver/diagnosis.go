package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/auth"
	"k8s-aiops.local/backend/internal/cluster"
	"k8s-aiops.local/backend/internal/diagnosis"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
	"k8s-aiops.local/backend/internal/metricshistory"
	"k8s-aiops.local/backend/internal/requestctx"
)

type diagnosisHandler struct {
	service *diagnosis.Service
	users   *auth.Service
}
type diagnoseRequest struct {
	ResourceKind string `json:"resource_kind" binding:"required"`
	Namespace    string `json:"namespace"`
	Name         string `json:"name" binding:"required"`
}
type diagnoseNodeMetricsRequest struct {
	Name          string `json:"name" binding:"required"`
	Metric        string `json:"metric" binding:"required"`
	Operator      string `json:"operator" binding:"required"`
	Threshold     int64  `json:"threshold" binding:"required"`
	ForSeconds    int    `json:"for_seconds" binding:"required"`
	MinimumPoints int    `json:"minimum_points"`
}
type transitionDiagnosisRequest struct {
	Status  string `json:"status" binding:"required"`
	Comment string `json:"comment"`
}
type diagnosisFeedbackRequest struct {
	Verdict string `json:"verdict" binding:"required"`
	Comment string `json:"comment"`
}
type diagnosisAssignmentRequest struct {
	AssigneeUserID int64  `json:"assignee_user_id" binding:"required"`
	Comment        string `json:"comment"`
}

func (h diagnosisHandler) create(c *gin.Context) {
	var request diagnoseRequest
	if err := c.ShouldBindJSON(&request); err != nil || strings.TrimSpace(request.ResourceKind) == "" || strings.TrimSpace(request.Name) == "" {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "resource_kind and name are required")
		return
	}
	request.ResourceKind = strings.TrimSpace(request.ResourceKind)
	request.Namespace = strings.TrimSpace(request.Namespace)
	request.Name = strings.TrimSpace(request.Name)
	if !strings.EqualFold(request.ResourceKind, "Node") && request.Namespace == "" {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "namespace is required for namespaced resources")
		return
	}
	setAuditTarget(c, request.ResourceKind, request.Namespace, request.Name)
	var record diagnosis.Record
	var err error
	switch {
	case strings.EqualFold(request.ResourceKind, "Pod"):
		record, err = h.service.DiagnosePod(c.Request.Context(), currentClusterID(c), request.Namespace, request.Name)
	case strings.EqualFold(request.ResourceKind, "Service"):
		record, err = h.service.DiagnoseService(c.Request.Context(), currentClusterID(c), request.Namespace, request.Name)
	case strings.EqualFold(request.ResourceKind, "Node"):
		record, err = h.service.DiagnoseNode(c.Request.Context(), currentClusterID(c), request.Name)
	case strings.EqualFold(request.ResourceKind, "Deployment"):
		record, err = h.service.DiagnoseDeployment(c.Request.Context(), currentClusterID(c), request.Namespace, request.Name)
	case strings.EqualFold(request.ResourceKind, "Ingress"):
		record, err = h.service.DiagnoseIngress(c.Request.Context(), currentClusterID(c), request.Namespace, request.Name)
	case strings.EqualFold(request.ResourceKind, "PersistentVolumeClaim"), strings.EqualFold(request.ResourceKind, "PVC"):
		record, err = h.service.DiagnosePersistentVolumeClaim(c.Request.Context(), currentClusterID(c), request.Namespace, request.Name)
	case strings.EqualFold(request.ResourceKind, "HorizontalPodAutoscaler"), strings.EqualFold(request.ResourceKind, "HPA"):
		record, err = h.service.DiagnoseHorizontalPodAutoscaler(c.Request.Context(), currentClusterID(c), request.Namespace, request.Name)
	default:
		writeError(c, http.StatusBadRequest, "UNSUPPORTED_RESOURCE", "supported diagnosis resources are Pod, Service, Node, Deployment, Ingress, PersistentVolumeClaim and HorizontalPodAutoscaler")
		return
	}
	if err == nil {
		c.JSON(http.StatusCreated, record)
		return
	}
	switch {
	case errors.Is(err, diagnosis.ErrNoRuleMatch):
		writeError(c, http.StatusUnprocessableEntity, "NO_RULE_MATCH", "no enabled diagnosis rule matched this resource")
	case errors.Is(err, cluster.ErrDisabled):
		writeError(c, http.StatusConflict, "CLUSTER_DISABLED", "cluster must be enabled before diagnosis")
	case errors.Is(err, cluster.ErrNotFound):
		writeError(c, http.StatusNotFound, "CLUSTER_NOT_FOUND", "cluster does not exist")
	case errors.Is(err, k8sgateway.ErrResourceNotFound):
		writeError(c, http.StatusNotFound, "RESOURCE_NOT_FOUND", "Kubernetes resource does not exist")
	default:
		writeError(c, http.StatusBadGateway, "DIAGNOSIS_FAILED", "unable to collect diagnosis evidence")
	}
}

func (h diagnosisHandler) diagnoseNodeMetrics(c *gin.Context) {
	var request diagnoseNodeMetricsRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "name, metric, operator, threshold and for_seconds are required")
		return
	}
	request.Name = strings.TrimSpace(request.Name)
	request.Metric = strings.TrimSpace(request.Metric)
	request.Operator = strings.TrimSpace(request.Operator)
	if request.Name == "" || request.Metric == "" || request.Operator == "" {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "name, metric and operator cannot be empty")
		return
	}
	if request.Operator != metricshistory.OperatorGreaterThanOrEqual && request.Operator != metricshistory.OperatorLessThanOrEqual {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "operator must be gte or lte")
		return
	}
	if request.ForSeconds < 60 || request.ForSeconds > 86400 {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "for_seconds must be between 60 and 86400")
		return
	}
	minimumPoints := request.MinimumPoints
	if minimumPoints < 2 {
		minimumPoints = 2
	}
	if minimumPoints > 1440 {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "minimum_points must not exceed 1440")
		return
	}
	var internalMetric string
	switch request.Metric {
	case "node_cpu":
		internalMetric = metricshistory.MetricCPU
	case "node_memory":
		internalMetric = metricshistory.MetricMemory
	default:
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "metric must be node_cpu or node_memory")
		return
	}
	setAuditTarget(c, metricshistory.ResourceNode, "", request.Name)
	record, err := h.service.DiagnoseNodeMetrics(c.Request.Context(), currentClusterID(c), request.Name, internalMetric, metricshistory.EvaluationRule{
		Operator: request.Operator, Threshold: request.Threshold, ForSeconds: request.ForSeconds, MinimumPoints: minimumPoints,
	})
	if err == nil {
		c.JSON(http.StatusCreated, record)
		return
	}
	switch {
	case errors.Is(err, diagnosis.ErrNoRuleMatch):
		writeError(c, http.StatusUnprocessableEntity, "NO_RULE_MATCH", "no sustained metric breach was detected in the evaluation window")
	case errors.Is(err, cluster.ErrDisabled):
		writeError(c, http.StatusConflict, "CLUSTER_DISABLED", "cluster must be enabled before diagnosis")
	case errors.Is(err, cluster.ErrNotFound):
		writeError(c, http.StatusNotFound, "CLUSTER_NOT_FOUND", "cluster does not exist")
	case errors.Is(err, metricshistory.ErrInvalidQuery), errors.Is(err, metricshistory.ErrInvalidEvaluation):
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "the evaluation rule or metric series query is invalid")
	case errors.Is(err, metricshistory.ErrClusterNotFound):
		writeError(c, http.StatusNotFound, "CLUSTER_NOT_FOUND", "cluster does not exist")
	default:
		writeError(c, http.StatusBadGateway, "DIAGNOSIS_FAILED", "unable to collect diagnosis evidence")
	}
}

func (h diagnosisHandler) get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("diagnosis_id"), 10, 64)
	if err != nil || id < 1 {
		writeError(c, http.StatusBadRequest, "INVALID_DIAGNOSIS_ID", "diagnosis_id must be a positive integer")
		return
	}
	record, err := h.service.Get(c.Request.Context(), id)
	if err == nil {
		c.JSON(http.StatusOK, record)
		return
	}
	if errors.Is(err, diagnosis.ErrRecordNotFound) {
		writeError(c, http.StatusNotFound, "DIAGNOSIS_NOT_FOUND", "diagnosis record does not exist")
		return
	}
	writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to read diagnosis")
}

func (h diagnosisHandler) transition(c *gin.Context) {
	id, ok := diagnosisID(c)
	if !ok {
		return
	}
	setAuditTarget(c, "Diagnosis", "", strconv.FormatInt(id, 10))
	var request transitionDiagnosisRequest
	if err := c.ShouldBindJSON(&request); err != nil || len(request.Comment) > 2000 {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "status is required and comment must not exceed 2000 characters")
		return
	}
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	record, err := h.service.Transition(c.Request.Context(), id, request.Status, diagnosis.ActorRef{ID: metadata.ActorID, Name: metadata.ActorDisplayName}, strings.TrimSpace(request.Comment))
	if err == nil {
		c.JSON(http.StatusOK, record)
		return
	}
	switch {
	case errors.Is(err, diagnosis.ErrRecordNotFound):
		writeError(c, http.StatusNotFound, "DIAGNOSIS_NOT_FOUND", "diagnosis record does not exist")
	case errors.Is(err, diagnosis.ErrInvalidTransition):
		writeError(c, http.StatusConflict, "INVALID_STATUS_TRANSITION", "the requested diagnosis status transition is not allowed")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to update diagnosis")
	}
}

func (h diagnosisHandler) feedback(c *gin.Context) {
	id, ok := diagnosisID(c)
	if !ok {
		return
	}
	setAuditTarget(c, "Diagnosis", "", strconv.FormatInt(id, 10))
	var request diagnosisFeedbackRequest
	if err := c.ShouldBindJSON(&request); err != nil || len(request.Comment) > 2000 {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "verdict is required and comment must not exceed 2000 characters")
		return
	}
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	record, err := h.service.AddFeedback(c.Request.Context(), id, request.Verdict, diagnosis.ActorRef{ID: metadata.ActorID, Name: metadata.ActorDisplayName}, strings.TrimSpace(request.Comment))
	if err == nil {
		c.JSON(http.StatusCreated, record)
		return
	}
	switch {
	case errors.Is(err, diagnosis.ErrRecordNotFound):
		writeError(c, http.StatusNotFound, "DIAGNOSIS_NOT_FOUND", "diagnosis record does not exist")
	case errors.Is(err, diagnosis.ErrInvalidFeedback):
		writeError(c, http.StatusBadRequest, "INVALID_FEEDBACK", "verdict must be accurate, inaccurate or uncertain")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to save diagnosis feedback")
	}
}

func (h diagnosisHandler) assign(c *gin.Context) {
	id, ok := diagnosisID(c)
	if !ok {
		return
	}
	setAuditTarget(c, "Diagnosis", "", strconv.FormatInt(id, 10))
	var request diagnosisAssignmentRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.AssigneeUserID < 1 || len(request.Comment) > 2000 {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "assignee_user_id is required and comment must not exceed 2000 characters")
		return
	}
	user, err := h.users.AssignableUser(c.Request.Context(), request.AssigneeUserID)
	if err != nil {
		writeError(c, http.StatusBadRequest, "ASSIGNEE_NOT_ALLOWED", "assignee must be an active system or operations administrator")
		return
	}
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	record, err := h.service.Assign(c.Request.Context(), id, diagnosis.ActorRef{ID: user.ID, Name: user.DisplayName}, diagnosis.ActorRef{ID: metadata.ActorID, Name: metadata.ActorDisplayName}, strings.TrimSpace(request.Comment))
	if err == nil {
		c.JSON(http.StatusOK, record)
		return
	}
	switch {
	case errors.Is(err, diagnosis.ErrRecordNotFound):
		writeError(c, http.StatusNotFound, "DIAGNOSIS_NOT_FOUND", "diagnosis record does not exist")
	case errors.Is(err, diagnosis.ErrAlreadyAssigned):
		writeError(c, http.StatusConflict, "ALREADY_ASSIGNED", "diagnosis is already assigned to this user")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to assign diagnosis")
	}
}

func (h diagnosisHandler) summary(c *gin.Context) {
	summary, err := h.service.Summary(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to summarize diagnoses")
		return
	}
	c.JSON(http.StatusOK, summary)
}

func diagnosisID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("diagnosis_id"), 10, 64)
	if err != nil || id < 1 {
		writeError(c, http.StatusBadRequest, "INVALID_DIAGNOSIS_ID", "diagnosis_id must be a positive integer")
		return 0, false
	}
	return id, true
}

func (h diagnosisHandler) list(c *gin.Context) {
	clusterID, err := strconv.ParseInt(defaultString(c.Query("cluster_id"), "0"), 10, 64)
	if err != nil || clusterID < 0 {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "cluster_id must be a positive integer")
		return
	}
	limit, err := strconv.Atoi(defaultString(c.Query("limit"), "50"))
	if err != nil || limit < 1 || limit > 100 {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "limit must be between 1 and 100")
		return
	}
	status := strings.TrimSpace(c.Query("status"))
	if status != "" && status != "open" && status != "confirmed" && status != "resolved" && status != "dismissed" {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "status must be open, confirmed, resolved or dismissed")
		return
	}
	var overdue *bool
	if raw := c.Query("overdue"); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "overdue must be true or false")
			return
		}
		overdue = &value
	}
	items, err := h.service.List(c.Request.Context(), diagnosis.ListFilter{ClusterID: clusterID, Status: status, Overdue: overdue, Limit: limit})
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to list diagnoses")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": len(items), "remaining": 0})
}
