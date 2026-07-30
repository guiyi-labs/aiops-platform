package httpserver

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/alert"
	"k8s-aiops.local/backend/internal/auth"
	"k8s-aiops.local/backend/internal/requestctx"
)

type alertHandler struct {
	service *alert.Service
	users   *auth.Service
}

func (h *alertHandler) createRule(c *gin.Context) {
	cid, ok := clusterID(c)
	if !ok {
		return
	}
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	var input struct {
		DisplayName   string `json:"display_name" binding:"required"`
		ResourceKind  string `json:"resource_kind" binding:"required"`
		ResourceName  string `json:"resource_name" binding:"required"`
		MetricName    string `json:"metric_name" binding:"required"`
		Operator      string `json:"operator" binding:"required"`
		Threshold     int64  `json:"threshold" binding:"required"`
		ForSeconds    int    `json:"for_seconds" binding:"required"`
		MinimumPoints int    `json:"minimum_points" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	rule, err := h.service.CreateRule(c.Request.Context(), alert.CreateRuleInput{
		ClusterID:     cid,
		DisplayName:   input.DisplayName,
		ResourceKind:  input.ResourceKind,
		ResourceName:  input.ResourceName,
		MetricName:    input.MetricName,
		Operator:      input.Operator,
		Threshold:     input.Threshold,
		ForSeconds:    input.ForSeconds,
		MinimumPoints: input.MinimumPoints,
	}, alert.ActorRef{ID: metadata.ActorID, Name: metadata.ActorDisplayName})
	if err != nil {
		switch err {
		case alert.ErrInvalidRule:
			writeError(c, http.StatusBadRequest, "INVALID_RULE", err.Error())
		case alert.ErrClusterLimit:
			writeError(c, http.StatusConflict, "CLUSTER_LIMIT", err.Error())
		case alert.ErrDuplicateName:
			writeError(c, http.StatusConflict, "DUPLICATE_NAME", err.Error())
		default:
			writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		}
		return
	}
	c.JSON(http.StatusCreated, rule)
}

func (h *alertHandler) listRules(c *gin.Context) {
	cid, ok := clusterID(c)
	if !ok {
		return
	}
	rules, err := h.service.ListRules(c.Request.Context(), alert.RuleListFilter{
		ClusterID: cid,
		Limit:     100,
	})
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	if rules == nil {
		rules = []alert.Rule{}
	}
	c.JSON(http.StatusOK, rules)
}

func (h *alertHandler) getRule(c *gin.Context) {
	cid, ok := clusterID(c)
	if !ok {
		return
	}
	ruleID, err := strconv.ParseInt(c.Param("rule_id"), 10, 64)
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_RULE_ID", "rule_id must be a positive integer")
		return
	}
	rule, err := h.service.GetRule(c.Request.Context(), cid, ruleID)
	if err != nil {
		if err == alert.ErrRuleNotFound {
			writeError(c, http.StatusNotFound, "NOT_FOUND", err.Error())
			return
		}
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	c.JSON(http.StatusOK, rule)
}

func (h *alertHandler) patchRule(c *gin.Context) {
	cid, ok := clusterID(c)
	if !ok {
		return
	}
	ruleID, err := strconv.ParseInt(c.Param("rule_id"), 10, 64)
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_RULE_ID", "rule_id must be a positive integer")
		return
	}
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	var input alert.PatchRuleInput
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	rule, err := h.service.PatchRule(c.Request.Context(), cid, ruleID, input, alert.ActorRef{ID: metadata.ActorID, Name: metadata.ActorDisplayName})
	if err != nil {
		switch err {
		case alert.ErrRuleNotFound, alert.ErrRuleDeleted:
			writeError(c, http.StatusNotFound, "NOT_FOUND", err.Error())
		case alert.ErrInvalidRule:
			writeError(c, http.StatusBadRequest, "INVALID_RULE", err.Error())
		case alert.ErrDuplicateName:
			writeError(c, http.StatusConflict, "DUPLICATE_NAME", err.Error())
		default:
			writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		}
		return
	}
	c.JSON(http.StatusOK, rule)
}

func (h *alertHandler) deleteRule(c *gin.Context) {
	cid, ok := clusterID(c)
	if !ok {
		return
	}
	ruleID, err := strconv.ParseInt(c.Param("rule_id"), 10, 64)
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_RULE_ID", "rule_id must be a positive integer")
		return
	}
	if err := h.service.DeleteRule(c.Request.Context(), cid, ruleID); err != nil {
		switch err {
		case alert.ErrRuleNotFound, alert.ErrRuleDeleted:
			writeError(c, http.StatusNotFound, "NOT_FOUND", err.Error())
		case alert.ErrRuleUnresolvedAlert:
			writeError(c, http.StatusConflict, "UNRESOLVED_ALERT", err.Error())
		default:
			writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		}
		return
	}
	c.JSON(http.StatusNoContent, nil)
}

func (h *alertHandler) listInstances(c *gin.Context) {
	cid, ok := clusterID(c)
	if !ok {
		return
	}
	filter := alert.InstanceListFilter{
		ClusterID: cid,
		State:     c.Query("state"),
		Limit:     50,
	}
	if ruleID, err := strconv.ParseInt(c.Query("rule_id"), 10, 64); err == nil {
		filter.RuleID = ruleID
	}
	if limit, err := strconv.Atoi(c.Query("limit")); err == nil && limit > 0 && limit <= 100 {
		filter.Limit = limit
	}
	instances, err := h.service.ListInstances(c.Request.Context(), filter)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	if instances == nil {
		instances = []alert.Instance{}
	}
	c.JSON(http.StatusOK, instances)
}

func (h *alertHandler) getInstance(c *gin.Context) {
	cid, ok := clusterID(c)
	if !ok {
		return
	}
	alertID, err := strconv.ParseInt(c.Param("alert_id"), 10, 64)
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_ALERT_ID", "alert_id must be a positive integer")
		return
	}
	instance, err := h.service.GetInstance(c.Request.Context(), cid, alertID)
	if err != nil {
		if err == alert.ErrAlertNotFound {
			writeError(c, http.StatusNotFound, "NOT_FOUND", err.Error())
			return
		}
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	c.JSON(http.StatusOK, instance)
}
