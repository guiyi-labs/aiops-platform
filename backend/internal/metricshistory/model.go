package metricshistory

import "time"

const (
	ResourceNode = "Node"
	ResourcePod  = "Pod"
	// ResourceDeployment carries M99-B workload readiness gauges. Namespace is
	// required; container must be empty (the sample is workload-scoped).
	ResourceDeployment = "Deployment"

	MetricCPU    = "cpu"
	MetricMemory = "memory"
	// Workload readiness gauges (M99-B): per-collection-run replica counts
	// for the SLO workload_readiness source. readiness_ready counts ready
	// replicas; readiness_total counts desired replicas. Both are sampled at
	// collection time and converted to cumulative counters by the SLO source.
	MetricReadinessReady = "readiness_ready"
	MetricReadinessTotal = "readiness_total"

	UnitNanocores = "nanocores"
	UnitBytes     = "bytes"
	UnitCount     = "count"

	SourceSucceeded   = "succeeded"
	SourceUnavailable = "unavailable"
	SourceTimedOut    = "timed_out"
	SourceFailed      = "failed"

	CollectionSucceeded   = "succeeded"
	CollectionPartial     = "partial"
	CollectionUnavailable = "unavailable"
	CollectionTimedOut    = "timed_out"
	CollectionFailed      = "failed"
)

type Config struct {
	Retention               time.Duration
	MaxSamplesPerCollection int
	MaxQueryWindow          time.Duration
	MaxQueryPoints          int
	CleanupBatchSize        int
}

type TargetCoverage struct {
	Status   string `json:"status"`
	Sampled  int    `json:"sampled"`
	Total    int    `json:"total"`
	Complete bool   `json:"complete"`
}

type SampleInput struct {
	ResourceKind      string
	ResourceNamespace string
	ResourceName      string
	ResourceUID       string
	ContainerName     string
	MetricName        string
	Value             int64
	SourceTimestamp   time.Time
	Window            time.Duration
}

type CollectionInput struct {
	ClusterID   int64
	Nodes       TargetCoverage
	Pods        TargetCoverage
	FailureCode string
	StartedAt   time.Time
	CompletedAt time.Time
	Samples     []SampleInput
}

type CollectionRun struct {
	ID          int64          `json:"id"`
	ClusterID   int64          `json:"cluster_id"`
	Status      string         `json:"status"`
	Nodes       TargetCoverage `json:"nodes"`
	Pods        TargetCoverage `json:"pods"`
	FailureCode string         `json:"failure_code,omitempty"`
	StartedAt   time.Time      `json:"started_at"`
	CompletedAt time.Time      `json:"completed_at"`
	ExpiresAt   time.Time      `json:"expires_at"`
}

type Sample struct {
	CollectionRunID    int64     `json:"-"`
	ClusterID          int64     `json:"-"`
	ResourceKind       string    `json:"resource_kind"`
	ResourceNamespace  string    `json:"resource_namespace,omitempty"`
	ResourceName       string    `json:"resource_name"`
	ResourceUID        string    `json:"resource_uid,omitempty"`
	ContainerName      string    `json:"container_name,omitempty"`
	MetricName         string    `json:"metric_name"`
	Value              int64     `json:"value"`
	Unit               string    `json:"unit"`
	SourceTimestamp    time.Time `json:"source_timestamp"`
	WindowMilliseconds int       `json:"window_milliseconds"`
	CollectedAt        time.Time `json:"collected_at"`
	ExpiresAt          time.Time `json:"-"`
}

type Collection struct {
	Run     CollectionRun
	Samples []Sample
}

type SeriesQuery struct {
	ClusterID         int64
	ResourceKind      string
	ResourceNamespace string
	ResourceName      string
	ContainerName     string
	MetricName        string
	From              time.Time
	To                time.Time
	Limit             int
}

type Series struct {
	ClusterID         int64  `json:"cluster_id"`
	ResourceKind      string `json:"resource_kind"`
	ResourceNamespace string `json:"resource_namespace,omitempty"`
	ResourceName      string `json:"resource_name"`
	ContainerName     string `json:"container_name,omitempty"`
	MetricName        string `json:"metric_name"`
	Unit              string `json:"unit"`
}

type Point struct {
	Value              int64     `json:"value"`
	SourceTimestamp    time.Time `json:"source_timestamp"`
	WindowMilliseconds int       `json:"window_milliseconds"`
	CollectedAt        time.Time `json:"collected_at"`
}

type QueryCoverage struct {
	Collections int `json:"collections"`
	Succeeded   int `json:"succeeded"`
	Partial     int `json:"partial"`
	Unavailable int `json:"unavailable"`
	TimedOut    int `json:"timed_out"`
	Failed      int `json:"failed"`
	Points      int `json:"points"`
	Missing     int `json:"missing"`
}

type QueryLimits struct {
	MaxWindowSeconds int `json:"max_window_seconds"`
	MaxPoints        int `json:"max_points"`
}

type SeriesResponse struct {
	Series    Series        `json:"series"`
	From      time.Time     `json:"from"`
	To        time.Time     `json:"to"`
	Points    []Point       `json:"points"`
	Coverage  QueryCoverage `json:"coverage"`
	Limits    QueryLimits   `json:"limits"`
	Truncated bool          `json:"truncated"`
}

type RepositorySeriesResult struct {
	Points   []Point
	Coverage QueryCoverage
	Total    int
}
