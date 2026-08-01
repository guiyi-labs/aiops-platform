package servicemesh

import (
	"errors"
	"time"
)

// Constants for M52 service-mesh read-only access.
const (
	APIGroupNetworking      = "networking.istio.io"
	APIVersionV1Beta1       = "v1beta1"
	APIVersionV1Alpha3      = "v1alpha3"
	ResourceVirtualService  = "virtualservices"
	ResourceDestinationRule = "destinationrules"

	MaxTrafficWindowHours = 24
	DefaultTrafficWindow  = 1 * time.Hour
	DefaultTopK           = 20
)

var (
	ErrIstioNotInstalled   = errors.New("istio service mesh is not installed on cluster")
	ErrMeshDataUnavailable = errors.New("service mesh metrics data unavailable")
	ErrInvalidWindow       = errors.New("invalid traffic time window")
)

// VirtualServiceView is the read-only projection of an Istio VirtualService.
// Sensitive fields (e.g. TLS secrets) are redacted; the full manifest is
// never returned via the read-only API (ADR 0067 §4).
type VirtualServiceView struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	UID         string            `json:"uid"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	CreatedAt   string            `json:"created_at"`
	// Hosts: short names or FQDNs exposed by this VS.
	Hosts []string `json:"hosts"`
	// Gateways: istio gateway names bound (empty = mesh-internal only).
	Gateways []string `json:"gateways,omitempty"`
	// HttpRouteCount is the number of HTTP route entries.
	HttpRouteCount int `json:"http_route_count"`
	// TcpRouteCount is the number of TCP route entries.
	TcpRouteCount int `json:"tcp_route_count"`
	// Destinations summarizes HTTP route destinations.
	Destinations []RouteDestinationView `json:"destinations,omitempty"`
}

// RouteDestinationView is a short summary of a route destination (host + subset).
type RouteDestinationView struct {
	Host   string `json:"host"`
	Subset string `json:"subset,omitempty"`
	Weight int32  `json:"weight,omitempty"`
}

// DestinationRuleView is the read-only projection of an Istio DestinationRule.
type DestinationRuleView struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	UID         string            `json:"uid"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	CreatedAt   string            `json:"created_at"`
	// Host the rule applies to (Service FQDN).
	Host string `json:"host"`
	// SubsetCount is the number of named subsets defined in the rule.
	SubsetCount int `json:"subset_count"`
	// SubsetNames lists the subset names (top-level only, no labels).
	SubsetNames []string `json:"subset_names,omitempty"`
	// TrafficPolicySummary describes outlier detection / LB / mTLS mode.
	TrafficPolicySummary string `json:"traffic_policy_summary,omitempty"`
}

// TrafficMetrics reports per-service (or top-N) request volume, error rate
// and latency. Values are aggregated from metricshistory over the requested
// window and used as SLO evidence and topology edge weight.
type TrafficMetrics struct {
	ClusterID   int64           `json:"cluster_id"`
	Namespace   string          `json:"namespace,omitempty"`
	WindowStart time.Time       `json:"window_start"`
	WindowEnd   time.Time       `json:"window_end"`
	Services    []ServiceMetric `json:"services"`
	Partial     bool            `json:"partial,omitempty"`
}

// ServiceMetric is a single service's traffic metrics.
type ServiceMetric struct {
	ServiceName    string  `json:"service_name"`
	Namespace      string  `json:"namespace"`
	RequestRateRPS float64 `json:"request_rate_rps"` // requests / sec over the window
	ErrorRatePct   float64 `json:"error_rate_pct"`   // 5xx / total * 100
	P50LatencyMs   float64 `json:"p50_latency_ms"`
	P95LatencyMs   float64 `json:"p95_latency_ms"`
	P99LatencyMs   float64 `json:"p99_latency_ms"`
	TotalRequests  int64   `json:"total_requests"`
	TotalErrors    int64   `json:"total_errors"`
	// SourceWorkloads lists top-5 caller workloads observed as origin.
	SourceWorkloads []string `json:"source_workloads,omitempty"`
}

// ListFilter bounds service-mesh list queries.
type ListFilter struct {
	Namespace string
	Name      string
	Limit     int
	Offset    int
}

// TrafficQuery bounds a traffic metrics query.
type TrafficQuery struct {
	ClusterID   int64
	Namespace   string
	ServiceName string
	WindowStart time.Time
	WindowEnd   time.Time
	TopK        int
}

// ListResponse is the paginated list wrapper.
type ListResponse[T any] struct {
	Items     []T  `json:"items"`
	Total     int  `json:"total"`
	Truncated bool `json:"truncated,omitempty"`
}
