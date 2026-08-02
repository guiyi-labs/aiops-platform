package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/inspection"
)

// inspectionHandler exposes M52 inspection routes: rule catalog, plan CRUD,
// on-demand runs, task status and result queries. The handler is mounted only
// when Options.InspectionService is non-nil; otherwise the routes remain
// unregistered (router.go gating).
type inspectionHandler struct {
	service *inspection.Service
}

// rolesSystemOpsAdmin is reused from router.go; declared here for audit gating
// on plan mutations (create/update/delete) and on-demand runs.
//
//nolint:unused // reserved for audit gating documentation
var rolesInspectionWriters = rolesSystemOpsAdmin // alias for documentation

// --- catalog (read-only, any authenticated user) ---

func (h inspectionHandler) listRules(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "INSPECTION_UNAVAILABLE", "inspection service is not configured")
		return
	}
	catalog := h.service.Catalog()
	items := make([]gin.H, 0, len(catalog))
	for _, r := range catalog {
		items = append(items, gin.H{
			"code":             r.Code,
			"schema_version":   r.SchemaVersion,
			"domain":           r.Domain,
			"default_severity": r.DefaultSeverity,
			"signal_code":      r.SignalCode,
			"description":      r.Description,
			"remediation":      r.Remediation,
			"timeout_seconds":  int(r.Timeout.Seconds()),
		})
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h inspectionHandler) effectiveRules(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "INSPECTION_UNAVAILABLE", "inspection service is not configured")
		return
	}
	clusterID, ok := clusterID(c)
	if !ok {
		return
	}
	rules, err := h.service.EffectiveRules(c.Request.Context(), clusterID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INSPECTION_RULES_FAILED", "failed to load effective rules")
		return
	}
	items := make([]gin.H, 0, len(rules))
	for _, r := range rules {
		items = append(items, gin.H{
			"code":             r.Code,
			"schema_version":   r.SchemaVersion,
			"domain":           r.Domain,
			"default_severity": r.DefaultSeverity,
			"signal_code":      r.SignalCode,
			"description":      r.Description,
			"remediation":      r.Remediation,
			"timeout_seconds":  int(r.Timeout.Seconds()),
		})
	}
	c.JSON(http.StatusOK, gin.H{"cluster_id": clusterID, "items": items})
}

// --- plans ---

func (h inspectionHandler) listPlans(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "INSPECTION_UNAVAILABLE", "inspection service is not configured")
		return
	}
	filter := inspection.PlanListFilter{CreatorID: pInt64(currentActorID(c))}
	if v := c.Query("enabled"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "enabled must be true or false")
			return
		}
		filter.Enabled = &b
	}
	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "limit must be a positive integer")
			return
		}
		filter.Limit = n
	}
	views, err := h.service.ListPlans(c.Request.Context(), filter)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INSPECTION_PLAN_QUERY_FAILED", "failed to list inspection plans")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": views})
}

type createInspectionPlanRequest struct {
	Name       string   `json:"name" binding:"required,max=128"`
	ClusterIDs []int64  `json:"cluster_ids"`
	RuleCodes  []string `json:"rule_codes"`
	CronSpec   string   `json:"cron_spec" binding:"max=64"`
	Enabled    bool     `json:"enabled"`
}

func (h inspectionHandler) createPlan(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "INSPECTION_UNAVAILABLE", "inspection service is not configured")
		return
	}
	var req createInspectionPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	plan := &inspection.Plan{
		Name:       strings.TrimSpace(req.Name),
		CreatorID:  currentActorID(c),
		ClusterIDs: inspection.Int64Array(append([]int64(nil), req.ClusterIDs...)),
		RuleCodes:  inspection.StringArray(append([]string(nil), req.RuleCodes...)),
		CronSpec:   req.CronSpec,
		Enabled:    req.Enabled,
	}
	created, err := h.service.CreatePlan(c.Request.Context(), plan)
	if err != nil {
		writeInspectionError(c, err)
		return
	}
	setAuditTarget(c, "InspectionPlan", "", strconv.FormatInt(created.ID, 10))
	// Expose the public PlanView projection (JSON contract) rather than the
	// raw GORM model, so the created plan matches GET/PATCH serialization.
	c.JSON(http.StatusCreated, inspection.PlanView{
		ID:         created.ID,
		Name:       created.Name,
		CreatorID:  created.CreatorID,
		ClusterIDs: append([]int64(nil), created.ClusterIDs...),
		RuleCodes:  append([]string(nil), created.RuleCodes...),
		CronSpec:   created.CronSpec,
		Enabled:    created.Enabled,
		LastRunAt:  created.LastRunAt,
		NextRunAt:  created.NextRunAt,
		CreatedAt:  created.CreatedAt,
		UpdatedAt:  created.UpdatedAt,
	})
}

func (h inspectionHandler) getPlan(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "INSPECTION_UNAVAILABLE", "inspection service is not configured")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "plan id must be a positive integer")
		return
	}
	view, err := h.service.GetPlan(c.Request.Context(), id)
	if err != nil {
		writeInspectionError(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

func (h inspectionHandler) deletePlan(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "INSPECTION_UNAVAILABLE", "inspection service is not configured")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "plan id must be a positive integer")
		return
	}
	if err := h.service.DeletePlan(c.Request.Context(), id, currentActorID(c)); err != nil {
		writeInspectionError(c, err)
		return
	}
	setAuditTarget(c, "InspectionPlan", "", strconv.FormatInt(id, 10))
	c.Status(http.StatusNoContent)
}

// --- ad-hoc runs (tasks) ---

type runInspectionRequest struct {
	ClusterIDs []int64  `json:"cluster_ids"` // empty = all reachable
	RuleCodes  []string `json:"rule_codes"`  // empty = all enabled
}

func (h inspectionHandler) runOnce(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "INSPECTION_UNAVAILABLE", "inspection service is not configured")
		return
	}
	var req runInspectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	task, err := h.service.RunInspectOnce(c.Request.Context(), currentActorID(c), req.ClusterIDs, req.RuleCodes)
	if err != nil {
		writeInspectionError(c, err)
		return
	}
	setAuditTarget(c, "InspectionTask", "", strconv.FormatInt(task.ID, 10))
	c.JSON(http.StatusAccepted, task)
}

func (h inspectionHandler) listTasks(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "INSPECTION_UNAVAILABLE", "inspection service is not configured")
		return
	}
	filter := inspection.TaskListFilter{TriggeredBy: pInt64(currentActorID(c))}
	if v := c.Query("plan_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "plan_id must be a positive integer")
			return
		}
		filter.PlanID = &id
	}
	if v := c.Query("status"); v != "" {
		filter.Status = v
	}
	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "limit must be a positive integer")
			return
		}
		filter.Limit = n
	}
	resp, err := h.service.ListTasks(c.Request.Context(), filter)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INSPECTION_TASK_QUERY_FAILED", "failed to list inspection tasks")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": resp.Items, "total": resp.Total})
}

func (h inspectionHandler) getTask(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "INSPECTION_UNAVAILABLE", "inspection service is not configured")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "task id must be a positive integer")
		return
	}
	view, err := h.service.GetTask(c.Request.Context(), id)
	if err != nil {
		writeInspectionError(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

// --- results ---

func (h inspectionHandler) listResults(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "INSPECTION_UNAVAILABLE", "inspection service is not configured")
		return
	}
	filter := inspection.ListFilter{}
	if v := c.Query("task_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "task_id must be a positive integer")
			return
		}
		filter.TaskID = &id
	}
	if v := c.Query("cluster_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "cluster_id must be a positive integer")
			return
		}
		filter.ClusterID = &id
	}
	filter.RuleCode = c.Query("rule_code")
	filter.SignalCode = c.Query("signal_code")
	filter.Severity = c.Query("severity")
	filter.State = c.Query("state")
	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "limit must be a positive integer")
			return
		}
		filter.Limit = n
	}
	if v := c.Query("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "offset must be a non-negative integer")
			return
		}
		filter.Offset = n
	}
	resp, err := h.service.ListResults(c.Request.Context(), filter)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INSPECTION_RESULT_QUERY_FAILED", "failed to list inspection results")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": resp.Items, "total": resp.Total})
}

func (h inspectionHandler) getResult(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "INSPECTION_UNAVAILABLE", "inspection service is not configured")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "result id must be a positive integer")
		return
	}
	view, err := h.service.GetResult(c.Request.Context(), id)
	if err != nil {
		writeInspectionError(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

// --- helpers ---

func writeInspectionError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, inspection.ErrPlanNotFound),
		errors.Is(err, inspection.ErrTaskNotFound),
		errors.Is(err, inspection.ErrResultNotFound):
		writeError(c, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, inspection.ErrInvalidRuleCode),
		errors.Is(err, inspection.ErrInvalidPlan):
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
	case errors.Is(err, inspection.ErrClusterUnreachable):
		writeError(c, http.StatusServiceUnavailable, "INSPECTION_CLUSTERS_UNAVAILABLE", "no reachable clusters to inspect")
	default:
		writeError(c, http.StatusInternalServerError, "INSPECTION_FAILED", "inspection operation failed")
	}
}

func pInt64(v int64) *int64 { return &v }
