package httpserver

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/topology"
)

// topologyHandler exposes the M40 topology service as read-only query
// endpoints. Edge collection and change-event ingestion are internal (the
// collector and normalizer are called by background workers or plan
// completion hooks); the HTTP surface is graph + timeline queries only.
type topologyHandler struct {
	service *topology.Service
}

// getTopologyGraph handles GET /api/v1/aiops/topology/graph.
//
// Query params:
//
//	cluster_id (required) — target cluster
//	namespace  (required) — target namespace
//	limit      (optional)  — max edges, default 200, max 500
//
// Returns the current active topology graph for the namespace, including
// nodes derived from edge endpoints and a completeness indicator.
func (h topologyHandler) getTopologyGraph(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "TOPOLOGY_UNAVAILABLE", "topology service is not configured")
		return
	}

	clusterIDStr := c.Query("cluster_id")
	if clusterIDStr == "" {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "cluster_id is required")
		return
	}
	clusterID, err := strconv.ParseInt(clusterIDStr, 10, 64)
	if err != nil || clusterID <= 0 {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "cluster_id must be a positive integer")
		return
	}

	namespace := c.Query("namespace")
	if namespace == "" {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "namespace is required")
		return
	}

	limit := 200
	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "limit must be a positive integer")
			return
		}
		limit = n
	}

	graph, err := h.service.GetTopologyGraph(c.Request.Context(), clusterID, namespace, limit)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "TOPOLOGY_QUERY_FAILED", "failed to query topology graph")
		return
	}
	c.JSON(http.StatusOK, graph)
}

// listChangeEvents handles GET /api/v1/aiops/topology/changes.
//
// Query params (all optional except cluster_id):
//
//	cluster_id (required) — target cluster
//	namespace  (optional) — filter by namespace
//	kind       (optional) — filter by change kind (promotion|backup|maintenance|restore|rollout|audit)
//	start      (optional) — RFC3339 start time (inclusive)
//	end        (optional) — RFC3339 end time (inclusive)
//	limit      (optional) — max events, default 100, max 200
func (h topologyHandler) listChangeEvents(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "TOPOLOGY_UNAVAILABLE", "topology service is not configured")
		return
	}

	clusterIDStr := c.Query("cluster_id")
	if clusterIDStr == "" {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "cluster_id is required")
		return
	}
	clusterID, err := strconv.ParseInt(clusterIDStr, 10, 64)
	if err != nil || clusterID <= 0 {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "cluster_id must be a positive integer")
		return
	}

	filter := topology.ChangeTimelineFilter{
		ClusterID: clusterID,
		Namespace: c.Query("namespace"),
		Kind:      c.Query("kind"),
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

	resp, err := h.service.GetChangeTimeline(c.Request.Context(), filter)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "TOPOLOGY_QUERY_FAILED", "failed to query change timeline")
		return
	}
	c.JSON(http.StatusOK, resp)
}
