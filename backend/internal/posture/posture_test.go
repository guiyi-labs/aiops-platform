package posture

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"k8s-aiops.local/backend/internal/finding"
	"k8s-aiops.local/backend/internal/finops"
	"k8s-aiops.local/backend/internal/optimization"
)

// fakeLister returns canned list items per API path; an unknown path yields no
// items (no error), mirroring the production collector's tolerance of
// uninstalled resources.
type fakeLister struct {
	data map[string][]json.RawMessage
}

func (f *fakeLister) List(_ context.Context, _ int64, path string) ([]json.RawMessage, error) {
	if items, ok := f.data[path]; ok {
		return items, nil
	}
	return nil, nil
}

func raw(s string) json.RawMessage { return json.RawMessage(s) }

// postureLister returns the minimal resources every collector needs without
// producing findings: an empty cluster (no pods, no workloads, no bindings).
func emptyLister() *fakeLister {
	return &fakeLister{data: map[string][]json.RawMessage{
		"/api/v1/pods":                                           {},
		"/api/v1/namespaces":                                     {},
		"/api/v1/nodes":                                          {},
		"/api/v1/services":                                       {},
		"/api/v1/endpoints":                                      {},
		"/api/v1/endpointslices":                                 {},
		"/apis/apps/v1/deployments":                              {},
		"/apis/apps/v1/statefulsets":                             {},
		"/apis/apps/v1/daemonsets":                               {},
		"/apis/apps/v1/replicasets":                              {},
		"/apis/batch/v1/jobs":                                    {},
		"/apis/batch/v1/cronjobs":                                {},
		"/apis/networking.k8s.io/v1/networkpolicies":             {},
		"/apis/networking.k8s.io/v1/ingresses":                   {},
		"/apis/rbac.authorization.k8s.io/v1/clusterrolebindings": {},
		"/apis/rbac.authorization.k8s.io/v1/clusterroles":        {},
		"/apis/rbac.authorization.k8s.io/v1/rolebindings":        {},
		"/apis/rbac.authorization.k8s.io/v1/roles":               {},
		"/apis/autoscaling/v2/horizontalpodautoscalers":          {},
		"/apis/policy/v1/poddisruptionbudgets":                   {},
	}}
}

func TestEvaluate_ProducesAllDomainsAndEmptyReport(t *testing.T) {
	col := optimization.NewCollector(emptyLister(), nil, nil)
	e := New(col, WithTargetVersion("1.31"))
	rep, err := e.Evaluate(context.Background(), 7, time.Now())
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if rep.ClusterID != 7 {
		t.Errorf("cluster_id = %d, want 7", rep.ClusterID)
	}
	if len(rep.Domains) != 11 {
		t.Errorf("domains = %d (%+v), want 11", len(rep.Domains), rep.Domains)
	}
	// Empty cluster: zero findings, but static checks (e.g. capacity node
	// baseline) may still be counted; every domain must be present.
	if len(rep.Findings) != 0 {
		t.Errorf("findings = %d, want 0 for empty cluster", len(rep.Findings))
	}
	if rep.FailedChecks != 0 {
		t.Errorf("failed_checks = %d, want 0 for empty cluster", rep.FailedChecks)
	}
	for _, d := range rep.Domains {
		if d.Domain == DomainDeprecatedAPI && d.Total != 0 {
			t.Errorf("deprecated-api should be skipped without target; got %+v", d)
		}
	}
}

func TestEvaluate_RiskSortsFindingsCriticalFirst(t *testing.T) {
	lister := &fakeLister{data: map[string][]json.RawMessage{
		// CIS: one privileged pod → critical finding.
		"/api/v1/pods": {raw(`{
			"apiVersion":"v1","kind":"Pod",
			"metadata":{"namespace":"dev","name":"priv","uid":"u1"},
			"spec":{"containers":[{"name":"c1","securityContext":{"privileged":true}}]}}`)},
		// Image policy: a mutable tag → warning finding.
		"/apis/apps/v1/deployments": {raw(`{
			"apiVersion":"apps/v1","kind":"Deployment",
			"metadata":{"namespace":"dev","name":"web"},
			"spec":{"template":{"spec":{"containers":[{"name":"c1","image":"nginx:latest"}]}}}}`)},
		"/api/v1/namespaces":     {},
		"/api/v1/nodes":          {},
		"/api/v1/services":       {},
		"/api/v1/endpoints":      {},
		"/api/v1/endpointslices": {},
		"/apis/rbac.authorization.k8s.io/v1/clusterrolebindings": {},
		"/apis/rbac.authorization.k8s.io/v1/clusterroles":        {},
		"/apis/rbac.authorization.k8s.io/v1/rolebindings":        {},
		"/apis/rbac.authorization.k8s.io/v1/roles":               {},
		"/apis/networking.k8s.io/v1/networkpolicies":             {},
		"/apis/networking.k8s.io/v1/ingresses":                   {},
		"/apis/autoscaling/v2/horizontalpodautoscalers":          {},
		"/apis/policy/v1/poddisruptionbudgets":                   {},
	}}
	col := optimization.NewCollector(lister, nil, nil)
	e := New(col, WithTargetVersion("1.31"))
	rep, err := e.Evaluate(context.Background(), 7, time.Now())
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if len(rep.Findings) < 2 {
		t.Fatalf("findings = %d, want >= 2", len(rep.Findings))
	}
	// The first finding must be critical.
	if rep.Findings[0].Severity != finding.SeverityCritical {
		t.Errorf("first finding severity = %q, want critical; all=%+v", rep.Findings[0].Severity, rep.Findings)
	}
}

func TestEvaluateNilCollectorReturnsEmptyReport(t *testing.T) {
	e := New(nil, WithTargetVersion("1.31"))
	rep, err := e.Evaluate(context.Background(), 7, time.Now())
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if len(rep.Domains) != 0 || len(rep.Findings) != 0 {
		t.Errorf("expected empty domains/findings for nil collector, got %d domains %d findings", len(rep.Domains), len(rep.Findings))
	}
}

func TestSeverityRank(t *testing.T) {
	if severityRank(finding.SeverityCritical) >= severityRank(finding.SeverityWarning) {
		t.Error("critical must rank before warning")
	}
	if severityRank(finding.SeverityWarning) >= severityRank(finding.SeverityInfo) {
		t.Error("warning must rank before info")
	}
}

func TestWithDefaultCostRateOverridesFinOps(t *testing.T) {
	e := New(nil)
	if e.defaultRate.PerCoreMonth <= 0 {
		t.Errorf("expected non-zero default cost rate, got %+v", e.defaultRate)
	}
	rate := finops.CostRate{PerCoreMonth: 9.99, PerGBMonth: 0.1}
	e2 := New(nil, WithDefaultCostRate(rate))
	if e2.defaultRate.PerCoreMonth != 9.99 {
		t.Errorf("cost rate not overridden: %+v", e2.defaultRate)
	}
}
