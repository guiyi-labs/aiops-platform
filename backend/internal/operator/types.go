// Package operator implements the ControlledOperation CRD types and a
// pure client-go controller (informer + reconciliation without
// controller-runtime). The CRD maps the platform's controlled-operation
// directory (dry-run + idempotency + audit) onto Kubernetes-native
// resources: a ControlledOperation object declares an intent, and the
// controller carries it out with idempotent status updates.
package operator

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// GroupVersion is the group/version of the ControlledOperation CRD.
var GroupVersion = schema.GroupVersion{Group: "aiops.platform", Version: "v1"}

// SchemeBuilder registers the CRD types into a runtime.Scheme.
var SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)

// AddToScheme adds the CRD types to the given scheme.
func AddToScheme(s *runtime.Scheme) error {
	return SchemeBuilder.AddToScheme(s)
}

func addKnownTypes(s *runtime.Scheme) error {
	s.AddKnownTypes(GroupVersion,
		&ControlledOperation{},
		&ControlledOperationList{},
	)
	metav1.AddToGroupVersion(s, GroupVersion)
	return nil
}

// ActionCode enumerates the supported controlled operations. The set is
// deliberately small and fixed: it mirrors the platform's controlled
// operation directory, so the operator never carries out arbitrary actions.
type ActionCode string

const (
	// ActionDeploymentRolloutRestart rolls the pods of a Deployment by
	// touching its pod template (restart rollout).
	ActionDeploymentRolloutRestart ActionCode = "deployment.rollout_restart"
	// ActionDeploymentScale scales a Deployment to the desired replicas.
	ActionDeploymentScale ActionCode = "deployment.scale"
	// ActionCronJobSuspend suspends a CronJob (sets spec.suspend=true).
	ActionCronJobSuspend ActionCode = "cronjob.suspend"
)

// ValidActions returns the allowed action codes.
func ValidActions() []string {
	return []string{
		string(ActionDeploymentRolloutRestart),
		string(ActionDeploymentScale),
		string(ActionCronJobSuspend),
	}
}

// Phase is the observed lifecycle phase of a ControlledOperation.
type Phase string

const (
	// PhasePending is the initial phase before reconciliation.
	PhasePending Phase = "Pending"
	// PhaseReconciling marks an in-flight reconciliation attempt.
	PhaseReconciling Phase = "Reconciling"
	// PhaseSucceeded means the operation was carried out successfully.
	PhaseSucceeded Phase = "Succeeded"
	// PhaseFailed means the operation failed after retries.
	PhaseFailed Phase = "Failed"
)

// ControlledOperationSpec declares the intent the operator carries out.
type ControlledOperationSpec struct {
	// Action is one of the fixed controlled-operation codes.
	Action ActionCode `json:"action"`
	// TargetKind is the resource kind (Deployment or CronJob).
	TargetKind string `json:"targetKind"`
	// TargetNamespace is the namespace of the target resource.
	TargetNamespace string `json:"targetNamespace"`
	// TargetName is the name of the target resource.
	TargetName string `json:"targetName"`
	// DesiredReplicas is required for deployment.scale.
	DesiredReplicas *int32 `json:"desiredReplicas,omitempty"`
	// DryRun defaults to true; when true the controller patches with the
	// dryRun=All query parameter and records a Succeeded status without
	// mutating the cluster.
	DryRun *bool `json:"dryRun,omitempty"`
	// IdempotencyKey, when set, makes the controller treat an operation
	// with the same key as already applied (no re-execution).
	IdempotencyKey string `json:"idempotencyKey,omitempty"`
}

// ControlledOperationStatus observes the reconciliation progress.
type ControlledOperationStatus struct {
	// Phase is Pending/Reconciling/Succeeded/Failed.
	Phase Phase `json:"phase,omitempty"`
	// Attempts counts reconciliation tries for the current generation.
	Attempts int32 `json:"attempts,omitempty"`
	// ObservedGeneration is the generation the status corresponds to.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// LastMessage is a human-readable summary of the last reconciliation.
	LastMessage string `json:"lastMessage,omitempty"`
	// AppliedID stores the idempotency key that was consumed, if any.
	AppliedID string `json:"appliedID,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ControlledOperation is the Schema for the controlledoperations CRD.
type ControlledOperation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ControlledOperationSpec   `json:"spec,omitempty"`
	Status ControlledOperationStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ControlledOperationList contains a list of ControlledOperation.
type ControlledOperationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ControlledOperation `json:"items"`
}

// IsDryRun returns the effective dry-run value (defaults to true).
func (s ControlledOperationSpec) IsDryRun() bool {
	if s.DryRun == nil {
		return true
	}
	return *s.DryRun
}

// DeepCopyInto copies the receiver into out.
func (in *ControlledOperation) DeepCopyInto(out *ControlledOperation) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy returns a deep copy of the receiver.
func (in *ControlledOperation) DeepCopy() *ControlledOperation {
	if in == nil {
		return nil
	}
	out := new(ControlledOperation)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject implements runtime.Object.
func (in *ControlledOperation) DeepCopyObject() runtime.Object {
	return in.DeepCopy()
}

// DeepCopyInto copies the receiver into out.
func (in *ControlledOperationSpec) DeepCopyInto(out *ControlledOperationSpec) {
	*out = *in
	if in.DesiredReplicas != nil {
		v := *in.DesiredReplicas
		out.DesiredReplicas = &v
	}
	if in.DryRun != nil {
		v := *in.DryRun
		out.DryRun = &v
	}
}

// DeepCopy returns a deep copy of the receiver.
func (in *ControlledOperationSpec) DeepCopy() *ControlledOperationSpec {
	if in == nil {
		return nil
	}
	out := new(ControlledOperationSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out.
func (in *ControlledOperationList) DeepCopyInto(out *ControlledOperationList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]ControlledOperation, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

// DeepCopy returns a deep copy of the receiver.
func (in *ControlledOperationList) DeepCopy() *ControlledOperationList {
	if in == nil {
		return nil
	}
	out := new(ControlledOperationList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject implements runtime.Object.
func (in *ControlledOperationList) DeepCopyObject() runtime.Object {
	return in.DeepCopy()
}
