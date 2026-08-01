package fleet

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"k8s-aiops.local/backend/internal/apiquery"
	"k8s-aiops.local/backend/internal/cluster"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

const (
	StatusHealthy     = "healthy"
	StatusDegraded    = "degraded"
	StatusPartial     = "partial"
	StatusUnavailable = "unavailable"
	StatusTimedOut    = "timed_out"
)

var ErrInvalidLimit = errors.New("fleet limit is invalid")

type Config struct {
	MaxClusters           int
	MaxConcurrentClusters int
	PerClusterTimeout     time.Duration
	ResourceSampleLimit   int
}

type ClusterSource interface {
	List(context.Context) ([]cluster.Cluster, error)
}

type ResourceSource interface {
	Nodes(context.Context, int64, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Node], error)
	Pods(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Pod], error)
	Deployments(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Deployment], error)
	Events(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Event], error)
}

type Service struct {
	config    Config
	clusters  ClusterSource
	resources ResourceSource
	now       func() time.Time
}

type Limits struct {
	MaxClusters           int `json:"max_clusters"`
	MaxConcurrentClusters int `json:"max_concurrent_clusters"`
	PerClusterTimeoutMS   int `json:"per_cluster_timeout_ms"`
	ResourceSampleLimit   int `json:"resource_sample_limit"`
}

type ResourceSummary struct {
	Healthy  int  `json:"healthy"`
	Sampled  int  `json:"sampled"`
	Total    int  `json:"total"`
	Complete bool `json:"complete"`
}

type WarningSummary struct {
	Count    int  `json:"count"`
	Sampled  int  `json:"sampled"`
	Total    int  `json:"total"`
	Complete bool `json:"complete"`
}

type Failure struct {
	Scope string `json:"scope"`
	Code  string `json:"code"`
}

type ClusterHealth struct {
	ClusterID         int64           `json:"cluster_id"`
	ClusterName       string          `json:"cluster_name"`
	KubernetesVersion string          `json:"kubernetes_version,omitempty"`
	Status            string          `json:"status"`
	Nodes             ResourceSummary `json:"nodes"`
	Pods              ResourceSummary `json:"pods"`
	Deployments       ResourceSummary `json:"deployments"`
	Warnings          WarningSummary  `json:"warnings"`
	Failures          []Failure       `json:"failures"`
	DurationMS        int64           `json:"duration_ms"`
}

type Response struct {
	Items     []ClusterHealth `json:"items"`
	Total     int             `json:"total"`
	Remaining int             `json:"remaining"`
	CheckedAt time.Time       `json:"checked_at"`
	Limits    Limits          `json:"limits"`
}

func NewService(config Config, clusters ClusterSource, resources ResourceSource) *Service {
	if config.MaxClusters <= 0 {
		config.MaxClusters = 20
	}
	if config.MaxConcurrentClusters <= 0 {
		config.MaxConcurrentClusters = 4
	}
	if config.MaxConcurrentClusters > config.MaxClusters {
		config.MaxConcurrentClusters = config.MaxClusters
	}
	if config.PerClusterTimeout <= 0 {
		config.PerClusterTimeout = 4 * time.Second
	}
	if config.ResourceSampleLimit <= 0 {
		config.ResourceSampleLimit = 100
	}
	return &Service{config: config, clusters: clusters, resources: resources, now: time.Now}
}

func (s *Service) Compare(ctx context.Context, limit int, visibleClusters []int64) (Response, error) {
	if limit < 1 || limit > s.config.MaxClusters {
		return Response{}, ErrInvalidLimit
	}
	items, err := s.clusters.List(ctx)
	if err != nil {
		return Response{}, err
	}
	visible := make(map[int64]bool, len(visibleClusters))
	for _, id := range visibleClusters {
		visible[id] = true
	}
	enabled := make([]cluster.Cluster, 0, len(items))
	for _, item := range items {
		if !item.Enabled {
			continue
		}
		if visibleClusters != nil && !visible[item.ID] {
			continue
		}
		enabled = append(enabled, item)
	}
	sort.Slice(enabled, func(i, j int) bool { return enabled[i].ID < enabled[j].ID })
	total := len(enabled)
	if len(enabled) > limit {
		enabled = enabled[:limit]
	}

	results := make([]ClusterHealth, len(enabled))
	jobs := make(chan int)
	workers := s.config.MaxConcurrentClusters
	if workers > len(enabled) {
		workers = len(enabled)
	}
	var group sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for index := range jobs {
				results[index] = s.inspect(ctx, enabled[index])
			}
		}()
	}
	for index := range enabled {
		jobs <- index
	}
	close(jobs)
	group.Wait()

	return Response{
		Items: results, Total: total, Remaining: total - len(results), CheckedAt: s.now().UTC(),
		Limits: Limits{
			MaxClusters: s.config.MaxClusters, MaxConcurrentClusters: s.config.MaxConcurrentClusters,
			PerClusterTimeoutMS: int(s.config.PerClusterTimeout / time.Millisecond), ResourceSampleLimit: s.config.ResourceSampleLimit,
		},
	}, nil
}

func (s *Service) inspect(parent context.Context, item cluster.Cluster) ClusterHealth {
	started := time.Now()
	ctx, cancel := context.WithTimeout(parent, s.config.PerClusterTimeout)
	defer cancel()
	query := apiquery.ListQuery{Page: 1, Limit: s.config.ResourceSampleLimit, SortBy: "name", Ascending: true}
	result := ClusterHealth{
		ClusterID: item.ID, ClusterName: item.Name, KubernetesVersion: item.KubernetesVersion,
		Failures: []Failure{},
	}

	nodes, err := s.resources.Nodes(ctx, item.ID, query)
	if err != nil {
		result.Failures = append(result.Failures, failure(ctx, "nodes"))
	} else {
		result.Nodes = summarizeNodes(nodes)
	}
	pods, err := s.resources.Pods(ctx, item.ID, "", query)
	if err != nil {
		result.Failures = append(result.Failures, failure(ctx, "pods"))
	} else {
		result.Pods = summarizePods(pods)
	}
	deployments, err := s.resources.Deployments(ctx, item.ID, "", query)
	if err != nil {
		result.Failures = append(result.Failures, failure(ctx, "deployments"))
	} else {
		result.Deployments = summarizeDeployments(deployments)
	}
	events, err := s.resources.Events(ctx, item.ID, "", query)
	if err != nil {
		result.Failures = append(result.Failures, failure(ctx, "events"))
	} else {
		result.Warnings = summarizeWarnings(events)
	}

	result.Status = healthStatus(ctx, result)
	result.DurationMS = time.Since(started).Milliseconds()
	return result
}

func summarizeNodes(response apiquery.ListResponse[k8sgateway.Node]) ResourceSummary {
	healthy := 0
	for _, item := range response.Items {
		for _, condition := range item.Status.Conditions {
			if condition.Type == "Ready" && condition.Status == "True" {
				healthy++
				break
			}
		}
	}
	return resourceSummary(healthy, len(response.Items), response.Total)
}

func summarizePods(response apiquery.ListResponse[k8sgateway.Pod]) ResourceSummary {
	healthy := 0
	for _, item := range response.Items {
		if podHealthy(item) {
			healthy++
		}
	}
	return resourceSummary(healthy, len(response.Items), response.Total)
}

func summarizeDeployments(response apiquery.ListResponse[k8sgateway.Deployment]) ResourceSummary {
	healthy := 0
	for _, item := range response.Items {
		desired := int32(1)
		if item.Spec.Replicas != nil {
			desired = *item.Spec.Replicas
		}
		if desired == 0 || (item.Status.ReadyReplicas >= desired && item.Status.AvailableReplicas >= desired && item.Status.UnavailableReplicas == 0) {
			healthy++
		}
	}
	return resourceSummary(healthy, len(response.Items), response.Total)
}

func summarizeWarnings(response apiquery.ListResponse[k8sgateway.Event]) WarningSummary {
	count := 0
	for _, item := range response.Items {
		if item.Type == "Warning" {
			count++
		}
	}
	return WarningSummary{Count: count, Sampled: len(response.Items), Total: response.Total, Complete: len(response.Items) == response.Total}
}

func resourceSummary(healthy, sampled, total int) ResourceSummary {
	return ResourceSummary{Healthy: healthy, Sampled: sampled, Total: total, Complete: sampled == total}
}

func podHealthy(item k8sgateway.Pod) bool {
	if item.Status.Phase == "Succeeded" {
		return true
	}
	if item.Status.Phase != "Running" || len(item.Status.ContainerStatuses) == 0 {
		return false
	}
	for _, status := range item.Status.ContainerStatuses {
		if !status.Ready {
			return false
		}
	}
	return true
}

func failure(ctx context.Context, scope string) Failure {
	code := "QUERY_FAILED"
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		code = "TIMEOUT"
	}
	return Failure{Scope: scope, Code: code}
}

func healthStatus(ctx context.Context, result ClusterHealth) string {
	if len(result.Failures) == 4 {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return StatusTimedOut
		}
		return StatusUnavailable
	}
	if len(result.Failures) > 0 || !result.Nodes.Complete || !result.Pods.Complete || !result.Deployments.Complete || !result.Warnings.Complete {
		return StatusPartial
	}
	if result.Nodes.Total == 0 || result.Nodes.Healthy < result.Nodes.Total || result.Pods.Healthy < result.Pods.Total || result.Deployments.Healthy < result.Deployments.Total || result.Warnings.Count > 0 {
		return StatusDegraded
	}
	return StatusHealthy
}
