package gitopsdrift

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	k8sfinding "k8s-aiops.local/backend/internal/finding"
)

// maxReportedFields caps how many differing paths are echoed into a single
// finding's details to keep the payload bounded; the total count is always
// reported in field_count.
const maxReportedFields = 30

// Evaluate runs the read-only GitOps drift analysis over the supplied
// observation bundle and returns an aggregated Status. It is pure: it never
// contacts a Git provider, never re-applies a manifest and never mutates
// anything.
//
// For every resource that carries a last-applied-configuration annotation, the
// analyzer compares the applied manifest against the live spec/data and
// reports any field-level divergence as drift. Resources in a GitOps-managed
// namespace that carry no such annotation are reported as unmanaged, so the
// operator knows drift detection is blind for them.
func Evaluate(clusterID int64, in Inputs, observedAt time.Time) Status {
	at := observedAt.UTC()
	status := Status{
		ClusterID:      clusterID,
		EvaluatedAt:    at,
		ResourcesTotal: len(in.Resources),
		BySeverity:     map[string]int{},
		ByFamily:       map[string]int{},
		Findings:       []Finding{},
	}

	managed := make(map[string]bool, len(in.ManagedNamespaces))
	for _, ns := range in.ManagedNamespaces {
		managed[ns] = true
	}

	for _, r := range in.Resources {
		status.Total++

		if len(r.AppliedConfig) == 0 {
			// No last-applied-configuration: drift cannot be computed.
			if managed[r.Namespace] {
				status.UnmanagedResources++
				status = status.withFinding(Finding{
					Code:     CodeUnmanaged,
					Severity: SeverityInfo,
					Summary:  "资源 " + resourceLabel(r) + " 位于 GitOps 受管命名空间 " + r.Namespace + "，但未携带 last-applied-configuration，漂移无法被检测或对齐",
					Resource: resourceCitation(r),
					Details: map[string]string{
						"family":      FamilyGitOpsDrift,
						"manager":     r.Manager,
						"namespace":   r.Namespace,
						"rationale":   "没有 last-applied-configuration 注解，kubectl/GitOps 无法做三方合并，线下改动的字段既检测不到也无法被自动回滚。",
						"remediation": "用 kubectl apply 或 GitOps 工具重新纳管该资源，确保写入 last-applied-configuration 注解。",
					},
					ObservedAt: k8sfinding.RFC3339(at),
				})
			}
			continue
		}

		var applied map[string]any
		if err := json.Unmarshal(r.AppliedConfig, &applied); err != nil {
			// Unparseable annotation: skip rather than raise a false finding.
			continue
		}
		var live map[string]any
		if len(r.LiveBody) > 0 {
			if err := json.Unmarshal(r.LiveBody, &live); err != nil {
				live = map[string]any{}
			}
		}
		if live == nil {
			live = map[string]any{}
		}

		// Compare only the fields the user last applied (GitOps intent) against
		// the live object. Fields present only in the live object (added by
		// controllers / webhooks / operators) are not the user's intent and are
		// ignored, so they never produce a false drift.
		paths := diffPaths(bodyOf(applied), bodyOf(live), "")
		if len(paths) == 0 {
			continue
		}
		status.DriftedResources++
		reported := paths
		if len(reported) > maxReportedFields {
			reported = reported[:maxReportedFields]
		}
		status = status.withFinding(Finding{
			Code:     CodeDriftDetected,
			Severity: SeverityWarning,
			Summary:  "资源 " + resourceLabel(r) + " 已偏离 GitOps 上次应用的配置（" + strconv.Itoa(len(paths)) + " 处字段不一致）",
			Resource: resourceCitation(r),
			Details: map[string]string{
				"family":      FamilyGitOpsDrift,
				"manager":     r.Manager,
				"fields":      strings.Join(reported, ","),
				"field_count": strconv.Itoa(len(paths)),
				"rationale":   "实况对象与 last-applied-configuration 注解不一致，说明发生了带外变更（手动 kubectl edit/patch、控制器回填或配置错误），GitOps 下一次同步可能覆盖或冲突。",
				"remediation": "确认变更是否有意：有意的请重新 kubectl apply 纳管；无意的用 GitOps 重新同步回滚；临时豁免可加注解标记。",
			},
			ObservedAt: k8sfinding.RFC3339(at),
		})
	}

	status.Passed = status.Total - status.Failed
	sortFindings(status.Findings)
	return status
}

// --- helpers ---------------------------------------------------------------

func resourceLabel(r ManagedResource) string {
	if r.Namespace != "" {
		return r.Kind + " " + r.Namespace + "/" + r.Name
	}
	return r.Kind + " " + r.Name
}

func resourceCitation(r ManagedResource) k8sfinding.ResourceCitation {
	return k8sfinding.ResourceCitation{
		Kind:      r.Kind,
		Namespace: r.Namespace,
		Name:      r.Name,
		UID:       r.UID,
	}
}

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

func sortFindings(findings []Finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if ra, rb := severityRank[a.Severity], severityRank[b.Severity]; ra != rb {
			return ra < rb
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		if a.Resource.Name != b.Resource.Name {
			return a.Resource.Name < b.Resource.Name
		}
		return a.Resource.Namespace < b.Resource.Namespace
	})
}

// bodyOf extracts the comparable body of an applied/live object: its "spec"
// when present, otherwise its "data" (ConfigMap/Secret), otherwise the whole
// object with the server-managed envelope (metadata/status/apiVersion/kind)
// stripped. The applied annotation and the live body are produced the same way
// so the comparison is apples-to-apples.
func bodyOf(obj map[string]any) map[string]any {
	if spec, ok := obj["spec"].(map[string]any); ok {
		return spec
	}
	if data, ok := obj["data"].(map[string]any); ok {
		return data
	}
	out := make(map[string]any, len(obj))
	for k, v := range obj {
		if k == "metadata" || k == "status" || k == "apiVersion" || k == "kind" {
			continue
		}
		out[k] = v
	}
	return out
}

// diffPaths returns the dotted paths where the applied value (a) diverges from
// the live value (b). Only fields present in a are considered — fields present
// only in b are ignored (controller-managed, not user intent).
func diffPaths(a, b any, path string) []string {
	var out []string
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok {
			return append(out, path+": 类型不一致 (期望 object)")
		}
		keys := make([]string, 0, len(av))
		for k := range av {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			child := k
			if path != "" {
				child = path + "." + k
			}
			if bv2, ok := bv[k]; ok {
				out = append(out, diffPaths(av[k], bv2, child)...)
			} else {
				out = append(out, child+": 字段被移除")
			}
		}
		return out
	case []any:
		bv, ok := b.([]any)
		if !ok {
			return append(out, path+": 类型不一致 (期望 array)")
		}
		// Order-insensitive comparison when every element is an object, so a
		// reordered env list or volume set is not reported as drift.
		if len(av) > 0 && allMaps(av) && len(bv) > 0 && allMaps(bv) {
			aSet := canonicalSet(av)
			bSet := canonicalSet(bv)
			for canon, label := range aSet {
				if _, ok := bSet[canon]; !ok {
					out = append(out, path+"."+label+": 数组元素被移除")
				}
			}
			return out
		}
		n := len(av)
		if len(bv) < n {
			n = len(bv)
		}
		for i := 0; i < n; i++ {
			child := path + "[" + strconv.Itoa(i) + "]"
			out = append(out, diffPaths(av[i], bv[i], child)...)
		}
		if len(av) != len(bv) {
			out = append(out, path+": 数组长度不一致 ("+strconv.Itoa(len(av))+" vs "+strconv.Itoa(len(bv))+")")
		}
		return out
	default:
		if !equalScalar(a, b) {
			return append(out, path+": 值不一致")
		}
		return out
	}
}

func allMaps(arr []any) bool {
	for _, e := range arr {
		if _, ok := e.(map[string]any); !ok {
			return false
		}
	}
	return true
}

// canonicalSet maps a canonical JSON form of each object element to a human
// label (its most identifying scalar field, e.g. name/key/container).
func canonicalSet(arr []any) map[string]string {
	out := make(map[string]string, len(arr))
	for _, e := range arr {
		c := canonical(e)
		out[c] = identify(e)
	}
	return out
}

func identify(e any) string {
	if m, ok := e.(map[string]any); ok {
		for _, key := range []string{"name", "key", "container", "mountPath", "port", "path", "type", "host"} {
			if v, ok := m[key]; ok {
				if s, ok := v.(string); ok && s != "" {
					return key + "=" + s
				}
			}
		}
	}
	return ""
}

// canonical renders a value to a deterministic JSON string with sorted object
// keys and sorted object-array elements, so two structurally-equal values
// always produce the same string regardless of Go map ordering.
func canonical(v any) string {
	switch val := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var b strings.Builder
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(strconv.Quote(k))
			b.WriteByte(':')
			b.WriteString(canonical(val[k]))
		}
		b.WriteByte('}')
		return b.String()
	case []any:
		parts := make([]string, len(val))
		for i, e := range val {
			parts[i] = canonical(e)
		}
		sort.Strings(parts)
		return "[" + strings.Join(parts, ",") + "]"
	default:
		bs, _ := json.Marshal(val)
		return string(bs)
	}
}

func equalScalar(a, b any) bool {
	switch av := a.(type) {
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case bool:
		bv, ok := b.(bool)
		return ok && av == bv
	case float64:
		bv, ok := b.(float64)
		return ok && av == bv
	case nil:
		return b == nil
	default:
		return false
	}
}
