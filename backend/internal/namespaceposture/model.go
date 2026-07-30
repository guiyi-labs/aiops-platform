package namespaceposture

import (
	"time"

	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

// SourceStatus describes the completeness of evidence for one posture section.
// It intentionally uses stable, user-facing strings so callers can render
// badges and warnings deterministically.
type SourceStatus string

const (
	SourceComplete    SourceStatus = "complete"
	SourcePartial     SourceStatus = "partial"
	SourceTruncated   SourceStatus = "truncated"
	SourceUnavailable SourceStatus = "unavailable"
)

// EvidenceCitation records where a posture section came from, whether it was
// truncated, and whether any upstream call failed partially. The posture is
// source-cited so operators can always trace each finding back to the exact
// Kubernetes API call that produced it.
type EvidenceCitation struct {
	APIPath     string       `json:"api_path"`
	Status      SourceStatus `json:"status"`
	Total       int          `json:"total"`
	Returned    int          `json:"returned"`
	Remaining   int          `json:"remaining"`
	Error       string       `json:"error,omitempty"`
	CollectedAt string       `json:"collected_at"`
}

// WorkloadKindCount is a compact per-kind rollup. We intentionally keep
// ReplicaSet out of the default posture because it is an internal Deployment
// mechanism; the posture only reports user-owned workload kinds that an
// operator would reason about directly.
type WorkloadKindCount struct {
	Kind              string `json:"kind"`
	DesiredReplicas   int32  `json:"desired_replicas"`
	ReadyReplicas     int32  `json:"ready_replicas"`
	AvailableReplicas int32  `json:"available_replicas"`
	UpdatedReplicas   int32  `json:"updated_replicas"`
	FailedReplicas    int32  `json:"failed_replicas"`
	Count             int32  `json:"count"`
}

// WorkloadSummary is the deterministic rollup across the six reviewed
// user-owned workload kinds. OwnerReferences are NOT expanded into the
// summary; the posture deliberately avoids inferring scheduler or
// controller-manager semantics.
type WorkloadSummary struct {
	Evidence     EvidenceCitation    `json:"evidence"`
	ByKind       []WorkloadKindCount `json:"by_kind"`
	TotalCount   int32               `json:"total_count"`
	DesiredTotal int32               `json:"desired_total"`
	ReadyTotal   int32               `json:"ready_total"`
}

// PodPhaseCount reports Pod phase distribution without classifying cause.
type PodPhaseCount struct {
	Phase string `json:"phase"`
	Count int32  `json:"count"`
}

// PodNodeSpread reports how many Pods in this Namespace land on each Node.
// The posture does NOT infer scheduler affinity or topology-spread results
// from this spread; it only cites the observable placement.
type PodNodeSpread struct {
	NodeName string `json:"node_name"`
	Count    int32  `json:"count"`
}

// PodSummary reports observable Pod facts only. Restart counts and per-Pod
// conditions are intentionally NOT rolled up to the Namespace posture; the
// existing Pod detail and diagnosis routes remain the authoritative source
// for those investigations.
type PodSummary struct {
	Evidence        EvidenceCitation `json:"evidence"`
	Total           int32            `json:"total"`
	Scheduled       int32            `json:"scheduled"`
	ByPhase         []PodPhaseCount  `json:"by_phase"`
	ByNode          []PodNodeSpread  `json:"by_node"`
	UniqueNodeCount int32            `json:"unique_node_count"`
}

// ResourceQuotaEntry flattens one ResourceQuota's hard/used map into a
// comparable list so the frontend can render a uniform table across all
// quotas in the Namespace. Zero-used entries are kept so the operator can
// distinguish "no usage" from "missing quota".
type ResourceQuotaEntry struct {
	Name string            `json:"name"`
	Hard map[string]string `json:"hard,omitempty"`
	Used map[string]string `json:"used,omitempty"`
}

// ResourceQuotaPosture is the aggregated ResourceQuota evidence for the
// target Namespace. It explicitly does NOT compute usage ratios or warn
// about saturation; those are derived in diagnosis when the required
// sustained-evidence contract exists.
type ResourceQuotaPosture struct {
	Evidence EvidenceCitation     `json:"evidence"`
	Quotas   []ResourceQuotaEntry `json:"quotas"`
}

// LimitRangePosture is the aggregated LimitRange evidence. The posture
// records every item verbatim; it does NOT attempt to detect conflicting
// defaults across multiple LimitRanges because Kubernetes applies them in
// object-creation order and that order is not observable from list data.
type LimitRangePosture struct {
	Evidence EvidenceCitation        `json:"evidence"`
	Ranges   []k8sgateway.LimitRange `json:"ranges"`
}

// PDBPosture reports the PDB inventory and observed disruptions headroom.
// The posture does NOT validate selector-match coverage against Pods because
// that requires exact label-set matching and is the responsibility of
// diagnosis rules.
type PDBEntry struct {
	Name               string `json:"name"`
	MinAvailable       string `json:"min_available,omitempty"`
	MaxUnavailable     string `json:"max_unavailable,omitempty"`
	CurrentHealthy     int32  `json:"current_healthy"`
	DesiredHealthy     int32  `json:"desired_healthy"`
	DisruptionsAllowed int32  `json:"disruptions_allowed"`
	ExpectedPods       int32  `json:"expected_pods"`
}

type PDBPosture struct {
	Evidence EvidenceCitation `json:"evidence"`
	PDBs     []PDBEntry       `json:"pdbs"`
	Count    int32            `json:"count"`
}

// NodeCapacityPosture cites the cluster-wide Node capacity/allocatable set.
// It is included as context for the Namespace posture because capacity is a
// cluster-level denominator; the posture explicitly does NOT compute per-
// Namespace share of cluster capacity because that would require scheduler-
// semantic inference (QoS class, preemption, overcommit) that we refuse.
type NodeCapacityEntry struct {
	Name        string            `json:"name"`
	Capacity    map[string]string `json:"capacity,omitempty"`
	Allocatable map[string]string `json:"allocatable,omitempty"`
	Schedulable bool              `json:"schedulable"`
}

type NodeCapacityPosture struct {
	Evidence EvidenceCitation    `json:"evidence"`
	Nodes    []NodeCapacityEntry `json:"nodes"`
	Count    int32               `json:"count"`
}

// NamespacePosture is the deterministic, source-cited posture for one
// Namespace. Every section carries its own EvidenceCitation so partial
// failures (for example the PDB list timing out while ResourceQuota
// succeeds) are rendered honestly rather than silently dropped. The posture
// is read-only; there is no mutation surface here.
//
// Explicit non-inferences are recorded in the type comments above so the
// frontend does not accidentally report "findings" that are really guesses.
type NamespacePosture struct {
	Name        string            `json:"name"`
	Phase       string            `json:"phase"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	CreatedAt   string            `json:"created_at"`

	ResourceQuotas ResourceQuotaPosture `json:"resource_quotas"`
	LimitRanges    LimitRangePosture    `json:"limit_ranges"`
	Workloads      WorkloadSummary      `json:"workloads"`
	Pods           PodSummary           `json:"pods"`
	PDBs           PDBPosture           `json:"pdbs"`
	NodeCapacity   NodeCapacityPosture  `json:"node_capacity"`

	// PartialSections lists the section names where evidence is NOT
	// complete. Operators and UIs can key on this to show warnings
	// without having to inspect each EvidenceCitation.Status field.
	PartialSections []string `json:"partial_sections"`
}

type PostureListEntry struct {
	Name            string   `json:"name"`
	Phase           string   `json:"phase"`
	CreatedAt       string   `json:"created_at"`
	WorkloadCount   int32    `json:"workload_count"`
	PodCount        int32    `json:"pod_count"`
	QuotaCount      int32    `json:"quota_count"`
	LimitRangeCount int32    `json:"limit_range_count"`
	PDBCount        int32    `json:"pdb_count"`
	PartialSections []string `json:"partial_sections"`
}

func newCitation(apiPath string, collectedAt time.Time) EvidenceCitation {
	return EvidenceCitation{
		APIPath:     apiPath,
		Status:      SourceComplete,
		CollectedAt: collectedAt.UTC().Format(time.RFC3339),
	}
}

func markTruncated(cite *EvidenceCitation, total, returned, remaining int) {
	cite.Total = total
	cite.Returned = returned
	cite.Remaining = remaining
	if remaining > 0 && cite.Status == SourceComplete {
		cite.Status = SourceTruncated
	}
}

func markError(cite *EvidenceCitation, err error) {
	if cite.Status == SourceComplete {
		cite.Status = SourceUnavailable
	} else {
		cite.Status = SourcePartial
	}
	cite.Error = err.Error()
}
