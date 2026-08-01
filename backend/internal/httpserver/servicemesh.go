package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
	"k8s-aiops.local/backend/internal/servicemesh"
)

// servicemeshHandler exposes M52 service mesh read-only routes:
// VirtualService list/detail, DestinationRule list/detail, and per-cluster
// traffic metrics. All routes are strictly read-only; there are no write,
// patch or delete endpoints for mesh resources.
type servicemeshHandler struct {
	service *servicemesh.Service
}

// --- VirtualServices ---

func (h servicemeshHandler) listVirtualServices(c *gin.Context) {
	if h.service == nil {
		writeServiceMeshUnavailable(c)
		return
	}
	clusterID, ok := clusterID(c)
	if !ok {
		return
	}
	filter, ok := parseMeshFilter(c)
	if !ok {
		return
	}
	resp, err := h.service.ListVirtualServices(c.Request.Context(), clusterID, filter)
	if err != nil {
		writeServiceMeshError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": resp.Items, "total": resp.Total, "truncated": resp.Truncated})
}

func (h servicemeshHandler) getVirtualService(c *gin.Context) {
	if h.service == nil {
		writeServiceMeshUnavailable(c)
		return
	}
	clusterID, ok := clusterID(c)
	if !ok {
		return
	}
	namespace := c.Param("namespace")
	name := c.Param("name")
	if namespace == "" || name == "" {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "namespace and name are required")
		return
	}
	view, err := h.service.GetVirtualService(c.Request.Context(), clusterID, namespace, name)
	if err != nil {
		writeServiceMeshError(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

// --- DestinationRules ---

func (h servicemeshHandler) listDestinationRules(c *gin.Context) {
	if h.service == nil {
		writeServiceMeshUnavailable(c)
		return
	}
	clusterID, ok := clusterID(c)
	if !ok {
		return
	}
	filter, ok := parseMeshFilter(c)
	if !ok {
		return
	}
	resp, err := h.service.ListDestinationRules(c.Request.Context(), clusterID, filter)
	if err != nil {
		writeServiceMeshError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": resp.Items, "total": resp.Total, "truncated": resp.Truncated})
}

func (h servicemeshHandler) getDestinationRule(c *gin.Context) {
	if h.service == nil {
		writeServiceMeshUnavailable(c)
		return
	}
	clusterID, ok := clusterID(c)
	if !ok {
		return
	}
	namespace := c.Param("namespace")
	name := c.Param("name")
	if namespace == "" || name == "" {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "namespace and name are required")
		return
	}
	view, err := h.service.GetDestinationRule(c.Request.Context(), clusterID, namespace, name)
	if err != nil {
		writeServiceMeshError(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

// --- traffic metrics ---

func (h servicemeshHandler) trafficMetrics(c *gin.Context) {
	if h.service == nil {
		writeServiceMeshUnavailable(c)
		return
	}
	clusterID, ok := clusterID(c)
	if !ok {
		return
	}
	q := servicemesh.TrafficQuery{ClusterID: clusterID}
	q.Namespace = c.Query("namespace")
	q.ServiceName = c.Query("service_name")
	if v := c.Query("window_start"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "window_start must be RFC3339")
			return
		}
		q.WindowStart = t
	}
	if v := c.Query("window_end"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "window_end must be RFC3339")
			return
		}
		q.WindowEnd = t
	}
	if v := c.Query("top_k"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 || n > 100 {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "top_k must be in 1..100")
			return
		}
		q.TopK = n
	}
	metrics, err := h.service.TrafficMetrics(c.Request.Context(), q)
	if err != nil {
		writeServiceMeshError(c, err)
		return
	}
	c.JSON(http.StatusOK, metrics)
}

// --- helpers ---

func parseMeshFilter(c *gin.Context) (servicemesh.ListFilter, bool) {
	var out servicemesh.ListFilter
	out.Namespace = c.Query("namespace")
	out.Name = c.Query("name")
	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "limit must be a positive integer")
			return out, false
		}
		out.Limit = n
	}
	if v := c.Query("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "offset must be non-negative")
			return out, false
		}
		out.Offset = n
	}
	if out.Limit == 0 {
		out.Limit = 50
	}
	return out, true
}

func writeServiceMeshUnavailable(c *gin.Context) {
	writeError(c, http.StatusServiceUnavailable, "SERVICEMESH_UNAVAILABLE", "service mesh service is not configured")
}

func writeServiceMeshError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, servicemesh.ErrIstioNotInstalled):
		writeError(c, http.StatusNotFound, "SERVICEMESH_NOT_INSTALLED", "istio service mesh is not installed on cluster")
	case errors.Is(err, servicemesh.ErrMeshDataUnavailable):
		writeError(c, http.StatusServiceUnavailable, "SERVICEMESH_TRAFFIC_UNAVAILABLE", "service mesh traffic metrics data is unavailable")
	case errors.Is(err, servicemesh.ErrInvalidWindow):
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", err.Error())
	case errors.Is(err, k8sgateway.ErrResourceNotFound):
		writeError(c, http.StatusNotFound, "RESOURCE_NOT_FOUND", "the requested resource does not exist")
	default:
		writeError(c, http.StatusInternalServerError, "SERVICEMESH_FAILED", "service mesh query failed")
	}
}
