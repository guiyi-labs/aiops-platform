// Package capacity performs a read-only cluster capacity-trend prediction.
//
// It answers the question a platform operator asks before a capacity incident:
// "at the current growth rate, when will this cluster run out of CPU or
// memory?" It aggregates node allocatable capacity and a window of observed
// node usage (from the metrics history store) into a per-resource (CPU/memory)
// time series, fits a linear trend, and projects utilization forward to a
// horizon. Resources predicted to saturate within the horizon are reported as
// findings.
//
// The analyzer is pure (ADR 0004): Evaluate takes only an Inputs bundle
// (collected read-only from the API server and metrics history) and returns a
// Status. It never reaches the cluster, never queries a metrics backend, and
// never mutates anything.
package capacity

import (
	"time"

	k8sfinding "k8s-aiops.local/backend/internal/finding"
)

// Finding reuses the platform's canonical read-only posture Finding contract
// (see internal/finding) so the frontend renders capacity findings uniformly
// with the other optimization analyzers.
type Finding = k8sfinding.Finding

const (
	SeverityInfo     = k8sfinding.SeverityInfo
	SeverityWarning  = k8sfinding.SeverityWarning
	SeverityCritical = k8sfinding.SeverityCritical
)

// FamilyCapacity is reported in Finding.Details["family"] and rolled up into
// Status.ByFamily.
const FamilyCapacity = "capacity-trend"

// Finding codes emitted by Evaluate.
const (
	// CodeSaturationRisk: a resource's utilization is projected to reach a
	// risky level (>=80%) within the horizon, or to saturate (>=100%) within a
	// short window; details carry the projection.
	CodeSaturationRisk = "CAPACITY_SATURATION_RISK"
)

// Sample is one utilization observation: a raw usage value at a point in time.
// Value is in the same unit as ResourceTrend.Capacity (nanocores for CPU,
// bytes for memory).
type Sample struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

// ResourceTrend is one resource's (CPU or memory) capacity plus its observed
// usage time series over the analysis window.
type ResourceTrend struct {
	Capacity int64    `json:"capacity"` // total allocatable (nanocores / bytes)
	Samples  []Sample `json:"samples"`
}

// Inputs is the read-only observation bundle for one cluster evaluation.
type Inputs struct {
	CPU         ResourceTrend `json:"cpu"`
	Memory      ResourceTrend `json:"memory"`
	HorizonDays int           `json:"horizon_days"` // projection window; 0 → DefaultHorizonDays
}

// DefaultHorizonDays is the projection window used when Inputs.HorizonDays is 0.
const DefaultHorizonDays = 30

// Empty reports whether the bundle carries nothing to analyze.
func (in Inputs) Empty() bool {
	return in.CPU.Capacity == 0 && len(in.CPU.Samples) == 0 &&
		in.Memory.Capacity == 0 && len(in.Memory.Samples) == 0
}

// Status is the rollup returned for one cluster evaluation.
type Status struct {
	ClusterID   int64     `json:"cluster_id"`
	EvaluatedAt time.Time `json:"evaluated_at"`
	// Total counts resources evaluated (CPU + memory); Failed the number that
	// produced a saturation-risk finding; Passed the remainder.
	Total  int `json:"total"`
	Failed int `json:"failed"`
	Passed int `json:"passed"`
	// Inventory counters give the console a one-line summary of the scope.
	CPUCapacityNanocores int64   `json:"cpu_capacity_nanocores"`
	MemCapacityBytes     int64   `json:"mem_capacity_bytes"`
	CPUCurrentPct        float64 `json:"cpu_current_pct"`
	MemCurrentPct        float64 `json:"mem_current_pct"`
	// SaturationInDays is days until 100% utilization at the fitted growth
	// rate; -1 when the resource is not growing toward saturation.
	CPUSaturationInDays float64        `json:"cpu_saturation_in_days"`
	MemSaturationInDays float64        `json:"mem_saturation_in_days"`
	BySeverity          map[string]int `json:"by_severity"`
	ByFamily            map[string]int `json:"by_family"`
	Findings            []Finding      `json:"findings"`
}
