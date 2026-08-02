// Package pdb performs a read-only PodDisruptionBudget (PDB) posture
// analysis for one cluster.
//
// It answers the question an operator asks before a maintenance window:
// "can I safely drain this node without taking the workload down?" It checks
// whether workloads that should be protected have a PDB, whether the PDB's
// availability budget is actually satisfiable, and whether disruptions are
// currently blocked.
//
// The analyzer is pure and offline (ADR 0004): Evaluate takes only an
// observation bundle (collected read-only from the API server) and returns a
// Status. It never mutates anything and never triggers evictions.
package pdb

import (
	"time"

	k8sfinding "k8s-aiops.local/backend/internal/finding"
)

// Finding reuses the platform's canonical read-only posture Finding contract
// (see internal/finding) so the frontend renders PDB findings uniformly with
// the other optimization analyzers.
type Finding = k8sfinding.Finding

const (
	SeverityInfo     = k8sfinding.SeverityInfo
	SeverityWarning  = k8sfinding.SeverityWarning
	SeverityCritical = k8sfinding.SeverityCritical
)

// FamilyPDB is reported in Finding.Details["family"] and rolled up into
// Status.ByFamily.
const FamilyPDB = "pdb-protection"

// Finding codes emitted by Evaluate.
const (
	// CodeWorkloadUnprotected: a replicable workload has no PDB selecting it,
	// so a drain/upgrade can take the whole workload down at once.
	CodeWorkloadUnprotected = "PDB_WORKLOAD_UNPROTECTED"
	// CodeBudgetUnachievable: minAvailable is at least the expected pod
	// count, leaving no room for any disruption.
	CodeBudgetUnachievable = "PDB_BUDGET_UNACHIEVABLE"
	// CodeDisruptionsBlocked: the PDB currently allows zero disruptions, so
	// maintenance (drain/cordon) would be blocked.
	CodeDisruptionsBlocked = "PDB_DISRUPTIONS_BLOCKED"
	// CodeSelectorNoMatches: the PDB selector matches no pods, so it protects
	// nothing.
	CodeSelectorNoMatches = "PDB_SELECTOR_NO_MATCHES"
)

// WorkloadRef identifies one replicable workload that PDB coverage is
// expected for.
type WorkloadRef struct {
	Kind      string            `json:"kind"` // Deployment / StatefulSet / DaemonSet
	Namespace string            `json:"namespace"`
	Name      string            `json:"name"`
	UID       string            `json:"uid,omitempty"`
	Replicas  int32             `json:"replicas"`
	Labels    map[string]string `json:"labels,omitempty"`
}

// PDBInfo is the protection-relevant subset of a PodDisruptionBudget.
type PDBInfo struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	UID       string `json:"uid,omitempty"`
	// MinAvailable and MaxUnavailable carry the raw spec values; the analyzer
	// converts them to an availability budget when possible.
	MinAvailable   string `json:"min_available,omitempty"`
	MaxUnavailable string `json:"max_unavailable,omitempty"`
	// SelectorLabels is the PDB's spec.selector.matchLabels. An empty map
	// selects nothing in practice (PDBs require a non-null selector, and an
	// empty selector matches no pod labels).
	SelectorLabels map[string]string `json:"selector_labels,omitempty"`
	// ExpectedPods / DisruptionsAllowed come from status.
	ExpectedPods       int32 `json:"expected_pods"`
	DisruptionsAllowed int32 `json:"disruptions_allowed"`
}

// Inputs is the read-only observation bundle for one cluster evaluation.
type Inputs struct {
	Workloads []WorkloadRef `json:"workloads,omitempty"`
	PDBs      []PDBInfo     `json:"pdbs,omitempty"`
}

// Empty reports whether the bundle carries nothing to analyze.
func (in Inputs) Empty() bool {
	return len(in.Workloads) == 0 && len(in.PDBs) == 0
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
	WorkloadsTotal int `json:"workloads_total"`
	PDBsTotal      int `json:"pdbs_total"`
	// UnprotectedWorkloads counts replicable workloads with no matching PDB.
	UnprotectedWorkloads int            `json:"unprotected_workloads"`
	BySeverity           map[string]int `json:"by_severity"`
	ByFamily             map[string]int `json:"by_family"`
	Findings             []Finding      `json:"findings"`
}
