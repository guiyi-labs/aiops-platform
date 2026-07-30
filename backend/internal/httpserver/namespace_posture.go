package httpserver

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/apiquery"
	"k8s-aiops.local/backend/internal/cluster"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
	"k8s-aiops.local/backend/internal/namespaceposture"
)

type namespacePostureHandler struct {
	service *namespaceposture.Service
}

func (h namespacePostureHandler) writeServiceError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, cluster.ErrNotFound):
		writeError(c, http.StatusNotFound, "CLUSTER_NOT_FOUND", "cluster does not exist")
	case errors.Is(err, cluster.ErrDisabled):
		writeError(c, http.StatusConflict, "CLUSTER_DISABLED", "cluster must be enabled before querying posture")
	case errors.Is(err, k8sgateway.ErrResourceNotFound):
		writeError(c, http.StatusNotFound, "NAMESPACE_NOT_FOUND", "namespace does not exist on the target cluster")
	default:
		writeError(c, http.StatusBadGateway, "NAMESPACE_POSTURE_ERROR", "unable to aggregate namespace posture")
	}
	return true
}

// list returns the compact summary-row posture for every Namespace matching
// the query. This endpoint is bounded and intended for navigation tables;
// call `get` for a single Namespace's full evidence-cited posture.
func (h namespacePostureHandler) list(c *gin.Context) {
	query, err := apiquery.Parse(c.Request, "name")
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", err.Error())
		return
	}
	response, err := h.service.List(c.Request.Context(), currentClusterID(c), query)
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, response)
	}
}

// get returns the full evidence-cited posture for one Namespace, including
// ResourceQuota, LimitRange, Workload, Pod, PDB and cluster-wide Node
// capacity sections. Every section carries its own EvidenceCitation so
// partial failures and truncation are explicit.
func (h namespacePostureHandler) get(c *gin.Context) {
	namespace := c.Param("namespace")
	if namespace == "" {
		writeError(c, http.StatusBadRequest, "INVALID_NAMESPACE", "namespace path parameter is required")
		return
	}
	posture, err := h.service.Get(c.Request.Context(), currentClusterID(c), namespace)
	if !h.writeServiceError(c, err) {
		c.JSON(http.StatusOK, posture)
	}
}
