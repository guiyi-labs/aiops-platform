package automation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

func newPatchTestService(k8s KubernetesSource) *Service {
	fixed := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	return NewService(newMemRepo(), NopCaseReader{}, k8s, WithNow(func() time.Time { return fixed }))
}

func TestBuildChangeBranches(t *testing.T) {
	scale := int32(3)
	cases := []struct {
		name  string
		plan  ActionPlan
		want  string
		empty bool
	}{
		{"scale full", ActionPlan{ActionCode: "deployment.scale", BeforeReplicas: &scale, DesiredReplicas: &scale}, "spec.replicas", false},
		{"scale no before", ActionPlan{ActionCode: "deployment.scale", DesiredReplicas: &scale}, "", true},
		{"cronjob suspend", ActionPlan{ActionCode: "cronjob.suspend", BeforeSuspended: boolPtr(false), DesiredSuspended: boolPtr(true)}, "spec.suspend", false},
		{"image update", ActionPlan{ActionCode: "deployment.image_update", ContainerName: "app", BeforeImage: "v1", DesiredImage: "v2"}, "spec.template.spec.containers[app].image", false},
		{"rollback", ActionPlan{ActionCode: "deployment.rollback", RollbackRevision: &scale}, "spec.template (revision rollback)", false},
		{"unknown", ActionPlan{ActionCode: "unknown"}, "", true},
	}
	for _, tc := range cases {
		got := buildChange(tc.plan)
		if tc.empty {
			if got != nil {
				t.Errorf("%s: buildChange = %+v, want nil", tc.name, got)
			}
			continue
		}
		if got == nil || got.Field != tc.want {
			t.Errorf("%s: buildChange = %+v, want field %s", tc.name, got, tc.want)
		}
	}
}

func TestBuildPatchBranches(t *testing.T) {
	s := newPatchTestService(nil)
	scale := int32(3)
	overflow := int32(2000)
	neg := int32(-1)
	tru := true
	fals := false
	cases := []struct {
		name     string
		plan     ActionPlan
		wantErr  error
		contains string
	}{
		{"restart", ActionPlan{ActionCode: "deployment.rollout_restart", ID: "p1", TargetUID: "u", TargetResourceVersion: "rv"}, nil, "restarted-at"},
		{"scale ok", ActionPlan{ActionCode: "deployment.scale", TargetUID: "u", TargetResourceVersion: "rv", DesiredReplicas: &scale}, nil, `"replicas":3`},
		{"scale nil", ActionPlan{ActionCode: "deployment.scale", TargetUID: "u", TargetResourceVersion: "rv"}, ErrInvalidOperation, ""},
		{"scale neg", ActionPlan{ActionCode: "deployment.scale", TargetUID: "u", TargetResourceVersion: "rv", DesiredReplicas: &neg}, ErrInvalidOperation, ""},
		{"scale overflow", ActionPlan{ActionCode: "deployment.scale", TargetUID: "u", TargetResourceVersion: "rv", DesiredReplicas: &overflow}, ErrInvalidOperation, ""},
		{"suspend ok", ActionPlan{ActionCode: "cronjob.suspend", TargetUID: "u", TargetResourceVersion: "rv", DesiredSuspended: &tru}, nil, `"suspend":true`},
		{"suspend mismatch", ActionPlan{ActionCode: "cronjob.suspend", TargetUID: "u", TargetResourceVersion: "rv", DesiredSuspended: &fals}, ErrInvalidOperation, ""},
		{"resume ok", ActionPlan{ActionCode: "cronjob.resume", TargetUID: "u", TargetResourceVersion: "rv", DesiredSuspended: &fals}, nil, `"suspend":false`},
		{"image ok", ActionPlan{ActionCode: "deployment.image_update", TargetUID: "u", TargetResourceVersion: "rv", ContainerName: "app", DesiredImage: "img:v2"}, nil, `"image":"img:v2"`},
		{"image missing", ActionPlan{ActionCode: "deployment.image_update", TargetUID: "u", TargetResourceVersion: "rv", ContainerName: ""}, ErrInvalidOperation, ""},
		{"unsupported", ActionPlan{ActionCode: "unknown"}, ErrUnsupportedAction, ""},
	}
	for _, tc := range cases {
		patch, err := s.buildPatch(context.Background(), tc.plan)
		if tc.wantErr != nil {
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("%s: err = %v, want %v", tc.name, err, tc.wantErr)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: unexpected err %v", tc.name, err)
			continue
		}
		if !strings.Contains(string(patch), tc.contains) {
			t.Errorf("%s: patch %s missing %s", tc.name, patch, tc.contains)
		}
	}
}

func TestBuildRollbackPatchBranches(t *testing.T) {
	int32Ptr := func(v int32) *int32 { return &v }
	setup := func(err error) (*Service, *fakeKubernetesSource) {
		src := &fakeKubernetesSource{}
		src.rsErr = err
		if err == nil {
			rs := k8sgateway.ReplicaSet{}
			rs.Metadata.UID = "rs-uid-1"
			rs.Metadata.ResourceVersion = "rs-rv-1"
			rs.Spec.Template.Raw = json.RawMessage(`{"metadata":{"labels":{"app":"d","pod-template-hash":"h"}},"spec":{"containers":[]}}`)
			src.replicaSet = rs
		}
		return newPatchTestService(src), src
	}
	base := ActionPlan{
		ClusterID:                         1,
		TargetNamespace:                   "ns",
		ID:                                "p1",
		ActionCode:                        "deployment.rollback",
		TargetUID:                         "dep-uid",
		TargetResourceVersion:             "dep-rv",
		RollbackRevision:                  int32Ptr(2),
		RollbackReplicaSetName:            "rs-1",
		RollbackReplicaSetUID:             "rs-uid-1",
		RollbackReplicaSetResourceVersion: "rs-rv-1",
	}

	// Missing replica set reference fails closed before any K8s call.
	s := newPatchTestService(&fakeKubernetesSource{})
	plan := base
	plan.RollbackReplicaSetName = ""
	if _, err := s.buildRollbackPatch(context.Background(), plan); !errors.Is(err, ErrInvalidOperation) {
		t.Errorf("missing RS ref: err = %v", err)
	}

	s2, _ := setup(nil)
	patch, err := s2.buildRollbackPatch(context.Background(), base)
	if err != nil {
		t.Fatalf("rollback success: %v", err)
	}
	raw := string(patch)
	if !strings.Contains(raw, "rollback-revision") || !strings.Contains(raw, "dep-uid") {
		t.Errorf("rollback patch = %s", raw)
	}
	if strings.Contains(raw, "pod-template-hash") {
		t.Errorf("rollback patch leaked pod-template-hash: %s", raw)
	}

	// K8s read failure propagates.
	s3, _ := setup(errors.New("boom"))
	if _, err := s3.buildRollbackPatch(context.Background(), base); err == nil {
		t.Error("expected RS read error")
	}

	// UID mismatch fails closed.
	s4, src4 := setup(nil)
	src4.replicaSet.Metadata.UID = "other-uid"
	if _, err := s4.buildRollbackPatch(context.Background(), base); !errors.Is(err, ErrTargetChanged) {
		t.Errorf("UID mismatch: err = %v", err)
	}

	// ResourceVersion mismatch fails closed when RV is pinned.
	s5, src5 := setup(nil)
	src5.replicaSet.Metadata.ResourceVersion = "other-rv"
	if _, err := s5.buildRollbackPatch(context.Background(), base); !errors.Is(err, ErrTargetChanged) {
		t.Errorf("RV mismatch: err = %v", err)
	}

	// Pinned RV empty skips the RV check (rolls back on hash-only trust).
	s6, src6 := setup(nil)
	src6.replicaSet.Metadata.ResourceVersion = "other-rv"
	plan6 := base
	plan6.RollbackReplicaSetResourceVersion = ""
	if _, err := s6.buildRollbackPatch(context.Background(), plan6); err != nil {
		t.Errorf("RV empty should pass: %v", err)
	}

	// Empty template fails closed.
	s7, src7 := setup(nil)
	src7.replicaSet.Spec.Template.Raw = nil
	if _, err := s7.buildRollbackPatch(context.Background(), base); !errors.Is(err, ErrInvalidOperation) {
		t.Errorf("empty template: err = %v", err)
	}

	// Malformed template JSON fails closed.
	s8, src8 := setup(nil)
	src8.replicaSet.Spec.Template.Raw = json.RawMessage(`{broken`)
	if _, err := s8.buildRollbackPatch(context.Background(), base); !errors.Is(err, ErrInvalidOperation) {
		t.Errorf("bad template: err = %v", err)
	}
}

func TestServiceOptionsTTLs(t *testing.T) {
	s := NewService(newMemRepo(), NopCaseReader{}, nil,
		WithPlanTTL(2*time.Hour),
		WithClaimTTL(3*time.Hour),
		WithCooldown(4*time.Hour),
	)
	if s.planTTL != 2*time.Hour || s.claimTTL != 3*time.Hour || s.cooldown != 4*time.Hour {
		t.Errorf("TTLs = %v %v %v", s.planTTL, s.claimTTL, s.cooldown)
	}
	if !s.enabled || s.repo == nil || s.gates == nil || s.verifier == nil {
		t.Error("service defaults missing")
	}
	disabled := NewService(nil, nil, nil, func(service *Service) { service.enabled = false })
	if disabled.enabled {
		t.Error("enabled should be false")
	}
	if disabled.planTTL != DefaultPlanTTLSeconds*time.Second {
		t.Errorf("default planTTL = %v", disabled.planTTL)
	}
}

func boolPtr(b bool) *bool { return &b }
