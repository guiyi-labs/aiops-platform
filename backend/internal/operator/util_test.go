package operator

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
)

func TestIsNotFound(t *testing.T) {
	ns := "default"
	name := "ghost"
	if IsNotFound(nil) {
		t.Fatal("nil must not be NotFound")
	}
	if IsNotFound(errors.New("boom")) {
		t.Fatal("generic error must not be NotFound")
	}
	// Build a real NotFound error via the fake dynamic client.
	dyn := newFakeDyn()
	_, err := dyn.Resource(Resource()).Namespace(ns).Get(context.Background(), name, metav1.GetOptions{})
	if err == nil {
		t.Fatal("expected NotFound from empty fake")
	}
	if !IsNotFound(err) {
		t.Fatalf("fake Get error = %v, want NotFound", err)
	}
}

func TestKeyForAndNamespacedNameFor(t *testing.T) {
	op := fakeOp("k", "ns")
	key, err := KeyFor(op)
	if err != nil {
		t.Fatalf("KeyFor: %v", err)
	}
	if key != "ns/k" {
		t.Fatalf("key = %q, want ns/k", key)
	}
	// Round-trip through cache.SplitMetaNamespaceKey.
	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil || ns != "ns" || name != "k" {
		t.Fatalf("split(%q) = %q/%q/%v", key, ns, name, err)
	}

	nn := NamespacedNameFor("ns", "k")
	if nn != (types.NamespacedName{Namespace: "ns", Name: "k"}) {
		t.Fatalf("NamespacedNameFor = %v", nn)
	}
}

func TestControlledOperationListDeepCopy(t *testing.T) {
	op1 := fakeOp("a", "ns")
	op2 := fakeOp("b", "ns")
	list := &ControlledOperationList{
		TypeMeta: TypeMetaOf(),
		ListMeta: metav1.ListMeta{ResourceVersion: "7"},
		Items:    []ControlledOperation{*op1, *op2},
	}

	cp := list.DeepCopy()
	if !reflect.DeepEqual(list, cp) {
		t.Fatalf("DeepCopy mismatch:\n%#v\n%#v", list, cp)
	}
	// Mutating a nested item's ObjectMeta must not affect the original.
	cp.Items[0].ObjectMeta.Name = "mutated"
	if list.Items[0].ObjectMeta.Name != "a" {
		t.Fatalf("DeepCopy shares nested pointer: %q", list.Items[0].ObjectMeta.Name)
	}

	obj := list.DeepCopyObject()
	if _, ok := obj.(*ControlledOperationList); !ok {
		t.Fatalf("DeepCopyObject type = %T", obj)
	}

	// nil receiver leaves.
	if (*ControlledOperationList)(nil).DeepCopy() != nil {
		t.Fatal("nil list DeepCopy must return nil")
	}
	if (*ControlledOperationSpec)(nil).DeepCopy() != nil {
		t.Fatal("nil spec DeepCopy must return nil")
	}
}

func TestListMetaRoundTrip(t *testing.T) {
	// The unstructured route preserves ListMeta; ensure the typed conversion
	// does not drop items.
	op := fakeOp("x", "ns")
	list := &ControlledOperationList{
		TypeMeta: TypeMetaOf(),
		Items:    []ControlledOperation{*op},
	}
	u := &unstructured.Unstructured{}
	obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(list)
	if err != nil {
		t.Fatalf("ToUnstructured: %v", err)
	}
	u.Object = obj
	back, err := UnstructuredFromUnstructuredList(u)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	if len(back.Items) != 1 || back.Items[0].Name != "x" {
		t.Fatalf("list items = %#v", back.Items)
	}
}

// UnstructuredFromUnstructuredList is a small helper mirroring the typed
// conversion used by client.go for lists.
func UnstructuredFromUnstructuredList(u *unstructured.Unstructured) (*ControlledOperationList, error) {
	list := &ControlledOperationList{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, list); err != nil {
		return nil, err
	}
	return list, nil
}

func TestEnqueueTombstoneAndBadKey(t *testing.T) {
	r := NewReconciler(newFakeClient(t), &recordingExecutor{})
	c := NewController(r)

	// DeletedFinalStateUnknown path.
	tomb := cache.DeletedFinalStateUnknown{Key: "ns/ghost"}
	if err := c.Enqueue(tomb); err != nil {
		t.Fatalf("Enqueue tombstone: %v", err)
	}
	if c.QueueLen() != 1 {
		t.Fatalf("queue len = %d, want 1", c.QueueLen())
	}
	// Process it: object gone -> no-op, no requeue.
	if !c.ProcessOne(context.Background()) {
		t.Fatal("ProcessOne must report work")
	}
	if c.QueueLen() != 0 {
		t.Fatalf("queue must drain without requeue, len=%d", c.QueueLen())
	}

	// A non-meta object must fail Enqueue.
	badObj := &struct{ plain string }{plain: "x"}
	if err := c.Enqueue(badObj); err == nil {
		t.Fatal("Enqueue on non-meta object must error")
	}
	c.ShutDown()
}

func TestMetaNamespaceKeyFuncDirect(t *testing.T) {
	op := fakeOp("m", "ns")
	key, err := cache.MetaNamespaceKeyFunc(op)
	if err != nil {
		t.Fatalf("MetaNamespaceKeyFunc: %v", err)
	}
	if key != "ns/m" {
		t.Fatalf("key = %q", key)
	}
}

var _ = meta.IsNoMatchError
