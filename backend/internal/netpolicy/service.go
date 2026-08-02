package netpolicy

import (
	"sort"
	"strconv"
	"strings"
	"time"

	k8sfinding "k8s-aiops.local/backend/internal/finding"
)

// worldCIDRs are the ipBlock values that open a rule to the entire internet.
var worldCIDRs = map[string]bool{"0.0.0.0/0": true, "::/0": true}

// Evaluate runs the read-only network connectivity and NetworkPolicy posture
// analysis over the supplied observation bundle and returns an aggregated
// Status. It is pure: it never contacts a cluster, never probes the network
// and never mutates anything.
//
// Only the domains that are supplied are checked; missing domains are skipped
// (neither pass nor fail), which keeps the evaluator safe to run against a
// partial observation.
func Evaluate(clusterID int64, in Inputs, observedAt time.Time) Status {
	at := observedAt.UTC()
	status := Status{
		ClusterID:       clusterID,
		EvaluatedAt:     at,
		NamespacesTotal: len(in.Namespaces),
		PodsTotal:       len(in.Pods),
		PoliciesTotal:   len(in.Policies),
		ServicesTotal:   len(in.Services),
		BySeverity:      map[string]int{},
		ByFamily:        map[string]int{},
		Findings:        []Finding{},
	}

	idx := newIndex(in)
	status = evalNamespaceBaseline(status, idx, at)
	status = evalPodCoverage(status, idx, at)
	status = evalPolicyHygiene(status, idx, at)
	status = evalServices(status, idx, at)

	status.Passed = status.Total - status.Failed
	sortFindings(status.Findings)
	return status
}

// --- indexing -------------------------------------------------------------

type index struct {
	namespaces   []string
	podsByNS     map[string][]PodInfo
	policiesByNS map[string][]Policy
	services     []ServiceInfo
}

func newIndex(in Inputs) index {
	idx := index{
		podsByNS:     make(map[string][]PodInfo, len(in.Namespaces)),
		policiesByNS: make(map[string][]Policy, len(in.Namespaces)),
		services:     in.Services,
	}
	seen := make(map[string]bool, len(in.Namespaces))
	add := func(ns string) {
		if ns == "" || seen[ns] {
			return
		}
		seen[ns] = true
		idx.namespaces = append(idx.namespaces, ns)
	}
	for _, ns := range in.Namespaces {
		add(ns.Name)
	}
	for _, p := range in.Pods {
		add(p.Namespace)
		idx.podsByNS[p.Namespace] = append(idx.podsByNS[p.Namespace], p)
	}
	for _, p := range in.Policies {
		add(p.Namespace)
		idx.policiesByNS[p.Namespace] = append(idx.policiesByNS[p.Namespace], p)
	}
	for _, s := range in.Services {
		add(s.Namespace)
	}
	sort.Strings(idx.namespaces)
	return idx
}

// ingressPolicyCovers reports whether the policy restricts ingress for the pod.
func ingressPolicyCovers(p Policy, pod PodInfo) bool {
	return p.Namespace == pod.Namespace && p.AppliesToIngress() &&
		(p.PodSelector.SelectsAll() || p.PodSelector.coversConservatively(pod.Labels))
}

func (idx index) ingressPoliciesFor(pod PodInfo) []Policy {
	var out []Policy
	for _, p := range idx.policiesByNS[pod.Namespace] {
		if ingressPolicyCovers(p, pod) {
			out = append(out, p)
		}
	}
	return out
}

func (idx index) egressCovered(pod PodInfo) bool {
	for _, p := range idx.policiesByNS[pod.Namespace] {
		if p.AppliesToEgress() && (p.PodSelector.SelectsAll() || p.PodSelector.coversConservatively(pod.Labels)) {
			return true
		}
	}
	return false
}

// backendsFor returns the pods a Service routes to. A Service with no
// selector has externally managed Endpoints and returns (nil, false).
func (idx index) backendsFor(svc ServiceInfo) ([]PodInfo, bool) {
	if len(svc.Selector) == 0 {
		return nil, false
	}
	out := make([]PodInfo, 0, 4)
	for _, pod := range idx.podsByNS[svc.Namespace] {
		if matchLabels(pod.Labels, svc.Selector) {
			out = append(out, pod)
		}
	}
	return out, true
}

// --- checks ---------------------------------------------------------------

// evalNamespaceBaseline flags namespaces that run workloads without a
// default-deny ingress baseline, i.e. without a policy selecting every pod.
func evalNamespaceBaseline(status Status, idx index, at time.Time) Status {
	for _, ns := range idx.namespaces {
		pods := idx.podsByNS[ns]
		if len(pods) == 0 {
			continue // nothing to protect yet
		}
		isolated := false
		for _, p := range idx.policiesByNS[ns] {
			if p.PodSelector.SelectsAll() && p.AppliesToIngress() {
				isolated = true
				break
			}
		}
		if isolated {
			status.IsolatedNamespaces++
		}
		status.Total++
		if isolated {
			continue
		}
		severity := SeverityWarning
		if systemNamespaces[ns] {
			severity = SeverityInfo
		}
		status = status.withFinding(Finding{
			Code:     CodeNamespaceNoDefaultDeny,
			Severity: severity,
			Summary:  "命名空间没有 default-deny 入向基线，集群内任意 Pod 都可访问其工作负载",
			Resource: k8sfinding.ResourceCitation{Kind: "Namespace", Name: ns},
			Details: map[string]string{
				"family":       FamilyCoverage,
				"pods":         strconv.Itoa(len(pods)),
				"policies":     strconv.Itoa(len(idx.policiesByNS[ns])),
				"rationale":    "没有任何 NetworkPolicy 选中全部 Pod 并声明 Ingress 类型时，命名空间内的所有工作负载默认接受来自集群任意来源的流量。",
				"remediation":  "创建一条 podSelector 为空、policyTypes 仅含 Ingress 且没有 ingress 规则的策略作为默认拒绝基线，再按需追加放行策略。",
				"system_scope": boolString(systemNamespaces[ns]),
			},
			ObservedAt: k8sfinding.RFC3339(at),
		})
	}
	return status
}

// evalPodCoverage flags pods left unprotected inside a namespace that already
// has at least one NetworkPolicy — the partial-coverage case, where somebody
// clearly intended to restrict traffic but missed this workload. Namespaces
// with no policy at all are already reported once at namespace level, so they
// are not repeated per pod.
func evalPodCoverage(status Status, idx index, at time.Time) Status {
	for _, ns := range idx.namespaces {
		policies := idx.policiesByNS[ns]
		for _, pod := range idx.podsByNS[ns] {
			covering := idx.ingressPoliciesFor(pod)
			if len(covering) > 0 {
				status.IngressCoveredPods++
			}
			if idx.egressCovered(pod) {
				status.EgressCoveredPods++
			}
			if len(policies) == 0 {
				continue
			}
			status.Total++
			if len(covering) == 0 {
				status = status.withFinding(Finding{
					Code:     CodePodIngressUnrestricted,
					Severity: SeverityWarning,
					Summary:  "命名空间已有 NetworkPolicy，但该 Pod 未被任何入向策略选中，处于无保护状态",
					Resource: k8sfinding.ResourceCitation{Kind: "Pod", Namespace: ns, Name: pod.Name, UID: pod.UID},
					Details: map[string]string{
						"family":              FamilyCoverage,
						"namespace_policies":  strconv.Itoa(len(policies)),
						"pod_labels":          labelString(pod.Labels),
						"rationale":           "同命名空间内其他工作负载已被策略约束，说明此处遗漏而非有意开放。",
						"remediation":         "检查策略的 podSelector 标签是否与该 Pod 匹配，或补一条覆盖它的入向策略。",
						"selected_by_ingress": "0",
					},
					ObservedAt: k8sfinding.RFC3339(at),
				})
				continue
			}
			if pod.HostNetwork {
				status.Total++
				status = status.withFinding(Finding{
					Code:     CodeHostNetworkIneffective,
					Severity: SeverityInfo,
					Summary:  "hostNetwork Pod 被入向策略选中，但多数 CNI 不会对主机网络流量生效",
					Resource: k8sfinding.ResourceCitation{Kind: "Pod", Namespace: ns, Name: pod.Name, UID: pod.UID},
					Details: map[string]string{
						"family":      FamilyPolicyHygiene,
						"policies":    strconv.Itoa(len(covering)),
						"rationale":   "hostNetwork Pod 直接使用节点网络栈，NetworkPolicy 通常作用于 Pod 网络，策略可能形同虚设。",
						"remediation": "改用 Pod 网络，或在节点防火墙/安全组层面限制该端口。",
					},
					ObservedAt: k8sfinding.RFC3339(at),
				})
			}
		}
	}
	return status
}

// evalPolicyHygiene inspects the policies themselves: dead selectors,
// allow-all rules, from-all-namespaces peers and world-open ipBlocks.
func evalPolicyHygiene(status Status, idx index, at time.Time) Status {
	for _, ns := range idx.namespaces {
		for _, policy := range idx.policiesByNS[ns] {
			res := k8sfinding.ResourceCitation{Kind: "NetworkPolicy", Namespace: policy.Namespace, Name: policy.Name, UID: policy.UID}

			// A selector that resolves to nothing is a typo, not a policy.
			// matchExpressions are not evaluated here, so such selectors are
			// skipped rather than guessed at.
			if !policy.PodSelector.SelectsAll() && !policy.PodSelector.HasExpressions {
				status.Total++
				matched := 0
				for _, pod := range idx.podsByNS[policy.Namespace] {
					if policy.PodSelector.Matches(pod.Labels) {
						matched++
					}
				}
				if matched == 0 {
					status = status.withFinding(Finding{
						Code:     CodePolicySelectsNoPods,
						Severity: SeverityWarning,
						Summary:  "NetworkPolicy 的 podSelector 未选中任何 Pod，该策略实际不生效",
						Resource: res,
						Details: map[string]string{
							"family":       FamilyPolicyHygiene,
							"pod_selector": labelString(policy.PodSelector.MatchLabels),
							"matched_pods": "0",
							"rationale":    "选择器标签与命名空间内任何 Pod 都不匹配，通常是标签拼写或大小写错误，会让人误以为流量已被限制。",
							"remediation":  "核对 podSelector 与目标工作负载的标签，或删除这条已失效的策略。",
						},
						ObservedAt: k8sfinding.RFC3339(at),
					})
				}
			}

			if policy.AppliesToIngress() {
				for i, rule := range policy.Ingress {
					status.Total++
					if len(rule.Peers) == 0 {
						status = status.withFinding(Finding{
							Code:     CodeIngressAllowAll,
							Severity: SeverityWarning,
							Summary:  "入向规则没有 from 限制，等同于放行所有来源",
							Resource: res,
							Details: map[string]string{
								"family":      FamilyPolicyHygiene,
								"rule_index":  strconv.Itoa(i),
								"ports":       portRuleString(rule.Ports),
								"rationale":   "ingress 规则省略 from 时，Kubernetes 视为允许任意来源，抵消了同命名空间内的 default-deny 基线。",
								"remediation": "为该规则补上 podSelector / namespaceSelector / ipBlock 限定来源。",
							},
							ObservedAt: k8sfinding.RFC3339(at),
						})
						continue
					}
					for _, peer := range rule.Peers {
						status = evalIngressPeer(status, res, i, peer, at)
					}
				}
			}

			if policy.AppliesToEgress() {
				for i, rule := range policy.Egress {
					for _, peer := range rule.Peers {
						if peer.IPBlockCIDR == "" || !worldCIDRs[peer.IPBlockCIDR] || len(peer.IPBlockExcept) > 0 {
							continue
						}
						status.Total++
						status = status.withFinding(Finding{
							Code:     CodeWideIPBlock,
							Severity: SeverityInfo,
							Summary:  "出向规则放行 " + peer.IPBlockCIDR + "，工作负载可访问整个互联网",
							Resource: res,
							Details: map[string]string{
								"family":      FamilyPolicyHygiene,
								"direction":   "egress",
								"rule_index":  strconv.Itoa(i),
								"cidr":        peer.IPBlockCIDR,
								"rationale":   "无限制出向会让被入侵的容器可以任意外联，是数据外泄和挖矿的常见通道。",
								"remediation": "按依赖收敛出向网段，或至少用 except 排除内网与元数据服务地址。",
							},
							ObservedAt: k8sfinding.RFC3339(at),
						})
					}
				}
			}
		}
	}
	return status
}

func evalIngressPeer(status Status, res k8sfinding.ResourceCitation, ruleIndex int, peer Peer, at time.Time) Status {
	if peer.NamespaceSelector != nil {
		status.Total++
		podUnrestricted := peer.PodSelector == nil || peer.PodSelector.SelectsAll()
		if peer.NamespaceSelector.SelectsAll() && podUnrestricted {
			status = status.withFinding(Finding{
				Code:     CodeIngressFromAllNS,
				Severity: SeverityWarning,
				Summary:  "入向规则的 namespaceSelector 为空，放行了集群内所有命名空间",
				Resource: res,
				Details: map[string]string{
					"family":      FamilyPolicyHygiene,
					"rule_index":  strconv.Itoa(ruleIndex),
					"rationale":   "空的 namespaceSelector 匹配全部命名空间，任何租户的 Pod 都能访问该工作负载。",
					"remediation": "用具体标签限定来源命名空间，并配合 podSelector 收敛到具体调用方。",
				},
				ObservedAt: k8sfinding.RFC3339(at),
			})
		}
	}
	if peer.IPBlockCIDR != "" && worldCIDRs[peer.IPBlockCIDR] && len(peer.IPBlockExcept) == 0 {
		status.Total++
		status = status.withFinding(Finding{
			Code:     CodeWideIPBlock,
			Severity: SeverityWarning,
			Summary:  "入向规则放行 " + peer.IPBlockCIDR + "，等同于对全网开放",
			Resource: res,
			Details: map[string]string{
				"family":      FamilyPolicyHygiene,
				"direction":   "ingress",
				"rule_index":  strconv.Itoa(ruleIndex),
				"cidr":        peer.IPBlockCIDR,
				"rationale":   "ipBlock 0.0.0.0/0 覆盖集群外的任意来源，使该策略失去隔离意义。",
				"remediation": "改为具体网段，或使用 except 排除公网地址。",
			},
			ObservedAt: k8sfinding.RFC3339(at),
		})
	}
	return status
}

// evalServices answers "would traffic actually arrive": Services that select
// no pods, ports whose targetPort resolves to nothing, ports the namespace's
// own policies would drop, and externally exposed Services with no ingress
// restriction at all.
func evalServices(status Status, idx index, at time.Time) Status {
	for _, svc := range idx.services {
		if equalFold(svc.Type, "ExternalName") {
			continue // no cluster backends by definition
		}
		if svc.Externally() {
			status.ExposedServices++
		}
		res := k8sfinding.ResourceCitation{Kind: "Service", Namespace: svc.Namespace, Name: svc.Name, UID: svc.UID}
		backends, selectorBased := idx.backendsFor(svc)
		if !selectorBased {
			continue // Endpoints managed outside the Service; nothing to infer
		}

		status.Total++
		if len(backends) == 0 {
			status = status.withFinding(Finding{
				Code:     CodeServiceNoBackends,
				Severity: SeverityCritical,
				Summary:  "Service 的 selector 未选中任何 Pod，访问该服务必然失败",
				Resource: res,
				Details: map[string]string{
					"family":       FamilyReachability,
					"selector":     labelString(svc.Selector),
					"service_type": defaultString(svc.Type, "ClusterIP"),
					"rationale":    "没有后端 Endpoint 时，请求会立即被拒绝或超时，是最常见的『服务访问不通』根因。",
					"remediation":  "核对 selector 与工作负载 Pod 模板标签是否一致，并确认 Pod 处于 Running。",
				},
				ObservedAt: k8sfinding.RFC3339(at),
			})
			continue
		}

		covering := ingressPoliciesForPods(idx, backends)
		for _, port := range svc.Ports {
			status = evalServicePort(status, svc, port, backends, covering, res, at)
		}

		if svc.Externally() {
			status.Total++
			if len(covering) == 0 {
				status = status.withFinding(Finding{
					Code:     CodeExposedUnrestricted,
					Severity: SeverityCritical,
					Summary:  "对外暴露的 " + defaultString(svc.Type, "Service") + " 服务，其后端 Pod 没有任何入向策略约束",
					Resource: res,
					Details: map[string]string{
						"family":       FamilyExposure,
						"service_type": svc.Type,
						"backends":     strconv.Itoa(len(backends)),
						"node_ports":   nodePortString(svc.Ports),
						"rationale":    "NodePort / LoadBalancer 把服务暴露到集群外，此时后端 Pod 完全没有 NetworkPolicy 约束，攻击面最大。",
						"remediation":  "为后端 Pod 增加入向策略，仅放行必要的来源与端口；如无需外部访问请改用 ClusterIP。",
					},
					ObservedAt: k8sfinding.RFC3339(at),
				})
			}
		}
	}
	return status
}

func ingressPoliciesForPods(idx index, pods []PodInfo) []Policy {
	seen := make(map[string]bool)
	var out []Policy
	for _, pod := range pods {
		for _, p := range idx.ingressPoliciesFor(pod) {
			key := p.Namespace + "/" + p.Name
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, p)
		}
	}
	return out
}

func evalServicePort(status Status, svc ServiceInfo, port ServicePort, backends []PodInfo, covering []Policy, res k8sfinding.ResourceCitation, at time.Time) Status {
	target := port.TargetPort
	if target == "" {
		target = strconv.Itoa(int(port.Port))
	}
	protocol := defaultString(port.Protocol, "TCP")

	number, numeric := parsePort(target)
	resolvedName := ""
	if !numeric {
		resolvedName = target
	}

	status.Total++
	matchedPods := 0
	for _, pod := range backends {
		for _, cp := range pod.Ports {
			if !protocolMatches(cp.Protocol, protocol) {
				continue
			}
			if numeric && cp.ContainerPort == number {
				matchedPods++
				break
			}
			if !numeric && cp.Name == resolvedName {
				number = cp.ContainerPort
				matchedPods++
				break
			}
		}
	}
	if matchedPods == 0 {
		severity := SeverityInfo
		summary := "Service 端口的数值 targetPort 未在任何后端容器中声明"
		rationale := "Kubernetes 允许转发到未声明的容器端口，只要进程确实在监听；若进程未监听则连接会被拒绝，通常是端口写错。"
		if !numeric {
			severity = SeverityCritical
			summary = "Service 的命名 targetPort 在任何后端 Pod 上都无法解析，流量无处可去"
			rationale = "命名 targetPort 必须能在后端容器的 ports 中找到同名端口，否则 Endpoint 无法生成。"
		}
		status = status.withFinding(Finding{
			Code:     CodeServicePortUnmatched,
			Severity: severity,
			Summary:  summary,
			Resource: res,
			Details: map[string]string{
				"family":      FamilyReachability,
				"port":        strconv.Itoa(int(port.Port)),
				"target_port": target,
				"protocol":    protocol,
				"backends":    strconv.Itoa(len(backends)),
				"rationale":   rationale,
				"remediation": "核对容器 ports 声明与 Service 的 targetPort（命名端口需完全一致，包括大小写）。",
			},
			ObservedAt: k8sfinding.RFC3339(at),
		})
		return status
	}

	if len(covering) == 0 {
		return status // no ingress policy applies, nothing can block the port
	}
	status.Total++
	if portAllowed(covering, number, resolvedName, protocol) {
		return status
	}
	return status.withFinding(Finding{
		Code:     CodeServicePortBlocked,
		Severity: SeverityWarning,
		Summary:  "后端 Pod 已被入向策略约束，但没有任何策略放行该 Service 端口，流量会被丢弃",
		Resource: res,
		Details: map[string]string{
			"family":          FamilyReachability,
			"port":            strconv.Itoa(int(port.Port)),
			"target_port":     target,
			"resolved_port":   strconv.Itoa(int(number)),
			"protocol":        protocol,
			"policies":        policyNames(covering),
			"rationale":       "NetworkPolicy 是白名单叠加模型：只要存在选中该 Pod 的入向策略，未被任何规则显式放行的端口都会被丢弃。",
			"remediation":     "在放行策略的 ingress.ports 中补上该端口，或确认此端口本就不应对外提供服务。",
			"covering_policy": strconv.Itoa(len(covering)),
		},
		ObservedAt: k8sfinding.RFC3339(at),
	})
}

// portAllowed reports whether any covering policy has an ingress rule that
// permits the resolved port. NetworkPolicies are additive, so a single
// permitting rule is enough.
func portAllowed(policies []Policy, number int32, name, protocol string) bool {
	for _, p := range policies {
		for _, rule := range p.Ingress {
			if len(rule.Ports) == 0 {
				return true // rule permits every port
			}
			for _, pr := range rule.Ports {
				if !protocolMatches(pr.Protocol, protocol) {
					continue
				}
				if pr.Port == "" {
					return true
				}
				if n, ok := parsePort(pr.Port); ok {
					if number == n {
						return true
					}
					if pr.EndPort > 0 && number > n && number <= pr.EndPort {
						return true
					}
					continue
				}
				if name != "" && pr.Port == name {
					return true
				}
			}
		}
	}
	return false
}

// --- rollup helpers -------------------------------------------------------

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

// --- small helpers --------------------------------------------------------

func equalFold(a, b string) bool { return strings.EqualFold(a, b) }

func matchLabels(labels, selector map[string]string) bool {
	if len(selector) == 0 {
		return false
	}
	for k, v := range selector {
		if labels[k] != v {
			return false
		}
	}
	return true
}

func parsePort(v string) (int32, bool) {
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 || n > 65535 {
		return 0, false
	}
	return int32(n), true
}

// protocolMatches compares two protocol values, treating an empty value as the
// Kubernetes default of TCP.
func protocolMatches(a, b string) bool {
	return equalFold(defaultString(a, "TCP"), defaultString(b, "TCP"))
}

func defaultString(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func labelString(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+labels[k])
	}
	return strings.Join(parts, ",")
}

func portRuleString(ports []PortRule) string {
	if len(ports) == 0 {
		return "*"
	}
	parts := make([]string, 0, len(ports))
	for _, p := range ports {
		parts = append(parts, defaultString(p.Protocol, "TCP")+"/"+defaultString(p.Port, "*"))
	}
	return strings.Join(parts, ",")
}

func nodePortString(ports []ServicePort) string {
	parts := make([]string, 0, len(ports))
	for _, p := range ports {
		if p.NodePort > 0 {
			parts = append(parts, strconv.Itoa(int(p.NodePort)))
		}
	}
	return strings.Join(parts, ",")
}

func policyNames(policies []Policy) string {
	names := make([]string, 0, len(policies))
	for _, p := range policies {
		names = append(names, p.Name)
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}
