package signal

import "time"

// Producer identifies the M21-M31 system that emitted a normalized signal.
type Producer string

const (
	ProducerDiagnosis Producer = "diagnosis"
	ProducerAlert     Producer = "alert"
	ProducerMetric    Producer = "metric"
	ProducerPosture   Producer = "posture"
	ProducerAudit     Producer = "audit"
	ProducerChange    Producer = "change"
	ProducerSLO       Producer = "slo"
)

// State is the lifecycle state of a signal occurrence.
type State string

const (
	StateActive    State = "active"
	StateResolved  State = "resolved"
	StateExpired   State = "expired"
	StateDismissed State = "dismissed"
)

// Coverage describes data completeness for a signal occurrence.
type Coverage string

const (
	CoverageComplete    Coverage = "complete"
	CoveragePartial     Coverage = "partial"
	CoverageUnavailable Coverage = "unavailable"
	CoverageTruncated   Coverage = "truncated"
)

// Severity is the normalized severity bucket. SignalDescriptor maps
// producer-specific severities into these stable buckets.
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityWarning  Severity = "warning"
	SeverityInfo     Severity = "info"
)

// ResourceCitation is the exact observed entity the signal refers to. The
// primary resource key is cluster_id + kind + UID; a name-only fallback is
// explicitly marked Incomplete and short-lived.
type ResourceCitation struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
	UID       string `json:"uid,omitempty"`
	// Incomplete is true when UID is missing. The signal is still accepted
	// but downstream correlation must treat it as low-confidence.
	Incomplete bool `json:"incomplete,omitempty"`
}

// EvidenceRef points at a stable, redacted evidence snapshot. It never
// contains raw telemetry, full manifests or secret values.
type EvidenceRef struct {
	// Kind identifies the evidence shape, e.g. "diagnosis_record",
	// "alert_instance", "metric_window", "audit_entry", "posture_finding".
	Kind string `json:"kind"`
	// ID is the stable producer-local identifier (e.g. diagnosis record id).
	ID int64 `json:"id"`
	// ContentHash is an optional SHA256 of the redacted evidence payload.
	// When non-empty it lets callers detect content drift without re-fetching.
	ContentHash string `json:"content_hash,omitempty"`
}

// Occurrence is the normalized signal envelope stored in signal_occurrences.
// One Occurrence per (signal_code, fingerprint, window) contract; duplicate
// producer delivery yields the same row.
type Occurrence struct {
	ID             int64             `json:"id"`
	SignalID       string            `json:"signal_id"`      // stable descriptor code, e.g. "diag.pod.pending.v1"
	SignalCode     string            `json:"signal_code"`    // alias kept for API parity with the plan
	SchemaVersion  string            `json:"schema_version"` // currently "1.0"
	Producer       Producer          `json:"producer"`
	ClusterID      int64             `json:"cluster_id"`
	Namespace      string            `json:"namespace,omitempty"`
	Resource       ResourceCitation  `json:"resource"`
	Severity       Severity          `json:"severity"`
	State          State             `json:"state"`
	Fingerprint    string            `json:"fingerprint"` // SHA256 over stable identity fields
	Coverage       Coverage          `json:"coverage"`
	Freshness      time.Time         `json:"freshness"` // last producer observation time
	WindowStart    *time.Time        `json:"window_start,omitempty"`
	WindowEnd      *time.Time        `json:"window_end,omitempty"`
	ObservedAt     time.Time         `json:"observed_at"`
	IngestedAt     time.Time         `json:"ingested_at"`
	ExpiresAt      *time.Time        `json:"expires_at,omitempty"`
	Attributes     map[string]string `json:"attributes,omitempty"`
	Evidence       []EvidenceRef     `json:"evidence,omitempty"`
	IngestionRunID string            `json:"ingestion_run_id,omitempty"`
}

// SignalDescriptor is the compiled catalog entry for a signal code. Unregistered
// signals fail closed at ingestion.
type SignalDescriptor struct {
	Code             string         `json:"code"`
	SchemaVersion    string         `json:"schema_version"`
	Domain           string         `json:"domain"` // e.g. "workload", "node", "network", "governance", "change"
	SeverityPolicy   SeverityPolicy `json:"severity_policy"`
	CorrelationDims  []string       `json:"correlation_dims"`  // e.g. ["resource_uid","cluster_id","namespace"]
	RequiredEvidence []string       `json:"required_evidence"` // evidence kinds that must be present
	AllowedActions   []string       `json:"allowed_actions"`   // action codes a consumer may propose
	Retention        time.Duration  `json:"retention"`         // TTL for occurrences of this signal
	Description      string         `json:"description"`
}

// SeverityPolicy maps producer-local severity strings to the stable Severity
// buckets. When a producer severity is not found, the Fallback bucket is used.
type SeverityPolicy struct {
	Mappings map[string]Severity `json:"mappings"`
	Fallback Severity            `json:"fallback"`
}

// IngestRequest is the internal input to the signal service. Each producer
// adapter builds one from its native model.
type IngestRequest struct {
	SignalID       string
	Producer       Producer
	ClusterID      int64
	Namespace      string // namespace fallback when Resource.Namespace is empty
	Resource       ResourceCitation
	Severity       string // producer-local severity, mapped via SeverityPolicy
	State          State
	Fingerprint    string // precomputed by the adapter for stable identity
	Coverage       Coverage
	Freshness      time.Time
	WindowStart    *time.Time
	WindowEnd      *time.Time
	ObservedAt     time.Time
	Attributes     map[string]string
	Evidence       []EvidenceRef
	IngestionRunID string
}

// ListFilter bounds signal occurrence queries.
type ListFilter struct {
	ClusterID   *int64
	Namespace   string
	SignalID    string
	Producer    Producer
	State       State
	Severity    Severity
	WindowStart *time.Time
	WindowEnd   *time.Time
	Limit       int
}

// ListResponse is the bounded paginated response.
type ListResponse[T any] struct {
	Items      []T    `json:"items"`
	Total      int64  `json:"total"`
	Truncated  bool   `json:"truncated,omitempty"`
	NextCursor string `json:"next_cursor,omitempty"`
}

// Overview is the bounded AIOps overview payload returned by
// GET /api/v1/aiops/overview. It aggregates source completeness, active
// diagnoses, top signals, recent changes and action outcomes — all filtered
// by M35 scope.
type Overview struct {
	SourceCompleteness map[Producer]Coverage `json:"source_completeness"`
	ActiveDiagnoses    int64                 `json:"active_diagnoses"`
	TopSignals         []OverviewSignal      `json:"top_signals"`
	RecentChanges      []OverviewChange      `json:"recent_changes"`
	ActionOutcomes     OverviewOutcomes      `json:"action_outcomes"`
	GeneratedAt        time.Time             `json:"generated_at"`
	// Partial is true when at least one producer is unavailable/truncated.
	Partial bool `json:"partial,omitempty"`
}

type OverviewSignal struct {
	SignalID  string    `json:"signal_id"`
	Producer  Producer  `json:"producer"`
	Severity  Severity  `json:"severity"`
	Count     int64     `json:"count"`
	LastSeen  time.Time `json:"last_seen"`
	Namespace string    `json:"namespace,omitempty"`
}

type OverviewChange struct {
	Kind       string     `json:"kind"` // "promotion", "backup", "maintenance", "restore", "audit"
	ID         int64      `json:"id"`
	ClusterID  int64      `json:"cluster_id"`
	Namespace  string     `json:"namespace,omitempty"`
	Status     string     `json:"status"`
	Actor      string     `json:"actor,omitempty"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

type OverviewOutcomes struct {
	Succeeded int64 `json:"succeeded"`
	Failed    int64 `json:"failed"`
	Pending   int64 `json:"pending"`
}
