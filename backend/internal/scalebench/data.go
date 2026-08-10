package scalebench

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"k8s-aiops.local/backend/internal/apiquery"
	"k8s-aiops.local/backend/internal/cluster"
	"k8s-aiops.local/backend/internal/globalsearch"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
	"k8s-aiops.local/backend/internal/scalefixture"
	"k8s-aiops.local/backend/internal/topology"
)

type namespaceData struct {
	workloads []scalefixture.WorkloadRecord
	pods      []k8sgateway.Pod
	events    []k8sgateway.Event
}

type Data struct {
	Config      scalefixture.Config
	Manifest    scalefixture.Manifest
	Nodes       []k8sgateway.Node
	Workloads   []scalefixture.WorkloadRecord
	Pods        []k8sgateway.Pod
	Events      []k8sgateway.Event
	Namespaces  []string
	ByNamespace map[string]*namespaceData
	History     map[string][]scalefixture.HistorySample
	search      *globalsearch.Service
}

type fixtureClusters struct{ cluster cluster.Cluster }

func (s fixtureClusters) List(context.Context) ([]cluster.Cluster, error) {
	return []cluster.Cluster{s.cluster}, nil
}

type fixtureResources struct{ data *Data }

func (s fixtureResources) Pods(ctx context.Context, _ int64, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Pod], error) {
	return s.data.PagePods(ctx, namespace, query)
}

func (s fixtureResources) Deployments(ctx context.Context, _ int64, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Deployment], error) {
	return pageWorkloads(ctx, s.data.workloads(namespace), query, func(item scalefixture.WorkloadRecord) k8sgateway.Deployment { return item.Deployment }, func(item k8sgateway.Deployment) string { return item.Metadata.Name })
}

func (s fixtureResources) Services(ctx context.Context, _ int64, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.ServiceResource], error) {
	return pageWorkloads(ctx, s.data.workloads(namespace), query, func(item scalefixture.WorkloadRecord) k8sgateway.ServiceResource { return item.Service }, func(item k8sgateway.ServiceResource) string { return item.Metadata.Name })
}

func (s fixtureResources) Ingresses(ctx context.Context, _ int64, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Ingress], error) {
	return pageWorkloads(ctx, s.data.workloads(namespace), query, func(item scalefixture.WorkloadRecord) k8sgateway.Ingress { return item.Ingress }, func(item k8sgateway.Ingress) string { return item.Metadata.Name })
}

func Load(ctx context.Context, fixtureDir string, config scalefixture.Config, manifest scalefixture.Manifest) (*Data, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	configHash, err := config.Hash()
	if err != nil || configHash != manifest.ConfigSHA256 {
		return nil, fmt.Errorf("fixture config does not match manifest")
	}
	data := &Data{
		Config: config, Manifest: manifest, ByNamespace: make(map[string]*namespaceData),
		History: make(map[string][]scalefixture.HistorySample),
	}
	for index := 0; index < config.NamespaceCount; index++ {
		namespace := fmt.Sprintf("fleet-%03d", index)
		data.Namespaces = append(data.Namespaces, namespace)
		data.ByNamespace[namespace] = &namespaceData{}
	}
	if err := readStream(ctx, filepath.Join(fixtureDir, "nodes.ndjson.gz"), func(decoder *json.Decoder) error {
		var item k8sgateway.Node
		if err := decoder.Decode(&item); err != nil {
			return err
		}
		data.Nodes = append(data.Nodes, item)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("load nodes: %w", err)
	}
	if err := readStream(ctx, filepath.Join(fixtureDir, "workloads.ndjson.gz"), func(decoder *json.Decoder) error {
		var item scalefixture.WorkloadRecord
		if err := decoder.Decode(&item); err != nil {
			return err
		}
		data.Workloads = append(data.Workloads, item)
		resources := data.ByNamespace[item.Deployment.Metadata.Namespace]
		if resources == nil {
			return fmt.Errorf("unknown workload namespace %q", item.Deployment.Metadata.Namespace)
		}
		resources.workloads = append(resources.workloads, item)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("load workloads: %w", err)
	}
	if err := readStream(ctx, filepath.Join(fixtureDir, "pods.ndjson.gz"), func(decoder *json.Decoder) error {
		var item k8sgateway.Pod
		if err := decoder.Decode(&item); err != nil {
			return err
		}
		data.Pods = append(data.Pods, item)
		resources := data.ByNamespace[item.Metadata.Namespace]
		if resources == nil {
			return fmt.Errorf("unknown pod namespace %q", item.Metadata.Namespace)
		}
		resources.pods = append(resources.pods, item)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("load pods: %w", err)
	}
	if err := readStream(ctx, filepath.Join(fixtureDir, "events.ndjson.gz"), func(decoder *json.Decoder) error {
		var item k8sgateway.Event
		if err := decoder.Decode(&item); err != nil {
			return err
		}
		data.Events = append(data.Events, item)
		resources := data.ByNamespace[item.Metadata.Namespace]
		if resources == nil {
			return fmt.Errorf("unknown event namespace %q", item.Metadata.Namespace)
		}
		resources.events = append(resources.events, item)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("load events: %w", err)
	}
	targetNode := scalefixture.Node(config, 0)
	targetPod := scalefixture.Pod(config, 0)
	for _, key := range []string{historyKey("Node", "", targetNode.Metadata.Name, "", "cpu"), historyKey("Pod", targetPod.Metadata.Namespace, targetPod.Metadata.Name, "app", "cpu")} {
		data.History[key] = make([]scalefixture.HistorySample, 0, config.HistoryPoints)
	}
	if err := readStream(ctx, filepath.Join(fixtureDir, "history.ndjson.gz"), func(decoder *json.Decoder) error {
		var item scalefixture.HistorySample
		if err := decoder.Decode(&item); err != nil {
			return err
		}
		key := historyKey(item.ResourceKind, item.ResourceNamespace, item.ResourceName, item.ContainerName, item.MetricName)
		if _, ok := data.History[key]; ok {
			data.History[key] = append(data.History[key], item)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("load history: %w", err)
	}
	sort.Slice(data.Nodes, func(i, j int) bool { return data.Nodes[i].Metadata.Name < data.Nodes[j].Metadata.Name })
	sort.Slice(data.Workloads, func(i, j int) bool {
		return data.Workloads[i].Deployment.Metadata.Name < data.Workloads[j].Deployment.Metadata.Name
	})
	sort.Slice(data.Pods, func(i, j int) bool { return data.Pods[i].Metadata.Name < data.Pods[j].Metadata.Name })
	for _, resources := range data.ByNamespace {
		sort.Slice(resources.workloads, func(i, j int) bool {
			return resources.workloads[i].Deployment.Metadata.Name < resources.workloads[j].Deployment.Metadata.Name
		})
		sort.Slice(resources.pods, func(i, j int) bool { return resources.pods[i].Metadata.Name < resources.pods[j].Metadata.Name })
		sort.Slice(resources.events, func(i, j int) bool { return resources.events[i].Metadata.Name < resources.events[j].Metadata.Name })
	}
	data.search = globalsearch.NewService(
		globalsearch.Config{MaxClusters: 1, MaxConcurrentClusters: 1, MaxResults: 100, PerKindLimit: 100},
		fixtureClusters{cluster: cluster.Cluster{ID: config.ClusterID, Name: "m96-fixture", Enabled: true}},
		fixtureResources{data: data},
	)
	return data, nil
}

func (d *Data) Search(ctx context.Context, term string) (globalsearch.Response, error) {
	return d.search.Search(ctx, globalsearch.Query{Term: term, ClusterLimit: 1, ResultLimit: 100}, []int64{d.Config.ClusterID})
}

func (d *Data) DeriveTopology(ctx context.Context) (int, error) {
	collector := topology.NewCollector(nil, 100)
	total := 0
	now := d.Config.ObservedAt
	for _, namespace := range d.Namespaces {
		resources := d.ByNamespace[namespace]
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		snapshot := topology.CollectorSnapshot{ClusterID: d.Config.ClusterID, Namespace: namespace, Pods: resources.pods}
		for _, record := range resources.workloads {
			snapshot.Deployments = append(snapshot.Deployments, record.Deployment)
			snapshot.ReplicaSets = append(snapshot.ReplicaSets, record.ReplicaSet)
			snapshot.Services = append(snapshot.Services, record.Service)
			snapshot.Ingresses = append(snapshot.Ingresses, record.Ingress)
		}
		total += len(collector.DeriveEdges(snapshot, now))
	}
	return total, nil
}

func (d *Data) PagePods(ctx context.Context, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Pod], error) {
	resources := d.ByNamespace[namespace]
	if resources == nil {
		return apiquery.ListResponse[k8sgateway.Pod]{}, nil
	}
	return pageItems(ctx, resources.pods, query, func(item k8sgateway.Pod) string { return item.Metadata.Name })
}

func (d *Data) PageEvents(ctx context.Context, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Event], error) {
	resources := d.ByNamespace[namespace]
	if resources == nil {
		return apiquery.ListResponse[k8sgateway.Event]{}, nil
	}
	return pageItems(ctx, resources.events, query, func(item k8sgateway.Event) string { return item.Metadata.Name })
}

func (d *Data) QueryHistory(ctx context.Context, kind, namespace, name, container, metric string, limit int) ([]scalefixture.HistorySample, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	items := d.History[historyKey(kind, namespace, name, container, metric)]
	if limit < 1 {
		return nil, fmt.Errorf("history limit must be positive")
	}
	if limit > len(items) {
		limit = len(items)
	}
	return append([]scalefixture.HistorySample(nil), items[:limit]...), nil
}

type BackpressureResult struct {
	Records       int64
	QueueCapacity int
	MaxQueue      int
}

func (d *Data) StreamPods(ctx context.Context, namespace string, queueCapacity int) (BackpressureResult, error) {
	if queueCapacity < 1 {
		return BackpressureResult{}, fmt.Errorf("queue capacity must be positive")
	}
	if err := ctx.Err(); err != nil {
		return BackpressureResult{}, err
	}
	resources := d.ByNamespace[namespace]
	if resources == nil {
		return BackpressureResult{}, nil
	}
	queue := make(chan k8sgateway.Pod, queueCapacity)
	producerErr := make(chan error, 1)
	go func() {
		defer close(queue)
		for _, item := range resources.pods {
			select {
			case <-ctx.Done():
				producerErr <- ctx.Err()
				return
			case queue <- item:
			}
		}
		producerErr <- nil
	}()
	result := BackpressureResult{QueueCapacity: queueCapacity}
	for range queue {
		if len(queue) > result.MaxQueue {
			result.MaxQueue = len(queue)
		}
		result.Records++
	}
	if err := <-producerErr; err != nil {
		return BackpressureResult{}, err
	}
	return result, nil
}

func readStream(ctx context.Context, path string, decode func(*json.Decoder) error) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzipReader.Close()
	reader := bufio.NewReaderSize(gzipReader, 1<<20)
	decoder := json.NewDecoder(reader)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := decode(decoder); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

func pageItems[T any](ctx context.Context, items []T, query apiquery.ListQuery, name func(T) string) (apiquery.ListResponse[T], error) {
	result := apiquery.ListResponse[T]{Items: make([]T, 0, query.Limit)}
	matched, returned := 0, 0
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return apiquery.ListResponse[T]{}, err
		}
		if query.Name != "" && !strings.Contains(strings.ToLower(name(item)), strings.ToLower(query.Name)) {
			continue
		}
		matched++
		if matched <= query.Offset || returned >= query.Limit {
			continue
		}
		result.Items = append(result.Items, item)
		returned++
	}
	result.Total = matched
	result.Remaining = matched - query.Offset - returned
	if result.Remaining < 0 {
		result.Remaining = 0
	}
	return result, nil
}

func pageWorkloads[T any](ctx context.Context, records []scalefixture.WorkloadRecord, query apiquery.ListQuery, project func(scalefixture.WorkloadRecord) T, name func(T) string) (apiquery.ListResponse[T], error) {
	result := apiquery.ListResponse[T]{Items: make([]T, 0, query.Limit)}
	matched, returned := 0, 0
	for _, record := range records {
		if err := ctx.Err(); err != nil {
			return apiquery.ListResponse[T]{}, err
		}
		item := project(record)
		if query.Name != "" && !strings.Contains(strings.ToLower(name(item)), strings.ToLower(query.Name)) {
			continue
		}
		matched++
		if matched <= query.Offset || returned >= query.Limit {
			continue
		}
		result.Items = append(result.Items, item)
		returned++
	}
	result.Total = matched
	result.Remaining = matched - query.Offset - returned
	if result.Remaining < 0 {
		result.Remaining = 0
	}
	return result, nil
}

func (d *Data) workloads(namespace string) []scalefixture.WorkloadRecord {
	if namespace == "" {
		return d.Workloads
	}
	if resources := d.ByNamespace[namespace]; resources != nil {
		return resources.workloads
	}
	return nil
}

func historyKey(kind, namespace, name, container, metric string) string {
	return strings.Join([]string{kind, namespace, name, container, metric}, "\x00")
}
