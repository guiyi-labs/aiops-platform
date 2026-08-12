package correlation

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"k8s-aiops.local/backend/internal/apiquery"
	"k8s-aiops.local/backend/internal/cluster"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

type fakeClusterLister struct {
	items []cluster.Cluster
	err   error
}

func (f fakeClusterLister) List(context.Context) ([]cluster.Cluster, error) { return f.items, f.err }

type fakeNamespaceLister struct {
	byCluster map[int64][]string
	err       error
}

func (f fakeNamespaceLister) Namespaces(_ context.Context, clusterID int64, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Namespace], error) {
	if f.err != nil {
		return apiquery.ListResponse[k8sgateway.Namespace]{}, f.err
	}
	names := f.byCluster[clusterID]
	items := make([]k8sgateway.Namespace, 0, len(names))
	for _, name := range names {
		items = append(items, k8sgateway.Namespace{Metadata: k8sgateway.ObjectMeta{Name: name}})
	}
	return apiquery.ListResponse[k8sgateway.Namespace]{Items: items, Total: len(items)}, nil
}

type scopeCall struct {
	clusterID int64
	namespace string
}

type recordingCorrelator struct {
	calls []scopeCall
}

func (r *recordingCorrelator) CorrelateNamespace(_ context.Context, clusterID int64, namespace string) (CorrelateResult, error) {
	r.calls = append(r.calls, scopeCall{clusterID: clusterID, namespace: namespace})
	return CorrelateResult{ClusterID: clusterID, Namespace: namespace}, nil
}

func TestWorkerRunPassScopesAndSkipsDisabled(t *testing.T) {
	clusters := fakeClusterLister{items: []cluster.Cluster{
		{ID: 1, Name: "prod", Enabled: true},
		{ID: 2, Name: "dr", Enabled: false},
		{ID: 3, Name: "staging", Enabled: true},
	}}
	namespaces := fakeNamespaceLister{byCluster: map[int64][]string{1: {"app", "data"}, 3: {"tools"}}}
	correlate := &recordingCorrelator{}
	w := NewWorker(WorkerConfig{Interval: time.Hour, PerClusterTimeout: time.Minute}, clusters, namespaces, correlate, zap.NewNop())

	w.runPass(context.Background())

	want := []scopeCall{
		{clusterID: 1, namespace: "app"},
		{clusterID: 1, namespace: "data"},
		{clusterID: 3, namespace: "tools"},
	}
	if len(correlate.calls) != len(want) {
		t.Fatalf("calls = %+v, want %+v", correlate.calls, want)
	}
	for i := range want {
		if correlate.calls[i] != want[i] {
			t.Errorf("call[%d] = %+v, want %+v", i, correlate.calls[i], want[i])
		}
	}
}

func TestWorkerRunPassNamespaceErrorFallsBackToAllNamespaces(t *testing.T) {
	clusters := fakeClusterLister{items: []cluster.Cluster{{ID: 1, Name: "prod", Enabled: true}}}
	namespaces := fakeNamespaceLister{err: errors.New("cluster unreachable")}
	correlate := &recordingCorrelator{}
	w := NewWorker(WorkerConfig{Interval: time.Hour}, clusters, namespaces, correlate, zap.NewNop())

	w.runPass(context.Background())

	want := []scopeCall{{clusterID: 1, namespace: ""}}
	if len(correlate.calls) != 1 || correlate.calls[0] != want[0] {
		t.Fatalf("calls = %+v, want %+v", correlate.calls, want)
	}
}

func TestWorkerRunPassEmptyNamespacesSkipsCluster(t *testing.T) {
	clusters := fakeClusterLister{items: []cluster.Cluster{{ID: 1, Name: "prod", Enabled: true}}}
	namespaces := fakeNamespaceLister{byCluster: map[int64][]string{}}
	correlate := &recordingCorrelator{}
	w := NewWorker(WorkerConfig{Interval: time.Hour}, clusters, namespaces, correlate, zap.NewNop())

	w.runPass(context.Background())

	if len(correlate.calls) != 0 {
		t.Fatalf("want no calls, got %+v", correlate.calls)
	}
}

func TestWorkerRunPassClusterListErrorLoggedNotFatal(t *testing.T) {
	clusters := fakeClusterLister{err: errors.New("db down")}
	correlate := &recordingCorrelator{}
	w := NewWorker(WorkerConfig{Interval: time.Hour}, clusters, fakeNamespaceLister{}, correlate, zap.NewNop())

	w.runPass(context.Background())

	if len(correlate.calls) != 0 {
		t.Fatalf("want no calls, got %+v", correlate.calls)
	}
}

func TestWorkerRunStopsOnContextCancel(t *testing.T) {
	clusters := fakeClusterLister{}
	correlate := &recordingCorrelator{}
	w := NewWorker(WorkerConfig{Interval: time.Hour}, clusters, fakeNamespaceLister{}, correlate, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after context cancellation")
	}
}
