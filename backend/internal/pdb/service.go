package pdb

import (
	"sort"
	"strconv"
	"strings"
	"time"

	k8sfinding "k8s-aiops.local/backend/internal/finding"
)

// budgetValue parses the minAvailable / maxUnavailable value: an integer, or
// a percentage string like "50%". Returns the value and whether parsing
// succeeded. Unparseable values yield (0, false).
func budgetValue(raw string) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	if strings.HasSuffix(raw, "%") {
		pct := strings.TrimSuffix(raw, "%")
		n, err := strconv.Atoi(strings.TrimSpace(pct))
		if err != nil || n < 0 {
			return 0, false
		}
		return n, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// Evaluate runs the read-only PDB posture analysis over the supplied
// observation bundle and returns an aggregated Status. It is pure: it never
// contacts a cluster and never mutates anything.
func Evaluate(clusterID int64, in Inputs, observedAt time.Time) Status {
	at := observedAt.UTC()
	status := Status{
		ClusterID:      clusterID,
		EvaluatedAt:    at,
		WorkloadsTotal: len(in.Workloads),
		PDBsTotal:      len(in.PDBs),
		BySeverity:     map[string]int{},
		ByFamily:       map[string]int{},
		Findings:       []Finding{},
	}

	// Index PDBs by namespace so workload coverage can be checked without a
	// quadratic scan, then evaluate each PDB's own health.
	pdbsByNS := map[string][]PDBInfo{}
	for i := range in.PDBs {
		p := in.PDBs[i]
		pdbsByNS[p.Namespace] = append(pdbsByNS[p.Namespace], p)
		status = evaluatePDB(status, p, at)
	}

	// Workload coverage: a replicable workload with replicas > 1 should be
	// selected by at least one PDB in its namespace.
	checks := len(in.Workloads)
	for i := range in.Workloads {
		w := in.Workloads[i]
		if w.Replicas <= 1 {
			continue
		}
		if !coveredByAny(w, pdbsByNS[w.Namespace]) {
			status.UnprotectedWorkloads++
			status = status.withFinding(findingWorkload(w, CodeWorkloadUnprotected, SeverityWarning, FamilyPDB,
				"可多副本工作负载没有 PodDisruptionBudget 保护，节点排空/升级时可能整体不可用",
				"创建 PDB（如 minAvailable: 1 或 maxUnavailable: 1），并确认 selector 匹配工作负载的 Pod 标签", at))
		}
	}
	// Workloads with replicas <= 1 still count toward Total as a "pass"
	// (they are not expected to have a PDB).
	status.Total = checks + status.PDBsTotal*2
	status.Failed = len(status.Findings)
	status.Passed = status.Total - status.Failed
	if status.Passed < 0 {
		status.Passed = 0
	}
	sortFindings(status.Findings)
	return status
}

// evaluatePDB checks one PDB: budget achievability and whether disruptions
// are currently blocked. Both are only meaningful when status.ExpectedPods
// is known (> 0).
func evaluatePDB(status Status, p PDBInfo, at time.Time) Status {
	// Selector-no-match check: an empty selector map cannot select anything.
	if len(p.SelectorLabels) == 0 {
		status = status.withFinding(findingPDB(p, CodeSelectorNoMatches, SeverityInfo, FamilyPDB,
			"PDB selector 为空，不会匹配任何 Pod，实际未提供保护",
			"为 PDB 设置 spec.selector.matchLabels，指向工作负载的 Pod 标签", at))
	}
	if p.ExpectedPods <= 0 {
		return status
	}

	// Budget achievable: when an explicit integer minAvailable is at least
	// the expected pod count, no disruption is ever allowed. Percentage
	// minAvailable and maxUnavailable forms structurally allow at least one
	// disruption unless they resolve to 100% / 0, so they are not flagged.
	if v, ok := budgetValue(p.MinAvailable); ok && p.MinAvailable != "" && !strings.HasSuffix(p.MinAvailable, "%") {
		if v >= int(p.ExpectedPods) {
			status = status.withFinding(findingPDB(p, CodeBudgetUnachievable, SeverityWarning, FamilyPDB,
				"PDB 的 minAvailable 不小于期望副本数，任何驱逐都会被拒绝，排空节点将被阻塞",
				"降低 minAvailable（如 minAvailable: 1）或改为 maxUnavailable: 1", at))
		}
	}

	// Disruptions blocked: status.disruptionsAllowed == 0 means the budget
	// currently allows zero evictions.
	if p.DisruptionsAllowed <= 0 {
		status = status.withFinding(findingPDB(p, CodeDisruptionsBlocked, SeverityWarning, FamilyPDB,
			"PDB 当前允许 0 次驱逐（disruptionsAllowed=0），节点排空/维护会被阻塞",
			"等待工作负载扩容出富余副本，或临时调整 PDB 预算后再执行维护", at))
	}
	return status
}

// coveredByAny reports whether the workload's labels match at least one PDB
// selector in the namespace (subset match: every PDB selector label must be
// present on the workload). When the workload has no label set the bundle is
// incomplete, and the analyzer errs towards "covered" so a partial
// observation never raises a false alarm.
func coveredByAny(w WorkloadRef, pdbs []PDBInfo) bool {
	for i := range pdbs {
		p := pdbs[i]
		if len(p.SelectorLabels) == 0 {
			continue
		}
		if w.Labels == nil {
			return true
		}
		if matchLabels(w.Labels, p.SelectorLabels) {
			return true
		}
	}
	return false
}

// matchLabels reports whether every key/value pair in selector is present in
// labels (the subset match Kubernetes uses).
func matchLabels(labels, selector map[string]string) bool {
	for k, v := range selector {
		if labels[k] != v {
			return false
		}
	}
	return true
}

// findingWorkload builds a finding citing the workload.
func findingWorkload(w WorkloadRef, code, severity, family, summary, remediation string, at time.Time) Finding {
	return Finding{
		Code:     code,
		Severity: severity,
		Summary:  summary,
		Resource: k8sfinding.ResourceCitation{
			Kind:      w.Kind,
			Namespace: w.Namespace,
			Name:      w.Name,
			UID:       w.UID,
		},
		Details: map[string]string{
			"family":      family,
			"rationale":   summary,
			"remediation": remediation,
			"replicas":    itoa(int(w.Replicas)),
		},
		ObservedAt: k8sfinding.RFC3339(at),
	}
}

// findingPDB builds a finding citing the PDB.
func findingPDB(p PDBInfo, code, severity, family, summary, remediation string, at time.Time) Finding {
	return Finding{
		Code:     code,
		Severity: severity,
		Summary:  summary,
		Resource: k8sfinding.ResourceCitation{
			Kind:      "PodDisruptionBudget",
			Namespace: p.Namespace,
			Name:      p.Name,
			UID:       p.UID,
		},
		Details: map[string]string{
			"family":      family,
			"rationale":   summary,
			"remediation": remediation,
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
