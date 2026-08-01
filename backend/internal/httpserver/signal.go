package httpserver

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/signal"
)

// signalHandler exposes the M39 signal service as read-only query endpoints.
// Ingestion is internal (adapters call service.Ingest directly); the HTTP
// surface is list + overview only.
type signalHandler struct {
	service *signal.Service
	sources signal.SourceReader
}

// listSignals handles GET /api/v1/aiops/signals.
//
// Query params (all optional except pagination defaults):
//
//	cluster_id, namespace, signal_id, producer, state, severity,
//	start, end, limit.
//
// The handler validates input shape; the service clamps the limit and
// reports truncation via the ListResponse.Truncated field.
func (h signalHandler) listSignals(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "SIGNAL_UNAVAILABLE", "signal service is not configured")
		return
	}
	filter := signal.ListFilter{}
	if v := c.Query("cluster_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "cluster_id must be a positive integer")
			return
		}
		filter.ClusterID = &id
	}
	filter.Namespace = c.Query("namespace")
	filter.SignalID = c.Query("signal_id")
	filter.Producer = signal.Producer(c.Query("producer"))
	filter.State = signal.State(c.Query("state"))
	filter.Severity = signal.Severity(c.Query("severity"))
	if v := c.Query("start"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "start must be RFC3339")
			return
		}
		filter.WindowStart = &t
	}
	if v := c.Query("end"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "end must be RFC3339")
			return
		}
		filter.WindowEnd = &t
	}
	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "limit must be a positive integer")
			return
		}
		filter.Limit = n
	}
	items, total, err := h.service.List(c.Request.Context(), filter)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "SIGNAL_QUERY_FAILED", "failed to list signals")
		return
	}
	limit := filter.Limit
	c.JSON(http.StatusOK, gin.H{
		"items":     items,
		"total":     total,
		"truncated": total > int64(limit),
	})
}

// overview handles GET /api/v1/aiops/overview.
//
// Query params (optional): cluster_id, namespace. When omitted the overview
// spans all clusters the caller is authorized to see; M35 scope filtering is
// applied by the middleware chain, not here.
func (h signalHandler) overview(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "SIGNAL_UNAVAILABLE", "signal service is not configured")
		return
	}
	var clusterID *int64
	if v := c.Query("cluster_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "cluster_id must be a positive integer")
			return
		}
		clusterID = &id
	}
	namespace := c.Query("namespace")
	sources := h.sources
	if sources == nil {
		sources = signal.NopSourceReader{}
	}
	overview, err := h.service.Overview(c.Request.Context(), clusterID, namespace, sources)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "OVERVIEW_FAILED", "failed to build overview")
		return
	}
	c.JSON(http.StatusOK, overview)
}

// listSignalCatalog handles GET /api/v1/aiops/signals/catalog.
//
// Returns the compiled SignalDescriptor catalog. This is a fixed contract:
// the catalog changes only when a new adapter is added, paired with tests.
func (h signalHandler) listSignalCatalog(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"items": signal.All(),
	})
}
