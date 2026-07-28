package fleet

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"k8s-aiops.local/backend/internal/apiquery"
	"k8s-aiops.local/backend/internal/cluster"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

type clusterSourceStub struct {
	items []cluster.Cluster
	err   error
}

func (s clusterSourceStub) List(context.Context) ([]cluster.Cluster, error) { return s.items, s.err }

type resourceSourceStub struct {
	delay     time.Duration
	block     bool
	failPods  map[int64]bool
	mu        sync.Mutex
	active    int
	maxActive int
}

func (s *resourceSourceStub) call(ctx context.Context, clusterID int64) error {
	s.mu.Lock()
	s.active++
	if s.active > s.maxActive {
		s.maxActive = s.active
	}
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.active--; s.mu.Unlock() }()
	if s.block {
		<-ctx.Done()
		return ctx.Err()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(s.delay):
		return nil
	}
}

func (s *resourceSourceStub) Nodes(ctx context.Context, clusterID int64, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Node], error) {
	if err := s.call(ctx, clusterID); err != nil {
		return apiquery.ListResponse[k8sgateway.Node]{}, err
	}
	var item k8sgateway.Node
	item.Status.Conditions = append(item.Status.Conditions, struct {
		Type               string `json:"type"`
		Status             string `json:"status"`
		Reason             string `json:"reason"`
		Message            string `json:"message"`
		LastTransitionTime string `json:"lastTransitionTime"`
	}{Type: "Ready", Status: "True"})
	return apiquery.ListResponse[k8sgateway.Node]{Items: []k8sgateway.Node{item}, Total: 1}, nil
}

func (s *resourceSourceStub) Pods(ctx context.Context, clusterID int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Pod], error) {
	if err := s.call(ctx, clusterID); err != nil {
		return apiquery.ListResponse[k8sgateway.Pod]{}, err
	}
	if s.failPods[clusterID] {
		return apiquery.ListResponse[k8sgateway.Pod]{}, errors.New("pod read failed")
	}
	var item k8sgateway.Pod
	item.Status.Phase = "Running"
	item.Status.ContainerStatuses = []k8sgateway.ContainerStatus{{Ready: true}}
	return apiquery.ListResponse[k8sgateway.Pod]{Items: []k8sgateway.Pod{item}, Total: 1}, nil
}

func (s *resourceSourceStub) Deployments(ctx context.Context, clusterID int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Deployment], error) {
	if err := s.call(ctx, clusterID); err != nil {
		return apiquery.ListResponse[k8sgateway.Deployment]{}, err
	}
	var item k8sgateway.Deployment
	replicas := int32(1)
	item.Spec.Replicas = &replicas
	item.Status.ReadyReplicas = 1
	item.Status.AvailableReplicas = 1
	return apiquery.ListResponse[k8sgateway.Deployment]{Items: []k8sgateway.Deployment{item}, Total: 1}, nil
}

func (s *resourceSourceStub) Events(ctx context.Context, clusterID int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Event], error) {
	if err := s.call(ctx, clusterID); err != nil {
		return apiquery.ListResponse[k8sgateway.Event]{}, err
	}
	return apiquery.ListResponse[k8sgateway.Event]{Items: []k8sgateway.Event{}, Total: 0}, nil
}

func TestCompareBoundsConcurrencyAndPreservesOrder(t *testing.T) {
	resources := &resourceSourceStub{delay: 5 * time.Millisecond}
	service := NewService(Config{MaxClusters: 3, MaxConcurrentClusters: 2, PerClusterTimeout: time.Second, ResourceSampleLimit: 100}, clusterSourceStub{items: []cluster.Cluster{
		{ID: 3, Name: "three", Enabled: true}, {ID: 1, Name: "one", Enabled: true}, {ID: 2, Name: "disabled", Enabled: false}, {ID: 4, Name: "four", Enabled: true},
	}}, resources)
	response, err := service.Compare(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if response.Total != 3 || response.Remaining != 1 || len(response.Items) != 2 || response.Items[0].ClusterID != 1 || response.Items[1].ClusterID != 3 {
		t.Fatalf("response = %#v", response)
	}
	if resources.maxActive > 2 || response.Items[0].Status != StatusHealthy || response.Items[1].Status != StatusHealthy {
		t.Fatalf("maxActive=%d items=%#v", resources.maxActive, response.Items)
	}
	if response.Limits.MaxClusters != 3 || response.Limits.ResourceSampleLimit != 100 {
		t.Fatalf("limits = %#v", response.Limits)
	}
}

func TestCompareReturnsPartialFailureWithoutFailingFleet(t *testing.T) {
	resources := &resourceSourceStub{failPods: map[int64]bool{7: true}}
	service := NewService(Config{MaxClusters: 20, MaxConcurrentClusters: 4, PerClusterTimeout: time.Second, ResourceSampleLimit: 100}, clusterSourceStub{items: []cluster.Cluster{{ID: 7, Name: "partial", Enabled: true}}}, resources)
	response, err := service.Compare(context.Background(), 20)
	if err != nil {
		t.Fatal(err)
	}
	item := response.Items[0]
	if item.Status != StatusPartial || len(item.Failures) != 1 || item.Failures[0] != (Failure{Scope: "pods", Code: "QUERY_FAILED"}) {
		t.Fatalf("item = %#v", item)
	}
}

func TestCompareMarksPerClusterTimeout(t *testing.T) {
	service := NewService(Config{MaxClusters: 20, MaxConcurrentClusters: 1, PerClusterTimeout: 10 * time.Millisecond, ResourceSampleLimit: 100}, clusterSourceStub{items: []cluster.Cluster{{ID: 9, Name: "slow", Enabled: true}}}, &resourceSourceStub{block: true})
	response, err := service.Compare(context.Background(), 20)
	if err != nil {
		t.Fatal(err)
	}
	item := response.Items[0]
	if item.Status != StatusTimedOut || len(item.Failures) != 4 {
		t.Fatalf("item = %#v", item)
	}
	for _, failure := range item.Failures {
		if failure.Code != "TIMEOUT" {
			t.Fatalf("failure = %#v", failure)
		}
	}
}

func TestCompareRejectsLimitAndDirectoryFailure(t *testing.T) {
	service := NewService(Config{MaxClusters: 2}, clusterSourceStub{}, &resourceSourceStub{})
	if _, err := service.Compare(context.Background(), 3); !errors.Is(err, ErrInvalidLimit) {
		t.Fatalf("limit error = %v", err)
	}
	directoryErr := errors.New("directory unavailable")
	service = NewService(Config{MaxClusters: 2}, clusterSourceStub{err: directoryErr}, &resourceSourceStub{})
	if _, err := service.Compare(context.Background(), 2); !errors.Is(err, directoryErr) {
		t.Fatalf("directory error = %v", err)
	}
}

func TestHealthStatusDoesNotCallEmptyOrTruncatedDataHealthy(t *testing.T) {
	complete := ClusterHealth{Nodes: ResourceSummary{Complete: true}, Pods: ResourceSummary{Complete: true}, Deployments: ResourceSummary{Complete: true}, Warnings: WarningSummary{Complete: true}}
	if status := healthStatus(context.Background(), complete); status != StatusDegraded {
		t.Fatalf("empty status = %q", status)
	}
	complete.Nodes = ResourceSummary{Healthy: 1, Sampled: 1, Total: 1, Complete: true}
	complete.Pods = ResourceSummary{Healthy: 100, Sampled: 100, Total: 101, Complete: false}
	if status := healthStatus(context.Background(), complete); status != StatusPartial {
		t.Fatalf("truncated status = %q", status)
	}
}
