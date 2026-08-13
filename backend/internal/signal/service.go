package signal

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// Service is the M39 signal ingestion and query entrypoint. It is the single
// place that validates ingest requests against the catalog, deduplicates via
// fingerprint, enforces retention cleanup and assembles the AIOps overview.
//
// The service is safe for concurrent use. It never imports producer packages
// at runtime — adapters call Ingest with a pre-built IngestRequest.
type Service struct {
	repo   Repository
	now    func() time.Time
	logger *zap.Logger

	// retentionBatch bounds DeleteExpired so cleanup transactions stay short.
	retentionBatch int
	// listLimit bounds the maximum page size returned by List.
	listLimit int
	// overviewTopN bounds the top-signals aggregation.
	overviewTopN int
	// overviewWindow bounds the overview time range.
	overviewWindow time.Duration
}

// Options configures the Service.
type ServiceOptions struct {
	Repository     Repository
	Logger         *zap.Logger
	Now            func() time.Time
	RetentionBatch int
	ListLimit      int
	OverviewTopN   int
	OverviewWindow time.Duration
}

// NewService returns a Service. When repo is nil the service uses NopRepository
// (all writes ignored, all reads empty) — this is how the server runs when the
// signal feature is disabled.
func NewService(opts ServiceOptions) *Service {
	if opts.Repository == nil {
		opts.Repository = NopRepository{}
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Logger == nil {
		opts.Logger = zap.NewNop()
	}
	if opts.RetentionBatch <= 0 {
		opts.RetentionBatch = 500
	}
	if opts.ListLimit <= 0 || opts.ListLimit > 200 {
		opts.ListLimit = 100
	}
	if opts.OverviewTopN <= 0 {
		opts.OverviewTopN = 10
	}
	if opts.OverviewWindow <= 0 {
		opts.OverviewWindow = 24 * time.Hour
	}
	return &Service{
		repo:           opts.Repository,
		now:            opts.Now,
		logger:         opts.Logger,
		retentionBatch: opts.RetentionBatch,
		listLimit:      opts.ListLimit,
		overviewTopN:   opts.OverviewTopN,
		overviewWindow: opts.OverviewWindow,
	}
}

// ErrUnregisteredSignal is returned when an ingest request references a signal
// id not in the compiled catalog. This is a fail-closed guard.
var ErrUnregisteredSignal = errors.New("signal is not registered: ingestion rejected")

// Ingest validates and persists a single signal occurrence. Duplicate
// producer delivery (same signal_id + fingerprint) updates the existing row
// rather than creating a new one.
func (s *Service) Ingest(ctx context.Context, req IngestRequest) (Occurrence, error) {
	now := s.now()
	occ, err := BuildOccurrence(req, now)
	if err != nil {
		if _, ok := Lookup(req.SignalID); !ok {
			return Occurrence{}, fmt.Errorf("%w: %s", ErrUnregisteredSignal, req.SignalID)
		}
		return Occurrence{}, err
	}
	if err := s.repo.Upsert(ctx, &occ); err != nil {
		return Occurrence{}, err
	}
	return occ, nil
}

// IngestBatch ingests multiple occurrences. It is not transactional — each
// occurrence is upserted independently so one bad signal does not roll back
// the batch. Returns the count of successful ingests and the first error.
func (s *Service) IngestBatch(ctx context.Context, reqs []IngestRequest) (int, error) {
	var ok int
	var firstErr error
	for _, req := range reqs {
		if _, err := s.Ingest(ctx, req); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			s.logger.Warn("signal ingest failed",
				zap.String("signal_id", req.SignalID),
				zap.Error(err))
			continue
		}
		ok++
	}
	return ok, firstErr
}

// List returns signal occurrences matching the filter. The filter limit is
// clamped to s.listLimit.
func (s *Service) List(ctx context.Context, filter ListFilter) ([]Occurrence, int64, error) {
	if filter.Limit <= 0 || filter.Limit > s.listLimit {
		filter.Limit = s.listLimit
	}
	return s.repo.List(ctx, filter)
}

// Get returns a single occurrence by id.
func (s *Service) Get(ctx context.Context, id int64) (Occurrence, error) {
	return s.repo.Get(ctx, id)
}

// Overview assembles the bounded AIOps overview. The clusterID/namespace
// pair scopes the result (empty namespace = cluster-wide). The overview
// never discloses hidden cluster/namespace data: callers must additionally
// filter by M35 scope before returning to the user.
func (s *Service) Overview(ctx context.Context, clusterID *int64, namespace string, sources SourceReader) (Overview, error) {
	now := s.now()
	since := now.Add(-s.overviewWindow)

	overview := Overview{
		SourceCompleteness: make(map[Producer]Coverage),
		GeneratedAt:        now,
	}

	// Source completeness
	if sources != nil {
		overview.SourceCompleteness = sources.Completeness(ctx, clusterID, namespace)
	}

	// Top signals
	topSignals, err := s.repo.CountBySignal(ctx, clusterID, namespace, since, s.overviewTopN)
	if err != nil {
		return Overview{}, err
	}
	overview.TopSignals = topSignals

	// Recent changes and action outcomes come from the SourceReader
	if sources != nil {
		changes, outcomes, err := sources.RecentChanges(ctx, clusterID, namespace, since)
		if err != nil {
			return Overview{}, err
		}
		overview.RecentChanges = changes
		overview.ActionOutcomes = outcomes
		overview.ActiveDiagnoses = sources.ActiveDiagnoses(ctx, clusterID, namespace)
	}

	// Partial flag
	for _, c := range overview.SourceCompleteness {
		if c == CoverageUnavailable || c == CoveragePartial || c == CoverageTruncated {
			overview.Partial = true
			break
		}
	}
	return overview, nil
}

// CleanupRetention removes expired occurrences. It is meant to be called by a
// periodic worker. Returns the number of rows deleted.
func (s *Service) CleanupRetention(ctx context.Context) (int64, error) {
	return s.repo.DeleteExpired(ctx, s.now(), s.retentionBatch)
}

// SourceReader is the interface the overview uses to read producer-native
// data that is not stored in signal_occurrences (active diagnoses count,
// recent change records, source completeness). Implementations are thin
// adapters over the existing M17-M31 services.
type SourceReader interface {
	// Completeness reports per-producer coverage for the scope.
	Completeness(ctx context.Context, clusterID *int64, namespace string) map[Producer]Coverage
	// ActiveDiagnoses returns the count of open/confirmed diagnosis records.
	ActiveDiagnoses(ctx context.Context, clusterID *int64, namespace string) int64
	// RecentChanges returns bounded recent platform-operation outcomes and
	// a summary of succeeded/failed/pending counts.
	RecentChanges(ctx context.Context, clusterID *int64, namespace string, since time.Time) ([]OverviewChange, OverviewOutcomes, error)
}

// NopSourceReader is the default when no producer services are wired.
type NopSourceReader struct{}

func (NopSourceReader) Completeness(context.Context, *int64, string) map[Producer]Coverage {
	return map[Producer]Coverage{}
}
func (NopSourceReader) ActiveDiagnoses(context.Context, *int64, string) int64 { return 0 }
func (NopSourceReader) RecentChanges(context.Context, *int64, string, time.Time) ([]OverviewChange, OverviewOutcomes, error) {
	return nil, OverviewOutcomes{}, nil
}
