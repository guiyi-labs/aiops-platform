package metricshistory

import (
	"context"
	"errors"
	"testing"
	"time"
)

type repositoryStub struct {
	collection  Collection
	result      RepositorySeriesResult
	query       SeriesQuery
	deletedAt   time.Time
	deleteLimit int
	downsampled []DownsampledSample
	err         error
}

func (r *repositoryStub) SaveCollection(_ context.Context, collection Collection) (int64, error) {
	r.collection = collection
	return 41, r.err
}

func (r *repositoryStub) QuerySeries(_ context.Context, query SeriesQuery) (RepositorySeriesResult, error) {
	r.query = query
	return r.result, r.err
}

func (r *repositoryStub) DeleteExpired(_ context.Context, before time.Time, limit int) (int64, error) {
	r.deletedAt, r.deleteLimit = before, limit
	return 7, r.err
}

func (r *repositoryStub) QueryArchiveSeries(_ context.Context, query SeriesQuery) (RepositorySeriesResult, error) {
	return r.result, r.err
}

func (r *repositoryStub) SaveDownsampledBatch(_ context.Context, batch []DownsampledSample) error {
	r.downsampled = append(r.downsampled, batch...)
	return r.err
}

func (r *repositoryStub) ListExpiringSamples(_ context.Context, before time.Time, limit int) ([]Sample, error) {
	r.deletedAt, r.deleteLimit = before, limit
	return nil, r.err
}

func TestRecordNormalizesBoundedSuccessfulCollection(t *testing.T) {
	repository := &repositoryStub{}
	service, err := NewService(Config{}, repository)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	started := time.Date(2026, 7, 28, 11, 0, 0, 0, time.FixedZone("test", 8*60*60))
	completed := started.Add(2 * time.Second)
	run, err := service.Record(context.Background(), CollectionInput{
		ClusterID: 9,
		Nodes:     TargetCoverage{Status: SourceSucceeded, Sampled: 1, Total: 1, Complete: true},
		Pods:      TargetCoverage{Status: SourceSucceeded, Sampled: 2, Total: 2, Complete: true},
		StartedAt: started, CompletedAt: completed,
		Samples: []SampleInput{{
			ResourceKind: ResourceNode, ResourceName: "worker-a", MetricName: MetricCPU,
			Value: 123000000, SourceTimestamp: completed.Add(-time.Second), Window: 15 * time.Second,
		}, {
			ResourceKind: ResourcePod, ResourceNamespace: "default", ResourceName: "api-0", ContainerName: "api",
			MetricName: MetricMemory, Value: 1048576, SourceTimestamp: completed.Add(-time.Second), Window: 15 * time.Second,
		}},
	})
	if err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	if run.ID != 41 || run.Status != CollectionSucceeded || run.FailureCode != "" {
		t.Fatalf("run = %#v", run)
	}
	if !run.StartedAt.Equal(started.UTC()) || !run.ExpiresAt.Equal(completed.UTC().Add(7*24*time.Hour)) {
		t.Fatalf("normalized times = %#v", run)
	}
	if len(repository.collection.Samples) != 2 || repository.collection.Samples[0].Unit != UnitNanocores || repository.collection.Samples[1].Unit != UnitBytes {
		t.Fatalf("samples = %#v", repository.collection.Samples)
	}
}

func TestRecordDerivesPartialAndRejectsInventedFailureShapes(t *testing.T) {
	service, _ := NewService(Config{}, &repositoryStub{})
	now := time.Now().UTC()
	valid := CollectionInput{
		ClusterID:   1,
		Nodes:       TargetCoverage{Status: SourceSucceeded, Sampled: 1, Total: 2, Complete: false},
		Pods:        TargetCoverage{Status: SourceUnavailable},
		FailureCode: "METRICS_API_UNAVAILABLE", StartedAt: now, CompletedAt: now,
	}
	run, err := service.Record(context.Background(), valid)
	if err != nil || run.Status != CollectionPartial {
		t.Fatalf("Record() run=%#v error=%v", run, err)
	}

	tests := []CollectionInput{
		{ClusterID: 1, Nodes: TargetCoverage{Status: SourceSucceeded, Sampled: 2, Total: 1}, Pods: TargetCoverage{Status: SourceUnavailable}, FailureCode: "FAILED", StartedAt: now, CompletedAt: now},
		{ClusterID: 1, Nodes: TargetCoverage{Status: SourceUnavailable, Sampled: 1}, Pods: TargetCoverage{Status: SourceUnavailable}, FailureCode: "FAILED", StartedAt: now, CompletedAt: now},
		{ClusterID: 1, Nodes: TargetCoverage{Status: SourceUnavailable}, Pods: TargetCoverage{Status: SourceUnavailable}, FailureCode: "raw upstream error", StartedAt: now, CompletedAt: now},
		{ClusterID: 1, Nodes: TargetCoverage{Status: SourceSucceeded, Complete: true}, Pods: TargetCoverage{Status: SourceSucceeded, Complete: true}, FailureCode: "SHOULD_BE_EMPTY", StartedAt: now, CompletedAt: now},
	}
	for index, input := range tests {
		if _, err := service.Record(context.Background(), input); !errors.Is(err, ErrInvalidCollection) {
			t.Fatalf("case %d error = %v, want ErrInvalidCollection", index, err)
		}
	}
}

func TestRecordRejectsDuplicateSeriesAndInvalidResourceShape(t *testing.T) {
	service, _ := NewService(Config{}, &repositoryStub{})
	now := time.Now().UTC()
	base := SampleInput{ResourceKind: ResourcePod, ResourceNamespace: "default", ResourceName: "api", ContainerName: "api", MetricName: MetricCPU, SourceTimestamp: now, Window: 15 * time.Second}
	input := CollectionInput{
		ClusterID: 1,
		Nodes:     TargetCoverage{Status: SourceSucceeded, Complete: true},
		Pods:      TargetCoverage{Status: SourceSucceeded, Sampled: 1, Total: 1, Complete: true},
		StartedAt: now, CompletedAt: now, Samples: []SampleInput{base, base},
	}
	if _, err := service.Record(context.Background(), input); !errors.Is(err, ErrInvalidCollection) {
		t.Fatalf("duplicate error = %v", err)
	}
	input.Samples = []SampleInput{{ResourceKind: ResourceNode, ResourceNamespace: "default", ResourceName: "worker", MetricName: MetricCPU, SourceTimestamp: now, Window: 15 * time.Second}}
	if _, err := service.Record(context.Background(), input); !errors.Is(err, ErrInvalidCollection) {
		t.Fatalf("invalid Node shape error = %v", err)
	}
	input.Nodes = TargetCoverage{Status: SourceUnavailable}
	input.Pods = TargetCoverage{Status: SourceSucceeded, Complete: true}
	input.FailureCode = "METRICS_API_UNAVAILABLE"
	input.Samples = []SampleInput{{ResourceKind: ResourceNode, ResourceName: "worker", MetricName: MetricCPU, SourceTimestamp: now, Window: 15 * time.Second}}
	if _, err := service.Record(context.Background(), input); !errors.Is(err, ErrInvalidCollection) {
		t.Fatalf("sample from unavailable source error = %v", err)
	}
}

func TestQueryKeepsMissingSamplesSparseAndBounded(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	repository := &repositoryStub{result: RepositorySeriesResult{
		Points:   []Point{{Value: 7, SourceTimestamp: now.Add(-time.Minute), CollectedAt: now.Add(-time.Minute), WindowMilliseconds: 15000}},
		Total:    1,
		Coverage: QueryCoverage{Collections: 3, Succeeded: 1, Partial: 1, Unavailable: 1, Points: 1, Missing: 2},
	}}
	service, _ := NewService(Config{}, repository)
	response, err := service.Query(context.Background(), SeriesQuery{
		ClusterID: 3, ResourceKind: ResourcePod, ResourceNamespace: "ops", ResourceName: "api-0",
		ContainerName: "api", MetricName: MetricMemory, From: now.Add(-time.Hour), To: now,
	})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(response.Points) != 1 || response.Coverage.Missing != 2 || response.Series.Unit != UnitBytes {
		t.Fatalf("response = %#v", response)
	}
	if repository.query.Limit != 1440 || response.Limits.MaxPoints != 1440 || response.Truncated {
		t.Fatalf("limits = %#v query=%#v", response.Limits, repository.query)
	}
}

func TestQueryValidatesClusterSeriesWindowAndPointLimit(t *testing.T) {
	service, _ := NewService(Config{}, &repositoryStub{})
	now := time.Now().UTC()
	valid := SeriesQuery{ClusterID: 1, ResourceKind: ResourceNode, ResourceName: "worker", MetricName: MetricCPU, From: now.Add(-time.Hour), To: now, Limit: 100}
	tests := []SeriesQuery{
		{ClusterID: 0, ResourceKind: ResourceNode, ResourceName: "worker", MetricName: MetricCPU, From: valid.From, To: valid.To},
		{ClusterID: 1, ResourceKind: ResourceNode, ResourceNamespace: "default", ResourceName: "worker", MetricName: MetricCPU, From: valid.From, To: valid.To},
		{ClusterID: 1, ResourceKind: ResourcePod, ResourceNamespace: "default", ResourceName: "api", MetricName: MetricCPU, From: valid.From, To: valid.To},
		{ClusterID: 1, ResourceKind: ResourceNode, ResourceName: "worker", MetricName: "temperature", From: valid.From, To: valid.To},
		{ClusterID: 1, ResourceKind: ResourceNode, ResourceName: "worker", MetricName: MetricCPU, From: now.Add(-25 * time.Hour), To: now},
		{ClusterID: 1, ResourceKind: ResourceNode, ResourceName: "worker", MetricName: MetricCPU, From: valid.From, To: valid.To, Limit: 1441},
	}
	for index, query := range tests {
		if _, err := service.Query(context.Background(), query); !errors.Is(err, ErrInvalidQuery) {
			t.Fatalf("case %d error = %v, want ErrInvalidQuery", index, err)
		}
	}
}

func TestCleanupUsesUTCAndConfiguredBatch(t *testing.T) {
	repository := &repositoryStub{}
	service, err := NewService(Config{CleanupBatchSize: 25}, repository)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	now := time.Date(2026, 7, 28, 16, 0, 0, 0, time.FixedZone("test", 8*60*60))
	deleted, err := service.Cleanup(context.Background(), now)
	if err != nil || deleted != 7 || repository.deleteLimit != 25 || !repository.deletedAt.Equal(now.UTC()) {
		t.Fatalf("Cleanup() deleted=%d error=%v before=%s limit=%d", deleted, err, repository.deletedAt, repository.deleteLimit)
	}
}

func TestNewServiceRejectsCapsOutsideReviewedEnvelope(t *testing.T) {
	repository := &repositoryStub{}
	tests := []Config{
		{Retention: 31 * 24 * time.Hour},
		{MaxSamplesPerCollection: 1801},
		{MaxQueryWindow: 25 * time.Hour},
		{MaxQueryPoints: 1441},
		{CleanupBatchSize: 5001},
	}
	for index, config := range tests {
		if _, err := NewService(config, repository); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("case %d error = %v, want ErrInvalidConfig", index, err)
		}
	}
	if _, err := NewService(Config{}, nil); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("nil repository error = %v", err)
	}
}

func TestDownsampleAndArchiveAggregatesHourlyBuckets(t *testing.T) {
	repository := &repositoryStub{}
	service, _ := NewService(Config{}, repository)
	hour := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	precise := []Sample{
		{ClusterID: 3, ResourceKind: ResourcePod, ResourceNamespace: "ops", ResourceName: "api-0", ContainerName: "api", MetricName: MetricCPU, Unit: UnitNanocores, Value: 100, SourceTimestamp: hour.Add(10 * time.Minute)},
		{ClusterID: 3, ResourceKind: ResourcePod, ResourceNamespace: "ops", ResourceName: "api-0", ContainerName: "api", MetricName: MetricCPU, Unit: UnitNanocores, Value: 300, SourceTimestamp: hour.Add(20 * time.Minute)},
		{ClusterID: 3, ResourceKind: ResourcePod, ResourceNamespace: "ops", ResourceName: "api-0", ContainerName: "api", MetricName: MetricCPU, Unit: UnitNanocores, Value: 500, SourceTimestamp: hour.Add(40 * time.Minute)},
		{ClusterID: 3, ResourceKind: ResourcePod, ResourceNamespace: "ops", ResourceName: "api-1", ContainerName: "api", MetricName: MetricCPU, Unit: UnitNanocores, Value: 900, SourceTimestamp: hour.Add(5 * time.Minute)},
	}
	written, err := service.DownsampleAndArchive(context.Background(), precise)
	if err != nil {
		t.Fatalf("DownsampleAndArchive() error = %v", err)
	}
	if written != 2 {
		t.Fatalf("written = %d, want 2 (one row per series-hour)", written)
	}
	if len(repository.downsampled) != 2 {
		t.Fatalf("saved rows = %d, want 2", len(repository.downsampled))
	}
	first := repository.downsampled[0]
	if first.ValueAvg != 300 || first.ValueMax != 500 || first.SampleCount != 3 ||
		first.WindowMilliseconds != 3600000 || !first.BucketHour.Equal(hour) {
		t.Fatalf("first bucket = %#v", first)
	}
}

func TestDownsampleAndArchiveEmptyInput(t *testing.T) {
	repository := &repositoryStub{}
	service, _ := NewService(Config{}, repository)
	written, err := service.DownsampleAndArchive(context.Background(), nil)
	if err != nil || written != 0 {
		t.Fatalf("DownsampleAndArchive() = %d, %v; want 0, nil", written, err)
	}
}

func TestQueryArchiveValidatesBoundsAndReturnsSeries(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	repository := &repositoryStub{result: RepositorySeriesResult{
		Points: []Point{{Value: 300, SourceTimestamp: now.Add(-2 * time.Hour), WindowMilliseconds: 3600000, CollectedAt: now.Add(-2 * time.Hour)}},
		Total:  1,
	}}
	service, _ := NewService(Config{}, repository)
	response, err := service.QueryArchive(context.Background(), ArchiveSeriesQuery{
		ClusterID: 3, ResourceKind: ResourcePod, ResourceNamespace: "ops", ResourceName: "api-0",
		ContainerName: "api", MetricName: MetricCPU, From: now.Add(-24 * time.Hour), To: now,
	})
	if err != nil {
		t.Fatalf("QueryArchive() error = %v", err)
	}
	if response.Series.Unit != UnitNanocores || len(response.Points) != 1 || response.Limits.MaxWindowSeconds != int((30*24*time.Hour)/time.Second) {
		t.Fatalf("response = %#v", response)
	}
	if response.Truncated {
		t.Fatalf("Truncated = true, want false")
	}
}

func TestQueryArchiveRejectsInvalidShapeWindowAndMetric(t *testing.T) {
	service, _ := NewService(Config{}, &repositoryStub{})
	now := time.Now().UTC()
	valid := ArchiveSeriesQuery{ClusterID: 1, ResourceKind: ResourceNode, ResourceName: "worker", MetricName: MetricCPU, From: now.Add(-time.Hour), To: now}
	tests := []ArchiveSeriesQuery{
		{ClusterID: 0, ResourceKind: ResourceNode, ResourceName: "worker", MetricName: MetricCPU, From: valid.From, To: valid.To},
		{ClusterID: 1, ResourceKind: ResourceNode, ResourceNamespace: "default", ResourceName: "worker", MetricName: MetricCPU, From: valid.From, To: valid.To},
		{ClusterID: 1, ResourceKind: ResourceNode, ResourceName: "worker", MetricName: "temperature", From: valid.From, To: valid.To},
		{ClusterID: 1, ResourceKind: ResourceNode, ResourceName: "worker", MetricName: MetricCPU, From: now.Add(-31 * 24 * time.Hour), To: now},
		{ClusterID: 1, ResourceKind: ResourceNode, ResourceName: "worker", MetricName: MetricCPU, From: now, To: now},
		{ClusterID: 1, ResourceKind: ResourceNode, ResourceName: "worker", MetricName: MetricCPU, From: valid.From, To: valid.To, Limit: 1441},
	}
	for index, query := range tests {
		if _, err := service.QueryArchive(context.Background(), query); !errors.Is(err, ErrInvalidQuery) {
			t.Fatalf("case %d error = %v, want ErrInvalidQuery", index, err)
		}
	}
}

func TestCleanupArchivesExpiringSamplesBeforeDelete(t *testing.T) {
	repository := &repositoryStub{}
	service, _ := NewService(Config{}, repository)
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.FixedZone("test", 8*60*60))
	deleted, err := service.Cleanup(context.Background(), now)
	if err != nil || deleted != 7 || !repository.deletedAt.Equal(now.UTC()) {
		t.Fatalf("Cleanup() deleted=%d error=%v before=%s", deleted, err, repository.deletedAt)
	}
}
