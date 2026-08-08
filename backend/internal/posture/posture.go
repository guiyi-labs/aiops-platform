// Package posture aggregates the read-only M61-M78 analyzers into a single
// cluster governance posture report. It turns the per-domain Status rollups
// into one view: a risk-sorted finding stream plus per-domain summaries, so
// the console can answer "how healthy is this cluster" at a glance (the
// polish roadmap W4 / M80 workstream).
//
// The package stays read-only (ADR 0004): it only collects observation
// bundles through optimization.Collector and runs pure analyzers. It never
// mutates cluster state.
package posture

import (
	"context"
	"sort"
	"time"

	"k8s-aiops.local/backend/internal/capacity"
	"k8s-aiops.local/backend/internal/cis"
	"k8s-aiops.local/backend/internal/deprecatedapi"
	"k8s-aiops.local/backend/internal/finding"
	"k8s-aiops.local/backend/internal/finops"
	"k8s-aiops.local/backend/internal/gitopsdrift"
	"k8s-aiops.local/backend/internal/hpa"
	"k8s-aiops.local/backend/internal/imagepolicy"
	"k8s-aiops.local/backend/internal/ingressposture"
	"k8s-aiops.local/backend/internal/netpolicy"
	"k8s-aiops.local/backend/internal/optimization"
	"k8s-aiops.local/backend/internal/pdb"
	"k8s-aiops.local/backend/internal/policy"
)

// Domain identifies one analyzer family in the aggregated report.
type Domain string

const (
	DomainCIS           Domain = "cis"
	DomainFinOps        Domain = "finops"
	DomainDeprecatedAPI Domain = "deprecated_api"
	DomainNetwork       Domain = "network"
	DomainImage         Domain = "image"
	DomainGitOps        Domain = "gitops"
	DomainCapacity      Domain = "capacity"
	DomainPolicy        Domain = "policy"
	DomainHPA           Domain = "hpa"
	DomainPDB           Domain = "pdb"
	DomainIngress       Domain = "ingress"
)

// DomainStatus is the per-domain rollup shown in the posture overview.
type DomainStatus struct {
	Domain     Domain         `json:"domain"`
	Total      int            `json:"total"`
	Failed     int            `json:"failed"`
	Passed     int            `json:"passed"`
	BySeverity map[string]int `json:"by_severity"`
}

// PostureFinding is a finding with its originating domain attached.
type PostureFinding struct {
	Domain     Domain                   `json:"domain"`
	Severity   string                   `json:"severity"`
	Code       string                   `json:"code"`
	Summary    string                   `json:"summary"`
	Resource   finding.ResourceCitation `json:"resource"`
	Details    map[string]string        `json:"details,omitempty"`
	ObservedAt string                   `json:"observed_at"`
}

// Report is the aggregated cluster governance posture.
type Report struct {
	ClusterID   int64          `json:"cluster_id"`
	EvaluatedAt time.Time      `json:"evaluated_at"`
	Domains     []DomainStatus `json:"domains"`
	// Findings is the flattened, risk-sorted finding stream (critical first).
	Findings []PostureFinding `json:"findings"`
	// BySeverity rolls all domains up into one histogram.
	BySeverity map[string]int `json:"by_severity"`
	// Summary counts for the headline.
	TotalChecks  int `json:"total_checks"`
	FailedChecks int `json:"failed_checks"`
	PassedChecks int `json:"passed_checks"`
}

// Evaluator runs the full posture evaluation for one cluster.
type Evaluator struct {
	collector     *optimization.Collector
	defaultRate   finops.CostRate
	targetVersion string
}

// Option customizes the Evaluator.
type Option func(*Evaluator)

// WithDefaultCostRate overrides the FinOps cost rate used for the aggregate.
func WithDefaultCostRate(rate finops.CostRate) Option {
	return func(e *Evaluator) { e.defaultRate = rate }
}

// WithTargetVersion supplies the Kubernetes version used by the deprecated-API
// check (e.g. "1.31"). When empty, the deprecated-API domain reports zero
// checks (the analyzer needs an explicit target to classify objects).
func WithTargetVersion(version string) Option {
	return func(e *Evaluator) { e.targetVersion = version }
}

// New builds an Evaluator over a collector. The collector is required: the
// aggregate always auto-collects from the live cluster (no manual bundles).
func New(collector *optimization.Collector, opts ...Option) *Evaluator {
	e := &Evaluator{
		collector:   collector,
		defaultRate: finops.DefaultCostRate(),
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Evaluate collects every observation bundle and runs every analyzer,
// returning the aggregated posture report. Per-domain collection failures are
// surfaced as domain errors (the report is still produced with whatever
// domains succeeded), so one broken endpoint never blanks the whole console.
func (e *Evaluator) Evaluate(ctx context.Context, clusterID int64, observedAt time.Time) (*Report, error) {
	report := &Report{
		ClusterID:   clusterID,
		EvaluatedAt: observedAt.UTC(),
		BySeverity:  map[string]int{},
	}
	if e.collector == nil {
		return report, nil
	}

	type job struct {
		domain Domain
		run    func() ([]PostureFinding, DomainStatus, error)
	}
	jobs := []job{
		{DomainCIS, func() ([]PostureFinding, DomainStatus, error) {
			in, err := e.collector.CollectCIS(ctx, clusterID)
			if err != nil {
				return nil, DomainStatus{}, err
			}
			st := cis.Evaluate(clusterID, in, observedAt)
			return findingsFrom(st.Findings, DomainCIS), statusFrom(DomainCIS, st.Total, st.Failed, st.BySeverity), nil
		}},
		{DomainFinOps, func() ([]PostureFinding, DomainStatus, error) {
			in, err := e.collector.CollectFinOps(ctx, clusterID)
			if err != nil {
				return nil, DomainStatus{}, err
			}
			sum := finops.Recommend(clusterID, in, e.defaultRate)
			var fs []PostureFinding
			for _, r := range sum.Recommendations {
				fs = append(fs, PostureFinding{
					Domain:   DomainFinOps,
					Severity: r.Severity,
					Code:     r.Code,
					Summary:  r.Rationale,
					Resource: finding.ResourceCitation{
						Kind:      r.WorkloadKind,
						Namespace: r.Namespace,
						Name:      r.WorkloadName,
					},
					ObservedAt: finding.RFC3339(observedAt),
				})
			}
			bySev := map[string]int{}
			for _, f := range fs {
				bySev[f.Severity]++
			}
			return fs, DomainStatus{Domain: DomainFinOps, Total: len(fs), Failed: len(fs), BySeverity: bySev}, nil
		}},
		{DomainDeprecatedAPI, func() ([]PostureFinding, DomainStatus, error) {
			if e.targetVersion == "" {
				return nil, DomainStatus{Domain: DomainDeprecatedAPI}, nil
			}
			in, err := e.collector.CollectDeprecatedAPI(ctx, clusterID)
			if err != nil {
				return nil, DomainStatus{}, err
			}
			st := deprecatedapi.Check(clusterID, e.targetVersion, in, observedAt)
			fs := findingsFrom(st.Findings, DomainDeprecatedAPI)
			bySev := map[string]int{}
			for _, f := range fs {
				bySev[f.Severity]++
			}
			return fs, statusFrom(DomainDeprecatedAPI, st.Total, len(fs), bySev), nil
		}},
		{DomainNetwork, func() ([]PostureFinding, DomainStatus, error) {
			in, err := e.collector.CollectNetPolicy(ctx, clusterID)
			if err != nil {
				return nil, DomainStatus{}, err
			}
			st := netpolicy.Evaluate(clusterID, in, observedAt)
			return findingsFrom(st.Findings, DomainNetwork), statusFrom(DomainNetwork, st.Total, st.Failed, st.BySeverity), nil
		}},
		{DomainImage, func() ([]PostureFinding, DomainStatus, error) {
			in, err := e.collector.CollectImagePolicy(ctx, clusterID)
			if err != nil {
				return nil, DomainStatus{}, err
			}
			st := imagepolicy.Evaluate(clusterID, in, observedAt)
			return findingsFrom(st.Findings, DomainImage), statusFrom(DomainImage, st.Total, st.Failed, st.BySeverity), nil
		}},
		{DomainGitOps, func() ([]PostureFinding, DomainStatus, error) {
			in, err := e.collector.CollectGitOpsDrift(ctx, clusterID)
			if err != nil {
				return nil, DomainStatus{}, err
			}
			st := gitopsdrift.Evaluate(clusterID, in, observedAt)
			return findingsFrom(st.Findings, DomainGitOps), statusFrom(DomainGitOps, st.Total, st.Failed, st.BySeverity), nil
		}},
		{DomainCapacity, func() ([]PostureFinding, DomainStatus, error) {
			in, err := e.collector.CollectCapacity(ctx, clusterID)
			if err != nil {
				return nil, DomainStatus{}, err
			}
			st := capacity.Evaluate(clusterID, in, observedAt)
			return findingsFrom(st.Findings, DomainCapacity), statusFrom(DomainCapacity, st.Total, st.Failed, st.BySeverity), nil
		}},
		{DomainPolicy, func() ([]PostureFinding, DomainStatus, error) {
			in, err := e.collector.CollectPolicy(ctx, clusterID)
			if err != nil {
				return nil, DomainStatus{}, err
			}
			st := policy.Evaluate(clusterID, in, observedAt)
			return findingsFrom(st.Findings, DomainPolicy), statusFrom(DomainPolicy, st.Total, st.Failed, st.BySeverity), nil
		}},
		{DomainHPA, func() ([]PostureFinding, DomainStatus, error) {
			in, err := e.collector.CollectHPA(ctx, clusterID)
			if err != nil {
				return nil, DomainStatus{}, err
			}
			st := hpa.Evaluate(clusterID, in, observedAt)
			return findingsFrom(st.Findings, DomainHPA), statusFrom(DomainHPA, st.Total, st.Failed, st.BySeverity), nil
		}},
		{DomainPDB, func() ([]PostureFinding, DomainStatus, error) {
			in, err := e.collector.CollectPDB(ctx, clusterID)
			if err != nil {
				return nil, DomainStatus{}, err
			}
			st := pdb.Evaluate(clusterID, in, observedAt)
			return findingsFrom(st.Findings, DomainPDB), statusFrom(DomainPDB, st.Total, st.Failed, st.BySeverity), nil
		}},
		{DomainIngress, func() ([]PostureFinding, DomainStatus, error) {
			in, err := e.collector.CollectIngress(ctx, clusterID)
			if err != nil {
				return nil, DomainStatus{}, err
			}
			st := ingressposture.Evaluate(clusterID, in, observedAt)
			return findingsFrom(st.Findings, DomainIngress), statusFrom(DomainIngress, st.Total, st.Failed, st.BySeverity), nil
		}},
	}

	for _, j := range jobs {
		fs, ds, err := j.run()
		if err != nil {
			// Surface the domain with a failed=0 state and continue: a single
			// collector error (e.g. a temporarily unavailable endpoint) must
			// not blank the whole posture.
			report.Domains = append(report.Domains, DomainStatus{Domain: j.domain})
			continue
		}
		report.Domains = append(report.Domains, ds)
		report.Findings = append(report.Findings, fs...)
		report.TotalChecks += ds.Total
		report.FailedChecks += ds.Failed
		report.PassedChecks += ds.Passed
		for sev, n := range ds.BySeverity {
			report.BySeverity[sev] += n
		}
	}

	sort.SliceStable(report.Findings, func(i, j int) bool {
		return severityRank(report.Findings[i].Severity) < severityRank(report.Findings[j].Severity)
	})
	sort.Slice(report.Domains, func(i, j int) bool { return report.Domains[i].Domain < report.Domains[j].Domain })
	return report, nil
}

func severityRank(sev string) int {
	switch sev {
	case finding.SeverityCritical:
		return 0
	case finding.SeverityWarning:
		return 1
	default:
		return 2
	}
}

func statusFrom(domain Domain, total, failed int, bySeverity map[string]int) DomainStatus {
	ds := DomainStatus{Domain: domain, Total: total, Failed: failed, BySeverity: bySeverity}
	if ds.BySeverity == nil {
		ds.BySeverity = map[string]int{}
	}
	ds.Passed = total - failed
	return ds
}

func findingsFrom(findings []finding.Finding, domain Domain) []PostureFinding {
	out := make([]PostureFinding, 0, len(findings))
	for _, f := range findings {
		out = append(out, PostureFinding{
			Domain:     domain,
			Severity:   f.Severity,
			Code:       f.Code,
			Summary:    f.Summary,
			Resource:   f.Resource,
			Details:    f.Details,
			ObservedAt: f.ObservedAt,
		})
	}
	return out
}
