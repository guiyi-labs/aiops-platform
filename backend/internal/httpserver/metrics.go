package httpserver

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type metricKey struct {
	method string
	route  string
	status string
}

type metricValue struct {
	count    uint64
	duration time.Duration
}

// Metrics stores only bounded route-template labels; raw URLs, IDs and user data are never labels.
type Metrics struct {
	mu       sync.RWMutex
	requests map[metricKey]metricValue
}

func NewMetrics() *Metrics { return &Metrics{requests: make(map[metricKey]metricValue)} }

func (m *Metrics) Observe(method, route string, status int, duration time.Duration) {
	if route == "" {
		route = "unmatched"
	}
	key := metricKey{method: method, route: route, status: fmt.Sprintf("%dxx", status/100)}
	m.mu.Lock()
	value := m.requests[key]
	value.count++
	value.duration += duration
	m.requests[key] = value
	m.mu.Unlock()
}

func (m *Metrics) Render() string {
	m.mu.RLock()
	keys := make([]metricKey, 0, len(m.requests))
	values := make(map[metricKey]metricValue, len(m.requests))
	for key, value := range m.requests {
		keys = append(keys, key)
		values[key] = value
	}
	m.mu.RUnlock()
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].method != keys[j].method {
			return keys[i].method < keys[j].method
		}
		if keys[i].route != keys[j].route {
			return keys[i].route < keys[j].route
		}
		return keys[i].status < keys[j].status
	})
	var builder strings.Builder
	builder.WriteString("# HELP aiops_http_requests_total Total HTTP requests by route template and status class.\n# TYPE aiops_http_requests_total counter\n")
	for _, key := range keys {
		value := values[key]
		labels := fmt.Sprintf("method=\"%s\",route=\"%s\",status_class=\"%s\"", escapeMetricLabel(key.method), escapeMetricLabel(key.route), key.status)
		fmt.Fprintf(&builder, "aiops_http_requests_total{%s} %d\n", labels, value.count)
		fmt.Fprintf(&builder, "aiops_http_request_duration_seconds_sum{%s} %.9f\n", labels, value.duration.Seconds())
		fmt.Fprintf(&builder, "aiops_http_request_duration_seconds_count{%s} %d\n", labels, value.count)
	}
	return builder.String()
}

func escapeMetricLabel(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return strings.ReplaceAll(value, "\n", `\n`)
}

func requestMetrics(metrics *Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()
		metrics.Observe(c.Request.Method, c.FullPath(), c.Writer.Status(), time.Since(started))
	}
}
