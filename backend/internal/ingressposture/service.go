package ingressposture

import (
	"sort"
	"strings"
	"time"

	k8sfinding "k8s-aiops.local/backend/internal/finding"
)

// Evaluate runs the read-only Ingress exposure audit over the supplied
// observation bundle and returns an aggregated Status. It is pure: it never
// contacts a cluster and never mutates anything.
func Evaluate(clusterID int64, in Inputs, observedAt time.Time) Status {
	at := observedAt.UTC()
	status := Status{
		ClusterID:      clusterID,
		EvaluatedAt:    at,
		IngressesTotal: len(in.Ingresses),
		BySeverity:     map[string]int{},
		ByFamily:       map[string]int{},
		Findings:       []Finding{},
	}

	// Index existing Services by (namespace, name).
	svcSet := map[string]bool{}
	for _, s := range in.Services {
		svcSet[s.Namespace+"/"+s.Name] = true
	}

	checks := 0
	for i := range in.Ingresses {
		ing := in.Ingresses[i]
		hasHostRules := len(ing.Hosts) > 0

		// TLS check: host rules without a TLS block.
		checks++
		if hasHostRules && !ing.HasTLS {
			status.NoTLSCount++
			status = status.withFinding(findingIngress(ing, CodeNoTLS, SeverityWarning, FamilyIngress,
				"Ingress 定义了 host 规则但未配置 TLS，流量以明文 HTTP 对外暴露",
				"为每个 host 配置 spec.tls（secretName 指向含证书的 Secret），并考虑强制跳转 HTTPS", at))
		}

		// Wildcard host check.
		checks++
		for _, host := range ing.Hosts {
			if strings.HasPrefix(host, "*.") {
				status = status.withFinding(findingIngress(ing, CodeWildcardHost, SeverityInfo, FamilyIngress,
					"Ingress 使用了通配符 host（"+host+"），暴露范围覆盖该域下全部子域",
					"如非必要，使用明确的 host 名称以缩小暴露面", at))
				break
			}
		}

		// Ingress class check.
		checks++
		if ing.IngressClassName == "" {
			status = status.withFinding(findingIngress(ing, CodeNoIngressClass, SeverityInfo, FamilyIngress,
				"Ingress 未指定 ingressClassName，实际生效的控制器依赖集群默认设置",
				"在 spec.ingressClassName 中显式指定控制器（如 nginx），避免行为不确定", at))
		}

		// Backend resolution check.
		seen := map[string]bool{}
		for _, b := range ing.Backends {
			key := b.Namespace + "/" + b.Name
			if seen[key] {
				continue
			}
			seen[key] = true
			checks++
			if !svcSet[key] {
				status.DeadBackendCount++
				status = status.withFinding(findingBackend(ing, b, CodeBackendServiceMissing, SeverityWarning, FamilyIngress,
					"Ingress 后端引用的 Service 不存在（"+b.Namespace+"/"+b.Name+"），该路由当前不可用",
					"创建对应的 Service，或修正 Ingress 的 backend 引用", at))
			}
		}
	}

	status.Total = checks
	status.Failed = len(status.Findings)
	status.Passed = status.Total - status.Failed
	if status.Passed < 0 {
		status.Passed = 0
	}
	sortFindings(status.Findings)
	return status
}

// findingIngress builds a finding citing the Ingress.
func findingIngress(ing IngressInfo, code, severity, family, summary, remediation string, at time.Time) Finding {
	return Finding{
		Code:     code,
		Severity: severity,
		Summary:  summary,
		Resource: k8sfinding.ResourceCitation{
			Kind:      "Ingress",
			Namespace: ing.Namespace,
			Name:      ing.Name,
			UID:       ing.UID,
		},
		Details: map[string]string{
			"family":      family,
			"rationale":   summary,
			"remediation": remediation,
		},
		ObservedAt: k8sfinding.RFC3339(at),
	}
}

// findingBackend builds a finding citing the Ingress with the missing backend
// recorded in details.
func findingBackend(ing IngressInfo, backend ServiceRef, code, severity, family, summary, remediation string, at time.Time) Finding {
	return Finding{
		Code:     code,
		Severity: severity,
		Summary:  summary,
		Resource: k8sfinding.ResourceCitation{
			Kind:      "Ingress",
			Namespace: ing.Namespace,
			Name:      ing.Name,
			UID:       ing.UID,
		},
		Details: map[string]string{
			"family":          family,
			"rationale":       summary,
			"remediation":     remediation,
			"backend_service": backend.Namespace + "/" + backend.Name,
		},
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
