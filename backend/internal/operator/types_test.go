package operator

import (
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestActionAndPhaseConstants(t *testing.T) {
	valid := ValidActions()
	want := []string{"deployment.rollout_restart", "deployment.scale", "cronjob.suspend"}
	if !reflect.DeepEqual(valid, want) {
		t.Fatalf("ValidActions() = %v, want %v", valid, want)
	}
}

func TestControlledOperationDeepCopyRoundTrip(t *testing.T) {
	replicas := int32(3)
	dry := false
	op := &ControlledOperation{
		TypeMeta:   TypeMetaOf(),
		ObjectMeta: objMeta("demo", "default"),
		Spec: ControlledOperationSpec{
			Action:          ActionDeploymentScale,
			TargetKind:      "Deployment",
			TargetNamespace: "default",
			TargetName:      "api",
			DesiredReplicas: &replicas,
			DryRun:          &dry,
			IdempotencyKey:  "scale-001",
		},
		Status: ControlledOperationStatus{
			Phase:              PhaseSucceeded,
			Attempts:           1,
			ObservedGeneration: 1,
			LastMessage:        "scaled to 3",
			AppliedID:          "scale-001",
		},
	}

	cp := op.DeepCopy()
	if !reflect.DeepEqual(op, cp) {
		t.Fatalf("DeepCopy mismatch:\n%#v\n%#v", op, cp)
	}
	// Mutating the copy must not affect the original.
	*cp.Spec.DesiredReplicas = 5
	if *op.Spec.DesiredReplicas != 3 {
		t.Fatalf("DeepCopy shares DesiredReplicas pointer: op=%d", *op.Spec.DesiredReplicas)
	}

	if _, ok := interface{}(op).(runtime.Object); !ok {
		t.Fatal("ControlledOperation must implement runtime.Object")
	}
	if _, ok := interface{}(op.DeepCopyObject()).(runtime.Object); !ok {
		t.Fatal("DeepCopyObject must return a runtime.Object")
	}
}

func TestSpecIsDryRunDefaultsTrue(t *testing.T) {
	if !(ControlledOperationSpec{}).IsDryRun() {
		t.Fatal("empty spec must default dryRun=true")
	}
	f := false
	if (ControlledOperationSpec{DryRun: &f}).IsDryRun() {
		t.Fatal("explicit false must be honored")
	}
}

func TestUnstructuredRoundTrip(t *testing.T) {
	op := &ControlledOperation{
		TypeMeta:   TypeMetaOf(),
		ObjectMeta: objMeta("restart", "prod"),
		Spec: ControlledOperationSpec{
			Action:          ActionDeploymentRolloutRestart,
			TargetKind:      "Deployment",
			TargetNamespace: "prod",
			TargetName:      "web",
		},
		Status: ControlledOperationStatus{Phase: PhasePending},
	}

	u, err := ControlledOperationToUnstructured(op)
	if err != nil {
		t.Fatalf("ToUnstructured: %v", err)
	}
	if u.GetKind() != "ControlledOperation" || u.GetAPIVersion() != "aiops.platform/v1" {
		t.Fatalf("unexpected type meta: kind=%s apiVersion=%s", u.GetKind(), u.GetAPIVersion())
	}
	if got := u.Object["spec"].(map[string]any)["action"]; got != "deployment.rollout_restart" {
		t.Fatalf("action = %v", got)
	}

	back, err := UnstructuredToControlledOperation(u)
	if err != nil {
		t.Fatalf("FromUnstructured: %v", err)
	}
	if !reflect.DeepEqual(op, back) {
		t.Fatalf("round trip mismatch:\n%#v\n%#v", op, back)
	}
}

func TestSchemeRegistration(t *testing.T) {
	s := runtime.NewScheme()
	if err := AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	op := &ControlledOperation{TypeMeta: TypeMetaOf()}
	if _, _, err := s.ObjectKinds(op); err != nil {
		t.Fatalf("ObjectKinds: %v", err)
	}
}

func TestAsGVR(t *testing.T) {
	cases := []struct {
		kind string
		ok   bool
	}{
		{"Deployment", true},
		{"CronJob", true},
		{"Pod", false},
		{"", false},
	}
	for _, c := range cases {
		_, ok := AsGVR(c.kind)
		if ok != c.ok {
			t.Fatalf("AsGVR(%q) ok=%v, want %v", c.kind, ok, c.ok)
		}
	}
}

func TestUnstructuredNil(t *testing.T) {
	var u *unstructured.Unstructured
	if _, err := UnstructuredToControlledOperation(u); err == nil {
		t.Fatal("nil unstructured must error")
	}
}

func TypeMetaOf() metav1.TypeMeta {
	return metav1.TypeMeta{Kind: "ControlledOperation", APIVersion: "aiops.platform/v1"}
}

func objMeta(name, ns string) metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: name, Namespace: ns, Generation: 1}
}
