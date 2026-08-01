// Package slo implements the M41 SLO, error-budget and impact model.
//
// M41 converts provider metrics and topology into user-visible service impact.
// V1 supports server-owned SLI templates only — the API never accepts raw
// PromQL. An SLO stores authorized service/scope, template and version,
// objective, rolling window, missing-data policy, owner, fast/slow burn
// settings and enabled state.
//
// The native M21-M31 signal path is unchanged; M41 is a deterministic
// evaluator that produces bounded SLO views and burn alerts that enter the
// existing M27 alert lifecycle. Unsupported or missing traffic metrics remain
// "unavailable"; workload readiness alone cannot satisfy a request SLO.
package slo

import "time"

// SLITemplate enumerates the server-owned SLI templates. These are the only
// template values accepted by SLO definitions; clients cannot inject
// arbitrary PromQL or custom formulas.
type SLITemplate string

const (
	// TemplateRequestSuccessRatio: good = successful requests, total = all
	// requests. Requires traffic metrics (request_rate + error_rate).
	TemplateRequestSuccessRatio SLITemplate = "request_success_ratio"
	// TemplateRequestLatencyTargetRatio: good = requests under the configured
	// latency threshold, total = all requests. Requires latency metrics.
	TemplateRequestLatencyTargetRatio SLITemplate = "request_latency_target_ratio"
	// TemplateWorkloadReadiness: good = ready pods, total = desired pods.
	// This is an explicitly labeled platform-health indicator — it is never
	// a substitute for request availability and cannot satisfy a request
	// SLO.
	TemplateWorkloadReadiness SLITemplate = "workload_readiness"
)

// TemplateVersion is the schema version of the SLI template evaluation
// contract. Bumped when the deterministic formula changes.
const TemplateVersion = "1.0"

// MissingDataPolicy declares how an evaluation behaves when traffic metrics
// are sparse or unavailable. The default and only safe policy is
// "unavailable" — the SLO reports StateUnavailable and never fabricates
// normal health. "fail_open" is admitted only for the workload_readiness
// template and only when explicitly configured by an operator.
type MissingDataPolicy string

const (
	MissingDataUnavailable MissingDataPolicy = "unavailable"
	MissingDataFailOpen    MissingDataPolicy = "fail_open"
)

// EvaluationState is the lifecycle state of an SLO evaluation.
type EvaluationState string

const (
	// StateHealthy: ratio >= objective (no budget consumed beyond target).
	StateHealthy EvaluationState = "healthy"
	// StateBurningSlow: slow burn rate exceeded (consuming budget too fast
	// over the slow window). Fires the slow burn alert.
	StateBurningSlow EvaluationState = "burning_slow"
	// StateBurningFast: fast burn rate exceeded (consuming budget too fast
	// over the fast window). Fires the fast burn alert.
	StateBurningFast EvaluationState = "burning_fast"
	// StateBreached: ratio < objective for the full rolling window — budget
	// exhausted. Fires the breach alert.
	StateBreached EvaluationState = "breached"
	// StateUnavailable: missing data or insufficient samples; no claim about
	// health is made.
	StateUnavailable EvaluationState = "unavailable"
)

// EvaluationCoverage describes data completeness for an SLO evaluation.
type EvaluationCoverage string

const (
	CoverageComplete    EvaluationCoverage = "complete"
	CoveragePartial     EvaluationCoverage = "partial"
	CoverageUnavailable EvaluationCoverage = "unavailable"
)

// ServiceRef is the authorized service/scope the SLO measures. The primary
// key is cluster_id + kind + UID; a name-only fallback is marked Incomplete.
// Mirrors signal.ResourceCitation and topology.ResourceCitation.
type ServiceRef struct {
	Kind       string `json:"kind"`
	Namespace  string `json:"namespace,omitempty"`
	Name       string `json:"name"`
	UID        string `json:"uid,omitempty"`
	Incomplete bool   `json:"incomplete,omitempty"`
}

// ActorRef is the owner or creator of an SLO. Mirrors alert.ActorRef.
type ActorRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// Definition is an SLO definition. Edits are versioned: every mutation
// increments Version and the historical evaluations remain queryable by
// version. SLO edits never rewrite historical evaluations.
type Definition struct {
	ID                    int64             `json:"id"`
	ClusterID             int64             `json:"cluster_id"`
	Service               ServiceRef        `json:"service"`
	Template              SLITemplate       `json:"template"`
	TemplateVersion       string            `json:"template_version"`
	Objective             float64           `json:"objective"`              // e.g., 0.99, 0.999
	RollingWindowSeconds  int               `json:"rolling_window_seconds"` // e.g., 30d = 2592000
	MissingDataPolicy     MissingDataPolicy `json:"missing_data_policy"`
	LatencyThresholdMs    int               `json:"latency_threshold_ms,omitempty"` // only for request_latency_target_ratio
	Owner                 ActorRef          `json:"owner"`
	FastBurnRate          float64           `json:"fast_burn_rate"`           // e.g., 14.4
	FastBurnWindowSeconds int               `json:"fast_burn_window_seconds"` // e.g., 3600 (1h)
	SlowBurnRate          float64           `json:"slow_burn_rate"`           // e.g., 1.0
	SlowBurnWindowSeconds int               `json:"slow_burn_window_seconds"` // e.g., 21600 (6h)
	Enabled               bool              `json:"enabled"`
	Version               int               `json:"version"` // incremented on each edit
	Creator               ActorRef          `json:"creator"`
	CreatedAt             time.Time         `json:"created_at"`
	UpdatedAt             time.Time         `json:"updated_at"`
}

// Evaluation is a single deterministic evaluation of an SLO over a window.
// Stored append-only in slo_evaluations; historical evaluations remain
// queryable by version.
type Evaluation struct {
	ID              int64              `json:"id"`
	SLOID           int64              `json:"slo_id"`
	Version         int                `json:"version"` // SLO version at evaluation time
	WindowStart     time.Time          `json:"window_start"`
	WindowEnd       time.Time          `json:"window_end"`
	GoodEvents      float64            `json:"good_events"`
	TotalEvents     float64            `json:"total_events"`
	Ratio           float64            `json:"ratio"`        // good / total (0..1)
	TargetRatio     float64            `json:"target_ratio"` // objective
	ErrorBudget     float64            `json:"error_budget"` // 1 - objective
	RemainingBudget float64            `json:"remaining_budget"`
	BurnRate        float64            `json:"burn_rate"` // (1 - ratio) / error_budget
	State           EvaluationState    `json:"state"`
	Coverage        EvaluationCoverage `json:"coverage"`
	EvaluatedAt     time.Time          `json:"evaluated_at"`
}

// DefinitionFilter bounds SLO definition queries.
type DefinitionFilter struct {
	ClusterID int64
	Namespace string
	Template  SLITemplate
	Enabled   *bool
	OwnerID   int64
	Limit     int
}

// DefinitionListResponse is the bounded paginated response.
type DefinitionListResponse struct {
	Items     []Definition `json:"items"`
	Total     int64        `json:"total"`
	Truncated bool         `json:"truncated,omitempty"`
}

// EvaluationFilter bounds SLO evaluation queries.
type EvaluationFilter struct {
	SLOID     int64
	Version   *int
	StartTime *time.Time
	EndTime   *time.Time
	State     EvaluationState
	Limit     int
}

// EvaluationListResponse is the bounded paginated response.
type EvaluationListResponse struct {
	Items     []Evaluation `json:"items"`
	Total     int64        `json:"total"`
	Truncated bool         `json:"truncated,omitempty"`
}

// SLOView is the impact view returned by the SLO API. It links the current
// evaluation to authorized signals, recent changes and diagnoses so callers
// can answer "what is the impact on this service right now?".
type SLOView struct {
	Definition        Definition `json:"definition"`
	CurrentEvaluation Evaluation `json:"current_evaluation"`
	RecentSignals     int        `json:"recent_signals"`
	RecentChanges     int        `json:"recent_changes"`
	RecentDiagnoses   int        `json:"recent_diagnoses"`
	BurnAlertActive   bool       `json:"burn_alert_active"`
	GeneratedAt       time.Time  `json:"generated_at"`
}

// CreateDefinitionInput is the typed input for creating an SLO definition.
// LatencyThresholdMs is required only for request_latency_target_ratio.
type CreateDefinitionInput struct {
	ClusterID             int64
	Service               ServiceRef
	Template              SLITemplate
	Objective             float64
	RollingWindowSeconds  int
	MissingDataPolicy     MissingDataPolicy
	LatencyThresholdMs    int
	Owner                 ActorRef
	FastBurnRate          float64
	FastBurnWindowSeconds int
	SlowBurnRate          float64
	SlowBurnWindowSeconds int
	Enabled               bool
	Creator               ActorRef
}

// PatchDefinitionInput is the typed input for editing an SLO definition.
// Nil fields are not modified. Every patch increments Version.
type PatchDefinitionInput struct {
	Objective             *float64
	RollingWindowSeconds  *int
	MissingDataPolicy     *MissingDataPolicy
	LatencyThresholdMs    *int
	Owner                 *ActorRef
	FastBurnRate          *float64
	FastBurnWindowSeconds *int
	SlowBurnRate          *float64
	SlowBurnWindowSeconds *int
	Enabled               *bool
	Actor                 ActorRef
}

// Evaluation bounds enforced by the catalog and migration CHECK constraints.
const (
	MinObjective            = 0.0
	MaxObjective            = 1.0
	MinBurnRate             = 0.0
	MinRollingWindowSeconds = 300            // 5 minutes
	MaxRollingWindowSeconds = 30 * 24 * 3600 // 30 days
	MinBurnWindowSeconds    = 60             // 1 minute
	MaxBurnWindowSeconds    = 24 * 3600      // 24 hours
	MinLatencyThresholdMs   = 1
	MaxLatencyThresholdMs   = 60000
	MaxServiceNameLength    = 253
	MaxNamespaceLength      = 63
)
