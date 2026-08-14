package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"k8s-aiops.local/backend/internal/alert"
	"k8s-aiops.local/backend/internal/correlation"
	"k8s-aiops.local/backend/internal/diagnosis"
	"k8s-aiops.local/backend/internal/incident"
	"k8s-aiops.local/backend/internal/inspection"
	"k8s-aiops.local/backend/internal/signal"
)

type fakeDiagnosisReader struct {
	records map[int64]diagnosis.Record
	err     error
}

func (f *fakeDiagnosisReader) Get(_ context.Context, id int64) (diagnosis.Record, error) {
	if f.err != nil {
		return diagnosis.Record{}, f.err
	}
	rec, ok := f.records[id]
	if !ok {
		return diagnosis.Record{}, diagnosis.ErrRecordNotFound
	}
	return rec, nil
}

type fakeAlertResolver struct {
	instances map[int64]alert.Instance
	err       error
}

func (f *fakeAlertResolver) Get(_ context.Context, _ int64, id int64) (alert.Instance, error) {
	if f.err != nil {
		return alert.Instance{}, f.err
	}
	inst, ok := f.instances[id]
	if !ok {
		return alert.Instance{}, alert.ErrAlertNotFound
	}
	return inst, nil
}

func resolverWithTypicalRecords() *incidentResolver {
	now := time.Now().UTC()
	return &incidentResolver{
		diagnosisRecords: &fakeDiagnosisReader{records: map[int64]diagnosis.Record{
			42: {
				ID:         42,
				ClusterID:  7,
				RuleID:     "node.not_ready.v1",
				Severity:   "critical",
				Summary:    "node NotReady sustained",
				Resource:   diagnosis.ResourceRef{Kind: "Node", Name: "demo-node", UID: "node-uid"},
				ObservedAt: now,
			},
		}},
		alerts: &fakeAlertResolver{instances: map[int64]alert.Instance{
			9: {ID: 9, RuleID: 3, DiagnosisID: 42, State: alert.StateFiring, FirstFiredAt: now},
		}},
	}
}

func TestResolveAlert(t *testing.T) {
	r := resolverWithTypicalRecords()
	info, err := r.Resolve(context.Background(), incident.SourceTypeAlert, "alert:9", 7)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if info.Title != "Alert demo-node node.not_ready.v1" {
		t.Errorf("title = %q", info.Title)
	}
	if info.Severity != "critical" || info.Resource.Name != "demo-node" {
		t.Errorf("severity/resource wrong: %+v", info)
	}
	if info.Domain != "node" || info.FindingCode != "node.not_ready.v1" {
		t.Errorf("runbook metadata wrong: %+v", info)
	}
}

func TestResolveDiagnosisRejectsForeignCluster(t *testing.T) {
	r := resolverWithTypicalRecords()
	if _, err := r.Resolve(context.Background(), incident.SourceTypeDiagnosis, "diagnosis:42", 8); err != incident.ErrInvalidSource {
		t.Errorf("foreign diagnosis cluster err = %v, want ErrInvalidSource", err)
	}
}

func TestResolveAlertInvalidSourceRefs(t *testing.T) {
	r := resolverWithTypicalRecords()
	for _, src := range []string{"alert:", "alert:abc", "alert:-3", "diagnosis:9", "finding:x"} {
		if _, err := r.Resolve(context.Background(), incident.SourceTypeAlert, src, 7); err != incident.ErrInvalidSource {
			t.Errorf("Resolve(%q) err = %v, want ErrInvalidSource", src, err)
		}
	}
}

func TestResolveAlertUnknownInstance(t *testing.T) {
	r := resolverWithTypicalRecords()
	if _, err := r.Resolve(context.Background(), incident.SourceTypeAlert, "alert:999", 7); err != incident.ErrInvalidSource {
		t.Errorf("unknown alert err = %v, want ErrInvalidSource", err)
	}
}

func TestResolveAlertErrorPropagates(t *testing.T) {
	targetErr := alert.ErrRuleNotFound
	r := &incidentResolver{
		diagnosisRecords: &fakeDiagnosisReader{records: map[int64]diagnosis.Record{}},
		alerts:           &fakeAlertResolver{err: targetErr},
	}
	if _, err := r.Resolve(context.Background(), incident.SourceTypeAlert, "alert:9", 7); err != incident.ErrInvalidSource {
		t.Errorf("rule-not-found err = %v, want ErrInvalidSource", err)
	}
}

func TestResolveNonexistentSourceType(t *testing.T) {
	r := resolverWithTypicalRecords()
	if _, err := r.Resolve(context.Background(), "bogus", "x", 7); err != incident.ErrInvalidSource {
		t.Errorf("bogus source err = %v, want ErrInvalidSource", err)
	}
}

type fakeInspectionReader struct {
	results map[int64]inspection.ResultView
	err     error
}

func (f *fakeInspectionReader) Get(_ context.Context, id int64) (inspection.ResultView, error) {
	if f.err != nil {
		return inspection.ResultView{}, f.err
	}
	r, ok := f.results[id]
	if !ok {
		return inspection.ResultView{}, inspection.ErrResultNotFound
	}
	return r, nil
}

func TestResolveInspection(t *testing.T) {
	r := &incidentResolver{
		diagnosisRecords: &fakeDiagnosisReader{records: map[int64]diagnosis.Record{}},
		alerts:           &fakeAlertResolver{instances: map[int64]alert.Instance{}},
		inspections: &fakeInspectionReader{results: map[int64]inspection.ResultView{
			11: {
				ID:           11,
				ClusterID:    7,
				RuleCode:     "node_not_ready",
				SignalCode:   "inspect.node.not_ready.v1",
				Severity:     "critical",
				State:        "firing",
				ResourceKind: "Node",
				ResourceName: "demo-node",
				ResourceUID:  "node-uid",
				ObservedAt:   time.Now().UTC(),
			},
		}},
	}
	info, err := r.Resolve(context.Background(), incident.SourceTypeInspection, "inspection:11", 7)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if info.Title != "Inspection node_not_ready demo-node" {
		t.Errorf("title = %q", info.Title)
	}
	if info.Severity != "critical" || info.Resource.Name != "demo-node" {
		t.Errorf("severity/resource wrong: %+v", info)
	}
}

func TestResolveInspectionRejectsForeignCluster(t *testing.T) {
	r := &incidentResolver{
		diagnosisRecords: &fakeDiagnosisReader{records: map[int64]diagnosis.Record{}},
		alerts:           &fakeAlertResolver{instances: map[int64]alert.Instance{}},
		inspections: &fakeInspectionReader{results: map[int64]inspection.ResultView{
			11: {ID: 11, ClusterID: 999, RuleCode: "x", Severity: "warning"},
		}},
	}
	if _, err := r.Resolve(context.Background(), incident.SourceTypeInspection, "inspection:11", 7); err != incident.ErrInvalidSource {
		t.Errorf("foreign cluster err = %v, want ErrInvalidSource", err)
	}
}

func TestResolveInspectionInvalidOrMissing(t *testing.T) {
	r := &incidentResolver{
		diagnosisRecords: &fakeDiagnosisReader{records: map[int64]diagnosis.Record{}},
		alerts:           &fakeAlertResolver{instances: map[int64]alert.Instance{}},
		inspections:      &fakeInspectionReader{results: map[int64]inspection.ResultView{}},
	}
	for _, src := range []string{"inspection:", "inspection:abc", "inspection:-3", "alert:9", "diagnosis:9"} {
		if _, err := r.Resolve(context.Background(), incident.SourceTypeInspection, src, 7); err != incident.ErrInvalidSource {
			t.Errorf("Resolve(%q) err = %v, want ErrInvalidSource", src, err)
		}
	}
	if _, err := r.Resolve(context.Background(), incident.SourceTypeInspection, "inspection:999", 7); err != incident.ErrInvalidSource {
		t.Errorf("missing result err = %v, want ErrInvalidSource", err)
	}
	r2 := &incidentResolver{diagnosisRecords: &fakeDiagnosisReader{records: map[int64]diagnosis.Record{}}, alerts: &fakeAlertResolver{instances: map[int64]alert.Instance{}}}
	if _, err := r2.Resolve(context.Background(), incident.SourceTypeInspection, "inspection:1", 7); err != incident.ErrInvalidSource {
		t.Errorf("nil inspections err = %v, want ErrInvalidSource", err)
	}
}

type fakeSignalReader struct {
	occurrences map[int64]signal.Occurrence
	err         error
}

func (f *fakeSignalReader) Get(_ context.Context, id int64) (signal.Occurrence, error) {
	if f.err != nil {
		return signal.Occurrence{}, f.err
	}
	o, ok := f.occurrences[id]
	if !ok {
		return signal.Occurrence{}, signal.ErrSignalNotFound
	}
	return o, nil
}

func TestResolveSignal(t *testing.T) {
	r := &incidentResolver{
		diagnosisRecords: &fakeDiagnosisReader{records: map[int64]diagnosis.Record{}},
		alerts:           &fakeAlertResolver{instances: map[int64]alert.Instance{}},
		inspections:      &fakeInspectionReader{results: map[int64]inspection.ResultView{}},
		signals: &fakeSignalReader{occurrences: map[int64]signal.Occurrence{
			21: {
				ID:         21,
				SignalID:   "slo.burn.fast.v1",
				SignalCode: "slo.burn.fast.v1",
				ClusterID:  7,
				Namespace:  "demo",
				Resource:   signal.ResourceCitation{Kind: "Deployment", Namespace: "demo", Name: "demo-app", UID: "deploy-uid"},
				Severity:   signal.SeverityCritical,
				State:      signal.StateActive,
				Coverage:   signal.CoverageComplete,
				ObservedAt: time.Now().UTC(),
			},
		}},
	}
	info, err := r.Resolve(context.Background(), incident.SourceTypeSignal, "signal:21", 7)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if info.Title != "Signal slo.burn.fast.v1 demo-app" {
		t.Errorf("title = %q", info.Title)
	}
	if info.Severity != "critical" || info.Resource.Name != "demo-app" {
		t.Errorf("severity/resource wrong: %+v", info)
	}
	if info.Domain != "slo" || info.FindingCode != "slo.burn.fast.v1" {
		t.Errorf("runbook metadata wrong: %+v", info)
	}
}

func TestResolveSignalRejectsForeignCluster(t *testing.T) {
	r := &incidentResolver{
		diagnosisRecords: &fakeDiagnosisReader{records: map[int64]diagnosis.Record{}},
		alerts:           &fakeAlertResolver{instances: map[int64]alert.Instance{}},
		inspections:      &fakeInspectionReader{results: map[int64]inspection.ResultView{}},
		signals: &fakeSignalReader{occurrences: map[int64]signal.Occurrence{
			21: {ID: 21, SignalID: "x", ClusterID: 999, Severity: signal.SeverityWarning},
		}},
	}
	if _, err := r.Resolve(context.Background(), incident.SourceTypeSignal, "signal:21", 7); err != incident.ErrInvalidSource {
		t.Errorf("foreign cluster err = %v, want ErrInvalidSource", err)
	}
}

func TestResolveSignalInvalidOrMissing(t *testing.T) {
	r := &incidentResolver{
		diagnosisRecords: &fakeDiagnosisReader{records: map[int64]diagnosis.Record{}},
		alerts:           &fakeAlertResolver{instances: map[int64]alert.Instance{}},
		inspections:      &fakeInspectionReader{results: map[int64]inspection.ResultView{}},
		signals:          &fakeSignalReader{occurrences: map[int64]signal.Occurrence{}},
	}
	for _, src := range []string{"signal:", "signal:abc", "signal:-3", "alert:9", "diagnosis:9"} {
		if _, err := r.Resolve(context.Background(), incident.SourceTypeSignal, src, 7); err != incident.ErrInvalidSource {
			t.Errorf("Resolve(%q) err = %v, want ErrInvalidSource", src, err)
		}
	}
	if _, err := r.Resolve(context.Background(), incident.SourceTypeSignal, "signal:999", 7); err != incident.ErrInvalidSource {
		t.Errorf("missing occurrence err = %v, want ErrInvalidSource", err)
	}
	r2 := &incidentResolver{diagnosisRecords: &fakeDiagnosisReader{records: map[int64]diagnosis.Record{}}, alerts: &fakeAlertResolver{instances: map[int64]alert.Instance{}}}
	if _, err := r2.Resolve(context.Background(), incident.SourceTypeSignal, "signal:1", 7); err != incident.ErrInvalidSource {
		t.Errorf("nil signals err = %v, want ErrInvalidSource", err)
	}
}

type fakeCorrelationCaseReader struct {
	views map[int64]correlation.CaseView
	err   error
}

func (f *fakeCorrelationCaseReader) GetCase(_ context.Context, id int64) (correlation.CaseView, error) {
	if f.err != nil {
		return correlation.CaseView{}, f.err
	}
	view, ok := f.views[id]
	if !ok {
		return correlation.CaseView{}, correlation.ErrCaseNotFound
	}
	return view, nil
}

func TestResolveCorrelation(t *testing.T) {
	now := time.Now().UTC()
	r := &incidentResolver{
		correlations: &fakeCorrelationCaseReader{views: map[int64]correlation.CaseView{
			11: {
				Case: correlation.Case{
					ID: 11, CaseKey: "abc123", ClusterID: 7, RuleID: "rollout.pod_failure.v1",
					Confidence:      correlation.ConfidenceCandidate,
					PrimaryResource: correlation.ResourceCitation{Kind: "Deployment", Namespace: "demo", Name: "web", UID: "dep-uid"},
					FirstObservedAt: now,
					LastObservedAt:  now,
				},
				SignalLinks: []correlation.SignalLink{{ID: 1, CaseID: 11, SignalOccurrenceID: 3, SignalID: "diag.deployment.replicas_unavailable.v1", Relation: correlation.SignalRelationTrigger}},
			},
		}},
	}
	info, err := r.Resolve(context.Background(), incident.SourceTypeCorrelation, "correlation:11", 7)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if info.Severity != incident.SeverityWarning {
		t.Errorf("severity = %q, want warning for candidate confidence", info.Severity)
	}
	if info.Resource.Name != "web" || info.Resource.UID != "dep-uid" {
		t.Errorf("resource wrong: %+v", info.Resource)
	}
	if info.Domain != "workload" || info.FindingCode != "rollout.pod_failure.v1" {
		t.Errorf("runbook metadata wrong: %+v", info)
	}
	if !strings.Contains(info.Summary, "abc123") {
		t.Errorf("summary missing case_key: %q", info.Summary)
	}
}

func TestResolveCorrelationAntiLeakage(t *testing.T) {
	r := &incidentResolver{
		correlations: &fakeCorrelationCaseReader{views: map[int64]correlation.CaseView{
			11: {Case: correlation.Case{ID: 11, ClusterID: 7, Confidence: correlation.ConfidenceConfirmed}},
		}},
	}
	// The caller asks for cluster 8, but the case belongs to cluster 7.
	if _, err := r.Resolve(context.Background(), incident.SourceTypeCorrelation, "correlation:11", 8); err != incident.ErrInvalidSource {
		t.Errorf("cross-cluster err = %v, want ErrInvalidSource", err)
	}
}

func TestResolveCorrelationInvalidSourceRefs(t *testing.T) {
	r := &incidentResolver{correlations: &fakeCorrelationCaseReader{views: map[int64]correlation.CaseView{}}}
	for _, src := range []string{"correlation:", "correlation:abc", "correlation:-3", "signal:9", "finding:x"} {
		if _, err := r.Resolve(context.Background(), incident.SourceTypeCorrelation, src, 7); err != incident.ErrInvalidSource {
			t.Errorf("Resolve(%q) err = %v, want ErrInvalidSource", src, err)
		}
	}
}

func TestResolveCorrelationUnknownCase(t *testing.T) {
	r := &incidentResolver{correlations: &fakeCorrelationCaseReader{views: map[int64]correlation.CaseView{}}}
	if _, err := r.Resolve(context.Background(), incident.SourceTypeCorrelation, "correlation:999", 7); err != incident.ErrInvalidSource {
		t.Errorf("unknown case err = %v, want ErrInvalidSource", err)
	}
}
