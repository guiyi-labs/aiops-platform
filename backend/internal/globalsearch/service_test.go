package globalsearch

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

type clusterStub struct {
	items []cluster.Cluster
	err   error
}

func (s clusterStub) List(context.Context) ([]cluster.Cluster, error) { return s.items, s.err }

type resourcesStub struct {
	delay       time.Duration
	block       bool
	failService map[int64]bool
	mu          sync.Mutex
	active      int
	maxActive   int
}

func (s *resourcesStub) call(ctx context.Context) error {
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

func (s *resourcesStub) Pods(ctx context.Context, clusterID int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Pod], error) {
	if err := s.call(ctx); err != nil {
		return apiquery.ListResponse[k8sgateway.Pod]{}, err
	}
	items := []k8sgateway.Pod{{Metadata: k8sgateway.ObjectMeta{Name: "api", Namespace: "prod"}}, {Metadata: k8sgateway.ObjectMeta{Name: "api-worker", Namespace: "prod"}}}
	for index := range items {
		items[index].Status.Phase = "Running"
		items[index].Status.ContainerStatuses = []k8sgateway.ContainerStatus{{Ready: true}}
	}
	return apiquery.ListResponse[k8sgateway.Pod]{Items: items, Total: len(items)}, nil
}

func (s *resourcesStub) Deployments(ctx context.Context, clusterID int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Deployment], error) {
	if err := s.call(ctx); err != nil {
		return apiquery.ListResponse[k8sgateway.Deployment]{}, err
	}
	replicas := int32(1)
	item := k8sgateway.Deployment{Metadata: k8sgateway.ObjectMeta{Name: "api", Namespace: "prod"}}
	item.Spec.Replicas = &replicas
	item.Status.ReadyReplicas, item.Status.AvailableReplicas = 1, 1
	return apiquery.ListResponse[k8sgateway.Deployment]{Items: []k8sgateway.Deployment{item}, Total: 1}, nil
}

func (s *resourcesStub) Services(ctx context.Context, clusterID int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.ServiceResource], error) {
	if err := s.call(ctx); err != nil {
		return apiquery.ListResponse[k8sgateway.ServiceResource]{}, err
	}
	if s.failService[clusterID] {
		return apiquery.ListResponse[k8sgateway.ServiceResource]{}, errors.New("service read failed")
	}
	item := k8sgateway.ServiceResource{Metadata: k8sgateway.ObjectMeta{Name: "api", Namespace: "prod"}}
	item.Spec.Type = "ClusterIP"
	return apiquery.ListResponse[k8sgateway.ServiceResource]{Items: []k8sgateway.ServiceResource{item}, Total: 1}, nil
}

func (s *resourcesStub) Ingresses(ctx context.Context, clusterID int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Ingress], error) {
	if err := s.call(ctx); err != nil {
		return apiquery.ListResponse[k8sgateway.Ingress]{}, err
	}
	return apiquery.ListResponse[k8sgateway.Ingress]{Items: []k8sgateway.Ingress{{Metadata: k8sgateway.ObjectMeta{Name: "api", Namespace: "prod"}}}, Total: 1}, nil
}

func TestSearchBoundsConcurrencySortsAndTruncates(t *testing.T) {
	resources := &resourcesStub{delay: 2 * time.Millisecond}
	service := NewService(Config{MaxClusters: 3, MaxConcurrentClusters: 2, PerClusterTimeout: time.Second, MaxResults: 5, PerKindLimit: 5}, clusterStub{items: []cluster.Cluster{
		{ID: 3, Name: "three", Enabled: true}, {ID: 1, Name: "one", Enabled: true}, {ID: 2, Name: "disabled", Enabled: false},
	}}, resources)
	response, err := service.Search(context.Background(), Query{Term: "api", Kinds: SupportedKinds(), ClusterLimit: 3, ResultLimit: 5}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if response.Total != 10 || response.Remaining != 5 || len(response.Items) != 5 || response.Complete {
		t.Fatalf("response = %#v", response)
	}
	if response.ClustersTotal != 2 || response.ClustersSearched != 2 || response.ClustersRemaining != 0 {
		t.Fatalf("cluster coverage = %#v", response)
	}
	if response.Items[0].ClusterID != 1 || response.Items[0].Kind != KindPod || response.Items[0].Name != "api" || response.Items[2].Kind != KindDeployment || response.Items[4].Kind != KindIngress {
		t.Fatalf("items = %#v", response.Items)
	}
	if resources.maxActive > 2 || response.Limits.MaxConcurrentClusters != 2 || response.Limits.MaxResults != 5 {
		t.Fatalf("maxActive=%d limits=%#v", resources.maxActive, response.Limits)
	}
}

func TestSearchReportsEnabledClustersOmittedByRequestLimit(t *testing.T) {
	service := NewService(Config{MaxClusters: 3}, clusterStub{items: []cluster.Cluster{
		{ID: 3, Name: "three", Enabled: true},
		{ID: 1, Name: "one", Enabled: true},
		{ID: 2, Name: "two", Enabled: true},
	}}, &resourcesStub{})
	response, err := service.Search(context.Background(), Query{Term: "api", Kinds: []Kind{KindPod}, ClusterLimit: 2, ResultLimit: 100}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if response.ClustersTotal != 3 || response.ClustersSearched != 2 || response.ClustersRemaining != 1 || response.Complete {
		t.Fatalf("response = %#v", response)
	}
	if len(response.Items) == 0 || response.Items[0].ClusterID != 1 {
		t.Fatalf("stable selected clusters = %#v", response.Items)
	}
}

func TestSearchKeepsKindFailureLocalAndReportsKnownTotal(t *testing.T) {
	service := NewService(Config{}, clusterStub{items: []cluster.Cluster{{ID: 7, Name: "partial", Enabled: true}}}, &resourcesStub{failService: map[int64]bool{7: true}})
	response, err := service.Search(context.Background(), Query{Term: "api", Kinds: SupportedKinds(), ClusterLimit: 20, ResultLimit: 100}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if response.Total != 4 || len(response.Items) != 4 || response.Complete || len(response.Failures) != 1 || response.Failures[0] != (Failure{ClusterID: 7, ClusterName: "partial", Kind: KindService, Code: FailureQueryFailed}) {
		t.Fatalf("response = %#v", response)
	}
}

func TestSearchMarksEveryRemainingKindTimedOut(t *testing.T) {
	service := NewService(Config{PerClusterTimeout: 10 * time.Millisecond}, clusterStub{items: []cluster.Cluster{{ID: 9, Name: "slow", Enabled: true}}}, &resourcesStub{block: true})
	response, err := service.Search(context.Background(), Query{Term: "api", Kinds: SupportedKinds(), ClusterLimit: 20, ResultLimit: 100}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if response.Complete || len(response.Items) != 0 || len(response.Failures) != 4 {
		t.Fatalf("response = %#v", response)
	}
	for _, failure := range response.Failures {
		if failure.Code != FailureTimeout {
			t.Fatalf("failure = %#v", failure)
		}
	}
}

func TestSearchValidatesFixedQueryShapeAndDirectoryFailure(t *testing.T) {
	service := NewService(Config{}, clusterStub{}, &resourcesStub{})
	invalid := []Query{
		{Term: "a", ClusterLimit: 20, ResultLimit: 100},
		{Term: "api", Namespace: "Invalid", ClusterLimit: 20, ResultLimit: 100},
		{Term: "api", Kinds: []Kind{"Secret"}, ClusterLimit: 20, ResultLimit: 100},
		{Term: "api", Kinds: []Kind{KindPod, KindPod}, ClusterLimit: 20, ResultLimit: 100},
		{Term: "api", ClusterLimit: 21, ResultLimit: 100},
		{Term: "api", ClusterLimit: 20, ResultLimit: 101},
	}
	for _, query := range invalid {
		if _, err := service.Search(context.Background(), query, nil); !errors.Is(err, ErrInvalidQuery) {
			t.Fatalf("query=%#v err=%v", query, err)
		}
	}
	if _, err := ParseKinds("pods,secrets"); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("kind parse error = %v", err)
	}
	directoryErr := errors.New("directory unavailable")
	service = NewService(Config{}, clusterStub{err: directoryErr}, &resourcesStub{})
	if _, err := service.Search(context.Background(), Query{Term: "api", ClusterLimit: 20, ResultLimit: 100}, nil); !errors.Is(err, directoryErr) {
		t.Fatalf("directory error = %v", err)
	}
}
