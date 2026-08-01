package servicemesh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"k8s-aiops.local/backend/internal/apiquery"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
	"k8s-aiops.local/backend/internal/metricshistory"
)

// CRDGateway is the read-only subset of k8sgateway.Service used for
// VirtualService / DestinationRule listing. The gateway returns raw
// map[string]interface{} via CustomResources / CustomResource; M52 projects
// them into strongly-typed views.
type CRDGateway interface {
	CustomResources(ctx context.Context, clusterID int64, group, version, resource, namespace string, q apiquery.ListQuery) (apiquery.ListResponse[map[string]interface{}], error)
	CustomResource(ctx context.Context, clusterID int64, group, version, resource, namespace, name string) (map[string]interface{}, error)
}

// MetricsHistoryReader is the read-only subset of metricshistory.Service used
// to synthesize traffic metrics from Istio telemetry stored in metrics_history.
// When nil, the traffic endpoint returns ErrMeshDataUnavailable (503).
type MetricsHistoryReader interface {
	Query(ctx context.Context, q metricshistory.SeriesQuery) (metricshistory.SeriesResponse, error)
}

// Service provides read-only access to Istio resources and traffic metrics.
// M52 intentionally surfaces no write surface (no VirtualService edit, no
// DestinationRule apply); the mesh remains strictly advisory for SLO evidence
// and topology edges.
type Service struct {
	crd     CRDGateway
	metrics MetricsHistoryReader
	now     func() time.Time
}

// NewService constructs a Service. crd may be nil (only in tests) — production
// wiring MUST supply a real k8sgateway.Service. metrics may be nil; traffic
// routes degrade to 503 when unset.
func NewService(crd CRDGateway, metrics MetricsHistoryReader) *Service {
	return &Service{
		crd:     crd,
		metrics: metrics,
		now:     time.Now,
	}
}

// --- VirtualServices ---

// ListVirtualServices returns the paginated list of Istio VirtualServices,
// projected into VirtualServiceView. Non-whitelisted or missing CRDs return
// ErrIstioNotInstalled so callers can show "mesh not installed".
func (s *Service) ListVirtualServices(ctx context.Context, clusterID int64, filter ListFilter) (ListResponse[VirtualServiceView], error) {
	if s.crd == nil {
		return ListResponse[VirtualServiceView]{}, ErrIstioNotInstalled
	}
	resp, err := s.crd.CustomResources(ctx, clusterID, APIGroupNetworking, APIVersionV1Beta1, ResourceVirtualService, filter.Namespace, apiquery.ListQuery{
		Page:  1 + filter.Offset/max(1, filter.Limit),
		Limit: max(10, filter.Limit),
	})
	if err != nil {
		if errors.Is(err, k8sgateway.ErrResourceNotFound) {
			return ListResponse[VirtualServiceView]{}, ErrIstioNotInstalled
		}
		return ListResponse[VirtualServiceView]{}, err
	}
	items := make([]VirtualServiceView, 0, len(resp.Items))
	for _, raw := range resp.Items {
		view, perr := projectVirtualService(raw)
		if perr != nil {
			continue
		}
		if filter.Name != "" && view.Name != filter.Name {
			continue
		}
		items = append(items, view)
	}
	return ListResponse[VirtualServiceView]{
		Items:     items,
		Total:     resp.Total,
		Truncated: resp.Remaining > 0,
	}, nil
}

// GetVirtualService returns a single VirtualService view by name.
func (s *Service) GetVirtualService(ctx context.Context, clusterID int64, namespace, name string) (VirtualServiceView, error) {
	if s.crd == nil {
		return VirtualServiceView{}, ErrIstioNotInstalled
	}
	resp, err := s.crd.CustomResource(ctx, clusterID, APIGroupNetworking, APIVersionV1Beta1, ResourceVirtualService, namespace, name)
	if err != nil {
		if errors.Is(err, k8sgateway.ErrResourceNotFound) {
			return VirtualServiceView{}, ErrIstioNotInstalled
		}
		return VirtualServiceView{}, err
	}
	return projectVirtualService(resp)
}

// --- DestinationRules ---

// ListDestinationRules returns the paginated list of Istio DestinationRules.
func (s *Service) ListDestinationRules(ctx context.Context, clusterID int64, filter ListFilter) (ListResponse[DestinationRuleView], error) {
	if s.crd == nil {
		return ListResponse[DestinationRuleView]{}, ErrIstioNotInstalled
	}
	resp, err := s.crd.CustomResources(ctx, clusterID, APIGroupNetworking, APIVersionV1Beta1, ResourceDestinationRule, filter.Namespace, apiquery.ListQuery{
		Page:  1 + filter.Offset/max(1, filter.Limit),
		Limit: max(10, filter.Limit),
	})
	if err != nil {
		if errors.Is(err, k8sgateway.ErrResourceNotFound) {
			return ListResponse[DestinationRuleView]{}, ErrIstioNotInstalled
		}
		return ListResponse[DestinationRuleView]{}, err
	}
	items := make([]DestinationRuleView, 0, len(resp.Items))
	for _, raw := range resp.Items {
		view, perr := projectDestinationRule(raw)
		if perr != nil {
			continue
		}
		if filter.Name != "" && view.Name != filter.Name {
			continue
		}
		items = append(items, view)
	}
	return ListResponse[DestinationRuleView]{
		Items:     items,
		Total:     resp.Total,
		Truncated: resp.Remaining > 0,
	}, nil
}

// GetDestinationRule returns a single DestinationRule view.
func (s *Service) GetDestinationRule(ctx context.Context, clusterID int64, namespace, name string) (DestinationRuleView, error) {
	if s.crd == nil {
		return DestinationRuleView{}, ErrIstioNotInstalled
	}
	resp, err := s.crd.CustomResource(ctx, clusterID, APIGroupNetworking, APIVersionV1Beta1, ResourceDestinationRule, namespace, name)
	if err != nil {
		if errors.Is(err, k8sgateway.ErrResourceNotFound) {
			return DestinationRuleView{}, ErrIstioNotInstalled
		}
		return DestinationRuleView{}, err
	}
	return projectDestinationRule(resp)
}

// --- Traffic metrics ---

// istioTrafficMetric is a small internal struct used to project SeriesResponse
// samples into per-service aggregations. M52 maps metric names with known
// istio_ prefixes into the TrafficMetrics dimensions; any other metric is
// skipped (fail-soft, partial mesh telemetry is acceptable per ADR 0067 §5).
type istioTrafficMetric struct {
	Namespace   string
	ServiceName string
	MetricName  string
	Points      []metricshistory.Point
}

// TrafficMetrics returns per-service traffic metrics aggregated over the
// requested window. At M52 this is an advisory projection from
// metricshistory samples tagged with Istio-style metric names.
func (s *Service) TrafficMetrics(ctx context.Context, q TrafficQuery) (*TrafficMetrics, error) {
	if s.metrics == nil {
		return nil, ErrMeshDataUnavailable
	}
	winStart := q.WindowStart
	winEnd := q.WindowEnd
	if winStart.IsZero() || winEnd.IsZero() {
		winEnd = s.now().UTC()
		winStart = winEnd.Add(-DefaultTrafficWindow)
	}
	if !winStart.Before(winEnd) {
		return nil, ErrInvalidWindow
	}
	if winEnd.Sub(winStart) > time.Duration(MaxTrafficWindowHours)*time.Hour {
		return nil, ErrInvalidWindow
	}
	topK := q.TopK
	if topK <= 0 {
		topK = DefaultTopK
	}

	// Query candidate series. M52 queries five known Istio metrics with a
	// generous point limit; filtering happens in-process because the
	// metricshistory query interface only supports one metric at a time.
	// This is intentionally bounded: the caller can constrain by namespace
	// and by service name.
	metricNames := []string{
		"istio_requests_total", "istio_requests_errors",
		"istio_latency_ms_p50", "istio_latency_ms_p95", "istio_latency_ms_p99",
	}
	collected := make([]istioTrafficMetric, 0, len(metricNames))
	partial := false
	for _, mname := range metricNames {
		sq := metricshistory.SeriesQuery{
			ClusterID:         q.ClusterID,
			ResourceKind:      "service",
			ResourceNamespace: q.Namespace,
			ResourceName:      q.ServiceName,
			MetricName:        mname,
			From:              winStart,
			To:                winEnd,
			Limit:             2000,
		}
		resp, err := s.metrics.Query(ctx, sq)
		if err != nil {
			partial = true
			continue // fail-soft: treat missing metric source as partial
		}
		if len(resp.Points) == 0 {
			continue
		}
		collected = append(collected, istioTrafficMetric{
			Namespace:   resp.Series.ResourceNamespace,
			ServiceName: resp.Series.ResourceName,
			MetricName:  resp.Series.MetricName,
			Points:      resp.Points,
		})
	}
	if len(collected) == 0 && !partial {
		// No mesh data at all — return empty services list, not an error.
		return &TrafficMetrics{
			ClusterID:   q.ClusterID,
			Namespace:   q.Namespace,
			WindowStart: winStart,
			WindowEnd:   winEnd,
			Partial:     partial,
		}, nil
	}
	return &TrafficMetrics{
		ClusterID:   q.ClusterID,
		Namespace:   q.Namespace,
		WindowStart: winStart,
		WindowEnd:   winEnd,
		Partial:     partial,
		Services:    aggregateTopK(collected, topK),
	}, nil
}

// --- projection helpers ---

func projectVirtualService(raw map[string]interface{}) (VirtualServiceView, error) {
	meta, _ := raw["metadata"].(map[string]interface{})
	if meta == nil {
		return VirtualServiceView{}, fmt.Errorf("no metadata")
	}
	spec, _ := raw["spec"].(map[string]interface{})
	vs := VirtualServiceView{
		Name:        s(meta["name"]),
		Namespace:   s(meta["namespace"]),
		UID:         s(meta["uid"]),
		Labels:      mss(meta["labels"]),
		Annotations: redactAnnotations(mss(meta["annotations"])),
		CreatedAt:   s(meta["creationTimestamp"]),
	}
	if spec != nil {
		vs.Hosts = sliceOfString(spec["hosts"])
		vs.Gateways = sliceOfString(spec["gateways"])
		httpRoutes, _ := spec["http"].([]interface{})
		vs.HttpRouteCount = len(httpRoutes)
		tcpRoutes, _ := spec["tcp"].([]interface{})
		vs.TcpRouteCount = len(tcpRoutes)
		vs.Destinations = collectHTTPDests(httpRoutes)
	}
	return vs, nil
}

func projectDestinationRule(raw map[string]interface{}) (DestinationRuleView, error) {
	meta, _ := raw["metadata"].(map[string]interface{})
	if meta == nil {
		return DestinationRuleView{}, fmt.Errorf("no metadata")
	}
	spec, _ := raw["spec"].(map[string]interface{})
	dr := DestinationRuleView{
		Name:        s(meta["name"]),
		Namespace:   s(meta["namespace"]),
		UID:         s(meta["uid"]),
		Labels:      mss(meta["labels"]),
		Annotations: redactAnnotations(mss(meta["annotations"])),
		CreatedAt:   s(meta["creationTimestamp"]),
	}
	if spec != nil {
		dr.Host = s(spec["host"])
		subsets, _ := spec["subsets"].([]interface{})
		dr.SubsetCount = len(subsets)
		dr.SubsetNames = subsetNames(subsets)
		dr.TrafficPolicySummary = summarizeTrafficPolicy(spec["trafficPolicy"])
	}
	return dr, nil
}

func collectHTTPDests(httpRoutes []interface{}) []RouteDestinationView {
	out := make([]RouteDestinationView, 0, 4)
	for _, r := range httpRoutes {
		route, _ := r.(map[string]interface{})
		actions, _ := route["route"].([]interface{})
		for _, a := range actions {
			action, _ := a.(map[string]interface{})
			dest, _ := action["destination"].(map[string]interface{})
			if dest == nil {
				continue
			}
			out = append(out, RouteDestinationView{
				Host:   s(dest["host"]),
				Subset: s(dest["subset"]),
				Weight: int32(intAt(action, "weight")),
			})
		}
	}
	return out
}

func subsetNames(subsets []interface{}) []string {
	out := make([]string, 0, len(subsets))
	for _, raw := range subsets {
		subset, _ := raw.(map[string]interface{})
		name := s(subset["name"])
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}

func summarizeTrafficPolicy(tp interface{}) string {
	if tp == nil {
		return ""
	}
	policy, _ := tp.(map[string]interface{})
	if policy == nil {
		return ""
	}
	parts := make([]string, 0, 3)
	if _, ok := policy["connectionPool"]; ok {
		parts = append(parts, "connectionPool")
	}
	if _, ok := policy["outlierDetection"]; ok {
		parts = append(parts, "outlierDetection")
	}
	if _, ok := policy["tls"]; ok {
		parts = append(parts, "tls")
	}
	if _, ok := policy["retries"]; ok {
		parts = append(parts, "retries")
	}
	if len(parts) == 0 {
		return "trafficPolicy: custom"
	}
	sort.Strings(parts)
	j, _ := json.Marshal(parts)
	return string(j)
}

func aggregateTopK(collected []istioTrafficMetric, topK int) []ServiceMetric {
	type aggKey struct {
		Namespace   string
		ServiceName string
	}
	type aggregate struct {
		requests     int64
		errors       int64
		latencySum   int64
		latencyCount int64
		p50          float64
		p95          float64
		p99          float64
	}
	aggs := make(map[aggKey]*aggregate)
	for _, c := range collected {
		key := aggKey{Namespace: c.Namespace, ServiceName: c.ServiceName}
		if key.ServiceName == "" {
			continue
		}
		a, ok := aggs[key]
		if !ok {
			a = &aggregate{}
			aggs[key] = a
		}
		// Aggregate value by metric semantics. For cumulative counters
		// (requests/errors) we take the last sample minus first sample (delta
		// over the window). For latency percentiles we keep the latest point.
		if len(c.Points) == 0 {
			continue
		}
		switch c.MetricName {
		case "istio_requests_total":
			a.requests = deltaSum(c.Points)
		case "istio_requests_errors":
			a.errors = deltaSum(c.Points)
		case "istio_latency_ms_p50":
			a.p50 = float64(c.Points[len(c.Points)-1].Value)
		case "istio_latency_ms_p95":
			a.p95 = float64(c.Points[len(c.Points)-1].Value)
		case "istio_latency_ms_p99":
			a.p99 = float64(c.Points[len(c.Points)-1].Value)
		}
	}
	out := make([]ServiceMetric, 0, len(aggs))
	for k, a := range aggs {
		errorRatePct := 0.0
		requestRateRPS := 0.0
		if a.requests > 0 {
			errorRatePct = 100.0 * float64(a.errors) / float64(a.requests)
			// Approximate rate over DefaultTrafficWindow (1h). M52 does not
			// require exact rate alignment with the query window — the
			// evidence is advisory.
			winSec := float64(DefaultTrafficWindow) / float64(time.Second)
			if winSec > 0 {
				requestRateRPS = float64(a.requests) / winSec
			}
		}
		out = append(out, ServiceMetric{
			ServiceName:    k.ServiceName,
			Namespace:      k.Namespace,
			RequestRateRPS: requestRateRPS,
			ErrorRatePct:   errorRatePct,
			P50LatencyMs:   a.p50,
			P95LatencyMs:   a.p95,
			P99LatencyMs:   a.p99,
			TotalRequests:  a.requests,
			TotalErrors:    a.errors,
		})
	}
	// Sort by request count desc for top-K.
	sort.Slice(out, func(i, j int) bool { return out[i].TotalRequests > out[j].TotalRequests })
	if len(out) > topK {
		out = out[:topK]
	}
	return out
}

func deltaSum(points []metricshistory.Point) int64 {
	if len(points) < 2 {
		if len(points) == 1 {
			return points[0].Value
		}
		return 0
	}
	first := points[0].Value
	last := points[len(points)-1].Value
	if last < first {
		// Counter reset occurred — just return the last value as a conservative
		// estimate of window traffic (fail-soft).
		return last
	}
	return last - first
}

// --- tiny coercions ---

func s(v interface{}) string {
	if v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

func mss(v interface{}) map[string]string {
	if v == nil {
		return nil
	}
	raw, ok := v.(map[string]interface{})
	if !ok {
		return nil
	}
	out := make(map[string]string, len(raw))
	for k, vv := range raw {
		out[k] = s(vv)
	}
	return out
}

func sliceOfString(v interface{}) []string {
	if v == nil {
		return nil
	}
	raw, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if sv, ok := item.(string); ok {
			out = append(out, sv)
		}
	}
	return out
}

func intAt(m map[string]interface{}, key string) int {
	if m == nil {
		return 0
	}
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch vv := v.(type) {
	case float64:
		return int(vv)
	case int:
		return vv
	case int32:
		return int(vv)
	case int64:
		return int(vv)
	}
	return 0
}

func redactAnnotations(annotations map[string]string) map[string]string {
	if annotations == nil {
		return nil
	}
	const redactValue = "[redacted]"
	out := make(map[string]string, len(annotations))
	for k, v := range annotations {
		if annotationKeySensitive(k) {
			out[k] = redactValue
			continue
		}
		if len(v) > 512 {
			out[k] = v[:512] + "…"
			continue
		}
		out[k] = v
	}
	return out
}

// annotationKeySensitive reports whether an annotation key typically contains
// secrets (tokens, registry credentials, TLS material). Mirrors the policy
// used by namespaceposture redaction (M40) to keep the project-wide policy
// consistent.
func annotationKeySensitive(key string) bool {
	if key == "" {
		return false
	}
	lower := strings.ToLower(key)
	sensitiveSubstrings := []string{
		"token", "secret", "password", "credential",
		"private-key", "privatekey", "tls.key", "tls.crt",
		"dockerconfigjson", "dockercfg", "cert",
	}
	for _, s := range sensitiveSubstrings {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}
