package metricshistory

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
)

const (
	defaultRetention               = 7 * 24 * time.Hour
	defaultMaxSamplesPerCollection = 1800
	defaultMaxQueryWindow          = 24 * time.Hour
	defaultMaxQueryPoints          = 1440
	defaultCleanupBatchSize        = 1000
	maxRetention                   = 30 * 24 * time.Hour
	maxSamplesPerCollection        = 1800
	maxQueryWindow                 = 24 * time.Hour
	maxQueryPoints                 = 1440
	maxCleanupBatchSize            = 5000
)

var (
	ErrInvalidConfig     = errors.New("metrics history configuration is invalid")
	ErrInvalidCollection = errors.New("metrics history collection is invalid")
	ErrInvalidQuery      = errors.New("metrics history query is invalid")
	ErrClusterNotFound   = errors.New("metrics history cluster not found")

	failureCodePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)
)

type Repository interface {
	SaveCollection(context.Context, Collection) (int64, error)
	QuerySeries(context.Context, SeriesQuery) (RepositorySeriesResult, error)
	DeleteExpired(context.Context, time.Time, int) (int64, error)
}

type Service struct {
	config     Config
	repository Repository
}

func NewService(config Config, repository Repository) (*Service, error) {
	config = withDefaults(config)
	if repository == nil || !validConfig(config) {
		return nil, ErrInvalidConfig
	}
	return &Service{config: config, repository: repository}, nil
}

func (s *Service) Record(ctx context.Context, input CollectionInput) (CollectionRun, error) {
	collection, err := s.normalizeCollection(input)
	if err != nil {
		return CollectionRun{}, err
	}
	id, err := s.repository.SaveCollection(ctx, collection)
	if err != nil {
		return CollectionRun{}, err
	}
	collection.Run.ID = id
	return collection.Run, nil
}

func (s *Service) Query(ctx context.Context, query SeriesQuery) (SeriesResponse, error) {
	normalized, unit, err := s.normalizeQuery(query)
	if err != nil {
		return SeriesResponse{}, err
	}
	result, err := s.repository.QuerySeries(ctx, normalized)
	if err != nil {
		return SeriesResponse{}, err
	}
	if result.Points == nil {
		result.Points = []Point{}
	}
	return SeriesResponse{
		Series: Series{
			ClusterID: normalized.ClusterID, ResourceKind: normalized.ResourceKind,
			ResourceNamespace: normalized.ResourceNamespace, ResourceName: normalized.ResourceName,
			ContainerName: normalized.ContainerName, MetricName: normalized.MetricName, Unit: unit,
		},
		From: normalized.From, To: normalized.To, Points: result.Points, Coverage: result.Coverage,
		Limits:    QueryLimits{MaxWindowSeconds: int(s.config.MaxQueryWindow / time.Second), MaxPoints: s.config.MaxQueryPoints},
		Truncated: result.Total > len(result.Points),
	}, nil
}

func (s *Service) Cleanup(ctx context.Context, now time.Time) (int64, error) {
	if now.IsZero() {
		return 0, ErrInvalidCollection
	}
	return s.repository.DeleteExpired(ctx, now.UTC(), s.config.CleanupBatchSize)
}

func (s *Service) normalizeCollection(input CollectionInput) (Collection, error) {
	startedAt, completedAt := input.StartedAt.UTC(), input.CompletedAt.UTC()
	if input.ClusterID < 1 || startedAt.IsZero() || completedAt.IsZero() || startedAt.After(completedAt) ||
		len(input.Samples) > s.config.MaxSamplesPerCollection || !validCoverage(input.Nodes) || !validCoverage(input.Pods) {
		return Collection{}, ErrInvalidCollection
	}
	status := collectionStatus(input.Nodes, input.Pods)
	failureCode := strings.TrimSpace(input.FailureCode)
	if (status == CollectionSucceeded && failureCode != "") ||
		(status != CollectionSucceeded && !failureCodePattern.MatchString(failureCode)) {
		return Collection{}, ErrInvalidCollection
	}
	expiresAt := completedAt.Add(s.config.Retention)
	collection := Collection{Run: CollectionRun{
		ClusterID: input.ClusterID, Status: status, Nodes: input.Nodes, Pods: input.Pods,
		FailureCode: failureCode, StartedAt: startedAt, CompletedAt: completedAt, ExpiresAt: expiresAt,
	}, Samples: make([]Sample, 0, len(input.Samples))}
	seen := make(map[string]struct{}, len(input.Samples))
	for _, candidate := range input.Samples {
		sample, key, err := normalizeSample(candidate, input.ClusterID, completedAt, expiresAt)
		if err != nil {
			return Collection{}, err
		}
		if (sample.ResourceKind == ResourceNode && input.Nodes.Status != SourceSucceeded) ||
			(sample.ResourceKind == ResourcePod && input.Pods.Status != SourceSucceeded) {
			return Collection{}, ErrInvalidCollection
		}
		if _, duplicate := seen[key]; duplicate {
			return Collection{}, ErrInvalidCollection
		}
		seen[key] = struct{}{}
		collection.Samples = append(collection.Samples, sample)
	}
	return collection, nil
}

func (s *Service) normalizeQuery(query SeriesQuery) (SeriesQuery, string, error) {
	query.ResourceKind = strings.TrimSpace(query.ResourceKind)
	query.ResourceNamespace = strings.TrimSpace(query.ResourceNamespace)
	query.ResourceName = strings.TrimSpace(query.ResourceName)
	query.ContainerName = strings.TrimSpace(query.ContainerName)
	query.MetricName = strings.TrimSpace(query.MetricName)
	query.From, query.To = query.From.UTC(), query.To.UTC()
	if query.Limit == 0 {
		query.Limit = s.config.MaxQueryPoints
	}
	if query.ClusterID < 1 || query.From.IsZero() || query.To.IsZero() || !query.From.Before(query.To) ||
		query.To.Sub(query.From) > s.config.MaxQueryWindow || query.Limit < 1 || query.Limit > s.config.MaxQueryPoints ||
		!validSeriesShape(query.ResourceKind, query.ResourceNamespace, query.ResourceName, query.ContainerName) {
		return SeriesQuery{}, "", ErrInvalidQuery
	}
	unit, ok := metricUnit(query.MetricName)
	if !ok {
		return SeriesQuery{}, "", ErrInvalidQuery
	}
	return query, unit, nil
}

func normalizeSample(input SampleInput, clusterID int64, collectedAt, expiresAt time.Time) (Sample, string, error) {
	input.ResourceKind = strings.TrimSpace(input.ResourceKind)
	input.ResourceNamespace = strings.TrimSpace(input.ResourceNamespace)
	input.ResourceName = strings.TrimSpace(input.ResourceName)
	input.ResourceUID = strings.TrimSpace(input.ResourceUID)
	input.ContainerName = strings.TrimSpace(input.ContainerName)
	input.MetricName = strings.TrimSpace(input.MetricName)
	if input.Value < 0 || input.SourceTimestamp.IsZero() || input.Window < time.Second || input.Window > time.Hour ||
		len(input.ResourceUID) > 128 || !validSeriesShape(input.ResourceKind, input.ResourceNamespace, input.ResourceName, input.ContainerName) {
		return Sample{}, "", ErrInvalidCollection
	}
	unit, ok := metricUnit(input.MetricName)
	if !ok {
		return Sample{}, "", ErrInvalidCollection
	}
	sample := Sample{
		ClusterID: clusterID, ResourceKind: input.ResourceKind, ResourceNamespace: input.ResourceNamespace,
		ResourceName: input.ResourceName, ResourceUID: input.ResourceUID, ContainerName: input.ContainerName,
		MetricName: input.MetricName, Value: input.Value, Unit: unit,
		SourceTimestamp: input.SourceTimestamp.UTC(), WindowMilliseconds: int(input.Window / time.Millisecond),
		CollectedAt: collectedAt, ExpiresAt: expiresAt,
	}
	key := strings.Join([]string{sample.ResourceKind, sample.ResourceNamespace, sample.ResourceName, sample.ContainerName, sample.MetricName}, "\x00")
	return sample, key, nil
}

func validCoverage(value TargetCoverage) bool {
	switch value.Status {
	case SourceSucceeded:
		return value.Sampled >= 0 && value.Total >= value.Sampled && value.Complete == (value.Sampled == value.Total)
	case SourceUnavailable, SourceTimedOut, SourceFailed:
		return value.Sampled == 0 && value.Total == 0 && !value.Complete
	default:
		return false
	}
}

func collectionStatus(nodes, pods TargetCoverage) string {
	if nodes.Status == SourceSucceeded && pods.Status == SourceSucceeded && nodes.Complete && pods.Complete {
		return CollectionSucceeded
	}
	if nodes.Status == SourceSucceeded || pods.Status == SourceSucceeded {
		return CollectionPartial
	}
	if nodes.Status == SourceUnavailable && pods.Status == SourceUnavailable {
		return CollectionUnavailable
	}
	if nodes.Status == SourceTimedOut || pods.Status == SourceTimedOut {
		return CollectionTimedOut
	}
	return CollectionFailed
}

func validSeriesShape(kind, namespace, name, container string) bool {
	if name == "" || len(name) > 253 || len(namespace) > 63 || len(container) > 253 {
		return false
	}
	switch kind {
	case ResourceNode:
		return namespace == "" && container == ""
	case ResourcePod:
		return namespace != "" && container != ""
	case ResourceDeployment:
		return namespace != "" && container == ""
	default:
		return false
	}
}

func metricUnit(metric string) (string, bool) {
	switch metric {
	case MetricCPU:
		return UnitNanocores, true
	case MetricMemory:
		return UnitBytes, true
	case MetricReadinessReady, MetricReadinessTotal:
		return UnitCount, true
	default:
		return "", false
	}
}

func withDefaults(config Config) Config {
	if config.Retention == 0 {
		config.Retention = defaultRetention
	}
	if config.MaxSamplesPerCollection == 0 {
		config.MaxSamplesPerCollection = defaultMaxSamplesPerCollection
	}
	if config.MaxQueryWindow == 0 {
		config.MaxQueryWindow = defaultMaxQueryWindow
	}
	if config.MaxQueryPoints == 0 {
		config.MaxQueryPoints = defaultMaxQueryPoints
	}
	if config.CleanupBatchSize == 0 {
		config.CleanupBatchSize = defaultCleanupBatchSize
	}
	return config
}

func validConfig(config Config) bool {
	return config.Retention >= time.Hour && config.Retention <= maxRetention &&
		config.MaxSamplesPerCollection >= 1 && config.MaxSamplesPerCollection <= maxSamplesPerCollection &&
		config.MaxQueryWindow >= time.Minute && config.MaxQueryWindow <= maxQueryWindow &&
		config.MaxQueryPoints >= 1 && config.MaxQueryPoints <= maxQueryPoints &&
		config.CleanupBatchSize >= 1 && config.CleanupBatchSize <= maxCleanupBatchSize
}
