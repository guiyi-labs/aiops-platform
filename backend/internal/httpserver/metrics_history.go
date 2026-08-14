package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/metricshistory"
)

type metricsHistoryHandler struct{ service *metricshistory.Service }

func (h metricsHistoryHandler) series(c *gin.Context) {
	from, ok := requiredHistoryTime(c, "from")
	if !ok {
		return
	}
	to, ok := requiredHistoryTime(c, "to")
	if !ok {
		return
	}
	limit := 0
	if raw := c.Query("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 1440 {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "limit must be an integer from 1 through 1440")
			return
		}
		limit = value
	}
	response, err := h.service.Query(c.Request.Context(), metricshistory.SeriesQuery{
		ClusterID: currentClusterID(c), ResourceKind: c.Query("resource_kind"),
		ResourceNamespace: c.Query("namespace"), ResourceName: c.Query("name"),
		ContainerName: c.Query("container"), MetricName: c.Query("metric"),
		From: from, To: to, Limit: limit,
	})
	switch {
	case errors.Is(err, metricshistory.ErrInvalidQuery):
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "the exact metric series, time window or point limit is invalid")
	case errors.Is(err, metricshistory.ErrClusterNotFound):
		writeError(c, http.StatusNotFound, "CLUSTER_NOT_FOUND", "cluster does not exist")
	case err != nil:
		writeError(c, http.StatusInternalServerError, "METRICS_HISTORY_QUERY_FAILED", "unable to query metric history")
	default:
		c.JSON(http.StatusOK, response)
	}
}

func requiredHistoryTime(c *gin.Context, name string) (time.Time, bool) {
	value, err := time.Parse(time.RFC3339Nano, c.Query(name))
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", name+" must be an RFC3339 timestamp")
		return time.Time{}, false
	}
	return value, true
}

// archiveSeries is the M114-3 downsampled 30-day metric history endpoint.
// It accepts the same parameters as the precise series endpoint but allows a
// window up to MaxArchiveQueryWindow (default 30 days) and returns 1-hour
// aggregated points bounded by MaxQueryPoints (1440).  Read-only.
func (h metricsHistoryHandler) archiveSeries(c *gin.Context) {
	from, ok := requiredHistoryTime(c, "from")
	if !ok {
		return
	}
	to, ok := requiredHistoryTime(c, "to")
	if !ok {
		return
	}
	limit := 0
	if raw := c.Query("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 1440 {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "limit must be an integer from 1 through 1440")
			return
		}
		limit = value
	}
	response, err := h.service.QueryArchive(c.Request.Context(), metricshistory.ArchiveSeriesQuery{
		ClusterID: currentClusterID(c), ResourceKind: c.Query("resource_kind"),
		ResourceNamespace: c.Query("namespace"), ResourceName: c.Query("name"),
		ContainerName: c.Query("container"), MetricName: c.Query("metric"),
		From: from, To: to, Limit: limit,
	})
	switch {
	case errors.Is(err, metricshistory.ErrInvalidQuery):
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "the exact metric series, time window or point limit is invalid")
	case errors.Is(err, metricshistory.ErrClusterNotFound):
		writeError(c, http.StatusNotFound, "CLUSTER_NOT_FOUND", "cluster does not exist")
	case err != nil:
		writeError(c, http.StatusInternalServerError, "METRICS_HISTORY_QUERY_FAILED", "unable to query metric history archive")
	default:
		c.JSON(http.StatusOK, response)
	}
}
