package cis

import (
	"strings"
	"time"

	k8sfinding "k8s-aiops.local/backend/internal/finding"
)

// Evaluate runs every compiled-in CIS control against the supplied read-only
// observation bundle and returns an aggregated Status. It is pure and
// read-only: it never contacts a cluster and never mutates anything.
//
// Only the domains that are supplied in in are checked; missing domains are
// skipped (not counted as pass or fail), which keeps the evaluator safe to run
// against partial observations.
func Evaluate(clusterID int64, in Inputs, observedAt time.Time) Status {
	status := Status{
		ClusterID:   clusterID,
		EvaluatedAt: observedAt.UTC(),
		BySeverity:  map[string]int{},
		ByFamily:    map[string]int{},
	}

	status = evalComponents(status, in.Components, observedAt)
	status = evalWorkloads(status, in.Workloads, observedAt)
	status = evalRBAC(status, in.Bindings, observedAt)
	status = evalNamespaces(status, in.Namespaces, observedAt)

	status.Passed = status.Total - status.Failed
	return status
}

func (s Status) withFinding(f k8sfinding.Finding) Status {
	s.Findings = append(s.Findings, f)
	s.Failed++
	s.BySeverity[f.Severity]++
	if s.ByFamily == nil {
		s.ByFamily = map[string]int{}
	}
	return s
}

func (s Status) counted(n int) Status {
	s.Total += n
	return s
}

// evalComponents evaluates every compiled-in component flag control against
// the supplied component configurations.
func evalComponents(status Status, comps []ComponentConfig, observedAt time.Time) Status {
	for _, comp := range comps {
		for _, c := range componentControls {
			if c.Component != comp.Component {
				continue
			}
			status = status.counted(1)
			val, present := comp.Flags[c.Flag]
			if flagFails(c, present, val) {
				status = status.withFinding(k8sfinding.Finding{
					Code:     c.ID,
					Severity: c.Severity,
					Summary:  c.Title,
					Resource: k8sfinding.ResourceCitation{
						Kind: comp.Component,
						Name: comp.Component,
					},
					Details: map[string]string{
						"family":       c.Family,
						"cis_level":    c.CISLevel,
						"flag":         c.Flag,
						"flag_present": boolStr(present),
						"flag_value":   val,
						"rationale":    c.Rationale,
						"remediation":  c.Remediation,
					},
					ObservedAt: observedAt.UTC().Format(time.RFC3339),
				})
			}
		}
	}
	return status
}

// flagFails reports whether a component control fails given the flag presence
// and value.
func flagFails(c ComponentControl, present bool, val string) bool {
	switch c.Kind {
	case FlagShouldBeFalse:
		return present && !strings.EqualFold(val, "false")
	case FlagMustBeSet:
		return !present
	case FlagMustBeAbsent:
		return present
	case FlagModeMustInclude:
		if !present {
			return true
		}
		tokens := splitList(val)
		for _, want := range c.Params.Contains {
			if !containsStr(tokens, want) {
				return true
			}
		}
		return false
	case FlagMustNotEqual:
		if !present {
			return false
		}
		return containsStr(c.Params.Disallow, val)
	case FlagEquals:
		if !present {
			return true
		}
		return !containsStr(c.Params.Allow, val)
	default:
		return false
	}
}

func splitList(v string) []string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func evalWorkloads(status Status, workloads []WorkloadSecurity, observedAt time.Time) Status {
	for _, w := range workloads {
		for _, c := range workloadChecks {
			for _, ct := range w.Containers {
				status = status.counted(1)
				if c.Violates(w, ct) {
					status = status.withFinding(k8sfinding.Finding{
						Code:     c.ID,
						Severity: c.Severity,
						Summary:  c.Title + " (" + ct.Name + ")",
						Resource: k8sfinding.ResourceCitation{
							Kind:      w.Kind,
							Namespace: w.Namespace,
							Name:      w.Name,
							UID:       w.UID,
						},
						Details: map[string]string{
							"family":      c.Family,
							"cis_level":   c.CISLevel,
							"container":   ct.Name,
							"finding":     c.Detail(w, ct),
							"rationale":   c.Rationale,
							"remediation": c.Remediation,
						},
						ObservedAt: observedAt.UTC().Format(time.RFC3339),
					})
				}
			}
		}
	}
	return status
}

func evalRBAC(status Status, bindings []RBACBinding, observedAt time.Time) Status {
	for _, b := range bindings {
		for _, c := range rbacChecks {
			status = status.counted(1)
			if c.Violates(b) {
				status = status.withFinding(k8sfinding.Finding{
					Code:     c.ID,
					Severity: c.Severity,
					Summary:  c.Title,
					Resource: k8sfinding.ResourceCitation{
						Kind:      b.Kind,
						Namespace: b.Namespace,
						Name:      b.Name,
						UID:       b.UID,
					},
					Details: map[string]string{
						"family":      c.Family,
						"cis_level":   c.CISLevel,
						"role":        b.RoleKind + "/" + b.RoleName,
						"finding":     c.Detail(b),
						"rationale":   c.Rationale,
						"remediation": c.Remediation,
					},
					ObservedAt: observedAt.UTC().Format(time.RFC3339),
				})
			}
		}
	}
	return status
}

func evalNamespaces(status Status, namespaces []NamespacePodSecurity, observedAt time.Time) Status {
	for _, ns := range namespaces {
		for _, c := range namespaceChecks {
			status = status.counted(1)
			if c.Violates(ns) {
				status = status.withFinding(k8sfinding.Finding{
					Code:     c.ID,
					Severity: c.Severity,
					Summary:  c.Title,
					Resource: k8sfinding.ResourceCitation{
						Kind: "Namespace",
						Name: ns.Name,
						UID:  ns.UID,
					},
					Details: map[string]string{
						"family":      c.Family,
						"cis_level":   c.CISLevel,
						"finding":     c.Detail(ns),
						"rationale":   c.Rationale,
						"remediation": c.Remediation,
					},
					ObservedAt: observedAt.UTC().Format(time.RFC3339),
				})
			}
		}
	}
	return status
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
