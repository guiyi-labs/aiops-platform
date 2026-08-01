package httpserver

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/aiinvestigator"
	"k8s-aiops.local/backend/internal/requestctx"
)

// aiInvestigatorHandler exposes the M43 AI investigator as read-only query
// endpoints plus one generation endpoint. Generation is the only write; it
// persists an investigation but never modifies the case, diagnosis or alert.
//
// Routes:
//
//	GET  /api/v1/aiops/investigator/runbooks                — list runbook catalog
//	GET  /api/v1/aiops/investigator/cases/:case_id/investigations — list investigations for a case
//	GET  /api/v1/aiops/investigator/investigations/:id      — get one investigation
//	POST /api/v1/aiops/investigator/cases/:case_id/investigations — generate a new investigation
type aiInvestigatorHandler struct {
	service *aiinvestigator.Service
}

// listRunbooks handles GET /api/v1/aiops/investigator/runbooks.
// Returns the server-owned runbook catalog.
func (h aiInvestigatorHandler) listRunbooks(c *gin.Context) {
	runbooks := h.service.ListRunbooks()
	c.JSON(http.StatusOK, gin.H{
		"items":                runbooks,
		"investigator_version": aiinvestigator.InvestigatorVersion,
	})
}

// listInvestigations handles GET /api/v1/aiops/investigator/cases/:case_id/investigations.
func (h aiInvestigatorHandler) listInvestigations(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "INVESTIGATOR_UNAVAILABLE", "ai investigator service is not configured")
		return
	}
	caseID, ok := parseInvestigatorCaseID(c)
	if !ok {
		return
	}
	limit := 100
	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 || n > 200 {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "limit must be a positive integer <= 200")
			return
		}
		limit = n
	}
	resp, err := h.service.ListByCase(c.Request.Context(), caseID, limit)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INVESTIGATOR_QUERY_FAILED", "failed to list investigations")
		return
	}
	c.JSON(http.StatusOK, resp)
}

// getInvestigation handles GET /api/v1/aiops/investigator/investigations/:id.
func (h aiInvestigatorHandler) getInvestigation(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "INVESTIGATOR_UNAVAILABLE", "ai investigator service is not configured")
		return
	}
	id, ok := parseInvestigatorID(c)
	if !ok {
		return
	}
	inv, err := h.service.GetInvestigation(c.Request.Context(), id)
	if err != nil {
		writeInvestigatorError(c, err)
		return
	}
	c.JSON(http.StatusOK, inv)
}

// generateInvestigation handles POST /api/v1/aiops/investigator/cases/:case_id/investigations.
// Generates a new cited investigation for the case. The actor is derived from
// the authenticated session. On provider failure, a failed investigation is
// persisted with failure_reason set.
func (h aiInvestigatorHandler) generateInvestigation(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "INVESTIGATOR_UNAVAILABLE", "ai investigator service is not configured")
		return
	}
	caseID, ok := parseInvestigatorCaseID(c)
	if !ok {
		return
	}
	meta, _ := requestctx.MetadataFrom(c.Request.Context())
	actor := aiinvestigator.ActorRef{ID: meta.ActorID, Name: meta.ActorName}
	inv, err := h.service.Investigate(c.Request.Context(), caseID, actor)
	if err != nil {
		if errors.Is(err, aiinvestigator.ErrCaseNotFound) {
			writeError(c, http.StatusNotFound, "CASE_NOT_FOUND", "correlation case not found")
			return
		}
		if errors.Is(err, aiinvestigator.ErrDisabled) {
			writeError(c, http.StatusServiceUnavailable, "INVESTIGATOR_DISABLED", "ai investigator is disabled")
			return
		}
		// Provider/validation failures still persist a failed investigation;
		// return it with 200 so the caller sees the failure_reason.
		c.JSON(http.StatusOK, inv)
		return
	}
	c.JSON(http.StatusOK, inv)
}

// parseInvestigatorCaseID parses a positive int64 :case_id path parameter.
func parseInvestigatorCaseID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("case_id"), 10, 64)
	if err != nil || id < 1 {
		writeError(c, http.StatusBadRequest, "INVALID_CASE_ID", "case_id must be a positive integer")
		return 0, false
	}
	return id, true
}

// parseInvestigatorID parses a positive int64 :id path parameter.
func parseInvestigatorID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(c, http.StatusBadRequest, "INVALID_ID", "id must be a positive integer")
		return 0, false
	}
	return id, true
}

// writeInvestigatorError maps investigator service errors to stable HTTP responses.
func writeInvestigatorError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, aiinvestigator.ErrInvestigationNotFound):
		writeError(c, http.StatusNotFound, "INVESTIGATION_NOT_FOUND", "ai investigation not found")
	default:
		writeError(c, http.StatusInternalServerError, "INVESTIGATOR_INTERNAL_ERROR", err.Error())
	}
}
