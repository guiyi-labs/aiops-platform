// Package hpa performs a read-only HorizontalPodAutoscaler posture and
// scaling-advice analysis for one cluster.
//
// It answers the question an operator asks before an incident: "is this HPA
// configured so it can actually protect the workload?" It checks the scaling
// bounds and target, whether the deployment is pinned at its maximum, and —
// when current utilization is supplied — whether the target is being met or
// wildly over-provisioned.
//
// The analyzer is pure and offline (ADR 0004): Evaluate takes only an
// observation bundle (collected read-only from the API server, optionally
// augmented with current utilization from metrics history) and returns a
// Status. It never writes HPA state, never mutates anything.
package hpa

import (
	"time"

	k8sfinding "k8s-aiops.local/backend/internal/finding"
)

// Finding reuses the platform's canonical read-only posture Finding contract
// (see internal/finding) so the frontend renders HPA findings uniformly with
// the other optimization analyzers.
type Finding = k8sfinding.Finding

const (
	SeverityInfo     = k8sfinding.SeverityInfo
	SeverityWarning  = k8sfinding.SeverityWarning
	SeverityCritical = k8sfinding.SeverityCritical
)

// FamilyHPA is reported in Finding.Details["family"] and rolled up into
// Status.ByFamily.
const FamilyHPA = "hpa-sizing"

// Finding codes emitted by Evaluate.
const (
	// CodeNoTarget: the HPA declares no scale target metric, so autoscaling
	// falls back to the Kubernetes default (80% CPU) implicitly.
	CodeNoTarget = "HPA_NO_TARGET_METRIC"
	// CodeAtMaxReplicas: current replicas are already at maxReplicas; the
	// workload cannot absorb more load.
	CodeAtMaxReplicas = "HPA_AT_MAX_REPLICAS"
	// CodeMaxReplicasLow: maxReplicas is very small, leaving almost no
	// headroom to scale.
	CodeMaxReplicasLow = "HPA_MAX_REPLICAS_LOW"
	// CodeOverTarget: current utilization exceeds the target, so the HPA is
	// actively scaling up (or is pinned at max).
	CodeOverTarget = "HPA_UTILIZATION_OVER_TARGET"
	// CodeUnderTarget: current utilization is far below the target; the
	// workload is over-provisioned relative to its scaling policy.
	CodeUnderTarget = "HPA_UTILIZATION_UNDER_TARGET"
)

// HPAInput is the scaling-relevant subset of one HorizontalPodAutoscaler.
type HPAInput struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	UID       string `json:"uid,omitempty"`
	// MinReplicas mirrors spec.minReplicas; nil means the Kubernetes default
	// of 1 applies.
	MinReplicas *int32 `json:"min_replicas,omitempty"`
	// MaxReplicas mirrors spec.maxReplicas (required by the API).
	MaxReplicas int32 `json:"max_replicas"`
	// CurrentReplicas mirrors status.currentReplicas.
	CurrentReplicas int32 `json:"current_replicas"`
	// TargetMetric is the name of the first scale target metric
	// ("cpu"/"memory"/"pods" or a custom name); empty when none is declared.
	TargetMetric string `json:"target_metric,omitempty"`
	// TargetValue is the target threshold: a utilization percentage for
	// cpu/memory, an absolute value for pods/custom metrics.
	TargetValue float64 `json:"target_value"`
	// CurrentUtilizationPct is the current utilization percentage reported in
	// status.currentMetrics (cpu/memory targets only); nil when unavailable.
	CurrentUtilizationPct *int32 `json:"current_utilization_pct,omitempty"`
}

// Inputs is the read-only observation bundle for one cluster evaluation.
type Inputs struct {
	HPAs []HPAInput `json:"hpas,omitempty"`
}

// Empty reports whether the bundle carries nothing to analyze.
func (in Inputs) Empty() bool {
	return len(in.HPAs) == 0
}

// Status is the rollup returned for one cluster evaluation.
type Status struct {
	ClusterID   int64     `json:"cluster_id"`
	EvaluatedAt time.Time `json:"evaluated_at"`
	// Total counts individual checks evaluated; Failed the checks that
	// produced a finding; Passed the remainder.
	Total  int `json:"total"`
	Failed int `json:"failed"`
	Passed int `json:"passed"`
	// Inventory counters give the console a one-line summary of the scope.
	HPAsTotal          int            `json:"hpas_total"`
	AtMaxReplicasCount int            `json:"at_max_replicas_count"`
	OverTargetCount    int            `json:"over_target_count"`
	BySeverity         map[string]int `json:"by_severity"`
	ByFamily           map[string]int `json:"by_family"`
	Findings           []Finding      `json:"findings"`
}
