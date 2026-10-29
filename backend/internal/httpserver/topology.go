package httpserver

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/topology"
)

// topologyHandler exposes the M40 topology service. Graph and change-timeline
// queries are read-only; collection is triggered explicitly through
// collectTopology because no background worker drives the collector in this
// deployment — without it the edge table stays empty and the graph page shows
// an empty canvas that looks like a rendering bug rather than missing input.
type topologyHandler struct {
	service *topology.Service
}

// collectTopology handles POST /api/v1/aiops/topology/collect.
//
// Body/query:
//
//	cluster_id (required) — target cluster
//	namespace  (optional) — collect one namespace; omitted = whole cluster
//
// Collection is a write (it upserts and closes edges), so it is audited and
// restricted to operators. It is bounded: one namespace pass lists eight
// resource kinds, and a cluster pass fans out over visible namespaces with the
// service's own concurrency limit.
func (h topologyHandler) collectTopology(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "TOPOLOGY_UNAVAILABLE", "topology service is not configured")
		return
	}
	clusterID, err := strconv.ParseInt(firstNonEmptyString(c.Query("cluster_id"), c.PostForm("cluster_id")), 10, 64)
	if err != nil || clusterID <= 0 {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "cluster_id is required and must be a positive integer")
		return
	}
	namespace := firstNonEmptyString(c.Query("namespace"), c.PostForm("namespace"))

	now := time.Now().UTC()
	if namespace != "" {
		result, collectErr := h.service.CollectNamespace(c.Request.Context(), clusterID, namespace, now)
		response := gin.H{
			"cluster_id":     result.ClusterID,
			"namespace":      result.Namespace,
			"edges_seen":     result.EdgesSeen,
			"edges_upserted": result.EdgesUpserted,
			"edges_closed":   result.EdgesClosed,
			"partial":        result.Partial,
		}
		if result.Error != nil {
			response["error"] = result.Error.Error()
		}
		// 部分失败仍返回 200：边已按可用数据写入，前端需要拿到计数来展示
		// 「采到多少、缺多少」，而不是一个只有错误码的 5xx。
		c.JSON(http.StatusOK, response)
		_ = collectErr
		return
	}

	result, collectErr := h.service.CollectCluster(c.Request.Context(), clusterID, now)
	messages := make([]string, 0, len(result.Errors))
	for _, item := range result.Errors {
		if item != nil {
			messages = append(messages, item.Error())
		}
	}
	response := gin.H{
		"cluster_id":     result.ClusterID,
		"namespaces":     result.Namespaces,
		"edges_seen":     result.TotalSeen,
		"edges_upserted": result.TotalUpserted,
		"edges_closed":   result.TotalClosed,
		"partial":        result.Partial,
	}
	if len(messages) > 0 {
		response["errors"] = messages
	}
	if collectErr != nil && len(messages) == 0 {
		response["error"] = collectErr.Error()
	}
	c.JSON(http.StatusOK, response)
}

// firstNonEmptyString 返回第一个非空值。用于接受 query 或 form 两种传参，
// 避免调用方必须记住用哪种。
func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
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

	// collapse (optional): when "1", duplicate edges between the same
	// source/target pair collapse into one edge with an aggregate count.
	collapse := c.Query("collapse") == "1"
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
	if err == nil && collapse {
		graph.Edges = collapseEdges(graph.Edges)
	}
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

// collapseEdges compresses repeated edges between the same source/target pair
// into a single representative edge carrying an advisory aggregate count.
func collapseEdges(edges []topology.Edge) []topology.Edge {
	sorted := make([]topology.Edge, len(edges))
	copy(sorted, edges)
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		if a.Source.UID != b.Source.UID {
			return a.Source.UID < b.Source.UID
		}
		if a.Target.UID != b.Target.UID {
			return a.Target.UID < b.Target.UID
		}
		return string(a.Kind) < string(b.Kind)
	})
	out := make([]topology.Edge, 0, len(sorted))
	for i := 0; i < len(sorted); {
		j := i + 1
		for j < len(sorted) && sorted[j].Source.UID == sorted[i].Source.UID && sorted[j].Target.UID == sorted[i].Target.UID && sorted[j].Kind == sorted[i].Kind {
			j++
		}
		e := sorted[i]
		topology.SetEdgeCount(&e, j-i)
		out = append(out, e)
		i = j
	}
	return out
}
