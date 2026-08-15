package operator

import (
	"context"
	"errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// Resource returns the GVR for ControlledOperation.
func Resource() schema.GroupVersionResource {
	return GroupVersion.WithResource("controlledoperations")
}

// Client is the typed read/write surface the reconciler depends on. It is a
// thin, typed wrapper over the dynamic client so that unit tests can inject a
// fake dynamic client without a full typed clientset generator.
type Client interface {
	// Get returns the ControlledOperation by namespace/name.
	Get(ctx context.Context, namespace, name string) (*ControlledOperation, error)
	// UpdateStatus persists the status subresource.
	UpdateStatus(ctx context.Context, op *ControlledOperation) (*ControlledOperation, error)
}

// dynamicClient adapts dynamic.Interface to the typed Client interface.
type dynamicClient struct {
	dyn dynamic.Interface
}

// NewClient wraps a dynamic client with the ControlledOperation types.
func NewClient(dyn dynamic.Interface) Client {
	return &dynamicClient{dyn: dyn}
}

// Get implements Client.
func (c *dynamicClient) Get(ctx context.Context, namespace, name string) (*ControlledOperation, error) {
	u, err := c.dyn.Resource(Resource()).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return UnstructuredToControlledOperation(u)
}

// UpdateStatus implements Client.
func (c *dynamicClient) UpdateStatus(ctx context.Context, op *ControlledOperation) (*ControlledOperation, error) {
	u, err := ControlledOperationToUnstructured(op)
	if err != nil {
		return nil, err
	}
	updated, err := c.dyn.Resource(Resource()).Namespace(op.Namespace).
		UpdateStatus(ctx, u, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	return UnstructuredToControlledOperation(updated)
}

// ControlledOperationToUnstructured converts a typed object to Unstructured.
func ControlledOperationToUnstructured(op *ControlledOperation) (*unstructured.Unstructured, error) {
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(op)
	if err != nil {
		return nil, err
	}
	return &unstructured.Unstructured{Object: u}, nil
}

// UnstructuredToControlledOperation converts an Unstructured object to the
// typed ControlledOperation.
func UnstructuredToControlledOperation(u *unstructured.Unstructured) (*ControlledOperation, error) {
	if u == nil || u.Object == nil {
		return nil, errors.New("operator: nil unstructured object")
	}
	op := &ControlledOperation{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, op); err != nil {
		return nil, err
	}
	return op, nil
}

// IsNotFound reports whether err is a Kubernetes NotFound error.
func IsNotFound(err error) bool {
	return apierrors.IsNotFound(err)
}

// AsGVR returns the GroupVersionResource for a given kind target used by the
// executor (Deployment / CronJob).
func AsGVR(kind string) (schema.GroupVersionResource, bool) {
	switch kind {
	case "Deployment":
		return schema.GroupVersionResource{
			Group: "apps", Version: "v1", Resource: "deployments",
		}, true
	case "CronJob":
		return schema.GroupVersionResource{
			Group: "batch", Version: "v1", Resource: "cronjobs",
		}, true
	default:
		return schema.GroupVersionResource{}, false
	}
}
