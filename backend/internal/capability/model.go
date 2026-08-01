// Package capability defines the M37A metrics and log provider contracts.
//
// MetricsProvider and LogProvider expose fixed template/AST query shapes only.
// Clients cannot supply PromQL, LogQL, arbitrary labels or provider URLs.
// Provider endpoints and credentials are server-configured; request input can
// never redirect a query. All adapters are disabled by default — NopMetricsProvider
// and NopLogProvider are the infrastructure defaults and return "unavailable".
package capability

import "time"

// Query range and result bounds mandated by ADR 0053 §5 and optimization-plan §11.1.
const (
	// MaxLogRange is the hard stop for historical log queries (7 days).
	MaxLogRange = 7 * 24 * time.Hour
	// DefaultLogRange is the default lookback when callers omit a range.
	DefaultLogRange = 1 * time.Hour
	// MaxLogLimit bounds the number of log entries returned per query.
	MaxLogLimit = 500
	// MaxLogBytes bounds the total bytes of log entries returned per query (1 MiB).
	MaxLogBytes = 1 << 20
	// MaxTextFilter bounds the text filter length accepted from clients.
	MaxTextFilter = 256
	// MetricsSchemaVersion is the schema version reported by every MetricsResult.
	MetricsSchemaVersion = "1.0"
)

// Fixed SLI template identifiers. These are the only template values accepted
// by MetricsQuery.Template; clients cannot inject arbitrary PromQL.
const (
	TemplateRequestRate = "request_rate"
	TemplateErrorRate   = "error_rate"
	TemplateLatencyP99  = "latency_p99"
	TemplateCPUUsage    = "cpu_usage"
	TemplateMemoryUsage = "memory_usage"
)

// Result states shared by metrics and log results.
const (
	StateComplete    = "complete"
	StatePartial     = "partial"
	StateUnavailable = "unavailable"
	StateTruncated   = "truncated"
)

// Log query directions accepted by LogQuery.
const (
	DirectionForward  = "forward"
	DirectionBackward = "backward"
)

// MetricsQuery is the fixed, client-supplied input for a metrics query. It
// carries identifying fields and a fixed template enum only — never PromQL.
type MetricsQuery struct {
	ClusterID   int64
	Namespace   string
	ServiceName string
	PodName     string
	Container   string
	Template    string
	Start       time.Time
	End         time.Time
	Step        time.Duration
}

// MetricsResult is the normalized outcome of a MetricsProvider.QueryMetrics call.
// State is one of "complete", "partial" or "unavailable". Error is sanitized and
// never contains provider endpoints or credentials.
type MetricsResult struct {
	Template      string
	SchemaVersion string
	Series        []MetricsSeries
	Coverage      CoverageInfo
	State         string
	Freshness     time.Time
	Error         string
}

// MetricsSeries is a single labelled time series.
type MetricsSeries struct {
	Labels map[string]string
	Points []MetricsPoint
}

// MetricsPoint is a single timestamped sample.
type MetricsPoint struct {
	Timestamp time.Time
	Value     float64
}

// CoverageInfo describes sample coverage, missing-data policy and source for a
// MetricsResult, satisfying ADR 0053 §5 (metrics expose sample coverage and a
// missing-data policy).
type CoverageInfo struct {
	TotalSamples    int
	ExpectedSamples int
	MissingSamples  int
	Source          string
}

// LogQuery is the fixed, client-supplied input for a historical log query. It
// carries identifying fields, bounded text, direction and limits only — never
// LogQL. Range defaults to 1 hour and hard-stops at MaxLogRange.
type LogQuery struct {
	ClusterID  int64
	Namespace  string
	PodName    string
	Container  string
	TextFilter string
	Start      time.Time
	End        time.Time
	Direction  string
	Limit      int
	MaxBytes   int
}

// LogResult is the normalized outcome of a LogProvider.QueryLogs call. State is
// one of "complete", "partial", "unavailable" or "truncated". Error is sanitized
// and never contains provider endpoints or credentials.
type LogResult struct {
	Entries       []LogEntry
	State         string
	TotalReturned int
	Error         string
}

// LogEntry is a single historical log line with its stream labels resolved.
type LogEntry struct {
	Timestamp time.Time
	Namespace string
	Pod       string
	Container string
	Stream    string
	Line      string
}
