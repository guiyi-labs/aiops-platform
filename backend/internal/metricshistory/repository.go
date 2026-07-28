package metricshistory

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type GormRepository struct{ db *gorm.DB }

func NewGormRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

type collectionRunRecord struct {
	ID            int64
	ClusterID     int64
	Status        string
	NodesStatus   string
	NodesSampled  int
	NodesTotal    int
	NodesComplete bool
	PodsStatus    string
	PodsSampled   int
	PodsTotal     int
	PodsComplete  bool
	FailureCode   string
	StartedAt     time.Time
	CompletedAt   time.Time
	ExpiresAt     time.Time
}

func (collectionRunRecord) TableName() string { return "metric_collection_runs" }

type sampleRecord struct {
	ID                 int64
	CollectionRunID    int64
	ClusterID          int64
	ResourceKind       string
	ResourceNamespace  string
	ResourceName       string
	ResourceUID        string
	ContainerName      string
	MetricName         string
	Value              int64
	Unit               string
	SourceTimestamp    time.Time
	WindowMilliseconds int
	CollectedAt        time.Time
	ExpiresAt          time.Time
}

func (sampleRecord) TableName() string { return "metric_samples" }

func (r *GormRepository) SaveCollection(ctx context.Context, collection Collection) (int64, error) {
	var id int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		run := collectionRunRecord{
			ClusterID: collection.Run.ClusterID, Status: collection.Run.Status,
			NodesStatus: collection.Run.Nodes.Status, NodesSampled: collection.Run.Nodes.Sampled,
			NodesTotal: collection.Run.Nodes.Total, NodesComplete: collection.Run.Nodes.Complete,
			PodsStatus: collection.Run.Pods.Status, PodsSampled: collection.Run.Pods.Sampled,
			PodsTotal: collection.Run.Pods.Total, PodsComplete: collection.Run.Pods.Complete,
			FailureCode: collection.Run.FailureCode, StartedAt: collection.Run.StartedAt,
			CompletedAt: collection.Run.CompletedAt, ExpiresAt: collection.Run.ExpiresAt,
		}
		if err := tx.Create(&run).Error; err != nil {
			return err
		}
		id = run.ID
		if len(collection.Samples) == 0 {
			return nil
		}
		records := make([]sampleRecord, 0, len(collection.Samples))
		for _, sample := range collection.Samples {
			records = append(records, sampleRecord{
				CollectionRunID: id, ClusterID: sample.ClusterID, ResourceKind: sample.ResourceKind,
				ResourceNamespace: sample.ResourceNamespace, ResourceName: sample.ResourceName,
				ResourceUID: sample.ResourceUID, ContainerName: sample.ContainerName,
				MetricName: sample.MetricName, Value: sample.Value, Unit: sample.Unit,
				SourceTimestamp: sample.SourceTimestamp, WindowMilliseconds: sample.WindowMilliseconds,
				CollectedAt: sample.CollectedAt, ExpiresAt: sample.ExpiresAt,
			})
		}
		return tx.CreateInBatches(records, 250).Error
	})
	return id, err
}

func (r *GormRepository) QuerySeries(ctx context.Context, query SeriesQuery) (RepositorySeriesResult, error) {
	var clusterExists bool
	if err := r.db.WithContext(ctx).Raw(`SELECT EXISTS (SELECT 1 FROM clusters WHERE id = ?)`, query.ClusterID).Row().Scan(&clusterExists); err != nil {
		return RepositorySeriesResult{}, err
	}
	if !clusterExists {
		return RepositorySeriesResult{}, ErrClusterNotFound
	}
	args := []any{query.ClusterID, query.From, query.To, query.ResourceKind, query.ResourceNamespace, query.ResourceName, query.ContainerName, query.MetricName}
	var coverage struct {
		Collections int
		Succeeded   int
		Partial     int
		Unavailable int
		TimedOut    int
		Failed      int
		Points      int
	}
	if err := r.db.WithContext(ctx).Raw(`SELECT
		COUNT(DISTINCT runs.id)::INTEGER AS collections,
		COUNT(DISTINCT runs.id) FILTER (WHERE runs.status = 'succeeded')::INTEGER AS succeeded,
		COUNT(DISTINCT runs.id) FILTER (WHERE runs.status = 'partial')::INTEGER AS partial,
		COUNT(DISTINCT runs.id) FILTER (WHERE runs.status = 'unavailable')::INTEGER AS unavailable,
		COUNT(DISTINCT runs.id) FILTER (WHERE runs.status = 'timed_out')::INTEGER AS timed_out,
		COUNT(DISTINCT runs.id) FILTER (WHERE runs.status = 'failed')::INTEGER AS failed,
		COUNT(samples.id)::INTEGER AS points
		FROM metric_collection_runs runs
		LEFT JOIN metric_samples samples ON samples.collection_run_id = runs.id
			AND samples.resource_kind = ? AND samples.resource_namespace = ?
			AND samples.resource_name = ? AND samples.container_name = ? AND samples.metric_name = ?
		WHERE runs.cluster_id = ? AND runs.completed_at >= ? AND runs.completed_at < ?`,
		query.ResourceKind, query.ResourceNamespace, query.ResourceName, query.ContainerName, query.MetricName,
		query.ClusterID, query.From, query.To).Scan(&coverage).Error; err != nil {
		return RepositorySeriesResult{}, err
	}
	var records []sampleRecord
	if err := r.db.WithContext(ctx).Raw(`SELECT samples.collection_run_id, samples.cluster_id,
		samples.resource_kind, samples.resource_namespace, samples.resource_name, samples.resource_uid,
		samples.container_name, samples.metric_name, samples.value, samples.unit,
		samples.source_timestamp, samples.window_milliseconds, samples.collected_at, samples.expires_at
		FROM metric_samples samples JOIN metric_collection_runs runs ON runs.id = samples.collection_run_id
		WHERE samples.cluster_id = ? AND runs.completed_at >= ? AND runs.completed_at < ?
			AND samples.resource_kind = ? AND samples.resource_namespace = ? AND samples.resource_name = ?
			AND samples.container_name = ? AND samples.metric_name = ?
		ORDER BY runs.completed_at ASC, samples.id ASC LIMIT ?`, append(args, query.Limit)...).Scan(&records).Error; err != nil {
		return RepositorySeriesResult{}, err
	}
	points := make([]Point, 0, len(records))
	for _, record := range records {
		points = append(points, Point{Value: record.Value, SourceTimestamp: record.SourceTimestamp.UTC(), WindowMilliseconds: record.WindowMilliseconds, CollectedAt: record.CollectedAt.UTC()})
	}
	missing := coverage.Collections - coverage.Points
	if missing < 0 {
		missing = 0
	}
	return RepositorySeriesResult{Points: points, Total: coverage.Points, Coverage: QueryCoverage{
		Collections: coverage.Collections, Succeeded: coverage.Succeeded, Partial: coverage.Partial,
		Unavailable: coverage.Unavailable, TimedOut: coverage.TimedOut, Failed: coverage.Failed,
		Points: coverage.Points, Missing: missing,
	}}, nil
}

func (r *GormRepository) DeleteExpired(ctx context.Context, before time.Time, limit int) (int64, error) {
	result := r.db.WithContext(ctx).Exec(`WITH expired AS (
		SELECT id FROM metric_collection_runs WHERE expires_at < ? ORDER BY expires_at ASC, id ASC LIMIT ?
	) DELETE FROM metric_collection_runs WHERE id IN (SELECT id FROM expired)`, before, limit)
	return result.RowsAffected, result.Error
}

var _ Repository = (*GormRepository)(nil)
