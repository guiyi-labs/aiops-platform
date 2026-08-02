package hpa

import (
	"sort"
	"strconv"
	"time"

	k8sfinding "k8s-aiops.local/backend/internal/finding"
)

// Evaluate runs the read-only HPA posture analysis over the supplied
// observation bundle and returns an aggregated Status. It is pure: it never
// contacts a cluster and never mutates anything.
//
// Each HPA contributes up to three checks: target-metric presence, current
// replicas vs max, and (when utilization is supplied) target compliance.
func Evaluate(clusterID int64, in Inputs, observedAt time.Time) Status {
	at := observedAt.UTC()
	status := Status{
		ClusterID:   clusterID,
		EvaluatedAt: at,
		HPAsTotal:   len(in.HPAs),
		BySeverity:  map[string]int{},
		ByFamily:    map[string]int{},
		Findings:    []Finding{},
	}

	checks := 0
	for i := range in.HPAs {
		h := in.HPAs[i]
		checks += 2 // target presence + max-replica headroom
		if h.TargetMetric == "" {
			status = status.withFinding(finding(h, CodeNoTarget, SeverityWarning, FamilyHPA,
				"HPA 未声明扩缩容目标指标，回退到 Kubernetes 默认（80% CPU 隐式目标）",
				"在 spec.metrics 中显式声明目标（如 type: Resource, resource: cpu, targetAverageUtilization: 80），避免隐式默认", at))
		}
		if h.CurrentReplicas >= h.MaxReplicas {
			status.AtMaxReplicasCount++
			status = status.withFinding(finding(h, CodeAtMaxReplicas, SeverityWarning, FamilyHPA,
				"当前副本数已到达 maxReplicas 上限，负载增长时无法继续扩容",
				"评估是否需要提高 maxReplicas；若长期触顶需配合容量预测（M70）判断是否真的需要更多资源", at))
		}
		if h.MaxReplicas <= 2 {
			checks++
			status = status.withFinding(finding(h, CodeMaxReplicasLow, SeverityInfo, FamilyHPA,
				"maxReplicas 过小（≤2），几乎没有扩缩容空间",
				"为有波动的工作负载设置更大的 maxReplicas（如 5-10）", at))
		}
		if h.CurrentUtilizationPct != nil {
			checks++
			util := *h.CurrentUtilizationPct
			target := h.TargetValue
			if h.TargetMetric == "" {
				target = 80 // Kubernetes default for cpu target when unset
			}
			if h.TargetMetric != "" && h.TargetMetric != "cpu" && h.TargetMetric != "memory" {
				// Utilization percentage only applies to resource targets;
				// for pods/custom metrics the current utilization field is
				// not a comparable percentage, so skip.
				checks--
			} else if target > 0 && util > int32(target) {
				status.OverTargetCount++
				status = status.withFinding(finding(h, CodeOverTarget, SeverityWarning, FamilyHPA,
					"当前利用率高于目标，HPA 正在持续扩容（或已触顶）",
					"确认 maxReplicas 充足；若利用率长期超过目标，考虑提高目标或扩容节点", at))
			} else if target > 0 && util*2 < int32(target) {
				status = status.withFinding(finding(h, CodeUnderTarget, SeverityInfo, FamilyHPA,
					"当前利用率远低于目标（不足一半），副本数可能过度配置",
					"考虑降低 minReplicas 或缩小副本规模（结合 FinOps 分析）", at))
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

// finding builds one canonical Finding from an HPA.
func finding(h HPAInput, code, severity, family, summary, remediation string, at time.Time) Finding {
	return Finding{
		Code:     code,
		Severity: severity,
		Summary:  summary,
		Resource: k8sfinding.ResourceCitation{
			Kind:      "HorizontalPodAutoscaler",
			Namespace: h.Namespace,
			Name:      h.Name,
			UID:       h.UID,
		},
		Details: map[string]string{
			"family":       family,
			"rationale":    summary,
			"remediation":  remediation,
			"max_replicas": itoa(int(h.MaxReplicas)),
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

func itoa(n int) string { return strconv.Itoa(n) }
