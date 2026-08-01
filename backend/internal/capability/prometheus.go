package capability

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultPrometheusTimeout   = 10 * time.Second
	defaultPrometheusMaxSeries = 100
	defaultPrometheusMaxPoints = 1440
	// prometheusResponseLimit caps the HTTP response body (1 MiB), enforcing
	// the result/byte bounds from ADR 0053 §5.
	prometheusResponseLimit = 1 << 20
)

// Compile-time assertion that PrometheusMetricsProvider implements MetricsProvider.
var _ MetricsProvider = (*PrometheusMetricsProvider)(nil)

// PrometheusConfig is the server-configured configuration for a
// PrometheusMetricsProvider. Endpoint is an HTTPS URL; credentials are supplied
// out-of-band and never appear in query input or results.
type PrometheusConfig struct {
	Endpoint       string
	RequestTimeout time.Duration
	MaxSeries      int
	MaxPoints      int
}

// PrometheusMetricsProvider implements MetricsProvider against a
// Prometheus-compatible HTTP API. It maps fixed SLI templates to PromQL
// internally; the rendered PromQL is never exposed to callers.
type PrometheusMetricsProvider struct {
	config PrometheusConfig
	client *http.Client
	now    func() time.Time
}

// NewPrometheusMetricsProvider validates the endpoint, applies defaults and
// returns a bounded provider. Returns an error if the endpoint is not an
// absolute HTTPS URL without userinfo.
func NewPrometheusMetricsProvider(config PrometheusConfig) (*PrometheusMetricsProvider, error) {
	if err := requireHTTPSURL(config.Endpoint); err != nil {
		return nil, fmt.Errorf("prometheus: %w", err)
	}
	if config.RequestTimeout <= 0 {
		config.RequestTimeout = defaultPrometheusTimeout
	}
	if config.MaxSeries <= 0 {
		config.MaxSeries = defaultPrometheusMaxSeries
	}
	if config.MaxPoints <= 0 {
		config.MaxPoints = defaultPrometheusMaxPoints
	}
	return &PrometheusMetricsProvider{
		config: config,
		client: newBoundedHTTPClient(config.RequestTimeout),
		now:    time.Now,
	}, nil
}

// Name returns the provider identifier.
func (p *PrometheusMetricsProvider) Name() string { return "prometheus" }

// QueryMetrics validates the query, renders the fixed template to PromQL and
// dispatches a bounded range query. Provider failures are reported as
// StateUnavailable with a sanitized Error; validation failures are returned as
// Go errors. Sample coverage, freshness and state are always populated.
func (p *PrometheusMetricsProvider) QueryMetrics(ctx context.Context, query MetricsQuery) (MetricsResult, error) {
	if err := validateMetricsQuery(query); err != nil {
		return MetricsResult{}, err
	}
	promql, err := renderPromQL(query)
	if err != nil {
		return MetricsResult{State: StateUnavailable, Error: sanitizeError(err)}, nil
	}

	result := MetricsResult{
		Template:      query.Template,
		SchemaVersion: MetricsSchemaVersion,
		Coverage:      CoverageInfo{Source: "prometheus"},
	}

	requestCtx, cancel := context.WithTimeout(ctx, p.config.RequestTimeout)
	defer cancel()

	series, err := p.queryRange(requestCtx, promql, query)
	if err != nil {
		result.State = StateUnavailable
		result.Error = sanitizeError(err)
		return result, nil
	}

	if len(series) > p.config.MaxSeries {
		series = series[:p.config.MaxSeries]
	}
	for i := range series {
		if len(series[i].Points) > p.config.MaxPoints {
			series[i].Points = series[i].Points[:p.config.MaxPoints]
		}
	}

	result.Series = series
	result.Coverage = computeCoverage(series, query)
	result.Freshness = computeFreshness(series, p.now)
	result.State = deriveMetricsState(result.Coverage)
	return result, nil
}

// promqlTemplates maps each fixed SLI template to its PromQL. The placeholders
// $ns, $svc and $pod are substituted from the validated query. This map is the
// single source of truth for PromQL generation; no client input reaches
// Prometheus as raw PromQL.
var promqlTemplates = map[string]string{
	TemplateRequestRate: `rate(http_requests_total{cluster_id=$cluster,namespace=$ns,service=$svc}[5m])`,
	TemplateErrorRate:   `rate(http_requests_total{cluster_id=$cluster,namespace=$ns,service=$svc,status=~"5.."}[5m])`,
	TemplateLatencyP99:  `histogram_quantile(0.99, rate(http_request_duration_seconds_bucket{cluster_id=$cluster,namespace=$ns,service=$svc}[5m]))`,
	TemplateCPUUsage:    `rate(container_cpu_usage_seconds_total{cluster_id=$cluster,namespace=$ns,pod=$pod}[5m])`,
	TemplateMemoryUsage: `container_memory_working_set_bytes{cluster_id=$cluster,namespace=$ns,pod=$pod}`,
}

// renderPromQL substitutes the validated query fields into the fixed template.
// The caller has already validated the template via validateMetricsQuery.
func renderPromQL(query MetricsQuery) (string, error) {
	template, ok := promqlTemplates[query.Template]
	if !ok {
		return "", fmt.Errorf("unsupported template %q", query.Template)
	}
	// strconv.Quote emits a valid PromQL string literal and prevents a caller
	// from closing a label matcher and injecting arbitrary PromQL.
	rendered := strings.ReplaceAll(template, "$cluster", strconv.Quote(strconv.FormatInt(query.ClusterID, 10)))
	rendered = strings.ReplaceAll(rendered, "$ns", strconv.Quote(query.Namespace))
	rendered = strings.ReplaceAll(rendered, "$svc", strconv.Quote(query.ServiceName))
	rendered = strings.ReplaceAll(rendered, "$pod", strconv.Quote(query.PodName))
	return rendered, nil
}

func (p *PrometheusMetricsProvider) queryRange(ctx context.Context, promql string, query MetricsQuery) ([]MetricsSeries, error) {
	params := url.Values{}
	params.Set("query", promql)
	params.Set("start", strconv.FormatFloat(float64(query.Start.Unix()), 'f', -1, 64))
	params.Set("end", strconv.FormatFloat(float64(query.End.Unix()), 'f', -1, 64))
	params.Set("step", strconv.FormatFloat(query.Step.Seconds(), 'f', -1, 64))

	requestURL := strings.TrimRight(p.config.Endpoint, "/") + "/api/v1/query_range?" + params.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")

	response, err := p.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, int64(prometheusResponseLimit)+1))
	if err != nil {
		return nil, err
	}
	if len(body) > prometheusResponseLimit {
		return nil, fmt.Errorf("prometheus response exceeds %d bytes", prometheusResponseLimit)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("prometheus returned status %d", response.StatusCode)
	}

	var apiResponse prometheusAPIResponse
	if err := json.Unmarshal(body, &apiResponse); err != nil {
		return nil, fmt.Errorf("decode prometheus response: %w", err)
	}
	if apiResponse.Status != "success" {
		return nil, fmt.Errorf("prometheus query failed: %s", apiResponse.Error)
	}
	if apiResponse.Data.ResultType != "matrix" {
		return nil, fmt.Errorf("prometheus returned unexpected result type %q", apiResponse.Data.ResultType)
	}

	series := make([]MetricsSeries, 0, len(apiResponse.Data.Result))
	for _, item := range apiResponse.Data.Result {
		points := make([]MetricsPoint, 0, len(item.Values))
		for _, sample := range item.Values {
			timestamp, value, ok := parsePrometheusSample(sample)
			if !ok {
				continue
			}
			points = append(points, MetricsPoint{Timestamp: timestamp, Value: value})
		}
		series = append(series, MetricsSeries{Labels: item.Metric, Points: points})
	}
	return series, nil
}

type prometheusAPIResponse struct {
	Status string            `json:"status"`
	Data   prometheusAPIData `json:"data"`
	Error  string            `json:"error,omitempty"`
}

type prometheusAPIData struct {
	ResultType string                `json:"resultType"`
	Result     []prometheusAPIResult `json:"result"`
}

type prometheusAPIResult struct {
	Metric map[string]string  `json:"metric"`
	Values []prometheusSample `json:"values"`
}

// prometheusSample is a [timestamp, value] pair from the Prometheus matrix
// result. The timestamp is a JSON number (Unix seconds) and the value is a JSON
// string (e.g. "1.5", "NaN"). Decoding into [2]any handles both representations.
type prometheusSample [2]any

func parsePrometheusSample(sample prometheusSample) (time.Time, float64, bool) {
	var ts float64
	switch v := sample[0].(type) {
	case float64:
		ts = v
	case string:
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return time.Time{}, 0, false
		}
		ts = f
	default:
		return time.Time{}, 0, false
	}
	var valueText string
	switch v := sample[1].(type) {
	case string:
		valueText = v
	case float64:
		valueText = strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return time.Time{}, 0, false
	}
	value, err := strconv.ParseFloat(valueText, 64)
	if err != nil {
		return time.Time{}, 0, false
	}
	sec := int64(ts)
	nsec := int64((ts - float64(sec)) * 1e9)
	if nsec < 0 {
		nsec = 0
	}
	return time.Unix(sec, nsec).UTC(), value, true
}

// computeCoverage derives sample coverage from the returned series and the
// requested range/step. ExpectedSamples is the per-series expected count
// multiplied by the number of series; MissingSamples is the deficit.
func computeCoverage(series []MetricsSeries, query MetricsQuery) CoverageInfo {
	expectedPerSeries := int(query.End.Sub(query.Start) / query.Step)
	if expectedPerSeries < 0 {
		expectedPerSeries = 0
	}
	expected := expectedPerSeries * len(series)
	total := 0
	for _, s := range series {
		total += len(s.Points)
	}
	missing := expected - total
	if missing < 0 {
		missing = 0
	}
	return CoverageInfo{
		TotalSamples:    total,
		ExpectedSamples: expected,
		MissingSamples:  missing,
		Source:          "prometheus",
	}
}

// computeFreshness returns the timestamp of the most recent point, or now if no
// points were returned, satisfying ADR 0053 §5 (metrics expose freshness).
func computeFreshness(series []MetricsSeries, now func() time.Time) time.Time {
	var latest time.Time
	for _, s := range series {
		for _, p := range s.Points {
			if p.Timestamp.After(latest) {
				latest = p.Timestamp
			}
		}
	}
	if latest.IsZero() {
		return now().UTC()
	}
	return latest.UTC()
}

// deriveMetricsState maps coverage to a result state: any missing samples
// (within a non-empty result) yield "partial", otherwise "complete".
func deriveMetricsState(coverage CoverageInfo) string {
	if coverage.MissingSamples > 0 {
		return StatePartial
	}
	return StateComplete
}
