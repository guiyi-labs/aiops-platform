// Package cis performs a read-only Kubernetes CIS Benchmark posture check,
// mirroring the control families of tools such as kube-bench and Kubescape.
// It is intentionally read-only and NEVER mutates any cluster object; it only
// reports which control-plane / worker-node component flags and which
// workload / RBAC / namespace objects fail the selected CIS controls.
//
// The catalog of controls is compiled into the binary (see catalog.go) so the
// check is deterministic and has no external dependency. The analyzer is
// deliberately scoped to the read-only, observation-only philosophy of the
// platform: it produces findings that a human reviews, and never remediates.
package cis

import (
	"time"

	k8sfinding "k8s-aiops.local/backend/internal/finding"
)

// Finding reuses the platform's canonical read-only posture Finding contract
// (see internal/finding) so the frontend can render CIS findings uniformly
// with namespace-posture, deprecated-API and FinOps findings.
type Finding = k8sfinding.Finding

const (
	SeverityInfo     = k8sfinding.SeverityInfo
	SeverityWarning  = k8sfinding.SeverityWarning
	SeverityCritical = k8sfinding.SeverityCritical
)

// ComponentConfig is the minimal flag contract the analyzer needs from a
// control-plane or worker-node component. Callers extract the flag map from
// the component manifest / process arguments; the analyzer never reaches into
// the node directly.
type ComponentConfig struct {
	// Component is the Logical component name, e.g. "kube-apiserver",
	// "kube-scheduler", "kube-controller-manager", "etcd", "kubelet".
	Component string `json:"component"`
	// Flags maps a flag name (without leading dashes) to its value, e.g.
	// {"anonymous-auth": "false", "authorization-mode": "Node,RBAC"}.
	Flags map[string]string `json:"flags"`
}

// ContainerSecurity is the security-relevant subset of a container spec.
type ContainerSecurity struct {
	Name                 string   `json:"name"`
	Privileged           *bool    `json:"privileged,omitempty"`
	AllowPrivilegeEscalation *bool `json:"allow_privilege_escalation,omitempty"`
	RunAsNonRoot         *bool    `json:"run_as_non_root,omitempty"`
	RunAsUser            *int64   `json:"run_as_user,omitempty"`
	ReadOnlyRootFilesystem *bool  `json:"read_only_root_filesystem,omitempty"`
	CapabilitiesDrop     []string `json:"capabilities_drop,omitempty"`
	HostNetwork          bool     `json:"host_network"`
	HostPID              bool     `json:"host_pid"`
	HostIPC              bool     `json:"host_ipc"`
	HostPathVolumes      int      `json:"host_path_volumes"`
}

// WorkloadSecurity is the security-relevant subset of a Pod / workload spec.
type WorkloadSecurity struct {
	Kind      string             `json:"kind"`
	Namespace string             `json:"namespace"`
	Name      string             `json:"name"`
	UID       string             `json:"uid,omitempty"`
	Containers []ContainerSecurity `json:"containers"`
}

// PolicyRule is a minimal RBAC rule used to detect wildcard grants.
type PolicyRule struct {
	Verbs     []string `json:"verbs"`
	Resources []string `json:"resources"`
	APIGroups []string `json:"api_groups,omitempty"`
}

// RBACBinding is the security-relevant subset of a RoleBinding /
// ClusterRoleBinding, including the resolved rules of the referenced Role.
type RBACBinding struct {
	Kind      string       `json:"kind"`
	Namespace string       `json:"namespace"`
	Name      string       `json:"name"`
	UID       string       `json:"uid,omitempty"`
	RoleName  string       `json:"role_name"`
	RoleKind  string       `json:"role_kind"`
	RoleRules []PolicyRule `json:"role_rules,omitempty"`
	Subjects  []RBACSubject `json:"subjects,omitempty"`
}

// RBACSubject is one subject of an RBAC binding.
type RBACSubject struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

// NamespacePodSecurity is the Pod Security Admission label state of a namespace.
type NamespacePodSecurity struct {
	Name   string `json:"name"`
	UID    string `json:"uid,omitempty"`
	Enforce string `json:"enforce,omitempty"`
	Audit   string `json:"audit,omitempty"`
	Warn    string `json:"warn,omitempty"`
}

// Inputs is the full read-only observation bundle for one cluster evaluation.
// Every field is optional; the evaluator only checks what is supplied.
type Inputs struct {
	Components []ComponentConfig     `json:"components,omitempty"`
	Workloads  []WorkloadSecurity    `json:"workloads,omitempty"`
	Bindings   []RBACBinding         `json:"bindings,omitempty"`
	Namespaces []NamespacePodSecurity `json:"namespaces,omitempty"`
}

// Status is the rollup returned for one cluster evaluation.
type Status struct {
	ClusterID  int64            `json:"cluster_id"`
	EvaluatedAt time.Time       `json:"evaluated_at"`
	Total      int              `json:"total"`
	Failed     int              `json:"failed"`
	Passed     int              `json:"passed"`
	BySeverity map[string]int   `json:"by_severity"`
	ByFamily   map[string]int   `json:"by_family"`
	Findings   []Finding        `json:"findings"`
}
