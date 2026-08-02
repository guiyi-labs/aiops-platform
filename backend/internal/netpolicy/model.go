// Package netpolicy performs a read-only network connectivity and
// NetworkPolicy posture analysis for one cluster.
//
// It answers two questions a human operator actually asks:
//
//  1. "Can this traffic get through?" — Services that select no backend pods,
//     Service ports whose targetPort resolves to nothing, and Service ports
//     that the namespace's own NetworkPolicies would drop.
//  2. "Is anything wide open?" — namespaces without a default-deny baseline,
//     pods left uncovered inside an otherwise protected namespace, dead
//     policies that select nothing, allow-all rules and world-open ipBlocks,
//     and externally exposed Services with no ingress restriction at all.
//
// The analyzer is pure and offline: it never contacts a cluster, never sends a
// probe packet, and never mutates anything (ADR 0004). It reasons statically
// over an observation bundle that the caller supplies (or that the M65
// collector gathers via read-only List calls).
package netpolicy

import (
	"time"

	k8sfinding "k8s-aiops.local/backend/internal/finding"
)

// Finding reuses the platform's canonical read-only posture Finding contract
// (see internal/finding) so the frontend renders network findings uniformly
// with namespace-posture, CIS, deprecated-API and FinOps findings.
type Finding = k8sfinding.Finding

const (
	SeverityInfo     = k8sfinding.SeverityInfo
	SeverityWarning  = k8sfinding.SeverityWarning
	SeverityCritical = k8sfinding.SeverityCritical
)

// Finding families, reported in Finding.Details["family"] and rolled up into
// Status.ByFamily.
const (
	// FamilyCoverage covers "is this workload protected by any policy at all".
	FamilyCoverage = "coverage"
	// FamilyPolicyHygiene covers defects in the policies themselves.
	FamilyPolicyHygiene = "policy-hygiene"
	// FamilyReachability covers "would traffic actually arrive".
	FamilyReachability = "reachability"
	// FamilyExposure covers externally reachable entry points.
	FamilyExposure = "exposure"
)

// Finding codes emitted by Evaluate.
const (
	CodeNamespaceNoDefaultDeny = "NETPOL_NS_NO_DEFAULT_DENY_INGRESS"
	CodePodIngressUnrestricted = "NETPOL_POD_INGRESS_UNRESTRICTED"
	CodePolicySelectsNoPods    = "NETPOL_POLICY_SELECTS_NO_PODS"
	CodeIngressAllowAll        = "NETPOL_INGRESS_ALLOW_ALL"
	CodeIngressFromAllNS       = "NETPOL_INGRESS_FROM_ALL_NAMESPACES"
	CodeWideIPBlock            = "NETPOL_WIDE_IPBLOCK"
	CodeHostNetworkIneffective = "NETPOL_HOSTNETWORK_POLICY_INEFFECTIVE"
	CodeServiceNoBackends      = "NETPOL_SERVICE_NO_BACKENDS"
	CodeServicePortUnmatched   = "NETPOL_SERVICE_PORT_UNMATCHED"
	CodeServicePortBlocked     = "NETPOL_SERVICE_PORT_BLOCKED"
	CodeExposedUnrestricted    = "NETPOL_EXPOSED_SERVICE_UNRESTRICTED"
)

// systemNamespaces are cluster-owned namespaces where a missing default-deny
// baseline is expected rather than alarming; findings about them are demoted
// to informational so they do not drown out application findings.
var systemNamespaces = map[string]bool{
	"kube-system":     true,
	"kube-public":     true,
	"kube-node-lease": true,
}

// Selector is the minimal LabelSelector contract the analyzer needs.
//
// The distinction between "absent" and "present but empty" is load-bearing in
// NetworkPolicy semantics (an empty podSelector selects every pod in the
// namespace), so callers must preserve it: use a nil *Selector for absent and
// a non-nil Selector with no MatchLabels for the empty selector.
//
// HasExpressions records that the original selector carried matchExpressions,
// which this analyzer does not evaluate. Checks that could produce a false
// alarm when expressions are unknown (dead-policy detection) skip such
// selectors, and coverage checks treat them as matching so a pod is never
// wrongly reported as unprotected.
type Selector struct {
	MatchLabels    map[string]string `json:"match_labels,omitempty"`
	HasExpressions bool              `json:"has_expressions,omitempty"`
}

// SelectsAll reports whether the selector matches every object in its scope.
func (s Selector) SelectsAll() bool {
	return len(s.MatchLabels) == 0 && !s.HasExpressions
}

// Matches reports whether every matchLabels pair is present in labels, the
// subset semantics Kubernetes uses. matchExpressions are not evaluated.
func (s Selector) Matches(labels map[string]string) bool {
	for k, v := range s.MatchLabels {
		if labels[k] != v {
			return false
		}
	}
	return true
}

// coversConservatively reports whether the selector may select labels. When
// matchExpressions are present the answer is "yes" so that coverage checks err
// towards silence instead of a false "unprotected" alarm.
func (s Selector) coversConservatively(labels map[string]string) bool {
	if s.HasExpressions {
		return true
	}
	return s.Matches(labels)
}

// ContainerPort is one declared container port of a pod.
type ContainerPort struct {
	Name          string `json:"name,omitempty"`
	ContainerPort int32  `json:"container_port"`
	Protocol      string `json:"protocol,omitempty"`
}

// PodInfo is the minimal pod identity used for selector matching and for
// resolving Service targetPorts.
type PodInfo struct {
	Namespace   string            `json:"namespace"`
	Name        string            `json:"name"`
	UID         string            `json:"uid,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Ports       []ContainerPort   `json:"ports,omitempty"`
	HostNetwork bool              `json:"host_network,omitempty"`
}

// NamespaceInfo is the minimal namespace identity used for peer matching.
type NamespaceInfo struct {
	Name   string            `json:"name"`
	UID    string            `json:"uid,omitempty"`
	Labels map[string]string `json:"labels,omitempty"`
}

// Peer is one entry of a NetworkPolicy rule's from/to list.
type Peer struct {
	NamespaceSelector *Selector `json:"namespace_selector,omitempty"`
	PodSelector       *Selector `json:"pod_selector,omitempty"`
	IPBlockCIDR       string    `json:"ip_block_cidr,omitempty"`
	IPBlockExcept     []string  `json:"ip_block_except,omitempty"`
}

// PortRule is one entry of a NetworkPolicy rule's ports list. An empty Port
// means "every port".
type PortRule struct {
	Protocol string `json:"protocol,omitempty"`
	Port     string `json:"port,omitempty"`
	EndPort  int32  `json:"end_port,omitempty"`
}

// Rule is one ingress or egress rule. Empty Peers means "from/to anywhere";
// empty Ports means "on every port".
type Rule struct {
	Peers []Peer     `json:"peers,omitempty"`
	Ports []PortRule `json:"ports,omitempty"`
}

// Policy is the security-relevant subset of a NetworkPolicy.
type Policy struct {
	Namespace   string   `json:"namespace"`
	Name        string   `json:"name"`
	UID         string   `json:"uid,omitempty"`
	PodSelector Selector `json:"pod_selector"`
	// PolicyTypes mirrors spec.policyTypes. When empty the Kubernetes default
	// applies: Ingress always, plus Egress when egress rules are present.
	PolicyTypes []string `json:"policy_types,omitempty"`
	Ingress     []Rule   `json:"ingress,omitempty"`
	Egress      []Rule   `json:"egress,omitempty"`
}

// AppliesToIngress reports whether the policy restricts ingress for the pods
// it selects, applying the Kubernetes default when policyTypes is omitted.
func (p Policy) AppliesToIngress() bool {
	if len(p.PolicyTypes) == 0 {
		return true
	}
	for _, t := range p.PolicyTypes {
		if equalFold(t, "Ingress") {
			return true
		}
	}
	return false
}

// AppliesToEgress reports whether the policy restricts egress for the pods it
// selects, applying the Kubernetes default when policyTypes is omitted.
func (p Policy) AppliesToEgress() bool {
	if len(p.PolicyTypes) == 0 {
		return len(p.Egress) > 0
	}
	for _, t := range p.PolicyTypes {
		if equalFold(t, "Egress") {
			return true
		}
	}
	return false
}

// ServicePort is one port of a Service. TargetPort may be numeric or a named
// container port; an empty TargetPort means it defaults to Port.
type ServicePort struct {
	Name       string `json:"name,omitempty"`
	Port       int32  `json:"port"`
	TargetPort string `json:"target_port,omitempty"`
	Protocol   string `json:"protocol,omitempty"`
	NodePort   int32  `json:"node_port,omitempty"`
}

// ServiceInfo is the routing-relevant subset of a Service.
type ServiceInfo struct {
	Namespace    string            `json:"namespace"`
	Name         string            `json:"name"`
	UID          string            `json:"uid,omitempty"`
	Type         string            `json:"type,omitempty"`
	Selector     map[string]string `json:"selector,omitempty"`
	Ports        []ServicePort     `json:"ports,omitempty"`
	ClusterIP    string            `json:"cluster_ip,omitempty"`
	ExternalName string            `json:"external_name,omitempty"`
}

// Externally reports whether the Service is reachable from outside the
// cluster network by virtue of its type.
func (s ServiceInfo) Externally() bool {
	return equalFold(s.Type, "NodePort") || equalFold(s.Type, "LoadBalancer")
}

// Inputs is the read-only observation bundle for one cluster evaluation.
// Every field is optional; the evaluator only checks what is supplied.
type Inputs struct {
	Namespaces []NamespaceInfo `json:"namespaces,omitempty"`
	Pods       []PodInfo       `json:"pods,omitempty"`
	Policies   []Policy        `json:"policies,omitempty"`
	Services   []ServiceInfo   `json:"services,omitempty"`
}

// Empty reports whether the bundle carries nothing to analyze.
func (in Inputs) Empty() bool {
	return len(in.Namespaces) == 0 && len(in.Pods) == 0 &&
		len(in.Policies) == 0 && len(in.Services) == 0
}

// Status is the rollup returned for one cluster evaluation.
type Status struct {
	ClusterID   int64     `json:"cluster_id"`
	EvaluatedAt time.Time `json:"evaluated_at"`
	// Total is the number of individual checks evaluated, Failed the number
	// that produced a finding, Passed the remainder.
	Total  int `json:"total"`
	Failed int `json:"failed"`
	Passed int `json:"passed"`
	// Inventory counters give the console a one-line summary of the scope.
	NamespacesTotal int `json:"namespaces_total"`
	PodsTotal       int `json:"pods_total"`
	PoliciesTotal   int `json:"policies_total"`
	ServicesTotal   int `json:"services_total"`
	// IngressCoveredPods counts pods selected by at least one ingress policy.
	IngressCoveredPods int `json:"ingress_covered_pods"`
	// EgressCoveredPods counts pods selected by at least one egress policy.
	EgressCoveredPods int `json:"egress_covered_pods"`
	// IsolatedNamespaces counts namespaces that have a default-deny ingress
	// baseline (a policy selecting every pod with the Ingress policy type).
	IsolatedNamespaces int `json:"isolated_namespaces"`
	// ExposedServices counts NodePort / LoadBalancer services.
	ExposedServices int            `json:"exposed_services"`
	BySeverity      map[string]int `json:"by_severity"`
	ByFamily        map[string]int `json:"by_family"`
	Findings        []Finding      `json:"findings"`
}
