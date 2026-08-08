package automation

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBuildGateContextDeployment(t *testing.T) {
	fixed := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	src := &fakeKubernetesSource{}
	s := NewService(newMemRepo(), NopCaseReader{}, src, WithNow(func() time.Time { return fixed }))

	plan := ActionPlan{
		ID:                    "p1",
		ClusterID:             1,
		TargetKind:            "Deployment",
		TargetNamespace:       "ns",
		TargetName:            "app",
		TargetUID:             "dep-uid",
		TargetResourceVersion: "dep-rv",
		ActionCode:            "deployment.scale",
	}

	// K8s read success populates the current snapshot; defaults stay safe.
	gc, err := s.buildGateContext(context.Background(), plan, true)
	if err != nil {
		t.Fatal(err)
	}
	if !gc.ScopeDecision.Allowed {
		t.Error("default scope denied")
	}
	if gc.PreviewSnapshot.Kind != "Deployment" || gc.PreviewSnapshot.UID != "dep-uid" {
		t.Errorf("preview snapshot = %+v", gc.PreviewSnapshot)
	}
	if gc.CurrentSnapshot.Kind != "Deployment" {
		t.Errorf("current snapshot = %+v", gc.CurrentSnapshot)
	}
	if gc.AttemptMax != MaxAttemptsPerTarget {
		t.Errorf("attempt max = %d", gc.AttemptMax)
	}

	// K8s read failure leaves the current snapshot empty (fail-closed).
	src.depErr = errors.New("boom")
	gc2, _ := s.buildGateContext(context.Background(), plan, true)
	if gc2.CurrentSnapshot.UID != "" {
		t.Errorf("expected empty current snapshot, got %+v", gc2.CurrentSnapshot)
	}
}

func TestBuildGateContextCronJobAndRollback(t *testing.T) {
	fixed := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	src := &fakeKubernetesSource{}
	s := NewService(newMemRepo(), NopCaseReader{}, src, WithNow(func() time.Time { return fixed }))

	// Unknown action kind with no K8s call: safe defaults.
	gc, err := s.buildGateContext(context.Background(), ActionPlan{
		TargetKind: "StatefulSet",
		ActionCode: "unknown",
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !gc.ScopeDecision.Allowed || gc.CurrentSnapshot.Kind != "" {
		t.Errorf("defaults = %+v", gc)
	}

	// CronJob read success populates the current snapshot.
	gc2, _ := s.buildGateContext(context.Background(), ActionPlan{
		TargetKind: "CronJob",
		ActionCode: "cronjob.suspend",
	}, false)
	if gc2.CurrentSnapshot.Kind != "CronJob" {
		t.Errorf("cronjob current snapshot = %+v", gc2.CurrentSnapshot)
	}

	// Rollback evidence: revision + replica set name → exists.
	rev := int32(2)
	gc3, _ := s.buildGateContext(context.Background(), ActionPlan{
		TargetKind:             "Deployment",
		ActionCode:             "deployment.rollback",
		RollbackRevision:       &rev,
		RollbackReplicaSetName: "rs-1",
	}, false)
	if !gc3.RollbackPoint.Exists || gc3.RollbackPoint.Revision != 2 {
		t.Errorf("rollback point = %+v", gc3.RollbackPoint)
	}

	// Rollback without replica set name → Exists false.
	gc4, _ := s.buildGateContext(context.Background(), ActionPlan{
		TargetKind:       "Deployment",
		ActionCode:       "deployment.rollback",
		RollbackRevision: &rev,
	}, false)
	if gc4.RollbackPoint.Exists {
		t.Errorf("rollback point should be missing, got %+v", gc4.RollbackPoint)
	}
}
