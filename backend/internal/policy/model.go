// Package policy performs a read-only "policy-as-code" posture evaluation of
// workload manifests.
//
// It answers the question a platform operator asks before a release: "does
// this workload violate the team's declarative baseline?" Instead of shipping
// a Rego/OPA engine, the analyzer evaluates a small, opinionated rule set
// (resource requests/limits, security context, host access, and probes) that
// mirrors what KubeSphere and similar consoles gate on by default. The rule
// set lives in service.go and is trivially extendable.
//
// The analyzer is pure and offline (ADR 0004): Evaluate takes only an
// observation bundle (collected read-only from the API server) and returns a
// Status. It never reaches the cluster, never applies anything, and never
// mutates any resource.
package policy

import (
	"time"

	k8sfinding "k8s-aiops.local/backend/internal/finding"
)

// Finding reuses the platform's canonical read-only posture Finding contract
// (see internal/finding) so the frontend renders policy findings uniformly
// with the other optimization analyzers.
type Finding = k8sfinding.Finding

const (
	SeverityInfo     = k8sfinding.SeverityInfo
	SeverityWarning  = k8sfinding.SeverityWarning
	SeverityCritical = k8sfinding.SeverityCritical
)

// Finding families, reported in Finding.Details["family"] and rolled up into
// Status.ByFamily.
const (
	FamilyResources  = "resources"   // requests / limits presence
	FamilySecurity   = "security"    // privileged, privilege escalation, run-as-root
	FamilyHostAccess = "host-access" // hostNetwork / hostPID / hostIPC
	FamilyProbes     = "probes"      // liveness / readiness / startup probes
)

// Finding codes emitted by Evaluate.
const (
	CodeNoResourceLimits = "POLICY_CONTAINER_NO_RESOURCE_LIMITS"
	CodeNoCPURequest     = "POLICY_CONTAINER_NO_CPU_REQUEST"
	CodeNoMemoryRequest  = "POLICY_CONTAINER_NO_MEMORY_REQUEST"
	CodePrivileged       = "POLICY_CONTAINER_PRIVILEGED"
	CodeAllowEscalation  = "POLICY_CONTAINER_ALLOW_PRIVILEGE_ESCALATION"
	CodeRunAsRoot        = "POLICY_CONTAINER_RUN_AS_ROOT"
	CodeHostNetwork      = "POLICY_WORKLOAD_HOST_NETWORK"
	CodeHostPIDOrIPC     = "POLICY_WORKLOAD_HOST_PID_OR_IPC"
	CodeNoLivenessProbe  = "POLICY_CONTAINER_NO_LIVENESS_PROBE"
	CodeNoReadinessProbe = "POLICY_CONTAINER_NO_READINESS_PROBE"
	CodeNoStartupProbe   = "POLICY_CONTAINER_NO_STARTUP_PROBE"
)

// ContainerPolicy is the security-relevant subset of one container spec.
// Pointer booleans distinguish "unset" from an explicit false, which matters
// for the privileged / allowPrivilegeEscalation defaults.
type ContainerPolicy struct {
	Name string `json:"name"`
	// Requests/limits presence. Missing limits is the most common drift from
	// a production baseline; missing requests silently degrades QoS.
	CPURequest        bool `json:"cpu_request"`
	MemoryRequest     bool `json:"memory_request"`
	HasResourceLimits bool `json:"has_resource_limits"`
	// Security context. Privileged is a pointer because the Kubernetes default
	// is false and only an explicit true is a finding.
	Privileged               *bool `json:"privileged,omitempty"`
	AllowPrivilegeEscalation *bool `json:"allow_privilege_escalation,omitempty"`
	RunAsNonRoot             *bool `json:"run_as_non_root,omitempty"`
	// Probe presence: liveness/readiness are expected on every serving
	// workload; startup probes matter for slow-booting containers.
	LivenessProbe  bool `json:"liveness_probe"`
	ReadinessProbe bool `json:"readiness_probe"`
	StartupProbe   bool `json:"startup_probe"`
}

// WorkloadPolicy is the evaluated subset of one workload controller.
type WorkloadPolicy struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	UID       string `json:"uid,omitempty"`
	// Host access at the pod level.
	HostNetwork bool              `json:"host_network"`
	HostPID     bool              `json:"host_pid"`
	HostIPC     bool              `json:"host_ipc"`
	Containers  []ContainerPolicy `json:"containers,omitempty"`
}

// Inputs is the read-only observation bundle for one cluster evaluation.
type Inputs struct {
	Workloads []WorkloadPolicy `json:"workloads,omitempty"`
}

// Empty reports whether the bundle carries nothing to analyze.
func (in Inputs) Empty() bool {
	return len(in.Workloads) == 0
}

// Status is the rollup returned for one cluster evaluation.
type Status struct {
	ClusterID   int64     `json:"cluster_id"`
	EvaluatedAt time.Time `json:"evaluated_at"`
	// Total counts individual rule checks evaluated; Failed the checks that
	// produced a finding; Passed the remainder.
	Total  int `json:"total"`
	Failed int `json:"failed"`
	Passed int `json:"passed"`
	// Inventory counters give the console a one-line summary of the scope.
	WorkloadsTotal  int `json:"workloads_total"`
	ContainersTotal int `json:"containers_total"`
	// CompliantWorkloads counts workloads whose every checked container passed
	// every rule.
	CompliantWorkloads int            `json:"compliant_workloads"`
	BySeverity         map[string]int `json:"by_severity"`
	ByFamily           map[string]int `json:"by_family"`
	Findings           []Finding      `json:"findings"`
}
