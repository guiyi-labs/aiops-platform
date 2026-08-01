// Package finops provides a read-only Kubernetes FinOps advisor: it turns
// already-collected resource requests/limits and actual usage metrics into
// right-sizing recommendations and cost-waste estimates. It is intentionally
// read-only and NEVER mutates workload specs (no VPA, no autopilot, no
// admission control). It reuses the metricshistory sample model (CPU in
// nanocores, memory in bytes) so it slots into the existing collection
// pipeline without new data sources.
package finops

import (
	"time"

	k8sfinding "k8s-aiops.local/backend/internal/finding"
)

const (
	// Unset is the sentinel for "resource quantity not specified". Kubernetes
	// rejects a zero request, so 0 is never a valid request value here.
	Unset int64 = -1

	SeverityInfo     = k8sfinding.SeverityInfo
	SeverityWarning  = k8sfinding.SeverityWarning
	SeverityCritical = k8sfinding.SeverityCritical
)

// Quantity holds resource requests/limits in the same units metricshistory
// uses: CPU in nanocores, memory in bytes. A value of Unset means the field
// was not specified on the container.
type Quantity struct {
	CPURequest int64 `json:"cpu_request"` // nanocores
	CPULimit   int64 `json:"cpu_limit"`   // nanocores
	MemRequest int64 `json:"mem_request"` // bytes
	MemLimit   int64 `json:"mem_limit"`   // bytes
}

// IsRequestSet reports whether a CPU/memory request was specified.
func (q Quantity) IsRequestSet() bool {
	return q.CPURequest != Unset || q.MemRequest != Unset
}

// ContainerInput is one container's spec requests/limits plus observed usage
// percentiles (p50/p95/max) in the same units. Replicas is the workload
// replica count, used to aggregate wasted cost across the fleet.
type ContainerInput struct {
	ClusterID     int64  `json:"cluster_id"`
	Namespace     string `json:"namespace"`
	WorkloadKind  string `json:"workload_kind"`
	WorkloadName  string `json:"workload_name"`
	ContainerName string `json:"container_name"`

	Requests Quantity `json:"requests"`
	Limits   Quantity `json:"limits"`

	CPUUsageP50 int64 `json:"cpu_usage_p50"` // nanocores
	CPUUsageP95 int64 `json:"cpu_usage_p95"` // nanocores
	CPUUsageMax int64 `json:"cpu_usage_max"` // nanocores
	MemUsageP50 int64 `json:"mem_usage_p50"` // bytes
	MemUsageP95 int64 `json:"mem_usage_p95"` // bytes
	MemUsageMax int64 `json:"mem_usage_max"` // bytes

	Replicas int32 `json:"replicas"`
}

// CostRate is the monthly unit price used to translate idle resources into
// dollars. Defaults are illustrative and MUST be overridden per cloud/region.
type CostRate struct {
	PerCoreMonth float64 `json:"per_core_month"` // USD per vCPU-month
	PerGBMonth   float64 `json:"per_gb_month"`   // USD per GB-month
}

// DefaultCostRate returns illustrative on-demand prices. Operators should
// override these with their actual cloud billing rates.
func DefaultCostRate() CostRate {
	return CostRate{PerCoreMonth: 30.0, PerGBMonth: 4.0}
}

// Recommendation is the right-sizing advice for one container.
type Recommendation struct {
	ClusterID     int64  `json:"cluster_id"`
	Namespace     string `json:"namespace"`
	WorkloadKind  string `json:"workload_kind"`
	WorkloadName  string `json:"workload_name"`
	ContainerName string `json:"container_name"`

	SuggestedRequests Quantity `json:"suggested_requests"`
	SuggestedLimits   Quantity `json:"suggested_limits"`

	Severity  string  `json:"severity"`
	Rationale string  `json:"rationale"`
	Code      string  `json:"code"`

	// MonthlyWasteUSD is the estimated monthly cost of idle requested
	// resources for this container across all replicas.
	MonthlyWasteUSD float64 `json:"monthly_waste_usd"`
	Replicas        int32   `json:"replicas"`
}

// WasteSummary is the rollup across all evaluated containers in a cluster.
type WasteSummary struct {
	ClusterID               int64            `json:"cluster_id"`
	ContainersEvaluated     int              `json:"containers_evaluated"`
	ContainersOverProvisioned int          `json:"containers_over_provisioned"`
	MonthlyWasteUSD         float64          `json:"monthly_waste_usd"`
	CPUIdleCores            float64          `json:"cpu_idle_cores"`
	MemIdleGB               float64          `json:"mem_idle_gb"`
	Recommendations         []Recommendation `json:"recommendations"`
	EvaluatedAt             time.Time        `json:"evaluated_at"`
}

// ToFindings converts the over-provisioned recommendations into the canonical
// read-only finding contract for uniform frontend rendering.
func (s WasteSummary) ToFindings() []k8sfinding.Finding {
	out := make([]k8sfinding.Finding, 0, len(s.Recommendations))
	for _, r := range s.Recommendations {
		out = append(out, k8sfinding.Finding{
			Code:     r.Code,
			Severity: r.Severity,
			Summary:  r.Rationale,
			Resource: k8sfinding.ResourceCitation{
				Kind:      r.WorkloadKind,
				Namespace: r.Namespace,
				Name:      r.WorkloadName,
			},
			Details: map[string]string{
				"container":         r.ContainerName,
				"monthly_waste_usd": formatUSD(r.MonthlyWasteUSD),
				"replicas":          itoa(int(r.Replicas)),
			},
			ObservedAt: k8sfinding.RFC3339(s.EvaluatedAt),
		})
	}
	return out
}
