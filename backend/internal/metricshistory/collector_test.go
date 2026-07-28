package metricshistory

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

type collectorClusterStub struct {
	items []cluster.Cluster
	err   error
}

func (s collectorClusterStub) List(context.Context) ([]cluster.Cluster, error) { return s.items, s.err }

type collectorMetricsStub struct {
	nodes func(context.Context, int64, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.NodeMetric], error)
	pods  func(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.PodMetric], error)
}

func (s collectorMetricsStub) NodeMetrics(ctx context.Context, clusterID int64, query apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.NodeMetric], error) {
	return s.nodes(ctx, clusterID, query)
}

func (s collectorMetricsStub) PodMetrics(ctx context.Context, clusterID int64, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.PodMetric], error) {
	return s.pods(ctx, clusterID, namespace, query)
}

type collectorHistoryStub struct {
	mu          sync.Mutex
	collections []CollectionInput
	deletedAt   []time.Time
	recordErr   error
	cleanupErr  error
	onRecord    func()
}

func (s *collectorHistoryStub) Record(_ context.Context, input CollectionInput) (CollectionRun, error) {
	s.mu.Lock()
	s.collections = append(s.collections, input)
	id := int64(len(s.collections))
	s.mu.Unlock()
	if s.onRecord != nil {
		s.onRecord()
	}
	return CollectionRun{ID: id}, s.recordErr
}

func (s *collectorHistoryStub) Cleanup(_ context.Context, now time.Time) (int64, error) {
	s.mu.Lock()
	s.deletedAt = append(s.deletedAt, now)
	s.mu.Unlock()
	return 3, s.cleanupErr
}

func TestCollectorRecordsEnabledClustersWithNormalizedSamples(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	history := &collectorHistoryStub{}
	metrics := successfulCollectorMetrics(now)
	collector, err := NewCollector(testCollectorConfig(), collectorClusterStub{items: []cluster.Cluster{
		{ID: 9, Enabled: true}, {ID: 3, Enabled: false}, {ID: 4, Enabled: true},
	}}, metrics, history, nil)
	if err != nil {
		t.Fatalf("NewCollector() error = %v", err)
	}
	collector.now = func() time.Time { return now }
	summary, err := collector.CollectOnce(context.Background())
	if err != nil {
		t.Fatalf("CollectOnce() error = %v", err)
	}
	if summary.Attempted != 2 || summary.Recorded != 2 || summary.Failed != 0 || len(history.collections) != 2 {
		t.Fatalf("summary=%#v collections=%d", summary, len(history.collections))
	}
	for _, input := range history.collections {
		if input.ClusterID != 4 && input.ClusterID != 9 {
			t.Fatalf("unexpected cluster input: %#v", input)
		}
		if input.Nodes != (TargetCoverage{Status: SourceSucceeded, Sampled: 1, Total: 1, Complete: true}) ||
			input.Pods != (TargetCoverage{Status: SourceSucceeded, Sampled: 1, Total: 1, Complete: true}) || input.FailureCode != "" {
			t.Fatalf("coverage input = %#v", input)
		}
		if len(input.Samples) != 4 || input.Samples[0].Value != 250000000 || input.Samples[1].Value != 5*1024*1024 {
			t.Fatalf("normalized samples = %#v", input.Samples)
		}
	}
}

func TestCollectorAppliesStableLimitWithNodePodRoundRobin(t *testing.T) {
	now := time.Now().UTC()
	history := &collectorHistoryStub{}
	metrics := collectorMetricsStub{
		nodes: func(context.Context, int64, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.NodeMetric], error) {
			return apiquery.ListResponse[k8sgateway.NodeMetric]{Items: []k8sgateway.NodeMetric{
				nodeMetric("node-b", now), nodeMetric("node-a", now),
			}, Total: 2}, nil
		},
		pods: func(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.PodMetric], error) {
			return apiquery.ListResponse[k8sgateway.PodMetric]{Items: []k8sgateway.PodMetric{
				podMetric("ops", "pod-b", now), podMetric("default", "pod-a", now),
			}, Total: 2}, nil
		},
	}
	config := testCollectorConfig()
	config.MaxSamples = 6
	collector, _ := NewCollector(config, collectorClusterStub{items: []cluster.Cluster{{ID: 1, Enabled: true}}}, metrics, history, nil)
	collector.now = func() time.Time { return now }
	if _, err := collector.CollectOnce(context.Background()); err != nil {
		t.Fatalf("CollectOnce() error = %v", err)
	}
	input := history.collections[0]
	if len(input.Samples) != 6 || input.Nodes.Sampled != 2 || input.Pods.Sampled != 1 || input.Pods.Complete || input.FailureCode != FailureCollectionLimit {
		t.Fatalf("bounded input = %#v", input)
	}
	if input.Samples[0].ResourceName != "node-a" || input.Samples[2].ResourceName != "pod-a" || input.Samples[4].ResourceName != "node-b" {
		t.Fatalf("sample allocation order = %#v", input.Samples)
	}
}

func TestCollectorMapsSourceFailuresWithoutPersistingRawErrors(t *testing.T) {
	now := time.Now().UTC()
	history := &collectorHistoryStub{}
	metrics := collectorMetricsStub{
		nodes: func(context.Context, int64, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.NodeMetric], error) {
			return apiquery.ListResponse[k8sgateway.NodeMetric]{}, context.DeadlineExceeded
		},
		pods: func(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.PodMetric], error) {
			return apiquery.ListResponse[k8sgateway.PodMetric]{}, k8sgateway.ErrMetricsAPIUnavailable
		},
	}
	collector, _ := NewCollector(testCollectorConfig(), collectorClusterStub{items: []cluster.Cluster{{ID: 1, Enabled: true}}}, metrics, history, nil)
	collector.now = func() time.Time { return now }
	if _, err := collector.CollectOnce(context.Background()); err != nil {
		t.Fatalf("CollectOnce() error = %v", err)
	}
	input := history.collections[0]
	if input.Nodes.Status != SourceTimedOut || input.Pods.Status != SourceUnavailable || input.FailureCode != FailureMetricsAPITimeout || len(input.Samples) != 0 {
		t.Fatalf("failure input = %#v", input)
	}
}

func TestCollectorRejectsInvalidQuantityAtomicallyPerSource(t *testing.T) {
	now := time.Now().UTC()
	history := &collectorHistoryStub{}
	metrics := successfulCollectorMetrics(now)
	originalNodes := metrics.nodes
	metrics.nodes = func(ctx context.Context, clusterID int64, query apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.NodeMetric], error) {
		response, err := originalNodes(ctx, clusterID, query)
		response.Items[0].Usage.CPU = "not-a-quantity"
		return response, err
	}
	collector, _ := NewCollector(testCollectorConfig(), collectorClusterStub{items: []cluster.Cluster{{ID: 1, Enabled: true}}}, metrics, history, nil)
	collector.now = func() time.Time { return now }
	if _, err := collector.CollectOnce(context.Background()); err != nil {
		t.Fatalf("CollectOnce() error = %v", err)
	}
	input := history.collections[0]
	if input.Nodes.Status != SourceFailed || input.Pods.Status != SourceSucceeded || input.FailureCode != FailureMetricsQuantity || len(input.Samples) != 2 {
		t.Fatalf("quantity failure input = %#v", input)
	}
}

func TestCollectorKeepsMixedFailureCodeConsistentWithFailedCollection(t *testing.T) {
	now := time.Now().UTC()
	history := &collectorHistoryStub{}
	metrics := collectorMetricsStub{
		nodes: func(context.Context, int64, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.NodeMetric], error) {
			return apiquery.ListResponse[k8sgateway.NodeMetric]{Items: []k8sgateway.NodeMetric{nodeMetric("node-a", now)}, Total: 0}, nil
		},
		pods: func(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.PodMetric], error) {
			return apiquery.ListResponse[k8sgateway.PodMetric]{}, k8sgateway.ErrMetricsAPIUnavailable
		},
	}
	collector, _ := NewCollector(testCollectorConfig(), collectorClusterStub{items: []cluster.Cluster{{ID: 1, Enabled: true}}}, metrics, history, nil)
	collector.now = func() time.Time { return now }
	if _, err := collector.CollectOnce(context.Background()); err != nil {
		t.Fatalf("CollectOnce() error = %v", err)
	}
	input := history.collections[0]
	if input.Nodes.Status != SourceFailed || input.Pods.Status != SourceUnavailable || input.FailureCode != FailureMetricsPayload {
		t.Fatalf("mixed failure input = %#v", input)
	}
}

func TestCollectorBoundsConcurrentMetricsRequests(t *testing.T) {
	var mu sync.Mutex
	active, maximum := 0, 0
	request := func(ctx context.Context) error {
		mu.Lock()
		active++
		if active > maximum {
			maximum = active
		}
		mu.Unlock()
		defer func() {
			mu.Lock()
			active--
			mu.Unlock()
		}()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
		return nil
	}
	metrics := collectorMetricsStub{
		nodes: func(ctx context.Context, _ int64, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.NodeMetric], error) {
			return apiquery.ListResponse[k8sgateway.NodeMetric]{}, request(ctx)
		},
		pods: func(ctx context.Context, _ int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.PodMetric], error) {
			return apiquery.ListResponse[k8sgateway.PodMetric]{}, request(ctx)
		},
	}
	config := testCollectorConfig()
	config.MaxConcurrentClusters = 2
	clusters := make([]cluster.Cluster, 5)
	for index := range clusters {
		clusters[index] = cluster.Cluster{ID: int64(index + 1), Enabled: true}
	}
	collector, _ := NewCollector(config, collectorClusterStub{items: clusters}, metrics, &collectorHistoryStub{}, nil)
	if _, err := collector.CollectOnce(context.Background()); err != nil {
		t.Fatalf("CollectOnce() error = %v", err)
	}
	if maximum > 4 {
		t.Fatalf("maximum concurrent Metrics API requests = %d, want <= 4", maximum)
	}
}

func TestCollectorRunPerformsImmediateCleanupAndCollection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	history := &collectorHistoryStub{onRecord: cancel}
	collector, _ := NewCollector(testCollectorConfig(), collectorClusterStub{items: []cluster.Cluster{{ID: 1, Enabled: true}}}, successfulCollectorMetrics(time.Now().UTC()), history, nil)
	collector.Run(ctx)
	if len(history.deletedAt) != 1 || len(history.collections) != 1 {
		t.Fatalf("cleanup=%d collections=%d", len(history.deletedAt), len(history.collections))
	}
}

func TestCollectorValidatesConfigurationAndListFailures(t *testing.T) {
	config := testCollectorConfig()
	config.MaxSamples = 1801
	if _, err := NewCollector(config, collectorClusterStub{}, successfulCollectorMetrics(time.Now()), &collectorHistoryStub{}, nil); !errors.Is(err, ErrInvalidCollectorConfig) {
		t.Fatalf("NewCollector() error = %v", err)
	}
	collector, _ := NewCollector(testCollectorConfig(), collectorClusterStub{err: errors.New("database unavailable")}, successfulCollectorMetrics(time.Now()), &collectorHistoryStub{}, nil)
	if _, err := collector.CollectOnce(context.Background()); err == nil {
		t.Fatal("CollectOnce() error = nil, want cluster list failure")
	}
}

func successfulCollectorMetrics(now time.Time) collectorMetricsStub {
	return collectorMetricsStub{
		nodes: func(context.Context, int64, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.NodeMetric], error) {
			return apiquery.ListResponse[k8sgateway.NodeMetric]{Items: []k8sgateway.NodeMetric{nodeMetric("node-a", now)}, Total: 1}, nil
		},
		pods: func(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.PodMetric], error) {
			return apiquery.ListResponse[k8sgateway.PodMetric]{Items: []k8sgateway.PodMetric{podMetric("default", "api-0", now)}, Total: 1}, nil
		},
	}
}

func nodeMetric(name string, now time.Time) k8sgateway.NodeMetric {
	return k8sgateway.NodeMetric{Metadata: k8sgateway.ObjectMeta{Name: name, UID: "node-uid-" + name}, Timestamp: now.Format(time.RFC3339Nano), Window: "15s", Usage: k8sgateway.ResourceUsage{CPU: "250m", Memory: "5Mi"}}
}

func podMetric(namespace, name string, now time.Time) k8sgateway.PodMetric {
	return k8sgateway.PodMetric{Metadata: k8sgateway.ObjectMeta{Namespace: namespace, Name: name, UID: "pod-uid-" + name}, Timestamp: now.Format(time.RFC3339Nano), Window: "15s", Containers: []k8sgateway.ContainerMetric{{Name: "app", Usage: k8sgateway.ResourceUsage{CPU: "125m", Memory: "2Mi"}}}}
}

func testCollectorConfig() CollectorConfig {
	return CollectorConfig{Enabled: true, CollectionInterval: time.Minute, PerClusterTimeout: time.Second, CleanupInterval: time.Hour, MaxClusters: 20, MaxConcurrentClusters: 4, MaxSamples: 1800}
}
