package globalsearch

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"k8s-aiops.local/backend/internal/apiquery"
	"k8s-aiops.local/backend/internal/cluster"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

type Kind string

const (
	KindPod        Kind = "Pod"
	KindDeployment Kind = "Deployment"
	KindService    Kind = "Service"
	KindIngress    Kind = "Ingress"

	FailureQueryFailed = "QUERY_FAILED"
	FailureTimeout     = "TIMEOUT"
)

var (
	ErrInvalidQuery = errors.New("global search query is invalid")
	namespaceName   = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	supportedKinds  = []Kind{KindPod, KindDeployment, KindService, KindIngress}
)

type Config struct {
	MaxClusters           int
	MaxConcurrentClusters int
	PerClusterTimeout     time.Duration
	MaxResults            int
	PerKindLimit          int
}

type ClusterSource interface {
	List(context.Context) ([]cluster.Cluster, error)
}

type ResourceSource interface {
	Pods(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Pod], error)
	Deployments(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Deployment], error)
	Services(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.ServiceResource], error)
	Ingresses(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Ingress], error)
}

type Query struct {
	Term         string
	Namespace    string
	Kinds        []Kind
	ClusterLimit int
	ResultLimit  int
}

type Limits struct {
	MaxClusters           int `json:"max_clusters"`
	MaxConcurrentClusters int `json:"max_concurrent_clusters"`
	PerClusterTimeoutMS   int `json:"per_cluster_timeout_ms"`
	MaxResults            int `json:"max_results"`
	PerKindLimit          int `json:"per_kind_limit"`
}

type Item struct {
	ClusterID   int64  `json:"cluster_id"`
	ClusterName string `json:"cluster_name"`
	Kind        Kind   `json:"kind"`
	Namespace   string `json:"namespace"`
	Name        string `json:"name"`
	Health      string `json:"health"`
	Summary     string `json:"summary"`
}

type Failure struct {
	ClusterID   int64  `json:"cluster_id"`
	ClusterName string `json:"cluster_name"`
	Kind        Kind   `json:"kind"`
	Code        string `json:"code"`
}

type Response struct {
	Query             string    `json:"query"`
	Namespace         string    `json:"namespace,omitempty"`
	Kinds             []Kind    `json:"kinds"`
	Items             []Item    `json:"items"`
	Total             int       `json:"total"`
	Remaining         int       `json:"remaining"`
	ClustersTotal     int       `json:"clusters_total"`
	ClustersSearched  int       `json:"clusters_searched"`
	ClustersRemaining int       `json:"clusters_remaining"`
	Complete          bool      `json:"complete"`
	Failures          []Failure `json:"failures"`
	CheckedAt         time.Time `json:"checked_at"`
	Limits            Limits    `json:"limits"`
}

type Service struct {
	config    Config
	clusters  ClusterSource
	resources ResourceSource
	now       func() time.Time
}

type clusterResult struct {
	items    []Item
	total    int
	failures []Failure
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
	if config.MaxResults <= 0 {
		config.MaxResults = 100
	}
	if config.PerKindLimit <= 0 {
		config.PerKindLimit = 100
	}
	return &Service{config: config, clusters: clusters, resources: resources, now: time.Now}
}

func SupportedKinds() []Kind {
	return append([]Kind(nil), supportedKinds...)
}

func ParseKinds(raw string) ([]Kind, error) {
	if strings.TrimSpace(raw) == "" {
		return SupportedKinds(), nil
	}
	lookup := map[string]Kind{
		"pods": KindPod, "deployments": KindDeployment,
		"services": KindService, "ingresses": KindIngress,
	}
	seen := map[Kind]bool{}
	for _, value := range strings.Split(raw, ",") {
		kind, ok := lookup[strings.ToLower(strings.TrimSpace(value))]
		if !ok || seen[kind] {
			return nil, ErrInvalidQuery
		}
		seen[kind] = true
	}
	result := make([]Kind, 0, len(seen))
	for _, kind := range supportedKinds {
		if seen[kind] {
			result = append(result, kind)
		}
	}
	return result, nil
}

func (s *Service) Search(ctx context.Context, query Query) (Response, error) {
	query.Term = strings.TrimSpace(query.Term)
	query.Namespace = strings.TrimSpace(query.Namespace)
	if len(query.Kinds) == 0 {
		query.Kinds = SupportedKinds()
	}
	if err := s.validate(query); err != nil {
		return Response{}, err
	}

	clusters, err := s.clusters.List(ctx)
	if err != nil {
		return Response{}, err
	}
	enabled := make([]cluster.Cluster, 0, len(clusters))
	for _, item := range clusters {
		if item.Enabled {
			enabled = append(enabled, item)
		}
	}
	sort.Slice(enabled, func(i, j int) bool { return enabled[i].ID < enabled[j].ID })
	clustersTotal := len(enabled)
	if len(enabled) > query.ClusterLimit {
		enabled = enabled[:query.ClusterLimit]
	}
	clustersRemaining := clustersTotal - len(enabled)

	results := make([]clusterResult, len(enabled))
	workers := s.config.MaxConcurrentClusters
	if workers > len(enabled) {
		workers = len(enabled)
	}
	jobs := make(chan int)
	var group sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for index := range jobs {
				results[index] = s.searchCluster(ctx, enabled[index], query)
			}
		}()
	}
	for index := range enabled {
		jobs <- index
	}
	close(jobs)
	group.Wait()

	items := []Item{}
	failures := []Failure{}
	total := 0
	for _, result := range results {
		items = append(items, result.items...)
		failures = append(failures, result.failures...)
		total += result.total
	}
	sortItems(items)
	if len(items) > query.ResultLimit {
		items = items[:query.ResultLimit]
	}
	remaining := total - len(items)
	if remaining < 0 {
		remaining = 0
	}
	return Response{
		Query: query.Term, Namespace: query.Namespace, Kinds: append([]Kind(nil), query.Kinds...),
		Items: items, Total: total, Remaining: remaining,
		ClustersTotal: clustersTotal, ClustersSearched: len(enabled), ClustersRemaining: clustersRemaining,
		Complete: len(failures) == 0 && remaining == 0 && clustersRemaining == 0, Failures: failures,
		CheckedAt: s.now().UTC(), Limits: Limits{
			MaxClusters: s.config.MaxClusters, MaxConcurrentClusters: s.config.MaxConcurrentClusters,
			PerClusterTimeoutMS: int(s.config.PerClusterTimeout / time.Millisecond),
			MaxResults:          s.config.MaxResults, PerKindLimit: s.config.PerKindLimit,
		},
	}, nil
}

func (s *Service) validate(query Query) error {
	if len(query.Term) < 2 || len(query.Term) > 64 || query.ClusterLimit < 1 || query.ClusterLimit > s.config.MaxClusters || query.ResultLimit < 1 || query.ResultLimit > s.config.MaxResults {
		return ErrInvalidQuery
	}
	if len(query.Namespace) > 63 || (query.Namespace != "" && !namespaceName.MatchString(query.Namespace)) {
		return ErrInvalidQuery
	}
	seen := map[Kind]bool{}
	for _, kind := range query.Kinds {
		if seen[kind] || kindIndex(kind) == len(supportedKinds) {
			return ErrInvalidQuery
		}
		seen[kind] = true
	}
	return nil
}

func (s *Service) searchCluster(parent context.Context, item cluster.Cluster, query Query) clusterResult {
	ctx, cancel := context.WithTimeout(parent, s.config.PerClusterTimeout)
	defer cancel()
	listQuery := apiquery.ListQuery{Page: 1, Limit: min(query.ResultLimit, s.config.PerKindLimit), SortBy: "name", Ascending: true, Name: query.Term}
	result := clusterResult{items: []Item{}, failures: []Failure{}}
	for _, kind := range query.Kinds {
		items, total, err := s.searchKind(ctx, item, kind, query.Namespace, listQuery)
		if err != nil {
			code := FailureQueryFailed
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				code = FailureTimeout
			}
			result.failures = append(result.failures, Failure{ClusterID: item.ID, ClusterName: item.Name, Kind: kind, Code: code})
			continue
		}
		result.items = append(result.items, items...)
		result.total += total
	}
	return result
}

func (s *Service) searchKind(ctx context.Context, clusterItem cluster.Cluster, kind Kind, namespace string, query apiquery.ListQuery) ([]Item, int, error) {
	base := func(kind Kind, meta k8sgateway.ObjectMeta, health, summary string) Item {
		return Item{ClusterID: clusterItem.ID, ClusterName: clusterItem.Name, Kind: kind, Namespace: meta.Namespace, Name: meta.Name, Health: health, Summary: summary}
	}
	switch kind {
	case KindPod:
		response, err := s.resources.Pods(ctx, clusterItem.ID, namespace, query)
		items := make([]Item, 0, len(response.Items))
		for _, item := range response.Items {
			health := "degraded"
			if podHealthy(item) {
				health = "healthy"
			}
			summary := item.Status.Phase
			if item.Status.Reason != "" {
				summary += " · " + item.Status.Reason
			}
			items = append(items, base(kind, item.Metadata, health, summary))
		}
		return items, response.Total, err
	case KindDeployment:
		response, err := s.resources.Deployments(ctx, clusterItem.ID, namespace, query)
		items := make([]Item, 0, len(response.Items))
		for _, item := range response.Items {
			desired := int32(1)
			if item.Spec.Replicas != nil {
				desired = *item.Spec.Replicas
			}
			health := "degraded"
			if desired == 0 || (item.Status.ReadyReplicas >= desired && item.Status.AvailableReplicas >= desired && item.Status.UnavailableReplicas == 0) {
				health = "healthy"
			}
			items = append(items, base(kind, item.Metadata, health, fmt.Sprintf("%d/%d Ready", item.Status.ReadyReplicas, desired)))
		}
		return items, response.Total, err
	case KindService:
		response, err := s.resources.Services(ctx, clusterItem.ID, namespace, query)
		items := make([]Item, 0, len(response.Items))
		for _, item := range response.Items {
			items = append(items, base(kind, item.Metadata, "unknown", fmt.Sprintf("%s · %d ports", item.Spec.Type, len(item.Spec.Ports))))
		}
		return items, response.Total, err
	case KindIngress:
		response, err := s.resources.Ingresses(ctx, clusterItem.ID, namespace, query)
		items := make([]Item, 0, len(response.Items))
		for _, item := range response.Items {
			backends := 0
			for _, rule := range item.Spec.Rules {
				if rule.HTTP != nil {
					backends += len(rule.HTTP.Paths)
				}
			}
			items = append(items, base(kind, item.Metadata, "unknown", fmt.Sprintf("%d hosts · %d backends", len(item.Spec.Rules), backends)))
		}
		return items, response.Total, err
	default:
		return nil, 0, ErrInvalidQuery
	}
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

func sortItems(items []Item) {
	sort.Slice(items, func(i, j int) bool {
		left, right := items[i], items[j]
		if left.ClusterID != right.ClusterID {
			return left.ClusterID < right.ClusterID
		}
		if kindIndex(left.Kind) != kindIndex(right.Kind) {
			return kindIndex(left.Kind) < kindIndex(right.Kind)
		}
		if left.Namespace != right.Namespace {
			return left.Namespace < right.Namespace
		}
		return left.Name < right.Name
	})
}

func kindIndex(kind Kind) int {
	for index, candidate := range supportedKinds {
		if candidate == kind {
			return index
		}
	}
	return len(supportedKinds)
}
