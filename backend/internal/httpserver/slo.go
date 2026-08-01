package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/requestctx"
	"k8s-aiops.local/backend/internal/slo"
)

// sloHandler exposes the M41 SLO service: definition CRUD, evaluation
// trigger and evaluation history. All write operations require SystemOpsAdmin
// role; reads are available to any authenticated user (cluster/namespace
// scope is enforced by M35 middleware on the underlying cluster resources
// and by the service's cluster_id binding at create time).
//
// Routes:
//
//	GET    /api/v1/aiops/slos                   — list definitions
//	POST   /api/v1/aiops/slos                   — create definition
//	GET    /api/v1/aiops/slos/templates         — list SLI templates
//	GET    /api/v1/aiops/slos/:id               — get definition
//	PATCH  /api/v1/aiops/slos/:id               — update definition
//	DELETE /api/v1/aiops/slos/:id               — disable definition
//	POST   /api/v1/aiops/slos/:id/evaluate      — run one evaluation
//	GET    /api/v1/aiops/slos/:id/evaluations   — list evaluations
type sloHandler struct {
	service *slo.Service
}

// parseSLOID parses a positive int64 :id path parameter.
func parseSLOID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(c, http.StatusBadRequest, "INVALID_ID", "id must be a positive integer")
		return 0, false
	}
	return id, true
}

// listSLODefinitions handles GET /api/v1/aiops/slos.
//
// Query params (all optional):
//
//	cluster_id (optional) — filter by cluster
//	namespace  (optional) — filter by service namespace
//	template   (optional) — filter by SLI template
//	enabled    (optional) — "true" or "false"
//	owner_id   (optional) — filter by owner
//	limit      (optional) — default 100, max 200
func (h sloHandler) listSLODefinitions(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "SLO_UNAVAILABLE", "slo service is not configured")
		return
	}
	filter := slo.DefinitionFilter{}
	if v := c.Query("cluster_id"); v != "" {
		cid, err := strconv.ParseInt(v, 10, 64)
		if err != nil || cid <= 0 {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "cluster_id must be a positive integer")
			return
		}
		filter.ClusterID = cid
	}
	if v := c.Query("namespace"); v != "" {
		filter.Namespace = v
	}
	if v := c.Query("template"); v != "" {
		filter.Template = slo.SLITemplate(v)
	}
	if v := c.Query("enabled"); v != "" {
		switch strings.ToLower(v) {
		case "true":
			b := true
			filter.Enabled = &b
		case "false":
			b := false
			filter.Enabled = &b
		default:
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "enabled must be true or false")
			return
		}
	}
	if v := c.Query("owner_id"); v != "" {
		oid, err := strconv.ParseInt(v, 10, 64)
		if err != nil || oid <= 0 {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "owner_id must be a positive integer")
			return
		}
		filter.OwnerID = oid
	}
	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "limit must be a positive integer")
			return
		}
		filter.Limit = n
	}
	resp, err := h.service.ListDefinitions(c.Request.Context(), filter)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "SLO_QUERY_FAILED", "failed to list slo definitions")
		return
	}
	c.JSON(http.StatusOK, resp)
}

// listSLITemplates handles GET /api/v1/aiops/slos/templates.
// Returns the server-owned SLI template catalog.
func (h sloHandler) listSLITemplates(c *gin.Context) {
	templates := slo.AllTemplates()
	c.JSON(http.StatusOK, gin.H{"items": templates, "template_version": slo.TemplateVersion})
}

type sloCreateRequest struct {
	ClusterID             int64   `json:"cluster_id" binding:"required"`
	ServiceKind           string  `json:"service_kind" binding:"required"`
	ServiceNamespace      string  `json:"service_namespace"`
	ServiceName           string  `json:"service_name" binding:"required"`
	ServiceUID            string  `json:"service_uid"`
	ServiceIncomplete     bool    `json:"service_incomplete"`
	Template              string  `json:"template" binding:"required"`
	Objective             float64 `json:"objective" binding:"required"`
	RollingWindowSeconds  int     `json:"rolling_window_seconds" binding:"required"`
	MissingDataPolicy     string  `json:"missing_data_policy"`
	LatencyThresholdMs    int     `json:"latency_threshold_ms"`
	OwnerID               int64   `json:"owner_id" binding:"required"`
	OwnerName             string  `json:"owner_name"`
	FastBurnRate          float64 `json:"fast_burn_rate" binding:"required"`
	FastBurnWindowSeconds int     `json:"fast_burn_window_seconds" binding:"required"`
	SlowBurnRate          float64 `json:"slow_burn_rate" binding:"required"`
	SlowBurnWindowSeconds int     `json:"slow_burn_window_seconds" binding:"required"`
	Enabled               bool    `json:"enabled"`
}

// createSLODefinition handles POST /api/v1/aiops/slos.
func (h sloHandler) createSLODefinition(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "SLO_UNAVAILABLE", "slo service is not configured")
		return
	}
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	var req sloCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	input := slo.CreateDefinitionInput{
		ClusterID: req.ClusterID,
		Service: slo.ServiceRef{
			Kind:       req.ServiceKind,
			Namespace:  req.ServiceNamespace,
			Name:       req.ServiceName,
			UID:        req.ServiceUID,
			Incomplete: req.ServiceIncomplete,
		},
		Template:              slo.SLITemplate(req.Template),
		Objective:             req.Objective,
		RollingWindowSeconds:  req.RollingWindowSeconds,
		MissingDataPolicy:     slo.MissingDataPolicy(req.MissingDataPolicy),
		LatencyThresholdMs:    req.LatencyThresholdMs,
		Owner:                 slo.ActorRef{ID: req.OwnerID, Name: firstNonEmpty(req.OwnerName, metadata.ActorDisplayName, metadata.ActorName)},
		FastBurnRate:          req.FastBurnRate,
		FastBurnWindowSeconds: req.FastBurnWindowSeconds,
		SlowBurnRate:          req.SlowBurnRate,
		SlowBurnWindowSeconds: req.SlowBurnWindowSeconds,
		Enabled:               req.Enabled,
		Creator:               slo.ActorRef{ID: metadata.ActorID, Name: firstNonEmpty(metadata.ActorDisplayName, metadata.ActorName)},
	}
	def, err := h.service.CreateDefinition(c.Request.Context(), input)
	if err != nil {
		writeSLOError(c, err)
		return
	}
	setAuditTarget(c, "SLODefinition", "", strconv.FormatInt(def.ID, 10))
	c.JSON(http.StatusCreated, def)
}

// getSLODefinition handles GET /api/v1/aiops/slos/:id.
func (h sloHandler) getSLODefinition(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "SLO_UNAVAILABLE", "slo service is not configured")
		return
	}
	id, ok := parseSLOID(c)
	if !ok {
		return
	}
	def, err := h.service.GetDefinition(c.Request.Context(), id)
	if err != nil {
		writeSLOError(c, err)
		return
	}
	c.JSON(http.StatusOK, def)
}

type sloPatchRequest struct {
	Objective             *float64 `json:"objective"`
	RollingWindowSeconds  *int     `json:"rolling_window_seconds"`
	MissingDataPolicy     *string  `json:"missing_data_policy"`
	LatencyThresholdMs    *int     `json:"latency_threshold_ms"`
	OwnerID               *int64   `json:"owner_id"`
	OwnerName             *string  `json:"owner_name"`
	FastBurnRate          *float64 `json:"fast_burn_rate"`
	FastBurnWindowSeconds *int     `json:"fast_burn_window_seconds"`
	SlowBurnRate          *float64 `json:"slow_burn_rate"`
	SlowBurnWindowSeconds *int     `json:"slow_burn_window_seconds"`
	Enabled               *bool    `json:"enabled"`
}

// patchSLODefinition handles PATCH /api/v1/aiops/slos/:id.
func (h sloHandler) patchSLODefinition(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "SLO_UNAVAILABLE", "slo service is not configured")
		return
	}
	id, ok := parseSLOID(c)
	if !ok {
		return
	}
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	var req sloPatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	patch := slo.PatchDefinitionInput{
		Objective:             req.Objective,
		RollingWindowSeconds:  req.RollingWindowSeconds,
		MissingDataPolicy:     (*slo.MissingDataPolicy)(req.MissingDataPolicy),
		LatencyThresholdMs:    req.LatencyThresholdMs,
		FastBurnRate:          req.FastBurnRate,
		FastBurnWindowSeconds: req.FastBurnWindowSeconds,
		SlowBurnRate:          req.SlowBurnRate,
		SlowBurnWindowSeconds: req.SlowBurnWindowSeconds,
		Enabled:               req.Enabled,
		Actor:                 slo.ActorRef{ID: metadata.ActorID, Name: firstNonEmpty(metadata.ActorDisplayName, metadata.ActorName)},
	}
	if req.OwnerID != nil {
		ownerName := ""
		if req.OwnerName != nil {
			ownerName = *req.OwnerName
		}
		patch.Owner = &slo.ActorRef{ID: *req.OwnerID, Name: ownerName}
	}
	updated, err := h.service.PatchDefinition(c.Request.Context(), id, patch)
	if err != nil {
		writeSLOError(c, err)
		return
	}
	setAuditTarget(c, "SLODefinition", "", strconv.FormatInt(updated.ID, 10))
	c.JSON(http.StatusOK, updated)
}

// deleteSLODefinition handles DELETE /api/v1/aiops/slos/:id.
// Marks the definition as disabled (enabled=false). The row is retained.
func (h sloHandler) deleteSLODefinition(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "SLO_UNAVAILABLE", "slo service is not configured")
		return
	}
	id, ok := parseSLOID(c)
	if !ok {
		return
	}
	if err := h.service.DeleteDefinition(c.Request.Context(), id); err != nil {
		writeSLOError(c, err)
		return
	}
	setAuditTarget(c, "SLODefinition", "", strconv.FormatInt(id, 10))
	c.Status(http.StatusNoContent)
}

// evaluateSLO handles POST /api/v1/aiops/slos/:id/evaluate.
// Runs a single deterministic evaluation and persists it. Burn transitions
// are emitted to the configured BurnAlertSink (M27 integration).
func (h sloHandler) evaluateSLO(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "SLO_UNAVAILABLE", "slo service is not configured")
		return
	}
	id, ok := parseSLOID(c)
	if !ok {
		return
	}
	eval, err := h.service.EvaluateSLO(c.Request.Context(), id)
	if err != nil {
		writeSLOError(c, err)
		return
	}
	setAuditTarget(c, "SLOEvaluation", "", strconv.FormatInt(eval.ID, 10))
	c.JSON(http.StatusOK, eval)
}

// listSLOEvaluations handles GET /api/v1/aiops/slos/:id/evaluations.
//
// Query params (all optional):
//
//	version (optional) — filter by SLO version
//	state   (optional) — filter by evaluation state
//	start   (optional) — RFC3339 window_end >= start
//	end     (optional) — RFC3339 window_start <= end
//	limit   (optional) — default 100, max 200
func (h sloHandler) listSLOEvaluations(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "SLO_UNAVAILABLE", "slo service is not configured")
		return
	}
	id, ok := parseSLOID(c)
	if !ok {
		return
	}
	filter := slo.EvaluationFilter{SLOID: id}
	if v := c.Query("version"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "version must be a positive integer")
			return
		}
		filter.Version = &n
	}
	if v := c.Query("state"); v != "" {
		filter.State = slo.EvaluationState(v)
	}
	if v := c.Query("start"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "start must be RFC3339")
			return
		}
		filter.StartTime = &t
	}
	if v := c.Query("end"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "end must be RFC3339")
			return
		}
		filter.EndTime = &t
	}
	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "limit must be a positive integer")
			return
		}
		filter.Limit = n
	}
	resp, err := h.service.ListEvaluations(c.Request.Context(), filter)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "SLO_QUERY_FAILED", "failed to list slo evaluations")
		return
	}
	c.JSON(http.StatusOK, resp)
}

// writeSLOError maps slo service errors to stable HTTP responses.
func writeSLOError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, slo.ErrDefinitionNotFound):
		writeError(c, http.StatusNotFound, "SLO_NOT_FOUND", "slo definition not found")
	case errors.Is(err, slo.ErrDefinitionDisabled):
		writeError(c, http.StatusConflict, "SLO_DISABLED", "slo definition is disabled")
	case errors.Is(err, slo.ErrEvaluatorUnavailable):
		writeError(c, http.StatusServiceUnavailable, "SLO_EVALUATOR_UNAVAILABLE", "slo evaluator is not configured")
	case errors.Is(err, slo.ErrEvaluationInvalidInput):
		writeError(c, http.StatusBadRequest, "SLO_INVALID_INPUT", err.Error())
	case errors.Is(err, slo.ErrDuplicateDefinition):
		writeError(c, http.StatusConflict, "SLO_DUPLICATE", "active slo definition already exists for this service and template")
	default:
		// Validation errors from ValidateCreate/ValidateDefinition come back
		// as plain fmt.Errorf messages; treat them as 400 when they mention
		// "must be" or "required", else 500.
		msg := err.Error()
		if strings.Contains(msg, "must be") || strings.Contains(msg, "required") || strings.Contains(msg, "unsupported") {
			writeError(c, http.StatusBadRequest, "SLO_INVALID_INPUT", msg)
			return
		}
		writeError(c, http.StatusInternalServerError, "SLO_INTERNAL_ERROR", msg)
	}
}
