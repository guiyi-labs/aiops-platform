package httpserver

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/correlation"
	"k8s-aiops.local/backend/internal/incident"
)

// correlationHandler exposes the M42 correlation service as read-only query
// endpoints. Case correlation is an internal operation (the service is called
// by background workers or signal-ingestion hooks); the HTTP surface is
// case, timeline, graph, action-candidate and rule-catalog queries only.
//
// Routes:
//
//	GET /api/v1/aiops/correlation/rules               — list correlation rule catalog
//	GET /api/v1/aiops/correlation/cases               — list cases
//	GET /api/v1/aiops/correlation/cases/timeline      — case timeline
//	GET /api/v1/aiops/correlation/cases/:id           — get full case view
//	GET /api/v1/aiops/correlation/cases/:id/graph     — case impact graph (resource links)
//	GET /api/v1/aiops/correlation/cases/:id/actions   — action candidates
type correlationHandler struct {
	service *correlation.Service
	// incidentBySource optionally resolves the incident linked to a
	// correlation source ref (correlation:<id>). Read-only enrichment; when
	// nil or the incident is missing, the case view simply omits it.
	incidentBySource func(ctx context.Context, sourceRef string) (*incident.Incident, error)
}

// correlationCaseViewResponse is the case view enriched with the linked
// incident workspace (M108 bidirectional deep link).
type correlationCaseViewResponse struct {
	correlation.CaseView
	Incident *incidentSummary `json:"incident,omitempty"`
}

// incidentSummary is the minimal incident reference embedded in the case view.
type incidentSummary struct {
	ID     int64  `json:"id"`
	Number string `json:"number"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

// listCorrelationRules handles GET /api/v1/aiops/correlation/rules.
// Returns the server-owned correlation rule catalog.
func (h correlationHandler) listCorrelationRules(c *gin.Context) {
	rules := correlation.AllRules()
	c.JSON(http.StatusOK, gin.H{
		"items":               rules,
		"correlation_version": correlation.CorrelationVersion,
	})
}

// listCorrelationCases handles GET /api/v1/aiops/correlation/cases.
//
// Query params (cluster_id required; rest optional):
//
//	cluster_id  (required) — target cluster
//	namespace   (optional) — filter by primary namespace
//	rule_id     (optional) — filter by correlation rule
//	status      (optional) — active|resolved|stale
//	confidence  (optional) — confirmed|candidate|contradicted|unknown
//	primary_kind(optional) — filter by primary resource kind
//	start       (optional) — RFC3339 last_observed_at >= start
//	end         (optional) — RFC3339 first_observed_at <= end
//	limit       (optional) — default 100, max 200
func (h correlationHandler) listCorrelationCases(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "CORRELATION_UNAVAILABLE", "correlation service is not configured")
		return
	}
	filter, ok := parseCaseFilter(c)
	if !ok {
		return
	}
	resp, err := h.service.ListCases(c.Request.Context(), filter)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "CORRELATION_QUERY_FAILED", "failed to list correlation cases")
		return
	}
	c.JSON(http.StatusOK, resp)
}

// listCorrelationTimeline handles GET /api/v1/aiops/correlation/cases/timeline.
//
// Same query params as listCorrelationCases. Returns cases ordered by
// first_observed_at ASC for the timeline view.
func (h correlationHandler) listCorrelationTimeline(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "CORRELATION_UNAVAILABLE", "correlation service is not configured")
		return
	}
	filter, ok := parseCaseFilter(c)
	if !ok {
		return
	}
	resp, err := h.service.ListTimeline(c.Request.Context(), filter)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "CORRELATION_QUERY_FAILED", "failed to query correlation timeline")
		return
	}
	c.JSON(http.StatusOK, resp)
}

// getCorrelationCase handles GET /api/v1/aiops/correlation/cases/:id.
// Returns the full case view (case + signal links + resource links + change candidates).
func (h correlationHandler) getCorrelationCase(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "CORRELATION_UNAVAILABLE", "correlation service is not configured")
		return
	}
	id, ok := parseCorrelationID(c)
	if !ok {
		return
	}
	view, err := h.service.GetCase(c.Request.Context(), id)
	if err != nil {
		writeCorrelationError(c, err)
		return
	}
	resp := correlationCaseViewResponse{CaseView: view}
	if h.incidentBySource != nil {
		sourceRef := incident.SourceRefForCorrelation(id)
		if inc, err := h.incidentBySource(c.Request.Context(), sourceRef); err == nil {
			resp.Incident = &incidentSummary{ID: inc.ID, Number: inc.Number, Title: inc.Title, Status: inc.Status}
		}
	}
	c.JSON(http.StatusOK, resp)
}

// getCorrelationCaseGraph handles GET /api/v1/aiops/correlation/cases/:id/graph.
// Returns the resource links (impact graph) for one case.
func (h correlationHandler) getCorrelationCaseGraph(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "CORRELATION_UNAVAILABLE", "correlation service is not configured")
		return
	}
	id, ok := parseCorrelationID(c)
	if !ok {
		return
	}
	links, err := h.service.GetCaseGraph(c.Request.Context(), id)
	if err != nil {
		writeCorrelationError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": links, "total": len(links)})
}

// listCorrelationActions handles GET /api/v1/aiops/correlation/cases/:id/actions.
// Returns the fixed, read-only action candidates for one case.
func (h correlationHandler) listCorrelationActions(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "CORRELATION_UNAVAILABLE", "correlation service is not configured")
		return
	}
	id, ok := parseCorrelationID(c)
	if !ok {
		return
	}
	resp, err := h.service.ListActionCandidates(c.Request.Context(), id)
	if err != nil {
		writeCorrelationError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

// parseCorrelationID parses a positive int64 :id path parameter.
func parseCorrelationID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(c, http.StatusBadRequest, "INVALID_ID", "id must be a positive integer")
		return 0, false
	}
	return id, true
}

// parseCaseFilter parses the shared case filter query params.
func parseCaseFilter(c *gin.Context) (correlation.CaseFilter, bool) {
	clusterIDStr := c.Query("cluster_id")
	if clusterIDStr == "" {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "cluster_id is required")
		return correlation.CaseFilter{}, false
	}
	clusterID, err := strconv.ParseInt(clusterIDStr, 10, 64)
	if err != nil || clusterID <= 0 {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "cluster_id must be a positive integer")
		return correlation.CaseFilter{}, false
	}
	filter := correlation.CaseFilter{ClusterID: clusterID}
	if v := c.Query("namespace"); v != "" {
		filter.Namespace = v
	}
	if v := c.Query("rule_id"); v != "" {
		filter.RuleID = v
	}
	if v := c.Query("status"); v != "" {
		filter.Status = correlation.CaseStatus(v)
	}
	if v := c.Query("confidence"); v != "" {
		filter.Confidence = correlation.ConfidenceClass(v)
	}
	if v := c.Query("primary_kind"); v != "" {
		filter.PrimaryKind = v
	}
	if v := c.Query("primary_uid"); v != "" {
		filter.PrimaryUID = v
	}
	if v := c.Query("start"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "start must be RFC3339")
			return correlation.CaseFilter{}, false
		}
		filter.StartTime = &t
	}
	if v := c.Query("end"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "end must be RFC3339")
			return correlation.CaseFilter{}, false
		}
		filter.EndTime = &t
	}
	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "limit must be a positive integer")
			return correlation.CaseFilter{}, false
		}
		filter.Limit = n
	}
	return filter, true
}

// writeCorrelationError maps correlation service errors to stable HTTP responses.
func writeCorrelationError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, correlation.ErrCaseNotFound):
		writeError(c, http.StatusNotFound, "CASE_NOT_FOUND", "correlation case not found")
	default:
		writeError(c, http.StatusInternalServerError, "CORRELATION_INTERNAL_ERROR", err.Error())
	}
}
