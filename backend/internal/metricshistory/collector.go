package metricshistory

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"

	"k8s-aiops.local/backend/internal/apiquery"
	"k8s-aiops.local/backend/internal/cluster"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

const (
	FailureMetricsAPIUnavailable = "METRICS_API_UNAVAILABLE"
	FailureMetricsAPITimeout     = "METRICS_API_TIMEOUT"
	FailureMetricsAPIRequest     = "METRICS_API_REQUEST_FAILED"
	FailureMetricsQuantity       = "METRICS_QUANTITY_INVALID"
	FailureMetricsPayload        = "METRICS_PAYLOAD_INVALID"
	FailureCollectionLimit       = "COLLECTION_LIMIT_REACHED"
)

var ErrInvalidCollectorConfig = errors.New("metrics history collector configuration is invalid")

type CollectorConfig struct {
	Enabled               bool
	CollectionInterval    time.Duration
	PerClusterTimeout     time.Duration
	CleanupInterval       time.Duration
	MaxClusters           int
	MaxConcurrentClusters int
	MaxSamples            int
}

type ClusterCollectionSummary struct {
	Attempted int
	Recorded  int
	Failed    int
	Skipped   int
}

type clusterSource interface {
	List(context.Context) ([]cluster.Cluster, error)
}

type metricsSource interface {
	NodeMetrics(context.Context, int64, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.NodeMetric], error)
	PodMetrics(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.PodMetric], error)
}

type historyStore interface {
	Record(context.Context, CollectionInput) (CollectionRun, error)
	Cleanup(context.Context, time.Time) (int64, error)
}

type Collector struct {
	config   CollectorConfig
	clusters clusterSource
	metrics  metricsSource
	history  historyStore
	logger   *zap.Logger
	now      func() time.Time
}

func NewCollector(config CollectorConfig, clusters clusterSource, metrics metricsSource, history historyStore, logger *zap.Logger) (*Collector, error) {
	if clusters == nil || metrics == nil || history == nil || !validCollectorConfig(config) {
		return nil, ErrInvalidCollectorConfig
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Collector{config: config, clusters: clusters, metrics: metrics, history: history, logger: logger, now: time.Now}, nil
}

func (c *Collector) Run(ctx context.Context) {
	if !c.config.Enabled {
		return
	}
	c.cleanup(ctx)
	c.collect(ctx)
	collectionTicker := time.NewTicker(c.config.CollectionInterval)
	cleanupTicker := time.NewTicker(c.config.CleanupInterval)
	defer collectionTicker.Stop()
	defer cleanupTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-collectionTicker.C:
			c.collect(ctx)
		case <-cleanupTicker.C:
			c.cleanup(ctx)
		}
	}
}

func (c *Collector) CollectOnce(ctx context.Context) (ClusterCollectionSummary, error) {
	if !c.config.Enabled {
		return ClusterCollectionSummary{}, nil
	}
	clusters, err := c.clusters.List(ctx)
	if err != nil {
		return ClusterCollectionSummary{}, err
	}
	enabled := make([]cluster.Cluster, 0, len(clusters))
	for _, item := range clusters {
		if item.Enabled {
			enabled = append(enabled, item)
		}
	}
	sort.Slice(enabled, func(i, j int) bool { return enabled[i].ID < enabled[j].ID })
	summary := ClusterCollectionSummary{}
	if len(enabled) > c.config.MaxClusters {
		summary.Skipped = len(enabled) - c.config.MaxClusters
		enabled = enabled[:c.config.MaxClusters]
	}
	summary.Attempted = len(enabled)

	semaphore := make(chan struct{}, c.config.MaxConcurrentClusters)
	var wait sync.WaitGroup
	var mu sync.Mutex
	for _, item := range enabled {
		if ctx.Err() != nil {
			break
		}
		semaphore <- struct{}{}
		wait.Add(1)
		go func(clusterID int64) {
			defer wait.Done()
			defer func() { <-semaphore }()
			err := c.collectCluster(ctx, clusterID)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				summary.Failed++
				if !errors.Is(err, context.Canceled) {
					c.logger.Error("record metrics history collection", zap.Int64("cluster_id", clusterID), zap.Error(err))
				}
				return
			}
			summary.Recorded++
		}(item.ID)
	}
	wait.Wait()
	return summary, nil
}

func (c *Collector) CleanupOnce(ctx context.Context) (int64, error) {
	if !c.config.Enabled {
		return 0, nil
	}
	return c.history.Cleanup(ctx, c.now().UTC())
}

func (c *Collector) collectCluster(parent context.Context, clusterID int64) error {
	startedAt := c.now().UTC()
	ctx, cancel := context.WithTimeout(parent, c.config.PerClusterTimeout)
	defer cancel()
	query := apiquery.ListQuery{Page: 1, Limit: c.config.MaxSamples, SortBy: "name", Ascending: true}

	var nodes apiquery.ListResponse[k8sgateway.NodeMetric]
	var pods apiquery.ListResponse[k8sgateway.PodMetric]
	var nodeErr, podErr error
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		nodes, nodeErr = c.metrics.NodeMetrics(ctx, clusterID, query)
	}()
	go func() {
		defer wait.Done()
		pods, podErr = c.metrics.PodMetrics(ctx, clusterID, "", query)
	}()
	wait.Wait()
	if parent.Err() != nil {
		return parent.Err()
	}

	nodeSnapshot := nodeSourceSnapshot(nodes, nodeErr)
	podSnapshot := podSourceSnapshot(pods, podErr)
	samples, nodeSampled, podSampled, limited := allocateSamples(nodeSnapshot.bundles, podSnapshot.bundles, c.config.MaxSamples)
	nodeCoverage := coverageFor(nodeSnapshot, nodeSampled)
	podCoverage := coverageFor(podSnapshot, podSampled)
	failureCode := collectionFailureCode(nodeSnapshot, podSnapshot, nodeCoverage, podCoverage, limited)
	_, err := c.history.Record(parent, CollectionInput{
		ClusterID: clusterID, Nodes: nodeCoverage, Pods: podCoverage, FailureCode: failureCode,
		StartedAt: startedAt, CompletedAt: c.now().UTC(), Samples: samples,
	})
	return err
}

type sampleBundle struct{ samples []SampleInput }

type sourceSnapshot struct {
	status  string
	code    string
	total   int
	bundles []sampleBundle
}

func nodeSourceSnapshot(response apiquery.ListResponse[k8sgateway.NodeMetric], err error) sourceSnapshot {
	if err != nil {
		status, code := sourceFailure(err)
		return sourceSnapshot{status: status, code: code}
	}
	if response.Total < 0 || response.Total < len(response.Items) {
		return sourceSnapshot{status: SourceFailed, code: FailureMetricsPayload}
	}
	items := append([]k8sgateway.NodeMetric(nil), response.Items...)
	sort.SliceStable(items, func(i, j int) bool { return items[i].Metadata.Name < items[j].Metadata.Name })
	bundles := make([]sampleBundle, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if item.Metadata.Name == "" {
			return sourceSnapshot{status: SourceFailed, code: FailureMetricsPayload}
		}
		if _, duplicate := seen[item.Metadata.Name]; duplicate {
			return sourceSnapshot{status: SourceFailed, code: FailureMetricsPayload}
		}
		seen[item.Metadata.Name] = struct{}{}
		timestamp, window, ok := metricTime(item.Timestamp, item.Window)
		if !ok {
			return sourceSnapshot{status: SourceFailed, code: FailureMetricsPayload}
		}
		samples, ok := usageSamples(ResourceNode, "", item.Metadata.Name, item.Metadata.UID, "", item.Usage, timestamp, window)
		if !ok {
			return sourceSnapshot{status: SourceFailed, code: FailureMetricsQuantity}
		}
		bundles = append(bundles, sampleBundle{samples: samples})
	}
	return sourceSnapshot{status: SourceSucceeded, total: response.Total, bundles: bundles}
}

func podSourceSnapshot(response apiquery.ListResponse[k8sgateway.PodMetric], err error) sourceSnapshot {
	if err != nil {
		status, code := sourceFailure(err)
		return sourceSnapshot{status: status, code: code}
	}
	if response.Total < 0 || response.Total < len(response.Items) {
		return sourceSnapshot{status: SourceFailed, code: FailureMetricsPayload}
	}
	items := append([]k8sgateway.PodMetric(nil), response.Items...)
	sort.SliceStable(items, func(i, j int) bool {
		left := items[i].Metadata.Namespace + "\x00" + items[i].Metadata.Name
		right := items[j].Metadata.Namespace + "\x00" + items[j].Metadata.Name
		return left < right
	})
	bundles := make([]sampleBundle, 0, len(items))
	seenPods := make(map[string]struct{}, len(items))
	for _, item := range items {
		podKey := item.Metadata.Namespace + "\x00" + item.Metadata.Name
		if item.Metadata.Namespace == "" || item.Metadata.Name == "" {
			return sourceSnapshot{status: SourceFailed, code: FailureMetricsPayload}
		}
		if _, duplicate := seenPods[podKey]; duplicate {
			return sourceSnapshot{status: SourceFailed, code: FailureMetricsPayload}
		}
		seenPods[podKey] = struct{}{}
		timestamp, window, ok := metricTime(item.Timestamp, item.Window)
		if !ok {
			return sourceSnapshot{status: SourceFailed, code: FailureMetricsPayload}
		}
		containers := append([]k8sgateway.ContainerMetric(nil), item.Containers...)
		if len(containers) == 0 {
			return sourceSnapshot{status: SourceFailed, code: FailureMetricsPayload}
		}
		sort.SliceStable(containers, func(i, j int) bool { return containers[i].Name < containers[j].Name })
		bundle := sampleBundle{samples: make([]SampleInput, 0, len(containers)*2)}
		seenContainers := make(map[string]struct{}, len(containers))
		for _, container := range containers {
			if container.Name == "" {
				return sourceSnapshot{status: SourceFailed, code: FailureMetricsPayload}
			}
			if _, duplicate := seenContainers[container.Name]; duplicate {
				return sourceSnapshot{status: SourceFailed, code: FailureMetricsPayload}
			}
			seenContainers[container.Name] = struct{}{}
			samples, ok := usageSamples(ResourcePod, item.Metadata.Namespace, item.Metadata.Name, item.Metadata.UID, container.Name, container.Usage, timestamp, window)
			if !ok {
				return sourceSnapshot{status: SourceFailed, code: FailureMetricsQuantity}
			}
			bundle.samples = append(bundle.samples, samples...)
		}
		bundles = append(bundles, bundle)
	}
	return sourceSnapshot{status: SourceSucceeded, total: response.Total, bundles: bundles}
}

func usageSamples(kind, namespace, name, uid, container string, usage k8sgateway.ResourceUsage, timestamp time.Time, window time.Duration) ([]SampleInput, bool) {
	cpu, err := cpuNanocores(usage.CPU)
	if err != nil {
		return nil, false
	}
	memory, err := memoryBytes(usage.Memory)
	if err != nil {
		return nil, false
	}
	base := SampleInput{ResourceKind: kind, ResourceNamespace: namespace, ResourceName: name, ResourceUID: uid, ContainerName: container, SourceTimestamp: timestamp, Window: window}
	cpuSample := base
	cpuSample.MetricName, cpuSample.Value = MetricCPU, cpu
	memorySample := base
	memorySample.MetricName, memorySample.Value = MetricMemory, memory
	return []SampleInput{cpuSample, memorySample}, true
}

func metricTime(timestamp, window string) (time.Time, time.Duration, bool) {
	parsedTimestamp, err := time.Parse(time.RFC3339Nano, timestamp)
	if err != nil || parsedTimestamp.IsZero() {
		return time.Time{}, 0, false
	}
	parsedWindow, err := time.ParseDuration(window)
	if err != nil || parsedWindow < time.Second || parsedWindow > time.Hour {
		return time.Time{}, 0, false
	}
	return parsedTimestamp.UTC(), parsedWindow, true
}

func allocateSamples(nodes, pods []sampleBundle, maximum int) ([]SampleInput, int, int, bool) {
	samples := make([]SampleInput, 0, maximum)
	nodeIndex, podIndex := 0, 0
	nodeSampled, podSampled := 0, 0
	limited := false
	for nodeIndex < len(nodes) || podIndex < len(pods) {
		if nodeIndex < len(nodes) {
			bundle := nodes[nodeIndex]
			nodeIndex++
			if len(samples)+len(bundle.samples) <= maximum {
				samples = append(samples, bundle.samples...)
				nodeSampled++
			} else {
				limited = true
			}
		}
		if podIndex < len(pods) {
			bundle := pods[podIndex]
			podIndex++
			if len(samples)+len(bundle.samples) <= maximum {
				samples = append(samples, bundle.samples...)
				podSampled++
			} else {
				limited = true
			}
		}
	}
	return samples, nodeSampled, podSampled, limited
}

func coverageFor(snapshot sourceSnapshot, sampled int) TargetCoverage {
	if snapshot.status != SourceSucceeded {
		return TargetCoverage{Status: snapshot.status}
	}
	return TargetCoverage{Status: SourceSucceeded, Sampled: sampled, Total: snapshot.total, Complete: sampled == snapshot.total}
}

func sourceFailure(err error) (string, string) {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return SourceTimedOut, FailureMetricsAPITimeout
	}
	if errors.Is(err, k8sgateway.ErrMetricsAPIUnavailable) {
		return SourceUnavailable, FailureMetricsAPIUnavailable
	}
	return SourceFailed, FailureMetricsAPIRequest
}

func collectionFailureCode(nodes, pods sourceSnapshot, nodeCoverage, podCoverage TargetCoverage, limited bool) string {
	status := collectionStatus(nodeCoverage, podCoverage)
	if status == CollectionSucceeded {
		return ""
	}
	if status == CollectionTimedOut {
		return FailureMetricsAPITimeout
	}
	if status == CollectionUnavailable {
		return FailureMetricsAPIUnavailable
	}
	if status == CollectionFailed {
		if code := firstFailureCode(nodes.code, pods.code, FailureMetricsPayload, FailureMetricsQuantity, FailureMetricsAPIRequest, FailureMetricsAPIUnavailable); code != "" {
			return code
		}
	}
	if code := firstFailureCode(nodes.code, pods.code, FailureMetricsAPITimeout, FailureMetricsAPIUnavailable, FailureMetricsPayload, FailureMetricsQuantity, FailureMetricsAPIRequest); code != "" {
		return code
	}
	if limited || !nodeCoverage.Complete || !podCoverage.Complete {
		return FailureCollectionLimit
	}
	return FailureMetricsAPIRequest
}

func firstFailureCode(left, right string, priorities ...string) string {
	for _, candidate := range priorities {
		if left == candidate || right == candidate {
			return candidate
		}
	}
	return ""
}

func (c *Collector) collect(ctx context.Context) {
	summary, err := c.CollectOnce(ctx)
	if err != nil {
		if ctx.Err() == nil {
			c.logger.Error("list clusters for metrics history", zap.Error(err))
		}
		return
	}
	c.logger.Info("metrics history collection completed", zap.Int("attempted", summary.Attempted), zap.Int("recorded", summary.Recorded), zap.Int("failed", summary.Failed), zap.Int("skipped", summary.Skipped))
}

func (c *Collector) cleanup(ctx context.Context) {
	deleted, err := c.CleanupOnce(ctx)
	if err != nil {
		if ctx.Err() == nil {
			c.logger.Error("clean expired metrics history", zap.Error(err))
		}
		return
	}
	if deleted > 0 {
		c.logger.Info("expired metrics history cleaned", zap.Int64("collections", deleted))
	}
}

func validCollectorConfig(config CollectorConfig) bool {
	return config.CollectionInterval >= 15*time.Second && config.CollectionInterval <= 24*time.Hour &&
		config.PerClusterTimeout >= time.Second && config.PerClusterTimeout <= time.Minute &&
		config.CleanupInterval >= time.Minute && config.CleanupInterval <= 24*time.Hour &&
		config.MaxClusters >= 1 && config.MaxClusters <= 20 &&
		config.MaxConcurrentClusters >= 1 && config.MaxConcurrentClusters <= 4 &&
		config.MaxConcurrentClusters <= config.MaxClusters &&
		config.MaxSamples >= 1 && config.MaxSamples <= maxSamplesPerCollection
}
