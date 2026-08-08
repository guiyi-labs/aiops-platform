package httpserver

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/insight"
)

// insightHandler exposes the M81 closed-loop endpoint. It maps a posture
// finding to a deterministic runbook (diagnosis routes, corroborating
// inspection rules, AI explanation entry point, dry-run operation
// candidates). The handler is a pure lookup: it never touches a cluster and
// never mutates state (ADR 0004), so it is safe for any authenticated user.
type insightHandler struct{}

// runbookRequest carries the finding identity from the query string.
type runbookRequest struct {
	ClusterID int64  `form:"cluster_id"`
	Domain    string `form:"domain"`
	Code      string `form:"code"`
	Kind      string `form:"kind"`
	Namespace string `form:"namespace"`
	Name      string `form:"name"`
}

// runbook resolves the closed-loop runbook for one posture finding.
func (h insightHandler) runbook(c *gin.Context) {
	var req runbookRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "query parameters are invalid")
		return
	}
	req.Domain = strings.TrimSpace(req.Domain)
	req.Kind = strings.TrimSpace(req.Kind)
	req.Name = strings.TrimSpace(req.Name)
	req.Namespace = strings.TrimSpace(req.Namespace)
	if req.ClusterID < 1 {
		writeError(c, http.StatusBadRequest, "INVALID_CLUSTER", "cluster_id must be a positive integer")
		return
	}
	if req.Domain == "" || req.Kind == "" || req.Name == "" {
		writeError(c, http.StatusBadRequest, "INVALID_FINDING", "domain, kind and name are required to resolve a runbook")
		return
	}
	rb := insight.Resolve(req.ClusterID, req.Domain, req.Kind, req.Namespace, req.Name, req.Code)
	c.JSON(http.StatusOK, rb)
}
