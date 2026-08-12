package main

import (
	"context"
	"testing"
	"time"

	"k8s-aiops.local/backend/internal/alert"
	"k8s-aiops.local/backend/internal/diagnosis"
	"k8s-aiops.local/backend/internal/incident"
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
