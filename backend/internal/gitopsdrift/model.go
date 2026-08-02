// Package gitopsdrift performs a read-only GitOps configuration-drift analysis
// for one cluster.
//
// It answers the question an platform operator asks between reconciles:
// "has anything in the cluster diverged from what GitOps last applied?" The
// signal is the standard kubectl annotation
// kubectl.kubernetes.io/last-applied-configuration, which records the exact
// manifest a GitOps tool (kubectl apply / Kustomize / Helm / Flux / Argo CD)
// wrote. When the live object no longer matches that record, the resource has
// drifted and GitOps can no longer cleanly reconcile it.
//
// The analyzer is pure and offline (ADR 0004): it only reasons over an
// observation bundle the caller supplies (or that the M65 collector gathers
// via read-only List calls) and never mutates cluster state, never talks to a
// Git provider and never re-applies anything.
package gitopsdrift

import (
	"encoding/json"
	"time"

	k8sfinding "k8s-aiops.local/backend/internal/finding"
)

// Finding reuses the platform's canonical read-only posture Finding contract
// (see internal/finding) so the frontend renders drift findings uniformly
// with the other optimization analyzers.
type Finding = k8sfinding.Finding

const (
	SeverityInfo     = k8sfinding.SeverityInfo
	SeverityWarning  = k8sfinding.SeverityWarning
	SeverityCritical = k8sfinding.SeverityCritical
)

// Finding family, reported in Finding.Details["family"] and rolled up into
// Status.ByFamily.
const FamilyGitOpsDrift = "gitops-drift"

// Finding codes emitted by Evaluate.
const (
	// CodeDriftDetected: the live object no longer matches the
	// kubectl.kubernetes.io/last-applied-configuration annotation, so it has
	// diverged from what GitOps last applied.
	CodeDriftDetected = "GITOPS_DRIFT_DETECTED"
	// CodeUnmanaged: a resource lives in a GitOps-managed namespace but carries
	// no last-applied-configuration annotation, so drift cannot be reconciled
	// or even detected for it.
	CodeUnmanaged = "GITOPS_UNMANAGED_RESOURCE"
)

// Manager identifiers reported in Finding.Details["manager"].
const (
	ManagerKubectl = "kubectl"
	ManagerFlux    = "flux"
	ManagerArgoCD  = "argocd"
)

// ManagedResource is one Kubernetes object observed for GitOps drift.
type ManagedResource struct {
	Kind string `json:"kind"`
	// Namespace is empty for cluster-scoped resources.
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
	UID       string `json:"uid,omitempty"`
	// Manager is the detected GitOps/kubectl manager, e.g. ManagerKubectl,
	// ManagerFlux or ManagerArgoCD; "" when no manager annotation is present.
	Manager string `json:"manager,omitempty"`
	// AppliedConfig is the raw value of the last-applied-configuration
	// annotation (a JSON manifest). Empty when the resource was not applied
	// through that path.
	AppliedConfig json.RawMessage `json:"applied_config,omitempty"`
	// LiveBody is the raw "spec" (or "data" for ConfigMap/Secret) of the live
	// object, used as the comparison target for drift.
	LiveBody json.RawMessage `json:"live_body,omitempty"`
}

// Inputs is the read-only observation bundle for one cluster evaluation.
type Inputs struct {
	Resources []ManagedResource `json:"resources,omitempty"`
	// ManagedNamespaces are namespaces detected as GitOps-managed (via Flux or
	// Argo CD annotations). Resources in these namespaces without a
	// last-applied-configuration annotation are reported as unmanaged.
	ManagedNamespaces []string `json:"managed_namespaces,omitempty"`
}

// Empty reports whether the bundle carries nothing to analyze.
func (in Inputs) Empty() bool {
	return len(in.Resources) == 0
}

// Status is the rollup returned for one cluster evaluation.
type Status struct {
	ClusterID   int64     `json:"cluster_id"`
	EvaluatedAt time.Time `json:"evaluated_at"`
	// Total is the number of individual resources evaluated, Failed the number
	// that produced a finding, Passed the remainder.
	Total  int `json:"total"`
	Failed int `json:"failed"`
	Passed int `json:"passed"`
	// Inventory counters give the console a one-line summary of the scope.
	ResourcesTotal     int            `json:"resources_total"`
	DriftedResources   int            `json:"drifted_resources"`
	UnmanagedResources int            `json:"unmanaged_resources"`
	BySeverity         map[string]int `json:"by_severity"`
	ByFamily           map[string]int `json:"by_family"`
	Findings           []Finding      `json:"findings"`
}
