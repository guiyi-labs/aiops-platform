package capability

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// Sentinel errors for client-side query validation. These are safe to surface
// to callers; provider-side failures are reported via MetricsResult/LogResult
// state instead, with sanitized Error fields.
var (
	ErrInvalidMetricsQuery = errors.New("invalid metrics query")
	ErrInvalidLogQuery     = errors.New("invalid log query")
)

// MetricsProvider is the compiled-time contract for fixed SLI template-based
// metrics queries. Implementations must not accept PromQL from callers.
type MetricsProvider interface {
	QueryMetrics(ctx context.Context, query MetricsQuery) (MetricsResult, error)
	Name() string
}

// LogProvider is the compiled-time contract for read-only historical log
// queries. Implementations must not accept LogQL from callers.
type LogProvider interface {
	QueryLogs(ctx context.Context, query LogQuery) (LogResult, error)
	Name() string
}

// Compile-time assertions that the Nop providers satisfy the interfaces.
var (
	_ MetricsProvider = NopMetricsProvider{}
	_ LogProvider     = NopLogProvider{}
)

// NopMetricsProvider is the disabled-by-default MetricsProvider. It returns
// "unavailable" for every query without contacting any upstream, satisfying
// ADR 0053 §6 (all adapters disabled by default).
type NopMetricsProvider struct{}

// QueryMetrics returns an unavailable result without side effects.
func (NopMetricsProvider) QueryMetrics(_ context.Context, _ MetricsQuery) (MetricsResult, error) {
	return MetricsResult{State: StateUnavailable, Error: "metrics provider is not configured"}, nil
}

// Name returns the provider identifier.
func (NopMetricsProvider) Name() string { return "nop" }

// NopLogProvider is the disabled-by-default LogProvider. It returns
// "unavailable" for every query without contacting any upstream.
type NopLogProvider struct{}

// QueryLogs returns an unavailable result without side effects.
func (NopLogProvider) QueryLogs(_ context.Context, _ LogQuery) (LogResult, error) {
	return LogResult{State: StateUnavailable, Error: "log provider is not configured"}, nil
}

// Name returns the provider identifier.
func (NopLogProvider) Name() string { return "nop" }

var validTemplates = map[string]struct{}{
	TemplateRequestRate: {},
	TemplateErrorRate:   {},
	TemplateLatencyP99:  {},
	TemplateCPUUsage:    {},
	TemplateMemoryUsage: {},
}

var validDirections = map[string]struct{}{
	DirectionForward:  {},
	DirectionBackward: {},
}

// validateMetricsQuery checks the structural validity of a MetricsQuery. It
// enforces the fixed template enum, a positive step, ordered bounds and the
// 7-day hard stop. It rejects any query shape that could inject PromQL.
func validateMetricsQuery(query MetricsQuery) error {
	if query.ClusterID <= 0 {
		return fmt.Errorf("%w: cluster id must be positive", ErrInvalidMetricsQuery)
	}
	if query.Namespace == "" {
		return fmt.Errorf("%w: namespace is required", ErrInvalidMetricsQuery)
	}
	if _, ok := validTemplates[query.Template]; !ok {
		return fmt.Errorf("%w: template %q is not supported", ErrInvalidMetricsQuery, query.Template)
	}
	if query.Step <= 0 {
		return fmt.Errorf("%w: step must be positive", ErrInvalidMetricsQuery)
	}
	if query.End.Before(query.Start) {
		return fmt.Errorf("%w: end must not precede start", ErrInvalidMetricsQuery)
	}
	if query.End.Sub(query.Start) > MaxLogRange {
		return fmt.Errorf("%w: range must not exceed %s", ErrInvalidMetricsQuery, MaxLogRange)
	}
	return nil
}

// validateLogQuery checks the structural validity of a LogQuery. It enforces
// the text-filter length, entry/byte limits, direction enum, ordered bounds and
// the 7-day hard stop. It rejects any query shape that could inject LogQL.
func validateLogQuery(query LogQuery) error {
	if query.ClusterID <= 0 {
		return fmt.Errorf("%w: cluster id must be positive", ErrInvalidLogQuery)
	}
	if query.Namespace == "" {
		return fmt.Errorf("%w: namespace is required", ErrInvalidLogQuery)
	}
	if len(query.TextFilter) > MaxTextFilter {
		return fmt.Errorf("%w: text filter must not exceed %d characters", ErrInvalidLogQuery, MaxTextFilter)
	}
	if query.Limit < 0 || query.Limit > MaxLogLimit {
		return fmt.Errorf("%w: limit must be between 0 and %d", ErrInvalidLogQuery, MaxLogLimit)
	}
	if query.MaxBytes < 0 || query.MaxBytes > MaxLogBytes {
		return fmt.Errorf("%w: max bytes must be between 0 and %d", ErrInvalidLogQuery, MaxLogBytes)
	}
	if _, ok := validDirections[query.Direction]; !ok {
		return fmt.Errorf("%w: direction %q is not supported", ErrInvalidLogQuery, query.Direction)
	}
	if query.End.Before(query.Start) {
		return fmt.Errorf("%w: end must not precede start", ErrInvalidLogQuery)
	}
	if query.End.Sub(query.Start) > MaxLogRange {
		return fmt.Errorf("%w: range must not exceed %s", ErrInvalidLogQuery, MaxLogRange)
	}
	return nil
}

// requireHTTPSURL validates that raw is an absolute HTTPS URL without userinfo.
// It is the runtime SSRF guard for provider endpoints: request input cannot
// redirect a query because the endpoint is server-configured and validated here.
func requireHTTPSURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid URL %q: %w", raw, err)
	}
	if parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return fmt.Errorf("endpoint %q must be an absolute HTTPS URL without userinfo", raw)
	}
	return nil
}

// newBoundedHTTPClient returns an HTTP client that enforces a bounded timeout
// and never follows redirects, matching the SSRF controls in ADR 0053 §5.
func newBoundedHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// sanitizeError converts a provider error into a generic, leak-free message for
// the result Error field. Provider endpoints and credentials never enter API,
// audit, logs, evidence or Git (ADR 0053 §5).
func sanitizeError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "provider request timed out"
	}
	return "provider request failed"
}
