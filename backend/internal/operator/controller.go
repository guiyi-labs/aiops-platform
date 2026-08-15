package operator

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

// TargetExecutor carries a ControlledOperation out against the target
// resource (Deployment / CronJob). It is an interface so unit tests can
// inject a recording fake; the production implementation performs a
// Kubernetes patch with dryRun=All when the spec requests it.
type TargetExecutor interface {
	// Execute performs the operation and returns a short human summary.
	Execute(ctx context.Context, op *ControlledOperation) (string, error)
}

// Reconciler resolves one ControlledOperation to a terminal status. It only
// depends on the typed Client and the TargetExecutor, so it is fully unit
// testable with a fake dynamic client.
type Reconciler struct {
	client   Client
	executor TargetExecutor
}

// NewReconciler constructs a Reconciler.
func NewReconciler(client Client, executor TargetExecutor) *Reconciler {
	return &Reconciler{client: client, executor: executor}
}

// Reconcile processes one ControlledOperation by namespace/name. It returns
// an error only for transient failures (API errors); permanent failures are
// recorded in status.Phase = Failed so the queue does not retry forever.
func (r *Reconciler) Reconcile(ctx context.Context, key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid resource key %q: %w", key, err)
	}

	op, err := r.client.Get(ctx, namespace, name)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object deleted while queued; nothing to do.
			return nil
		}
		return fmt.Errorf("get %s/%s: %w", namespace, name, err)
	}

	// Deletion: clear the finalizer when the operation already reached a
	// terminal state, so the object can be garbage collected.
	if !op.DeletionTimestamp.IsZero() {
		return r.finalize(ctx, op)
	}

	// Idempotency: an operation whose key was already consumed is done.
	if op.Status.Phase == PhaseSucceeded && op.Status.AppliedID != "" &&
		op.Status.AppliedID == op.Spec.IdempotencyKey {
		return nil
	}

	// Validate the action belongs to the fixed controlled-operation directory.
	if !r.isSupported(op) {
		return r.recordFailure(ctx, op, fmt.Sprintf("unsupported action %q", op.Spec.Action))
	}

	// Carried out while Reconciling; failures are recorded and not retried
	// indefinitely (the controller requeues only on transient API errors).
	summary, err := r.executor.Execute(ctx, op)
	if err != nil {
		return r.recordFailure(ctx, op, err.Error())
	}

	op.Status.Phase = PhaseSucceeded
	op.Status.Attempts++
	op.Status.ObservedGeneration = op.Generation
	op.Status.LastMessage = summary
	op.Status.AppliedID = op.Spec.IdempotencyKey
	if _, err := r.client.UpdateStatus(ctx, op); err != nil {
		return fmt.Errorf("update status %s/%s: %w", namespace, name, err)
	}
	return nil
}

func (r *Reconciler) isSupported(op *ControlledOperation) bool {
	switch op.Spec.Action {
	case ActionDeploymentRolloutRestart, ActionDeploymentScale, ActionCronJobSuspend:
		_, okTarget := AsGVR(op.Spec.TargetKind)
		return okTarget
	default:
		return false
	}
}

func (r *Reconciler) recordFailure(ctx context.Context, op *ControlledOperation, msg string) error {
	op.Status.Phase = PhaseFailed
	op.Status.Attempts++
	op.Status.ObservedGeneration = op.Generation
	op.Status.LastMessage = msg
	if _, err := r.client.UpdateStatus(ctx, op); err != nil {
		return fmt.Errorf("update failure status %s/%s: %w", op.Namespace, op.Name, err)
	}
	return nil
}

func (r *Reconciler) finalize(ctx context.Context, op *ControlledOperation) error {
	if !containsString(op.Finalizers, ControlledOperationFinalizer) {
		return nil
	}
	if op.Status.Phase != PhaseSucceeded {
		// A deletion racing an in-flight operation must not silently drop
		// the audit trail; record a Failed terminal phase first.
		_ = r.recordFailure(ctx, op, "deleted before completion")
	}
	for i, f := range op.Finalizers {
		if f == ControlledOperationFinalizer {
			op.Finalizers = append(op.Finalizers[:i], op.Finalizers[i+1:]...)
			break
		}
	}
	_, err := r.client.UpdateStatus(ctx, op)
	if err != nil {
		return fmt.Errorf("remove finalizer %s/%s: %w", op.Namespace, op.Name, err)
	}
	return nil
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// Controller wires a workqueue and the Reconciler into a classic
// single-worker controller loop using only client-go primitives. Informer
// event handling is wired by the run entry point (cmd/controlled-operation-
// operator), which calls Enqueue on add/update and OnDelete on tombstones.
type Controller struct {
	reconciler *Reconciler
	queue      workqueue.TypedRateLimitingInterface[string]
}

// NewController builds the controller around a Reconciler.
func NewController(r *Reconciler) *Controller {
	return &Controller{
		reconciler: r,
		queue: workqueue.NewTypedRateLimitingQueue[string](
			workqueue.DefaultTypedControllerRateLimiter[string](),
		),
	}
}

// Enqueue adds a key for the given object (cache.DeletedFinalStateUnknown
// aware). It is wired to informer event handlers by the run entry point.
func (c *Controller) Enqueue(obj any) error {
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		c.queue.Add(tombstone.Key)
		return nil
	}
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		return err
	}
	c.queue.Add(key)
	return nil
}

// ProcessOne handles the next queue item. It returns whether work remains.
// It is exported so tests can drive reconciliation deterministically
// without a running informer.
func (c *Controller) ProcessOne(ctx context.Context) bool {
	key, shutdown := c.queue.Get()
	if shutdown {
		return false
	}
	defer c.queue.Done(key)

	if err := c.reconciler.Reconcile(ctx, key); err != nil {
		// Only transient errors are requeued.
		c.queue.AddRateLimited(key)
		return true
	}
	c.queue.Forget(key)
	return true
}

// QueueLen returns the number of pending items (used by tests and probes).
func (c *Controller) QueueLen() int {
	return c.queue.Len()
}

// ShutDown stops the queue.
func (c *Controller) ShutDown() {
	c.queue.ShutDown()
}

// KeyFor returns a cache key for an object (used to seed tests).
func KeyFor(obj any) (string, error) {
	return cache.MetaNamespaceKeyFunc(obj)
}

// NamespacedNameFor builds a types.NamespacedName (used by tests).
func NamespacedNameFor(ns, name string) types.NamespacedName {
	return types.NamespacedName{Namespace: ns, Name: name}
}

// Finalizer marker for ControlledOperation.
const ControlledOperationFinalizer = "aiops.platform/finalizer"