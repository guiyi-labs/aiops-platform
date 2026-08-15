package operator

import (
	"context"
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func AsDeploymentGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
}

// recordingExecutor implements TargetExecutor for tests and records calls.
type recordingExecutor struct {
	calls   int
	lastOp  *ControlledOperation
	lastDry bool
	err     error
	summary string
}

func (e *recordingExecutor) Execute(_ context.Context, op *ControlledOperation) (string, error) {
	e.calls++
	e.lastOp = op
	e.lastDry = op.Spec.IsDryRun()
	if e.err != nil {
		return "", e.err
	}
	if e.summary != "" {
		return e.summary, nil
	}
	return "executed", nil
}

func newFakeClient(t *testing.T, ops ...*ControlledOperation) Client {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	objs := make([]runtime.Object, 0, len(ops))
	for _, op := range ops {
		u, err := ControlledOperationToUnstructured(op)
		if err != nil {
			t.Fatalf("to unstructured: %v", err)
		}
		objs = append(objs, u)
	}
	return NewClient(dynamicfake.NewSimpleDynamicClient(scheme, objs...))
}

func fakeOp(name, ns string) *ControlledOperation {
	return &ControlledOperation{
		TypeMeta:   TypeMetaOf(),
		ObjectMeta: objMeta(name, ns),
		Spec: ControlledOperationSpec{
			Action:          ActionDeploymentRolloutRestart,
			TargetKind:      "Deployment",
			TargetNamespace: ns,
			TargetName:      "api",
		},
	}
}

func TestReconcileHappyPath(t *testing.T) {
	op := fakeOp("restart-1", "prod")
	exec := &recordingExecutor{}
	r := NewReconciler(newFakeClient(t, op), exec)

	if err := r.Reconcile(context.Background(), "prod/restart-1"); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if exec.calls != 1 {
		t.Fatalf("executor calls = %d, want 1", exec.calls)
	}
	if !exec.lastDry {
		t.Fatal("dry-run must default to true")
	}

	got, err := r.client.(*dynamicClient).Get(context.Background(), "prod", "restart-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status.Phase != PhaseSucceeded {
		t.Fatalf("phase = %q, want Succeeded", got.Status.Phase)
	}
	if got.Status.Attempts != 1 || got.Status.ObservedGeneration != 1 {
		t.Fatalf("status = attempts %d gen %d, want 1/1", got.Status.Attempts, got.Status.ObservedGeneration)
	}
}

func TestReconcileUnsupportedAction(t *testing.T) {
	op := fakeOp("bad", "prod")
	op.Spec.Action = "deployment.delete_all"
	exec := &recordingExecutor{}
	r := NewReconciler(newFakeClient(t, op), exec)

	if err := r.Reconcile(context.Background(), "prod/bad"); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if exec.calls != 0 {
		t.Fatalf("executor must not run for unsupported action, calls=%d", exec.calls)
	}
	got, _ := r.client.Get(context.Background(), "prod", "bad")
	if got.Status.Phase != PhaseFailed {
		t.Fatalf("phase = %q, want Failed", got.Status.Phase)
	}
	if got.Status.LastMessage == "" {
		t.Fatal("failure must record a lastMessage")
	}
}

func TestReconcileUnsupportedTargetKind(t *testing.T) {
	op := fakeOp("pod-dst", "prod")
	op.Spec.TargetKind = "Pod"
	exec := &recordingExecutor{}
	r := NewReconciler(newFakeClient(t, op), exec)

	if err := r.Reconcile(context.Background(), "prod/pod-dst"); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if got, _ := r.client.Get(context.Background(), "prod", "pod-dst"); got.Status.Phase != PhaseFailed {
		t.Fatalf("phase = %q, want Failed", got.Status.Phase)
	}
}

func TestReconcileExecutorError(t *testing.T) {
	op := fakeOp("fail", "prod")
	exec := &recordingExecutor{err: errors.New("boom")}
	r := NewReconciler(newFakeClient(t, op), exec)

	if err := r.Reconcile(context.Background(), "prod/fail"); err != nil {
		t.Fatalf("Reconcile must record failure, not return err: %v", err)
	}
	got, _ := r.client.Get(context.Background(), "prod", "fail")
	if got.Status.Phase != PhaseFailed || got.Status.LastMessage != "boom" {
		t.Fatalf("phase=%q msg=%q", got.Status.Phase, got.Status.LastMessage)
	}
}

func TestReconcileIdempotencyKey(t *testing.T) {
	op := fakeOp("idem", "prod")
	op.Spec.IdempotencyKey = "key-1"
	exec := &recordingExecutor{}
	r := NewReconciler(newFakeClient(t, op), exec)

	// First pass executes.
	if err := r.Reconcile(context.Background(), "prod/idem"); err != nil {
		t.Fatalf("Reconcile 1: %v", err)
	}
	if exec.calls != 1 {
		t.Fatalf("calls after 1st = %d, want 1", exec.calls)
	}

	// Second pass with the same consumed key must be a no-op.
	if err := r.Reconcile(context.Background(), "prod/idem"); err != nil {
		t.Fatalf("Reconcile 2: %v", err)
	}
	if exec.calls != 1 {
		t.Fatalf("idempotency violated: calls = %d, want 1", exec.calls)
	}
}

func TestReconcileNotFoundIsNoop(t *testing.T) {
	r := NewReconciler(newFakeClient(t), &recordingExecutor{})
	if err := r.Reconcile(context.Background(), "prod/ghost"); err != nil {
		t.Fatalf("Reconcile on missing object must not error: %v", err)
	}
}

func TestReconcileDeletionFinalizes(t *testing.T) {
	op := fakeOp("del", "prod")
	op.Finalizers = []string{ControlledOperationFinalizer}
	now := metav1.Now()
	op.DeletionTimestamp = &now
	op.Status.Phase = PhaseSucceeded

	r := NewReconciler(newFakeClient(t, op), &recordingExecutor{})
	if err := r.Reconcile(context.Background(), "prod/del"); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	got, err := r.client.Get(context.Background(), "prod", "del")
	if err != nil {
		t.Fatalf("get after finalize: %v", err)
	}
	if len(got.Finalizers) != 0 {
		t.Fatalf("finalizers = %v, want empty", got.Finalizers)
	}
}

func TestControllerQueueAndProcessOne(t *testing.T) {
	op := fakeOp("queue-1", "prod")
	r := NewReconciler(newFakeClient(t, op), &recordingExecutor{})
	c := NewController(r)

	if err := c.Enqueue(op); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if c.QueueLen() != 1 {
		t.Fatalf("queue len = %d, want 1", c.QueueLen())
	}
	if !c.ProcessOne(context.Background()) {
		t.Fatal("ProcessOne must report work remains")
	}
	if c.QueueLen() != 0 {
		t.Fatalf("queue len after process = %d, want 0", c.QueueLen())
	}
	c.ShutDown()
	if c.ProcessOne(context.Background()) {
		t.Fatal("ProcessOne after shutdown must report empty")
	}
}

func TestDynamicExecutorPatches(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	dyn := dynamicfake.NewSimpleDynamicClient(scheme)
	// Seed a Deployment so the patch has a target.
	dep := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name": "api", "namespace": "prod",
		},
		"spec": map[string]any{"replicas": int64(1)},
	}}
	if _, err := dyn.Resource(AsDeploymentGVR()).Namespace("prod").Create(
		context.Background(), dep, metav1.CreateOptions{}); err != nil {
		t.Fatalf("seed deployment: %v", err)
	}

	replicas := int32(3)
	op := &ControlledOperation{
		TypeMeta:   TypeMetaOf(),
		ObjectMeta: objMeta("scale", "prod"),
		Spec: ControlledOperationSpec{
			Action:          ActionDeploymentScale,
			TargetKind:      "Deployment",
			TargetNamespace: "prod",
			TargetName:      "api",
			DesiredReplicas: &replicas,
		},
	}
	dry := false
	op.Spec.DryRun = &dry

	exec := NewDynamicExecutor(dyn)
	summary, err := exec.Execute(context.Background(), op)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if summary != "scaled to 3" {
		t.Fatalf("summary = %q", summary)
	}

	// Verify the patch was persisted on the target Deployment.
	got, err := dyn.Resource(AsDeploymentGVR()).Namespace("prod").Get(
		context.Background(), "api", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	reps, _, _ := unstructured.NestedInt64(got.Object, "spec", "replicas")
	if reps != 3 {
		t.Fatalf("replicas = %d, want 3", reps)
	}
}

func TestDynamicExecutorScaleMissingReplicas(t *testing.T) {
	op := &ControlledOperation{
		TypeMeta:   TypeMetaOf(),
		ObjectMeta: objMeta("scale", "prod"),
		Spec: ControlledOperationSpec{
			Action:          ActionDeploymentScale,
			TargetKind:      "Deployment",
			TargetNamespace: "prod",
			TargetName:      "api",
		},
	}
	exec := NewDynamicExecutor(dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()))
	if _, err := exec.Execute(context.Background(), op); err == nil {
		t.Fatal("scale without desiredReplicas must error")
	}
}
