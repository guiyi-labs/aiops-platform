package signal

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"k8s-aiops.local/backend/internal/diagnosis"
)

// DiagnosisReader lists persisted diagnosis records for normalization. main
// wires *diagnosis.GormRepository; tests use a fake.
type DiagnosisReader interface {
	List(context.Context, diagnosis.ListFilter) ([]diagnosis.Record, error)
}

// DrainConfig bounds one diagnosis→signal drain pass and its lifecycle.
type DrainConfig struct {
	// Interval between drain passes.
	Interval time.Duration
	// PageSize bounds one list call so a pass never scans unbounded history.
	PageSize int
}

// DefaultDrainInterval is the drain cadence when DrainConfig.Interval is zero.
const DefaultDrainInterval = 5 * time.Second

// DefaultDrainPageSize caps one list call when DrainConfig.PageSize is zero.
const DefaultDrainPageSize = 50

// DiagnosisDrain periodically normalizes new/updated diagnosis records into
// signal occurrences (producer=diagnosis). The DiagnosisNormalizer was
// compiled but never wired on the server path; without a drain, diagnosis
// records never reached the M39 signal layer (overview / correlation /
// incident signal source). The drain is a strict-`updated_at` cursor over
// diagnosis_records: each pass re-normalizes records updated after the last
// watermark and upserts by fingerprint (idempotent), so create and
// state-transition changes both converge. Unmapped rules are skipped;
// ingest failures are logged and never crash the drain.
type DiagnosisDrain struct {
	config     DrainConfig
	diagnoses  DiagnosisReader
	signals    *Service
	normalizer DiagnosisNormalizer
	now        func() time.Time
	logger     *zap.Logger
	watermark  time.Time
}

// NewDiagnosisDrain constructs a DiagnosisDrain. Zero config fields receive
// sane defaults; a nil logger becomes a no-op logger.
func NewDiagnosisDrain(config DrainConfig, diagnoses DiagnosisReader, signals *Service, logger *zap.Logger) *DiagnosisDrain {
	if config.Interval <= 0 {
		config.Interval = DefaultDrainInterval
	}
	if config.PageSize <= 0 {
		config.PageSize = DefaultDrainPageSize
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &DiagnosisDrain{
		config:     config,
		diagnoses:  diagnoses,
		signals:    signals,
		normalizer: DiagnosisNormalizer{},
		now:        time.Now,
		logger:     logger,
	}
}

// Run drains immediately and then every Interval until the context is
// cancelled. The watermark starts at now so existing history is not replayed
// on boot; only records updated after startup are normalized.
func (d *DiagnosisDrain) Run(ctx context.Context) {
	d.watermark = d.now().UTC()
	d.drainOnce(ctx)
	ticker := time.NewTicker(d.config.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.drainOnce(ctx)
		}
	}
}

// drainOnce lists records updated after the watermark (strictly), normalizes
// and ingests each, then advances the watermark to the newest updated_at in
// the batch. Errors are logged per record; one bad record never blocks the
// rest of the batch.
func (d *DiagnosisDrain) drainOnce(ctx context.Context) {
	if d.diagnoses == nil || d.signals == nil {
		return
	}
	records, err := d.diagnoses.List(ctx, diagnosis.ListFilter{
		UpdatedAfter: &d.watermark,
		Limit:        d.config.PageSize,
	})
	if err != nil {
		d.logger.Warn("diagnosis signal drain: list records", zap.Error(err))
		return
	}
	runID := fmt.Sprintf("drain-%d", d.now().UTC().Unix())
	cutoff := d.watermark
	for _, record := range records {
		if record.UpdatedAt.After(cutoff) {
			cutoff = record.UpdatedAt
		}
		req, err := d.normalizer.FromRecord(record, runID)
		if err != nil {
			// Unmapped rule — not a pipeline failure. Diagnostic-only rules
			// intentionally have no signal id yet.
			d.logger.Debug("diagnosis signal drain: skip unmapped rule",
				zap.Int64("diagnosis_id", record.ID),
				zap.String("rule_id", record.RuleID),
				zap.Error(err))
			continue
		}
		if _, err := d.signals.Ingest(ctx, req); err != nil {
			d.logger.Warn("diagnosis signal drain: ingest failed",
				zap.Int64("diagnosis_id", record.ID),
				zap.String("signal_id", req.SignalID),
				zap.Error(err))
			continue
		}
		d.logger.Debug("diagnosis signal drain: ingested",
			zap.Int64("diagnosis_id", record.ID),
			zap.String("signal_id", req.SignalID))
	}
	d.watermark = cutoff
}
