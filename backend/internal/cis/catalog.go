package cis

// FlagCheckKind enumerates the rule shapes for component flag controls.
type FlagCheckKind string

const (
	// FlagShouldBeFalse: pass if flag is unset OR value == "false" (case-insensitive).
	FlagShouldBeFalse FlagCheckKind = "should_be_false"
	// FlagMustBeSet: pass if flag is present.
	FlagMustBeSet FlagCheckKind = "must_be_set"
	// FlagMustBeAbsent: pass if flag is NOT present.
	FlagMustBeAbsent FlagCheckKind = "must_be_absent"
	// FlagModeMustInclude: value is a comma-separated list that must contain
	// every token in Params.Contains.
	FlagModeMustInclude FlagCheckKind = "mode_must_include"
	// FlagMustNotEqual: pass if flag is unset OR value is not in Params.Disallow.
	FlagMustNotEqual FlagCheckKind = "must_not_equal"
	// FlagEquals: pass if flag is present AND value is in Params.Allow.
	FlagEquals FlagCheckKind = "equals"
)

// FlagParams carries the parameters for flag-based controls.
type FlagParams struct {
	Contains []string
	Disallow []string
	Allow    []string
}

// ComponentControl is a compiled-in CIS control for a control-plane / node
// component flag. It is modelled after the kube-bench CIS Kubernetes Benchmark
// control families (1.2.x API Server, 1.3.x Scheduler, 1.4.x Controller
// Manager, 1.5.x etcd, 4.2.x kubelet).
type ComponentControl struct {
	ID          string
	Title       string
	CISLevel    string // "low" | "medium" | "high" | "critical"
	Severity    string // finding severity used for emission
	Family      string
	Component   string
	Flag        string
	Kind        FlagCheckKind
	Params      FlagParams
	Rationale   string
	Remediation string
}

// componentControls is the compiled-in CIS control catalog. The CIS control
// numbers (e.g. 1.2.1) are kept in the ID for traceability to the upstream
// benchmark. Remediation references point at the CIS Kubernetes Benchmark.
var componentControls = []ComponentControl{
	// ---- Control Plane: kube-apiserver (CIS 1.2.x) ----
	{
		ID: "CIS-1.2.1", Title: "Ensure that the --anonymous-auth argument is set to false",
		CISLevel: "critical", Severity: SeverityCritical, Family: "Control Plane", Component: "kube-apiserver",
		Flag: "anonymous-auth", Kind: FlagShouldBeFalse,
		Rationale:   "Enabling anonymous requests allows unauthenticated access to the API server.",
		Remediation: "Set --anonymous-auth=false on the kube-apiserver.",
	},
	{
		ID: "CIS-1.2.2", Title: "Ensure that the --token-auth-file argument is not set",
		CISLevel: "critical", Severity: SeverityCritical, Family: "Control Plane", Component: "kube-apiserver",
		Flag: "token-auth-file", Kind: FlagMustBeAbsent,
		Rationale:   "Token-based authentication using static files is insecure and deprecated.",
		Remediation: "Remove the --token-auth-file argument from the kube-apiserver.",
	},
	{
		ID: "CIS-1.2.6", Title: "Ensure that the --authorization-mode argument includes Node",
		CISLevel: "medium", Severity: SeverityWarning, Family: "Control Plane", Component: "kube-apiserver",
		Flag: "authorization-mode", Kind: FlagModeMustInclude, Params: FlagParams{Contains: []string{"Node"}},
		Rationale:   "Node authorization restricts kubelet access to the API objects required for its operation.",
		Remediation: "Set --authorization-mode=Node,RBAC on the kube-apiserver.",
	},
	{
		ID: "CIS-1.2.7", Title: "Ensure that the --authorization-mode argument includes RBAC",
		CISLevel: "critical", Severity: SeverityCritical, Family: "Control Plane", Component: "kube-apiserver",
		Flag: "authorization-mode", Kind: FlagModeMustInclude, Params: FlagParams{Contains: []string{"RBAC"}},
		Rationale:   "RBAC authorization limits the operations a principal may perform on the cluster.",
		Remediation: "Set --authorization-mode=Node,RBAC on the kube-apiserver.",
	},
	{
		ID: "CIS-1.2.12", Title: "Ensure that the --profiling argument is set to false",
		CISLevel: "low", Severity: SeverityInfo, Family: "Control Plane", Component: "kube-apiserver",
		Flag: "profiling", Kind: FlagShouldBeFalse,
		Rationale:   "Profiling exposes potentially sensitive runtime information.",
		Remediation: "Set --profiling=false on the kube-apiserver.",
	},
	{
		ID: "CIS-1.2.13", Title: "Ensure that the --audit-log-path argument is set",
		CISLevel: "medium", Severity: SeverityWarning, Family: "Control Plane", Component: "kube-apiserver",
		Flag: "audit-log-path", Kind: FlagMustBeSet,
		Rationale:   "Auditing provides a chronological record of operations for forensic analysis.",
		Remediation: "Set --audit-log-path=/var/log/apiserver/audit.log on the kube-apiserver.",
	},
	{
		ID: "CIS-1.2.18", Title: "Ensure that the --allow-privileged argument is set to false",
		CISLevel: "low", Severity: SeverityInfo, Family: "Control Plane", Component: "kube-apiserver",
		Flag: "allow-privileged", Kind: FlagShouldBeFalse,
		Rationale:   "Allowing privileged containers on the control plane weakens isolation.",
		Remediation: "Set --allow-privileged=false on the kube-apiserver.",
	},
	{
		ID: "CIS-1.2.33", Title: "Ensure that the --enable-admission-plugins argument includes PodSecurity",
		CISLevel: "medium", Severity: SeverityWarning, Family: "Control Plane", Component: "kube-apiserver",
		Flag: "enable-admission-plugins", Kind: FlagModeMustInclude, Params: FlagParams{Contains: []string{"PodSecurity"}},
		Rationale:   "The PodSecurity admission plugin enforces the Pod Security Standards.",
		Remediation: "Add PodSecurity to --enable-admission-plugins on the kube-apiserver.",
	},
	{
		ID: "CIS-1.2.35", Title: "Ensure that the --tls-cert-file and --tls-private-key-file arguments are set",
		CISLevel: "medium", Severity: SeverityWarning, Family: "Control Plane", Component: "kube-apiserver",
		Flag: "tls-cert-file", Kind: FlagMustBeSet,
		Rationale:   "API server communication must be encrypted with a valid TLS certificate.",
		Remediation: "Set --tls-cert-file and --tls-private-key-file on the kube-apiserver.",
	},
	{
		ID: "CIS-1.2.36", Title: "Ensure that the --tls-cipher-suites argument is set",
		CISLevel: "medium", Severity: SeverityWarning, Family: "Control Plane", Component: "kube-apiserver",
		Flag: "tls-cipher-suites", Kind: FlagMustBeSet,
		Rationale:   "Restricting cipher suites avoids weak/legacy TLS negotiation.",
		Remediation: "Set --tls-cipher-suites to a modern suite list on the kube-apiserver.",
	},

	// ---- Control Plane: kube-scheduler (CIS 1.3.x) ----
	{
		ID: "CIS-1.3.1", Title: "Ensure that the --profiling argument is set to false",
		CISLevel: "low", Severity: SeverityInfo, Family: "Control Plane", Component: "kube-scheduler",
		Flag: "profiling", Kind: FlagShouldBeFalse,
		Rationale:   "Profiling exposes potentially sensitive runtime information.",
		Remediation: "Set --profiling=false on the kube-scheduler.",
	},
	{
		ID: "CIS-1.3.2", Title: "Ensure that the --address argument is set to 127.0.0.1",
		CISLevel: "medium", Severity: SeverityWarning, Family: "Control Plane", Component: "kube-scheduler",
		Flag: "address", Kind: FlagEquals, Params: FlagParams{Allow: []string{"127.0.0.1"}},
		Rationale:   "The scheduler health/metrics port should not be bound to a non-loopback address.",
		Remediation: "Set --address=127.0.0.1 on the kube-scheduler.",
	},

	// ---- Control Plane: kube-controller-manager (CIS 1.4.x) ----
	{
		ID: "CIS-1.4.1", Title: "Ensure that the --profiling argument is set to false",
		CISLevel: "low", Severity: SeverityInfo, Family: "Control Plane", Component: "kube-controller-manager",
		Flag: "profiling", Kind: FlagShouldBeFalse,
		Rationale:   "Profiling exposes potentially sensitive runtime information.",
		Remediation: "Set --profiling=false on the kube-controller-manager.",
	},
	{
		ID: "CIS-1.4.2", Title: "Ensure that the --use-service-account-credentials argument is set to true",
		CISLevel: "medium", Severity: SeverityWarning, Family: "Control Plane", Component: "kube-controller-manager",
		Flag: "use-service-account-credentials", Kind: FlagEquals, Params: FlagParams{Allow: []string{"true"}},
		Rationale:   "Per-controller service-account credentials limit the blast radius of a compromise.",
		Remediation: "Set --use-service-account-credentials=true on the kube-controller-manager.",
	},
	{
		ID: "CIS-1.4.6", Title: "Ensure that the --root-ca-file argument is set",
		CISLevel: "medium", Severity: SeverityWarning, Family: "Control Plane", Component: "kube-controller-manager",
		Flag: "root-ca-file", Kind: FlagMustBeSet,
		Rationale:   "The root CA is injected into service-account tokens for API trust.",
		Remediation: "Set --root-ca-file on the kube-controller-manager.",
	},
	{
		ID: "CIS-1.4.7", Title: "Ensure that the --cluster-signing-cert-file argument is set",
		CISLevel: "medium", Severity: SeverityWarning, Family: "Control Plane", Component: "kube-controller-manager",
		Flag: "cluster-signing-cert-file", Kind: FlagMustBeSet,
		Rationale:   "Certificate signing requires a configured CA certificate/key.",
		Remediation: "Set --cluster-signing-cert-file and --cluster-signing-key-file on the kube-controller-manager.",
	},

	// ---- etcd (CIS 1.5.x) ----
	{
		ID: "CIS-1.5.1", Title: "Ensure that the --cert-file and --key-file arguments are set",
		CISLevel: "medium", Severity: SeverityWarning, Family: "Control Plane", Component: "etcd",
		Flag: "cert-file", Kind: FlagMustBeSet,
		Rationale:   "etcd peer/client communication must be encrypted.",
		Remediation: "Set --cert-file and --key-file on etcd.",
	},
	{
		ID: "CIS-1.5.2", Title: "Ensure that the --client-cert-auth argument is set to true",
		CISLevel: "medium", Severity: SeverityWarning, Family: "Control Plane", Component: "etcd",
		Flag: "client-cert-auth", Kind: FlagEquals, Params: FlagParams{Allow: []string{"true"}},
		Rationale:   "Client certificate authentication prevents unauthenticated etcd access.",
		Remediation: "Set --client-cert-auth=true on etcd.",
	},
	{
		ID: "CIS-1.5.4", Title: "Ensure that the --peer-cert-file argument is set",
		CISLevel: "medium", Severity: SeverityWarning, Family: "Control Plane", Component: "etcd",
		Flag: "peer-cert-file", Kind: FlagMustBeSet,
		Rationale:   "Peer communication must be authenticated with certificates.",
		Remediation: "Set --peer-cert-file and --peer-key-file on etcd.",
	},
	{
		ID: "CIS-1.5.5", Title: "Ensure that the --peer-client-cert-auth argument is set to true",
		CISLevel: "medium", Severity: SeverityWarning, Family: "Control Plane", Component: "etcd",
		Flag: "peer-client-cert-auth", Kind: FlagEquals, Params: FlagParams{Allow: []string{"true"}},
		Rationale:   "Peer client certificate authentication prevents unauthenticated peer access.",
		Remediation: "Set --peer-client-cert-auth=true on etcd.",
	},

	// ---- Worker Node: kubelet (CIS 4.2.x) ----
	{
		ID: "CIS-4.2.1", Title: "Ensure that the --anonymous-auth argument is set to false",
		CISLevel: "critical", Severity: SeverityCritical, Family: "Worker Node", Component: "kubelet",
		Flag: "anonymous-auth", Kind: FlagShouldBeFalse,
		Rationale:   "Anonymous kubelet auth allows unauthenticated node access.",
		Remediation: "Set --anonymous-auth=false in the kubelet config.",
	},
	{
		ID: "CIS-4.2.2", Title: "Ensure that the --authorization-mode argument is not set to AlwaysAllow",
		CISLevel: "critical", Severity: SeverityCritical, Family: "Worker Node", Component: "kubelet",
		Flag: "authorization-mode", Kind: FlagMustNotEqual, Params: FlagParams{Disallow: []string{"AlwaysAllow"}},
		Rationale:   "AlwaysAllow authorization on the kubelet lets any caller perform any action.",
		Remediation: "Set --authorization-mode=Webhook on the kubelet.",
	},
	{
		ID: "CIS-4.2.3", Title: "Ensure that the --client-ca-file argument is set",
		CISLevel: "medium", Severity: SeverityWarning, Family: "Worker Node", Component: "kubelet",
		Flag: "client-ca-file", Kind: FlagMustBeSet,
		Rationale:   "The client CA validates certificates presented to the kubelet API.",
		Remediation: "Set --client-ca-file in the kubelet config.",
	},
	{
		ID: "CIS-4.2.4", Title: "Ensure that the --read-only-port argument is set to 0",
		CISLevel: "low", Severity: SeverityInfo, Family: "Worker Node", Component: "kubelet",
		Flag: "read-only-port", Kind: FlagEquals, Params: FlagParams{Allow: []string{"0"}},
		Rationale:   "The read-only port exposes unauthenticated node information.",
		Remediation: "Set --read-only-port=0 in the kubelet config.",
	},
	{
		ID: "CIS-4.2.6", Title: "Ensure that the --protect-kernel-defaults argument is set to true",
		CISLevel: "medium", Severity: SeverityWarning, Family: "Worker Node", Component: "kubelet",
		Flag: "protect-kernel-defaults", Kind: FlagEquals, Params: FlagParams{Allow: []string{"true"}},
		Rationale:   "Kernel default protection stops the kubelet from relaxing sysctl defaults.",
		Remediation: "Set --protect-kernel-defaults=true in the kubelet config.",
	},
	{
		ID: "CIS-4.2.10", Title: "Ensure that the --tls-cert-file and --tls-private-key-file arguments are set",
		CISLevel: "medium", Severity: SeverityWarning, Family: "Worker Node", Component: "kubelet",
		Flag: "tls-cert-file", Kind: FlagMustBeSet,
		Rationale:   "Kubelet API communication must be encrypted with a valid TLS certificate.",
		Remediation: "Set --tls-cert-file and --tls-private-key-file in the kubelet config.",
	},
}

// WorkloadCheck is a compiled-in CIS control for a Pod / workload security
// context (CIS 5.2.x / Pod Security Standards). Violates reports true when the
// (workload, container) pair fails the control.
type WorkloadCheck struct {
	ID          string
	Title       string
	CISLevel    string
	Severity    string
	Family      string
	Rationale   string
	Remediation string
	Violates    func(w WorkloadSecurity, c ContainerSecurity) bool
	Detail      func(w WorkloadSecurity, c ContainerSecurity) string
}

var workloadChecks = []WorkloadCheck{
	{
		ID: "CIS-WL-PRIV", Title: "Privileged container",
		CISLevel: "critical", Severity: SeverityCritical, Family: "Workloads",
		Rationale:   "A privileged container gains access to all devices on the host and escapes most isolation.",
		Remediation: "Set securityContext.privileged=false (or remove it) on the container.",
		Violates:    func(_ WorkloadSecurity, c ContainerSecurity) bool { return boolVal(c.Privileged) },
		Detail:      func(_ WorkloadSecurity, c ContainerSecurity) string { return "container is privileged" },
	},
	{
		ID: "CIS-WL-ALLOW-ESC", Title: "Allow privilege escalation",
		CISLevel: "high", Severity: SeverityWarning, Family: "Workloads",
		Rationale:   "allowPrivilegeEscalation lets a process gain more privileges than its parent.",
		Remediation: "Set securityContext.allowPrivilegeEscalation=false on the container.",
		Violates:    func(_ WorkloadSecurity, c ContainerSecurity) bool { return boolVal(c.AllowPrivilegeEscalation) },
		Detail:      func(_ WorkloadSecurity, c ContainerSecurity) string { return "allowPrivilegeEscalation=true" },
	},
	{
		ID: "CIS-WL-RUN-AS-NON-ROOT", Title: "Container may run as root",
		CISLevel: "high", Severity: SeverityWarning, Family: "Workloads",
		Rationale:   "Running as UID 0 (root) increases the blast radius of a container compromise.",
		Remediation: "Set securityContext.runAsNonRoot=true and an explicit non-zero runAsUser.",
		Violates: func(_ WorkloadSecurity, c ContainerSecurity) bool {
			if boolVal(c.RunAsNonRoot) {
				return false
			}
			if c.RunAsUser != nil && *c.RunAsUser > 0 {
				return false
			}
			return true
		},
		Detail: func(_ WorkloadSecurity, c ContainerSecurity) string {
			if c.RunAsUser != nil && *c.RunAsUser == 0 {
				return "runAsUser=0 (root)"
			}
			if c.RunAsNonRoot != nil && !*c.RunAsNonRoot {
				return "runAsNonRoot=false"
			}
			return "no non-root constraint"
		},
	},
	{
		ID: "CIS-WL-HOST-NS", Title: "Host namespace sharing",
		CISLevel: "high", Severity: SeverityWarning, Family: "Workloads",
		Rationale:   "Sharing host PID/IPC/network namespaces exposes the host to the container.",
		Remediation: "Unset hostNetwork/hostPID/hostIPC on the pod spec.",
		Violates:    func(_ WorkloadSecurity, c ContainerSecurity) bool { return c.HostNetwork || c.HostPID || c.HostIPC },
		Detail: func(_ WorkloadSecurity, c ContainerSecurity) string {
			switch {
			case c.HostPID:
				return "hostPID=true"
			case c.HostIPC:
				return "hostIPC=true"
			default:
				return "hostNetwork=true"
			}
		},
	},
	{
		ID: "CIS-WL-HOSTPATH", Title: "HostPath volume mounted",
		CISLevel: "medium", Severity: SeverityWarning, Family: "Workloads",
		Rationale:   "HostPath volumes grant the container read/write access to the host filesystem.",
		Remediation: "Replace hostPath volumes with a safer volume type or restrict the path.",
		Violates:    func(_ WorkloadSecurity, c ContainerSecurity) bool { return c.HostPathVolumes > 0 },
		Detail:      func(_ WorkloadSecurity, c ContainerSecurity) string { return "hostPath volumes mounted" },
	},
	{
		ID: "CIS-WL-DROP-NET-RAW", Title: "CAP_NET_RAW not dropped",
		CISLevel: "low", Severity: SeverityInfo, Family: "Workloads",
		Rationale:   "Dropping CAP_NET_RAW prevents raw socket crafting inside the container.",
		Remediation: "Add NET_RAW to securityContext.capabilities.drop.",
		Violates:    func(_ WorkloadSecurity, c ContainerSecurity) bool { return !containsStr(c.CapabilitiesDrop, "NET_RAW") },
		Detail:      func(_ WorkloadSecurity, c ContainerSecurity) string { return "NET_RAW not in capabilities.drop" },
	},
}

// RBACCheck is a compiled-in CIS control for RBAC bindings / roles (CIS 5.1.x).
type RBACCheck struct {
	ID          string
	Title       string
	CISLevel    string
	Severity    string
	Family      string
	Rationale   string
	Remediation string
	Violates    func(b RBACBinding) bool
	Detail      func(b RBACBinding) string
}

var rbacChecks = []RBACCheck{
	{
		ID: "CIS-RBAC-CLUSTER-ADMIN", Title: "cluster-admin granted to a non-system subject",
		CISLevel: "critical", Severity: SeverityCritical, Family: "RBAC",
		Rationale:   "Binding cluster-admin to a user/group/SA outside the system: namespace is a major privilege escalation risk.",
		Remediation: "Remove the cluster-admin binding or scope it to the minimal required principal.",
		Violates: func(b RBACBinding) bool {
			if b.RoleKind != "ClusterRole" || b.RoleName != "cluster-admin" {
				return false
			}
			for _, s := range b.Subjects {
				if s.Kind == "ServiceAccount" && s.Namespace == "kube-system" {
					continue
				}
				if s.Name == "system:kube-controller-manager" || s.Name == "system:kube-scheduler" || s.Name == "system:apiserver" {
					continue
				}
				if len(s.Name) >= 7 && s.Name[:7] == "system:" {
					continue
				}
				return true
			}
			return false
		},
		Detail: func(b RBACBinding) string { return "cluster-admin bound to non-system principal" },
	},
	{
		ID: "CIS-RBAC-WILDCARD", Title: "Role grants wildcard verb/resource",
		CISLevel: "high", Severity: SeverityWarning, Family: "RBAC",
		Rationale:   "A rule with verb \"*\" or resource \"*\" grants unrestricted access within its scope.",
		Remediation: "Tighten the Role to the minimal verbs/resources required.",
		Violates: func(b RBACBinding) bool {
			for _, r := range b.RoleRules {
				if containsStr(r.Verbs, "*") || containsStr(r.Resources, "*") {
					return true
				}
			}
			return false
		},
		Detail: func(b RBACBinding) string { return "role contains a wildcard verb or resource" },
	},
}

// NamespaceCheck is a compiled-in CIS control for Pod Security Admission labels
// (CIS 5.2.x / Pod Security Standards).
type NamespaceCheck struct {
	ID          string
	Title       string
	CISLevel    string
	Severity    string
	Family      string
	Rationale   string
	Remediation string
	Violates    func(ns NamespacePodSecurity) bool
	Detail      func(ns NamespacePodSecurity) string
}

var namespaceChecks = []NamespaceCheck{
	{
		ID: "CIS-PSA-ENFORCE", Title: "Namespace has no (or privileged) Pod Security enforce level",
		CISLevel: "medium", Severity: SeverityWarning, Family: "Pod Security",
		Rationale:   "Without a Pod Security Admission enforce level, privileged pods may be admitted.",
		Remediation: "Set pod-security.kubernetes.io/enforce=restricted (or baseline) on the namespace.",
		Violates:    func(ns NamespacePodSecurity) bool { return ns.Enforce == "" || ns.Enforce == "privileged" },
		Detail:      func(ns NamespacePodSecurity) string { return "enforce=" + ns.Enforce },
	},
}

// boolVal returns the value of a *bool, treating nil as false.
func boolVal(p *bool) bool {
	if p == nil {
		return false
	}
	return *p
}

func containsStr(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// ComponentCatalog returns the compiled-in component flag controls.
func ComponentCatalog() []ComponentControl { return componentControls }

// WorkloadCatalog returns the compiled-in workload security controls.
func WorkloadCatalog() []WorkloadCheck { return workloadChecks }

// RBACCatalog returns the compiled-in RBAC controls.
func RBACCatalog() []RBACCheck { return rbacChecks }

// NamespaceCatalog returns the compiled-in namespace Pod Security controls.
func NamespaceCatalog() []NamespaceCheck { return namespaceChecks }
