// Package ingressposture performs a read-only Ingress exposure audit for one
// cluster.
//
// It answers the question an operator asks before publishing a route:
// "what am I actually exposing, and is it protected?" It checks whether the
// Ingress terminates TLS, whether hosts are wildcards, whether an ingress
// class is pinned, and whether the backend Services actually exist.
//
// The analyzer is pure and offline (ADR 0004): Evaluate takes only an
// observation bundle (collected read-only from the API server) and returns a
// Status. It never mutates anything and never reconfigures routing.
package ingressposture

import (
	"time"

	k8sfinding "k8s-aiops.local/backend/internal/finding"
)

// Finding reuses the platform's canonical read-only posture Finding contract
// (see internal/finding) so the frontend renders Ingress findings uniformly
// with the other optimization analyzers.
type Finding = k8sfinding.Finding

const (
	SeverityInfo     = k8sfinding.SeverityInfo
	SeverityWarning  = k8sfinding.SeverityWarning
	SeverityCritical = k8sfinding.SeverityCritical
)

// FamilyIngress is reported in Finding.Details["family"] and rolled up into
// Status.ByFamily.
const FamilyIngress = "ingress-exposure"

// Finding codes emitted by Evaluate.
const (
	// CodeNoTLS: the Ingress has host rules but no TLS block, so traffic is
	// served in cleartext.
	CodeNoTLS = "INGRESS_NO_TLS"
	// CodeWildcardHost: a rule uses a wildcard host (e.g. "*.example.com"),
	// which widens the exposure beyond a single name.
	CodeWildcardHost = "INGRESS_WILDCARD_HOST"
	// CodeNoIngressClass: no ingressClassName is pinned, so the effective
	// controller depends on cluster defaults.
	CodeNoIngressClass = "INGRESS_NO_INGRESS_CLASS"
	// CodeBackendServiceMissing: a rule backend references a Service that
	// does not exist in the snapshot, so the route is dead.
	CodeBackendServiceMissing = "INGRESS_BACKEND_SERVICE_MISSING"
)

// ServiceRef identifies a Service that an Ingress backend points at.
type ServiceRef struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

// IngressInfo is the exposure-relevant subset of an Ingress.
type IngressInfo struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	UID       string `json:"uid,omitempty"`
	// IngressClassName mirrors spec.ingressClassName; empty when unset.
	IngressClassName string `json:"ingress_class_name,omitempty"`
	// Hosts lists every host referenced by rules (wildcards kept verbatim).
	Hosts []string `json:"hosts,omitempty"`
	// HasTLS reports whether spec.tls is non-empty.
	HasTLS bool `json:"has_tls"`
	// Backends lists every Service referenced by a rule backend (default
	// backend plus each path backend).
	Backends []ServiceRef `json:"backends,omitempty"`
}

// Inputs is the read-only observation bundle for one cluster evaluation.
type Inputs struct {
	Ingresses []IngressInfo `json:"ingresses,omitempty"`
	// Services lists the Service names that exist, for backend resolution.
	Services []ServiceRef `json:"services,omitempty"`
}

// Empty reports whether the bundle carries nothing to analyze.
func (in Inputs) Empty() bool {
	return len(in.Ingresses) == 0
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
	IngressesTotal int `json:"ingresses_total"`
	// NoTLSCount counts Ingresses with host rules but no TLS.
	NoTLSCount int `json:"no_tls_count"`
	// DeadBackendCount counts backend Service references that do not exist.
	DeadBackendCount int            `json:"dead_backend_count"`
	BySeverity       map[string]int `json:"by_severity"`
	ByFamily         map[string]int `json:"by_family"`
	Findings         []Finding      `json:"findings"`
}
