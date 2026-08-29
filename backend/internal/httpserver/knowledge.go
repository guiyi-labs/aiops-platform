package httpserver

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/knowledge"
)

// knowledgeHandler exposes the P1 RAG case library as read-only query
// endpoints. Writes happen exclusively through the diagnosis resolution hook
// (diagnosis.KnowledgeIngester); there is intentionally no HTTP write surface.
//
// Routes:
//
//	GET /api/v1/aiops/knowledge         — list/filter distilled cases
//	GET /api/v1/aiops/knowledge/stats   — total count (library observability)
type knowledgeHandler struct {
	repo knowledge.Repository
}

// listKnowledge handles GET /api/v1/aiops/knowledge.
//
// Query params (all optional):
//
//	rule_id, severity, resource_kind, min_severity, limit.
//
// severity filters exactly; min_severity keeps entries at or above the given
// rank (info < warning < high < critical). limit is clamped to [1,100] with a
// default of 20; truncation is disclosed via the response envelope.
func (h knowledgeHandler) listKnowledge(c *gin.Context) {
	if h.repo == nil {
		writeError(c, http.StatusServiceUnavailable, "KNOWLEDGE_UNAVAILABLE", "knowledge service is not configured")
		return
	}
	filter := knowledge.Filter{}
	filter.RuleID = c.Query("rule_id")
	filter.ResourceKind = c.Query("resource_kind")
	switch {
	case c.Query("severity") != "" && c.Query("min_severity") != "":
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "use either severity or min_severity, not both")
		return
	case c.Query("severity") != "":
		sev := knowledge.Severity(c.Query("severity"))
		if _, ok := knowledge.SeverityRank[string(sev)]; !ok {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "severity must be one of info|warning|high|critical")
			return
		}
		filter.Severity = string(sev)
	case c.Query("min_severity") != "":
		minSev := knowledge.Severity(c.Query("min_severity"))
		rank, ok := knowledge.SeverityRank[string(minSev)]
		if !ok {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "min_severity must be one of info|warning|high|critical")
			return
		}
		_ = rank
		filter.MinSeverity = string(minSev)
	}
	limit := 20
	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 || n > 100 {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "limit must be an integer in [1,100]")
			return
		}
		limit = n
	}
	filter.Limit = limit

	resp, err := h.repo.ListByFilter(c.Request.Context(), filter)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "KNOWLEDGE_QUERY_FAILED", "failed to list knowledge entries")
		return
	}
	c.JSON(http.StatusOK, resp)
}

// knowledgeStats handles GET /api/v1/aiops/knowledge/stats.
func (h knowledgeHandler) knowledgeStats(c *gin.Context) {
	if h.repo == nil {
		writeError(c, http.StatusServiceUnavailable, "KNOWLEDGE_UNAVAILABLE", "knowledge service is not configured")
		return
	}
	count, err := h.repo.Count(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "KNOWLEDGE_QUERY_FAILED", "failed to count knowledge entries")
		return
	}
	c.JSON(http.StatusOK, gin.H{"total": count})
}
