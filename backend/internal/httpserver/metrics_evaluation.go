package httpserver

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/metricshistory"
)

const evaluationQueryLimit = 1440

func (h metricsHistoryHandler) evaluate(c *gin.Context) {
	from, ok := requiredHistoryTime(c, "from")
	if !ok {
		return
	}
	to, ok := requiredHistoryTime(c, "to")
	if !ok {
		return
	}
	operator := c.Query("operator")
	if operator != metricshistory.OperatorGreaterThanOrEqual && operator != metricshistory.OperatorLessThanOrEqual {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "operator must be gte or lte")
		return
	}
	threshold, ok := requiredNonNegativeInt64(c, "threshold")
	if !ok {
		return
	}
	forSeconds, ok := requiredBoundedInt(c, "for_seconds", 60, 86400)
	if !ok {
		return
	}
	minimumPoints := 2
	if raw, present := c.GetQuery("minimum_points"); present {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 2 || value > 1440 {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "minimum_points must be an integer from 2 through 1440")
			return
		}
		minimumPoints = value
	}
	response, err := h.service.Evaluate(c.Request.Context(), metricshistory.EvaluationQuery{
		SeriesQuery: metricshistory.SeriesQuery{
			ClusterID: currentClusterID(c), ResourceKind: c.Query("resource_kind"),
			ResourceNamespace: c.Query("namespace"), ResourceName: c.Query("name"),
			ContainerName: c.Query("container"), MetricName: c.Query("metric"),
			From: from, To: to, Limit: evaluationQueryLimit,
		},
		EvaluationRule: metricshistory.EvaluationRule{
			Operator: operator, Threshold: threshold, ForSeconds: forSeconds, MinimumPoints: minimumPoints,
		},
	})
	switch {
	case errors.Is(err, metricshistory.ErrInvalidQuery), errors.Is(err, metricshistory.ErrInvalidEvaluation):
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "the exact metric series, time window or evaluation rule is invalid")
	case errors.Is(err, metricshistory.ErrClusterNotFound):
		writeError(c, http.StatusNotFound, "CLUSTER_NOT_FOUND", "cluster does not exist")
	case err != nil:
		writeError(c, http.StatusInternalServerError, "METRICS_EVALUATION_FAILED", "unable to evaluate metric history")
	default:
		c.JSON(http.StatusOK, response)
	}
}

func requiredNonNegativeInt64(c *gin.Context, name string) (int64, bool) {
	raw, present := c.GetQuery(name)
	value, err := strconv.ParseInt(raw, 10, 64)
	if !present || err != nil || value < 0 {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", name+" must be a non-negative 64-bit integer")
		return 0, false
	}
	return value, true
}

func requiredBoundedInt(c *gin.Context, name string, minimum, maximum int) (int, bool) {
	raw, present := c.GetQuery(name)
	value, err := strconv.Atoi(raw)
	if !present || err != nil || value < minimum || value > maximum {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", name+" must be an integer from "+strconv.Itoa(minimum)+" through "+strconv.Itoa(maximum))
		return 0, false
	}
	return value, true
}
