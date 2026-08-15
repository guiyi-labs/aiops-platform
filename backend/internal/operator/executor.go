package operator

import (
	"context"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

// dynamicExecutor implements TargetExecutor against a live cluster using a
// dynamic client with strategic/merge patches. When the operation requests
// dry-run, the patch is issued with dryRun=All and nothing is mutated.
type dynamicExecutor struct {
	dyn dynamic.Interface
}

// NewDynamicExecutor constructs a TargetExecutor backed by a dynamic client.
func NewDynamicExecutor(dyn dynamic.Interface) TargetExecutor {
	return &dynamicExecutor{dyn: dyn}
}

// Execute implements TargetExecutor.
func (e *dynamicExecutor) Execute(ctx context.Context, op *ControlledOperation) (string, error) {
	gvr, ok := AsGVR(op.Spec.TargetKind)
	if !ok {
		return "", fmt.Errorf("unsupported target kind %q", op.Spec.TargetKind)
	}

	ri := e.dyn.Resource(gvr).Namespace(op.Spec.TargetNamespace)
	// dryRun=All is passed through the patch options; nothing is persisted.
	opts := metav1.PatchOptions{}
	if op.Spec.IsDryRun() {
		opts.DryRun = []string{metav1.DryRunAll}
	}

	var patchBytes []byte
	var patchType types.PatchType
	summary := ""

	switch op.Spec.Action {
	case ActionDeploymentRolloutRestart:
		// Touch the pod template to trigger the rollout restart.
		patchBytes = []byte(`{"spec":{"template":{"metadata":{"annotations":{"aiops.platform/restartedAt":"now"}}}}}`)
		patchType = types.MergePatchType
		summary = "rollout restart issued"
	case ActionDeploymentScale:
		if op.Spec.DesiredReplicas == nil {
			return "", fmt.Errorf("deployment.scale requires spec.desiredReplicas")
		}
		p, err := json.Marshal(map[string]any{
			"spec": map[string]any{"replicas": *op.Spec.DesiredReplicas},
		})
		if err != nil {
			return "", fmt.Errorf("marshal scale patch: %w", err)
		}
		patchBytes = p
		patchType = types.MergePatchType
		summary = fmt.Sprintf("scaled to %d", *op.Spec.DesiredReplicas)
	case ActionCronJobSuspend:
		patchBytes = []byte(`{"spec":{"suspend":true}}`)
		patchType = types.MergePatchType
		summary = "cronjob suspended"
	default:
		return "", fmt.Errorf("unsupported action %q", op.Spec.Action)
	}

	if _, err := ri.Patch(ctx, op.Spec.TargetName, patchType, patchBytes, opts); err != nil {
		return "", fmt.Errorf("%s %s/%s: %w", op.Spec.TargetKind, op.Spec.TargetNamespace, op.Spec.TargetName, err)
	}
	if op.Spec.IsDryRun() {
		summary += " (dry-run)"
	}
	return summary, nil
}