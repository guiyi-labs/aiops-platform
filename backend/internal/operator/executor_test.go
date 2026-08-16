package operator

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

func seedDeployment(t *testing.T, dyn dynamic.Interface, name, ns string) {
	t.Helper()
	dep := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name": name, "namespace": ns,
			"annotations": map[string]any{"spec.template.annotations.preexisting": "keep"},
		},
		"spec": map[string]any{
			"replicas": int64(1),
			"template": map[string]any{
				"metadata": map[string]any{"labels": map[string]any{"app": name}},
				"spec":     map[string]any{"containers": []any{map[string]any{"name": "c", "image": "busybox"}}},
			},
		},
	}}
	if _, err := dyn.Resource(AsDeploymentGVR()).Namespace(ns).Create(context.Background(), dep, metav1.CreateOptions{}); err != nil {
		t.Fatalf("seed deployment: %v", err)
	}
}

func seedCronJob(t *testing.T, dyn dynamic.Interface, name, ns string) {
	t.Helper()
	cj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "batch/v1",
		"kind":       "CronJob",
		"metadata":   map[string]any{"name": name, "namespace": ns},
		"spec": map[string]any{
			"schedule": "0 * * * *",
			"jobTemplate": map[string]any{
				"spec": map[string]any{
					"template": map[string]any{
						"spec": map[string]any{"containers": []any{map[string]any{"name": "c", "image": "busybox"}}},
					},
				},
			},
		},
	}}
	if _, err := dyn.Resource(AsCronJobGVR()).Namespace(ns).Create(context.Background(), cj, metav1.CreateOptions{}); err != nil {
		t.Fatalf("seed cronjob: %v", err)
	}
}

func newFakeDyn() dynamic.Interface {
	return dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
}

func TestDynamicExecutorRolloutRestart(t *testing.T) {
	dyn := newFakeDyn()
	seedDeployment(t, dyn, "web", "prod")
	op := &ControlledOperation{
		TypeMeta:   TypeMetaOf(),
		ObjectMeta: objMeta("restart", "prod"),
		Spec: ControlledOperationSpec{
			Action:          ActionDeploymentRolloutRestart,
			TargetKind:      "Deployment",
			TargetNamespace: "prod",
			TargetName:      "web",
		},
	}
	dry := false
	op.Spec.DryRun = &dry

	exec := NewDynamicExecutor(dyn)
	summary, err := exec.Execute(context.Background(), op)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if summary != "rollout restart issued" {
		t.Fatalf("summary = %q", summary)
	}
	got, err := dyn.Resource(AsDeploymentGVR()).Namespace("prod").Get(context.Background(), "web", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	anns, _, _ := unstructured.NestedStringMap(got.Object, "spec", "template", "metadata", "annotations")
	if anns["aiops.platform/restartedAt"] == "" {
		t.Fatalf("restart annotation missing: %v", got.Object["spec"])
	}
}

func TestDynamicExecutorCronJobSuspend(t *testing.T) {
	dyn := newFakeDyn()
	seedCronJob(t, dyn, "nightly", "prod")
	op := &ControlledOperation{
		TypeMeta:   TypeMetaOf(),
		ObjectMeta: objMeta("suspend", "prod"),
		Spec: ControlledOperationSpec{
			Action:          ActionCronJobSuspend,
			TargetKind:      "CronJob",
			TargetNamespace: "prod",
			TargetName:      "nightly",
		},
	}
	dry := false
	op.Spec.DryRun = &dry

	exec := NewDynamicExecutor(dyn)
	summary, err := exec.Execute(context.Background(), op)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if summary != "cronjob suspended" {
		t.Fatalf("summary = %q", summary)
	}
	got, err := dyn.Resource(AsCronJobGVR()).Namespace("prod").Get(context.Background(), "nightly", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	suspend, _, _ := unstructured.NestedBool(got.Object, "spec", "suspend")
	if !suspend {
		t.Fatalf("cronjob not suspended: %v", got.Object["spec"])
	}
}

func TestDynamicExecutorDryRunOptionPassed(t *testing.T) {
	dyn := newFakeDyn()
	seedDeployment(t, dyn, "api", "prod")
	op := &ControlledOperation{
		TypeMeta:   TypeMetaOf(),
		ObjectMeta: objMeta("dry", "prod"),
		Spec: ControlledOperationSpec{
			Action:          ActionDeploymentRolloutRestart,
			TargetKind:      "Deployment",
			TargetNamespace: "prod",
			TargetName:      "api",
			// DryRun omitted: defaults to true.
		},
	}

	// Capture the dryRun option on the outgoing patch call. The fake dynamic
	// client applies patches regardless of dryRun, so asserting the option
	// itself is the correct observable for dry-run passthrough.
	var gotDryRun []string
	dyn.(*dynamicfake.FakeDynamicClient).PrependReactor("patch", "deployments", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if pa, ok := action.(k8stesting.PatchActionImpl); ok {
			gotDryRun = pa.GetPatchOptions().DryRun
		}
		return false, nil, nil
	})

	exec := NewDynamicExecutor(dyn)
	summary, err := exec.Execute(context.Background(), op)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if summary != "rollout restart issued (dry-run)" {
		t.Fatalf("summary = %q, want dry-run marker", summary)
	}
	if len(gotDryRun) != 1 || gotDryRun[0] != metav1.DryRunAll {
		t.Fatalf("dryRun option = %v, want [%s]", gotDryRun, metav1.DryRunAll)
	}
}

func TestDynamicExecutorNonDryRunNoOption(t *testing.T) {
	dyn := newFakeDyn()
	seedDeployment(t, dyn, "api", "prod")
	dry := false
	op := &ControlledOperation{
		TypeMeta:   TypeMetaOf(),
		ObjectMeta: objMeta("live", "prod"),
		Spec: ControlledOperationSpec{
			Action:          ActionDeploymentRolloutRestart,
			TargetKind:      "Deployment",
			TargetNamespace: "prod",
			TargetName:      "api",
			DryRun:          &dry,
		},
	}

	var gotDryRun []string
	dyn.(*dynamicfake.FakeDynamicClient).PrependReactor("patch", "deployments", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if pa, ok := action.(k8stesting.PatchActionImpl); ok {
			gotDryRun = pa.GetPatchOptions().DryRun
		}
		return false, nil, nil
	})

	exec := NewDynamicExecutor(dyn)
	summary, err := exec.Execute(context.Background(), op)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if summary != "rollout restart issued" {
		t.Fatalf("summary = %q, want no dry-run marker", summary)
	}
	if len(gotDryRun) != 0 {
		t.Fatalf("dryRun option = %v, want empty", gotDryRun)
	}
}

func TestDynamicExecutorUnsupportedKindAndAction(t *testing.T) {
	dyn := newFakeDyn()

	// Unsupported target kind before any patch work.
	op := &ControlledOperation{
		TypeMeta:   TypeMetaOf(),
		ObjectMeta: objMeta("pod", "prod"),
		Spec: ControlledOperationSpec{
			Action:          ActionDeploymentScale,
			TargetKind:      "Pod",
			TargetNamespace: "prod",
			TargetName:      "x",
		},
	}
	exec := NewDynamicExecutor(dyn)
	if _, err := exec.Execute(context.Background(), op); err == nil {
		t.Fatal("unsupported target kind must error")
	}

	// Unknown action on a supported kind.
	op2 := &ControlledOperation{
		TypeMeta:   TypeMetaOf(),
		ObjectMeta: objMeta("unknown", "prod"),
		Spec: ControlledOperationSpec{
			Action:          "deployment.delete_all",
			TargetKind:      "Deployment",
			TargetNamespace: "prod",
			TargetName:      "x",
		},
	}
	if _, err := exec.Execute(context.Background(), op2); err == nil {
		t.Fatal("unknown action must error")
	}
}
