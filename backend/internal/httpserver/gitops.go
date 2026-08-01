package httpserver

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/apiquery"
	"k8s-aiops.local/backend/internal/gitops"
)

type gitopsHandler struct {
	service *gitops.Service
}

func (h gitopsHandler) capability(c *gin.Context) {
	clusterID, ok := clusterID(c)
	if !ok {
		return
	}
	cap, err := h.service.Capability(c.Request.Context(), clusterID)
	if err != nil {
		h.writeError(c, err, "unable to probe GitOps capability")
		return
	}
	setAuditClusterID(c, clusterID)
	c.JSON(http.StatusOK, cap)
}

func (h gitopsHandler) list(c *gin.Context) {
	clusterID, ok := clusterID(c)
	if !ok {
		return
	}
	query, err := apiquery.Parse(c.Request, "name")
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", err.Error())
		return
	}
	resp, err := h.service.List(c.Request.Context(), clusterID, query)
	if err != nil {
		h.writeError(c, err, "unable to list GitOps applications")
		return
	}
	setAuditClusterID(c, clusterID)
	c.JSON(http.StatusOK, gin.H{"items": resp.Items, "total": resp.Total, "remaining": resp.Remaining})
}

func (h gitopsHandler) get(c *gin.Context) {
	clusterID, ok := clusterID(c)
	if !ok {
		return
	}
	name := strings.TrimSpace(c.Param("name"))
	if name == "" {
		writeError(c, http.StatusBadRequest, "INVALID_NAME", "application name is required")
		return
	}
	app, err := h.service.Get(c.Request.Context(), clusterID, name)
	if err != nil {
		h.writeError(c, err, "unable to fetch GitOps application")
		return
	}
	setAuditClusterID(c, clusterID)
	setAuditTarget(c, "GitOpsApplication", "", name)
	c.JSON(http.StatusOK, app)
}

func (h gitopsHandler) writeError(c *gin.Context, err error, fallback string) {
	switch {
	case errors.Is(err, gitops.ErrInvalidRequest):
		writeError(c, http.StatusBadRequest, "INVALID_GITOPS_REQUEST", "gitops request parameters are invalid")
	case errors.Is(err, gitops.ErrGitOpsUnavailable):
		writeError(c, http.StatusServiceUnavailable, "GITOPS_UNAVAILABLE", "ArgoCD is not installed on the target cluster")
	case errors.Is(err, gitops.ErrNotFound):
		writeError(c, http.StatusNotFound, "GITOPS_APPLICATION_NOT_FOUND", "gitops application not found")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", fallback)
	}
}
