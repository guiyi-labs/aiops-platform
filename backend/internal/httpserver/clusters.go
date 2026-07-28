package httpserver

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/cluster"
)

type clusterHandler struct{ service *cluster.Service }

type createClusterRequest struct {
	Name       string `json:"name" binding:"required,max=128"`
	Kubeconfig string `json:"kubeconfig" binding:"required,max=1048576"`
}

type setClusterEnabledRequest struct {
	Enabled *bool `json:"enabled" binding:"required"`
}

type updateClusterCredentialRequest struct {
	Kubeconfig string `json:"kubeconfig" binding:"required,max=1048576"`
}

func (h clusterHandler) list(c *gin.Context) {
	items, err := h.service.List(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to list clusters")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": len(items), "remaining": 0})
}

func (h clusterHandler) get(c *gin.Context) {
	id, ok := clusterID(c)
	if !ok {
		return
	}
	item, err := h.service.Get(c.Request.Context(), id)
	if errors.Is(err, cluster.ErrNotFound) {
		writeError(c, http.StatusNotFound, "CLUSTER_NOT_FOUND", "cluster does not exist")
		return
	}
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to get cluster")
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h clusterHandler) create(c *gin.Context) {
	var request createClusterRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "name and kubeconfig are required")
		return
	}
	setAuditTarget(c, "Cluster", "", request.Name)
	item, err := h.service.Create(c.Request.Context(), request.Name, []byte(request.Kubeconfig))
	if errors.Is(err, cluster.ErrInvalidKubeconfig) {
		writeError(c, http.StatusBadRequest, "INVALID_KUBECONFIG", err.Error())
		return
	}
	if errors.Is(err, cluster.ErrNameRequired) {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if err != nil {
		writeError(c, http.StatusConflict, "CLUSTER_CREATE_FAILED", "cluster name already exists or cannot be saved")
		return
	}
	setAuditClusterID(c, item.ID)
	c.JSON(http.StatusCreated, item)
}

func (h clusterHandler) setEnabled(c *gin.Context) {
	id, ok := clusterID(c)
	if !ok {
		return
	}
	setAuditTarget(c, "Cluster", "", strconv.FormatInt(id, 10))
	var request setClusterEnabledRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.Enabled == nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "enabled is required")
		return
	}
	err := h.service.SetEnabled(c.Request.Context(), id, *request.Enabled)
	if errors.Is(err, cluster.ErrNotFound) {
		writeError(c, http.StatusNotFound, "CLUSTER_NOT_FOUND", "cluster does not exist")
		return
	}
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to update cluster")
		return
	}
	c.Status(http.StatusNoContent)
}

func (h clusterHandler) updateCredential(c *gin.Context) {
	id, ok := clusterID(c)
	if !ok {
		return
	}
	setAuditTarget(c, "ClusterCredential", "", strconv.FormatInt(id, 10))
	var request updateClusterCredentialRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "kubeconfig is required")
		return
	}
	item, err := h.service.UpdateCredential(c.Request.Context(), id, []byte(request.Kubeconfig))
	if errors.Is(err, cluster.ErrInvalidKubeconfig) {
		writeError(c, http.StatusBadRequest, "INVALID_KUBECONFIG", err.Error())
		return
	}
	if errors.Is(err, cluster.ErrNotFound) {
		writeError(c, http.StatusNotFound, "CLUSTER_NOT_FOUND", "cluster does not exist")
		return
	}
	if err != nil {
		writeError(c, http.StatusInternalServerError, "CREDENTIAL_UPDATE_FAILED", "unable to replace cluster credentials")
		return
	}
	setAuditClusterID(c, item.ID)
	setAuditTarget(c, "Cluster", "", item.Name)
	c.JSON(http.StatusOK, item)
}

func (h clusterHandler) probe(c *gin.Context) {
	id, ok := clusterID(c)
	if !ok {
		return
	}
	setAuditTarget(c, "Cluster", "", strconv.FormatInt(id, 10))
	item, err := h.service.Probe(c.Request.Context(), id)
	if errors.Is(err, cluster.ErrNotFound) {
		writeError(c, http.StatusNotFound, "CLUSTER_NOT_FOUND", "cluster does not exist")
		return
	}
	if item.ID != 0 {
		c.JSON(http.StatusOK, item)
		return
	}
	if err != nil {
		writeError(c, http.StatusBadGateway, "CLUSTER_PROBE_FAILED", "unable to contact Kubernetes API")
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h clusterHandler) delete(c *gin.Context) {
	id, ok := clusterID(c)
	if !ok {
		return
	}
	setAuditTarget(c, "Cluster", "", strconv.FormatInt(id, 10))
	err := h.service.Delete(c.Request.Context(), id)
	if errors.Is(err, cluster.ErrNotFound) {
		writeError(c, http.StatusNotFound, "CLUSTER_NOT_FOUND", "cluster does not exist")
		return
	}
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to delete cluster")
		return
	}
	c.Status(http.StatusNoContent)
}

func clusterID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("cluster_id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(c, http.StatusBadRequest, "INVALID_CLUSTER_ID", "cluster_id must be a positive integer")
		return 0, false
	}
	return id, true
}
