package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/capability"
)

// capabilityHandler exposes the M37A capability providers as read-only query
// endpoints. Both providers are optional: when nil the corresponding route
// returns 503 so the server can run with metrics, logs, or both disabled.
// The M60 provider registry is also carried here; nil skips the provider
// surface.
type capabilityHandler struct {
	metricsProvider capability.MetricsProvider
	logProvider     capability.LogProvider
	registry        *capability.Registry
}

// capabilityTemplates is the fixed set of SLI templates accepted by the metrics
// endpoint. Clients cannot inject arbitrary PromQL; only these identifiers are
// mapped to queries by the provider.
var capabilityTemplates = map[string]struct{}{
	capability.TemplateRequestRate: {},
	capability.TemplateErrorRate:   {},
	capability.TemplateLatencyP99:  {},
	capability.TemplateCPUUsage:    {},
	capability.TemplateMemoryUsage: {},
}

// queryMetrics handles GET /api/v1/capability/metrics.
//
// Query params: cluster_id, namespace, service (optional), pod (optional),
// container (optional), template, start, end, step. The handler validates the
// presence and shape of the inputs; the provider enforces the semantic bounds
// (template enum, ordered bounds, range limits) and reports provider-side
// failures via the result state rather than a Go error.
func (h capabilityHandler) queryMetrics(c *gin.Context) {
	if h.metricsProvider == nil {
		writeError(c, http.StatusServiceUnavailable, "CAPABILITY_UNAVAILABLE", "capability metrics is not configured")
		return
	}
	clusterID, err := strconv.ParseInt(c.Query("cluster_id"), 10, 64)
	if err != nil || clusterID <= 0 {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "cluster_id must be a positive integer")
		return
	}
	namespace := c.Query("namespace")
	if namespace == "" {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "namespace is required")
		return
	}
	template := c.Query("template")
	if _, ok := capabilityTemplates[template]; !ok {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "template is not supported")
		return
	}
	start, ok := parseCapabilityTime(c, "start")
	if !ok {
		return
	}
	end, ok := parseCapabilityTime(c, "end")
	if !ok {
		return
	}
	step, ok := parseCapabilityStep(c)
	if !ok {
		return
	}
	query := capability.MetricsQuery{
		ClusterID:   clusterID,
		Namespace:   namespace,
		ServiceName: c.Query("service"),
		PodName:     c.Query("pod"),
		Container:   c.Query("container"),
		Template:    template,
		Start:       start,
		End:         end,
		Step:        step,
	}
	result, err := h.metricsProvider.QueryMetrics(c.Request.Context(), query)
	if err != nil {
		if errors.Is(err, capability.ErrInvalidMetricsQuery) {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", err.Error())
			return
		}
		writeError(c, http.StatusInternalServerError, "CAPABILITY_QUERY_FAILED", "unable to query metrics")
		return
	}
	c.JSON(http.StatusOK, result)
}

// queryLogs handles POST /api/v1/capability/logs.
//
// Body: { cluster_id, namespace, pod, container, text_filter, start, end,
// direction, limit }. The handler validates the body shape and parses the
// timestamp/direction inputs; the provider enforces the semantic bounds.
func (h capabilityHandler) queryLogs(c *gin.Context) {
	if h.logProvider == nil {
		writeError(c, http.StatusServiceUnavailable, "CAPABILITY_UNAVAILABLE", "capability logs is not configured")
		return
	}
	var request capabilityLogsRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	start, err := time.Parse(time.RFC3339Nano, request.Start)
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "start must be an RFC3339 timestamp")
		return
	}
	end, err := time.Parse(time.RFC3339Nano, request.End)
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "end must be an RFC3339 timestamp")
		return
	}
	direction := request.Direction
	if direction == "" {
		direction = capability.DirectionForward
	}
	query := capability.LogQuery{
		ClusterID:  request.ClusterID,
		Namespace:  request.Namespace,
		PodName:    request.Pod,
		Container:  request.Container,
		TextFilter: request.TextFilter,
		Start:      start,
		End:        end,
		Direction:  direction,
		Limit:      request.Limit,
	}
	result, err := h.logProvider.QueryLogs(c.Request.Context(), query)
	if err != nil {
		if errors.Is(err, capability.ErrInvalidLogQuery) {
			writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
			return
		}
		writeError(c, http.StatusInternalServerError, "CAPABILITY_QUERY_FAILED", "unable to query logs")
		return
	}
	c.JSON(http.StatusOK, result)
}

// capabilityLogsRequest is the JSON body for the logs endpoint. Start and End
// are RFC3339 strings so the body is human-readable; the handler parses them
// into time.Time before calling the provider.
type capabilityLogsRequest struct {
	ClusterID  int64  `json:"cluster_id" binding:"required"`
	Namespace  string `json:"namespace" binding:"required"`
	Pod        string `json:"pod"`
	Container  string `json:"container"`
	TextFilter string `json:"text_filter"`
	Start      string `json:"start" binding:"required"`
	End        string `json:"end" binding:"required"`
	Direction  string `json:"direction"`
	Limit      int    `json:"limit"`
}

func parseCapabilityTime(c *gin.Context, name string) (time.Time, bool) {
	value, err := time.Parse(time.RFC3339Nano, c.Query(name))
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", name+" must be an RFC3339 timestamp")
		return time.Time{}, false
	}
	return value, true
}

func parseCapabilityStep(c *gin.Context) (time.Duration, bool) {
	step, err := time.ParseDuration(c.Query("step"))
	if err != nil || step <= 0 {
		writeError(c, http.StatusBadRequest, "INVALID_QUERY", "step must be a positive duration")
		return 0, false
	}
	return step, true
}

// listProviders handles GET /api/v1/capability/providers.
//
// The response is the full compile-time provider catalog projected with
// per-provider runtime state (started_at, last_check, state). When the
// `refresh=true` query param is supplied the handler runs CheckHealth on
// every configured provider before returning; otherwise the cached state
// is returned verbatim. This endpoint is SystemOpsAdmin only because the
// catalog exposes deployment topology (which providers run on which
// cluster roles) and health reasons that may reference internal endpoints.
func (h capabilityHandler) listProviders(c *gin.Context) {
	if h.registry == nil {
		writeError(c, http.StatusServiceUnavailable, "CAPABILITY_UNAVAILABLE", "provider registry is not configured")
		return
	}
	items := h.registry.List()
	if c.Query("refresh") == "true" {
		refreshed := make([]capability.ProviderInfo, 0, len(items))
		for _, info := range items {
			if info.State == capability.ProviderStateDisabled {
				refreshed = append(refreshed, info)
				continue
			}
			upd, err := h.registry.CheckHealth(c.Request.Context(), info.Name)
			if err != nil {
				continue
			}
			refreshed = append(refreshed, upd)
		}
		items = refreshed
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "generated_at": time.Now().UTC()})
}

// getProvider handles GET /api/v1/capability/providers/:name.
//
// It returns a single provider with an inline, always-fresh health check
// (bypassing the 1s cache). Unknown names return 404 to avoid leaking
// which compile-time providers exist without authorization.
func (h capabilityHandler) getProvider(c *gin.Context) {
	if h.registry == nil {
		writeError(c, http.StatusServiceUnavailable, "CAPABILITY_UNAVAILABLE", "provider registry is not configured")
		return
	}
	name := c.Param("name")
	info, err := h.registry.Get(name)
	if err != nil {
		if errors.Is(err, capability.ErrProviderNotFound) {
			writeError(c, http.StatusNotFound, "PROVIDER_NOT_FOUND", "provider does not exist")
			return
		}
		writeError(c, http.StatusInternalServerError, "PROVIDER_QUERY_FAILED", "unable to read provider")
		return
	}
	if info.State != capability.ProviderStateDisabled {
		upd, probeErr := h.registry.CheckHealth(c.Request.Context(), name)
		if probeErr == nil {
			info = upd
		}
	}
	c.JSON(http.StatusOK, info)
}
