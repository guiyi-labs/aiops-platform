package policy

import (
	"sort"
	"time"

	k8sfinding "k8s-aiops.local/backend/internal/finding"
)

// Evaluate runs the read-only policy evaluation over the supplied observation
// bundle and returns an aggregated Status. It is pure: it never contacts a
// cluster and never mutates anything.
//
// Each workload contributes pod-level checks (host access) plus per-container
// checks (resource requests/limits, security context, probes). Status.Total
// is the number of checks evaluated, Failed the ones that produced a finding.
func Evaluate(clusterID int64, in Inputs, observedAt time.Time) Status {
	at := observedAt.UTC()
	status := Status{
		ClusterID:      clusterID,
		EvaluatedAt:    at,
		WorkloadsTotal: len(in.Workloads),
		BySeverity:     map[string]int{},
		ByFamily:       map[string]int{},
		Findings:       []Finding{},
	}
	for _, wl := range in.Workloads {
		status.ContainersTotal += len(wl.Containers)
	}

	checks := 0
	for _, wl := range in.Workloads {
		for _, ctr := range wl.Containers {
			var checked int
			status, checked = evaluateContainer(status, wl, ctr, at)
			checks += checked
		}
		checks += evaluatePodLevel(&status, wl, at)
	}

	status.CompliantWorkloads = compliantCount(in, status.Findings)

	status.Total = checks
	status.Failed = len(status.Findings)
	status.Passed = status.Total - status.Failed
	if status.Passed < 0 {
		status.Passed = 0
	}
	sortFindings(status.Findings)
	return status
}

// evaluateContainer runs the per-container rules and returns the number of
// checks evaluated.
func evaluateContainer(status Status, wl WorkloadPolicy, ctr ContainerPolicy, at time.Time) (Status, int) {
	checked := 0

	// Resource rules: cpu request, memory request, limits.
	checked += 3
	if !ctr.CPURequest {
		status = status.withFinding(finding(wl, ctr, CodeNoCPURequest, SeverityWarning, FamilyResources,
			"容器未声明 CPU requests，突发流量可能得不到 QoS 保障",
			"在 resources.requests 中设置 cpu 请求值（如 100m），并基于实际用量（FinOps 分析）校准", at))
	}
	if !ctr.MemoryRequest {
		status = status.withFinding(finding(wl, ctr, CodeNoMemoryRequest, SeverityWarning, FamilyResources,
			"容器未声明内存 requests，OOM 时可能被直接杀死而无法优雅退出",
			"在 resources.requests 中设置 memory 请求值（如 128Mi）", at))
	}
	if !ctr.HasResourceLimits {
		status = status.withFinding(finding(wl, ctr, CodeNoResourceLimits, SeverityWarning, FamilyResources,
			"容器未声明资源 limits，单个 Pod 可无限抢占节点 CPU/内存",
			"在 resources.limits 中设置 cpu/memory 上限（如 cpu: 1, memory: 1Gi）", at))
	}

	// Security-context rules. privileged defaults to false; only an explicit
	// true is a finding. allowPrivilegeEscalation defaults to true in
	// Kubernetes, so both an explicit true and an unset value on an
	// unprivileged container are reported (the safe value is false).
	checked += 3
	if ctr.Privileged != nil && *ctr.Privileged {
		status = status.withFinding(finding(wl, ctr, CodePrivileged, SeverityCritical, FamilySecurity,
			"容器以 privileged 运行，拥有宿主机全部能力，逃逸即接管节点",
			"移除 privileged: true；若需特殊能力，用 capabilities.add 精确授予", at))
	}
	esc := ctr.AllowPrivilegeEscalation == nil || (ctr.AllowPrivilegeEscalation != nil && *ctr.AllowPrivilegeEscalation)
	if esc {
		status = status.withFinding(finding(wl, ctr, CodeAllowEscalation, SeverityWarning, FamilySecurity,
			"容器允许权限提升（allowPrivilegeEscalation 未显式设为 false）",
			"设置 securityContext.allowPrivilegeEscalation: false，并 drop ALL capabilities", at))
	}
	if ctr.RunAsNonRoot == nil || !*ctr.RunAsNonRoot {
		status = status.withFinding(finding(wl, ctr, CodeRunAsRoot, SeverityInfo, FamilySecurity,
			"容器未强制以非 root 用户运行（runAsNonRoot 未设为 true）",
			"设置 securityContext.runAsNonRoot: true，或指定 runAsUser > 0 并以非 root 镜像构建", at))
	}

	// Probe rules: liveness, readiness, startup.
	checked += 3
	if !ctr.LivenessProbe {
		status = status.withFinding(finding(wl, ctr, CodeNoLivenessProbe, SeverityWarning, FamilyProbes,
			"容器未配置 livenessProbe，进程挂死时不会被重启",
			"配置 livenessProbe（如 httpGet /healthz），periodSeconds 建议 10-30", at))
	}
	if !ctr.ReadinessProbe {
		status = status.withFinding(finding(wl, ctr, CodeNoReadinessProbe, SeverityWarning, FamilyProbes,
			"容器未配置 readinessProbe，启动中或过载的实例仍会被路由流量",
			"配置 readinessProbe 与 livenessProbe 分离（readiness 检查依赖就绪状态）", at))
	}
	if !ctr.StartupProbe {
		status = status.withFinding(finding(wl, ctr, CodeNoStartupProbe, SeverityInfo, FamilyProbes,
			"容器未配置 startupProbe，慢启动应用可能被 livenessProbe 反复误杀",
			"对启动超过 1-2 分钟的应用配置 startupProbe，并放宽 failureThreshold", at))
	}

	return status, checked
}

// evaluatePodLevel runs the pod-level (host access) rules and returns the
// number of checks evaluated.
func evaluatePodLevel(status *Status, wl WorkloadPolicy, at time.Time) int {
	if wl.HostNetwork {
		*status = status.withFinding(finding(wl, ContainerPolicy{}, CodeHostNetwork, SeverityWarning, FamilyHostAccess,
			"工作负载使用 hostNetwork，Pod 与宿主机共享网络栈，绕过 Service/NetworkPolicy",
			"移除 hostNetwork: true，改用 NodePort / LoadBalancer / Ingress 暴露", at))
	}
	if wl.HostPID || wl.HostIPC {
		*status = status.withFinding(finding(wl, ContainerPolicy{}, CodeHostPIDOrIPC, SeverityWarning, FamilyHostAccess,
			"工作负载共享宿主机 PID 或 IPC 命名空间，容器可观察/干扰宿主进程",
			"移除 hostPID / hostIPC；确需系统级观测时改用特权 DaemonSet 并严格管控", at))
	}
	return 2
}

// compliantCount returns the number of workloads with no finding at all.
func compliantCount(in Inputs, findings []Finding) int {
	flagged := map[string]bool{}
	for _, f := range findings {
		if f.Resource.Kind == "" {
			continue
		}
		key := f.Resource.Kind + "/" + f.Resource.Namespace + "/" + f.Resource.Name
		flagged[key] = true
	}
	compliant := 0
	for _, wl := range in.Workloads {
		key := wl.Kind + "/" + wl.Namespace + "/" + wl.Name
		if !flagged[key] {
			compliant++
		}
	}
	return compliant
}

// finding builds one canonical Finding from a workload/container pair. The
// container may be empty (pod-level rules); the resource citation still
// points at the workload.
func finding(wl WorkloadPolicy, ctr ContainerPolicy, code, severity, family, summary, remediation string, at time.Time) Finding {
	details := map[string]string{
		"family":      family,
		"rationale":   summary,
		"remediation": remediation,
	}
	if ctr.Name != "" {
		details["container"] = ctr.Name
	}
	return Finding{
		Code:     code,
		Severity: severity,
		Summary:  summary,
		Resource: k8sfinding.ResourceCitation{
			Kind:      wl.Kind,
			Namespace: wl.Namespace,
			Name:      wl.Name,
			UID:       wl.UID,
		},
		Details:    details,
		ObservedAt: k8sfinding.RFC3339(at),
	}
}

// withFinding appends a finding and updates the rollup counters.
func (s Status) withFinding(f Finding) Status {
	s.Findings = append(s.Findings, f)
	s.Failed++
	if s.BySeverity == nil {
		s.BySeverity = map[string]int{}
	}
	s.BySeverity[f.Severity]++
	if s.ByFamily == nil {
		s.ByFamily = map[string]int{}
	}
	if family := f.Details["family"]; family != "" {
		s.ByFamily[family]++
	}
	return s
}

var severityRank = map[string]int{SeverityCritical: 0, SeverityWarning: 1, SeverityInfo: 2}

// sortFindings orders findings most-actionable first and deterministically:
// severity, then code, then namespace/name.
func sortFindings(findings []Finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if ra, rb := severityRank[a.Severity], severityRank[b.Severity]; ra != rb {
			return ra < rb
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		if a.Resource.Namespace != b.Resource.Namespace {
			return a.Resource.Namespace < b.Resource.Namespace
		}
		return a.Resource.Name < b.Resource.Name
	})
}
