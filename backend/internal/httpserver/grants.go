package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/authz"
	"k8s-aiops.local/backend/internal/requestctx"
)

type grantHandler struct {
	manager *authz.GrantManager
}

type clusterGrantCreate struct {
	ClusterID int64 `json:"cluster_id" binding:"required"`
}

type namespaceGrantCreate struct {
	ClusterID int64  `json:"cluster_id" binding:"required"`
	Namespace string `json:"namespace" binding:"required"`
}

func parseUserID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("user_id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(c, http.StatusBadRequest, "INVALID_USER_ID", "user_id must be a positive integer")
		return 0, false
	}
	return id, true
}

func (h grantHandler) listClusterGrants(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}
	grants, err := h.manager.ListClusterGrants(c.Request.Context(), userID)
	if !h.writeGrantError(c, err) {
		c.JSON(http.StatusOK, gin.H{"items": grants})
	}
}

func (h grantHandler) createClusterGrant(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}
	var request clusterGrantCreate
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	grant, err := h.manager.CreateClusterGrant(c.Request.Context(), userID, request.ClusterID)
	if !h.writeGrantError(c, err) {
		setAuditTarget(c, "ClusterGrant", "", "")
		setAuditClusterID(c, request.ClusterID)
		c.JSON(http.StatusCreated, grant)
	}
}

func (h grantHandler) deleteClusterGrant(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}
	clusterID, err := strconv.ParseInt(c.Param("cluster_id"), 10, 64)
	if err != nil || clusterID <= 0 {
		writeError(c, http.StatusBadRequest, "INVALID_CLUSTER_ID", "cluster_id must be a positive integer")
		return
	}
	err = h.manager.DeleteClusterGrant(c.Request.Context(), userID, clusterID)
	if !h.writeGrantError(c, err) {
		setAuditTarget(c, "ClusterGrant", "", "")
		setAuditClusterID(c, clusterID)
		c.Status(http.StatusNoContent)
	}
}

func (h grantHandler) listNamespaceGrants(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}
	grants, err := h.manager.ListNamespaceGrants(c.Request.Context(), userID)
	if !h.writeGrantError(c, err) {
		c.JSON(http.StatusOK, gin.H{"items": grants})
	}
}

func (h grantHandler) createNamespaceGrant(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}
	var request namespaceGrantCreate
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	namespace := strings.TrimSpace(request.Namespace)
	if namespace == "" {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "namespace must not be empty")
		return
	}
	grant, err := h.manager.CreateNamespaceGrant(c.Request.Context(), userID, request.ClusterID, namespace)
	if !h.writeGrantError(c, err) {
		setAuditTarget(c, "NamespaceGrant", namespace, "")
		setAuditClusterID(c, request.ClusterID)
		c.JSON(http.StatusCreated, grant)
	}
}

func (h grantHandler) deleteNamespaceGrant(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}
	clusterID, err := strconv.ParseInt(c.Param("cluster_id"), 10, 64)
	if err != nil || clusterID <= 0 {
		writeError(c, http.StatusBadRequest, "INVALID_CLUSTER_ID", "cluster_id must be a positive integer")
		return
	}
	namespace := c.Param("namespace")
	if namespace == "" {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "namespace must not be empty")
		return
	}
	err = h.manager.DeleteNamespaceGrant(c.Request.Context(), userID, clusterID, namespace)
	if !h.writeGrantError(c, err) {
		setAuditTarget(c, "NamespaceGrant", namespace, "")
		setAuditClusterID(c, clusterID)
		c.Status(http.StatusNoContent)
	}
}

// myGrants returns the current user's cluster and namespace grants. This lets
// the frontend show which clusters/namespaces the user can access without
// exposing grants belonging to other users.
func (h grantHandler) myGrants(c *gin.Context) {
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	clusterGrants, err := h.manager.ListClusterGrants(c.Request.Context(), metadata.ActorID)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}
	namespaceGrants, err := h.manager.ListNamespaceGrants(c.Request.Context(), metadata.ActorID)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"cluster_grants":   clusterGrants,
		"namespace_grants": namespaceGrants,
	})
}

func (h grantHandler) writeGrantError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, authz.ErrGrantAlreadyExists):
		writeError(c, http.StatusConflict, "GRANT_EXISTS", "access grant already exists")
	case errors.Is(err, authz.ErrGrantNotFound):
		writeError(c, http.StatusNotFound, "GRANT_NOT_FOUND", "access grant not found")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	return true
}
