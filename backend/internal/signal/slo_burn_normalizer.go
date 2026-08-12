package signal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"go.uber.org/zap"

	"k8s-aiops.local/backend/internal/slo"
)

// SLODefinitionReader loads an SLO definition by ID. The normalizer uses the
// definition's FastBurnRate to classify fast vs slow burn; when the reader is
// unavailable it falls back to DefaultFastBurnRate so the pipeline never
// blocks an SLO evaluation.
type SLODefinitionReader interface {
	GetDefinition(ctx context.Context, id int64) (slo.Definition, error)
}

// DefaultFastBurnRate is the fast/slow burn boundary used when the SLO
// definition cannot be loaded. Mirrors the SRE book's canonical fast burn
// rate; definitions may override it with their own FastBurnRate.
const DefaultFastBurnRate = 14.4

// SLOBurnSignalSink implements slo.BurnAlertSink and translates burn-state
// transitions into normalized signal occurrences (slo.burn.fast.v1 /
// slo.burn.slow.v1 / slo.burn.recovery.v1). It is the M99 integration point
// between SLO error-budget evaluations and the signal pipeline.
//
// The sink is best-effort and idempotent, matching the slo contract: a sink
// failure is logged, never propagated to the evaluation. Re-delivering the
// same transition yields the same fingerprint and deduplicates via the
// occurrence upsert.
type SLOBurnSignalSink struct {
	svc    *Service
	reader SLODefinitionReader
	logger *zap.Logger
}

// NewSLOBurnSignalSink constructs a sink. svc may be nil (sink disabled); the
// reader may be nil (fast/slow classification falls back to the default).
func NewSLOBurnSignalSink(svc *Service, reader SLODefinitionReader) *SLOBurnSignalSink {
	return &SLOBurnSignalSink{svc: svc, reader: reader, logger: zap.NewNop()}
}

// WithLogger sets the logger used for best-effort ingest failures.
func (s *SLOBurnSignalSink) WithLogger(l *zap.Logger) *SLOBurnSignalSink {
	if l != nil {
		s.logger = l
	}
	return s
}

// OnBurnTransition implements slo.BurnAlertSink. Steady-state transitions
// (healthy→healthy, unavailable→healthy, healthy→unavailable) emit nothing.
func (s *SLOBurnSignalSink) OnBurnTransition(ctx context.Context, t slo.BurnTransition) error {
	if s.svc == nil {
		return nil
	}
	req, ok, err := s.Normalize(ctx, t)
	if err != nil || !ok {
		return err
	}
	if _, err := s.svc.Ingest(ctx, req); err != nil {
		s.logger.Warn("slo burn signal ingest failed",
			zap.Int64("slo_id", t.SLOID),
			zap.String("signal_id", req.SignalID),
			zap.Error(err))
	}
	return nil
}

// Normalize converts a burn transition into an IngestRequest. ok=false when
// the transition is not a breach or recovery and no signal should be emitted.
// The conversion is deterministic: identical transitions produce identical
// requests (fingerprint included).
func (s *SLOBurnSignalSink) Normalize(ctx context.Context, t slo.BurnTransition) (IngestRequest, bool, error) {
	code, severity, state, err := s.classify(ctx, t)
	if err != nil {
		return IngestRequest{}, false, err
	}
	if code == "" {
		return IngestRequest{}, false, nil
	}
	req := IngestRequest{
		SignalID:    code,
		Producer:    ProducerSLO,
		ClusterID:   t.ClusterID,
		Namespace:   t.Service.Namespace,
		Resource:    ResourceCitation{Kind: t.Service.Kind, Namespace: t.Service.Namespace, Name: t.Service.Name, UID: t.Service.UID, Incomplete: t.Service.Incomplete},
		Severity:    severity,
		State:       state,
		Fingerprint: burnFingerprint(code, t),
		Coverage:    mapCoverage(t.Coverage),
		Freshness:   t.EvaluatedAt,
		WindowEnd:   &t.WindowEnd,
		ObservedAt:  t.EvaluatedAt,
		Attributes: map[string]string{
			"slo_id":        fmt.Sprintf("%d", t.SLOID),
			"slo_version":   fmt.Sprintf("%d", t.Version),
			"template":      string(t.Template),
			"ratio":         fmt.Sprintf("%.6f", t.Ratio),
			"burn_rate":     fmt.Sprintf("%.4f", t.BurnRate),
			"target_kind":   t.Service.Kind,
			"target_name":   t.Service.Name,
			"window_end":    t.WindowEnd.UTC().Format("2006-01-02T15:04:05Z"),
			"burn_previous": string(t.Previous),
			"burn_current":  string(t.Current),
		},
		Evidence: []EvidenceRef{{
			Kind:        "slo_burn_window",
			ID:          t.SLOID,
			ContentHash: burnWindowHash(t),
		}},
	}
	return req, true, nil
}

func (s *SLOBurnSignalSink) classify(ctx context.Context, t slo.BurnTransition) (code string, severity string, state State, err error) {
	switch {
	case t.Current == slo.StateBreached:
		rate := DefaultFastBurnRate
		if s.reader != nil {
			if def, derr := s.reader.GetDefinition(ctx, t.SLOID); derr == nil && def.FastBurnRate > 0 {
				rate = def.FastBurnRate
			}
		}
		if t.BurnRate >= rate {
			return "slo.burn.fast.v1", "critical", StateActive, nil
		}
		return "slo.burn.slow.v1", "warning", StateActive, nil
	case t.Previous == slo.StateBreached && t.Current == slo.StateHealthy:
		return "slo.burn.recovery.v1", "info", StateResolved, nil
	default:
		// healthy→healthy, unavailable→healthy, healthy→unavailable, etc.
		return "", "", "", nil
	}
}

func mapCoverage(c slo.EvaluationCoverage) Coverage {
	switch c {
	case slo.CoverageComplete:
		return CoverageComplete
	case slo.CoveragePartial:
		return CoveragePartial
	default:
		return CoverageUnavailable
	}
}

// burnFingerprint is stable per (signal code, SLO, cluster, target, window).
// Two sink deliveries for the same evaluation window deduplicate; a new
// window produces a new occurrence.
func burnFingerprint(code string, t slo.BurnTransition) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%d|%d|%s|%s|%s|%s|%s",
		code,
		t.SLOID,
		t.ClusterID,
		t.Service.Kind,
		t.Service.Namespace,
		t.Service.Name,
		t.Service.UID,
		t.WindowEnd.UTC().Format("2006-01-02T15:04:05Z"),
	)
	return hex.EncodeToString(h.Sum(nil))
}

// burnWindowHash is a content hash over the observable burn facts, used as
// evidence content_hash so consumers can detect drift without re-fetching.
func burnWindowHash(t slo.BurnTransition) string {
	h := sha256.New()
	fmt.Fprintf(h, "%d|%d|%.6f|%.4f|%s|%s|%s",
		t.SLOID,
		t.Version,
		t.Ratio,
		t.BurnRate,
		t.WindowEnd.UTC().Format("2006-01-02T15:04:05Z"),
		t.Previous,
		t.Current,
	)
	return hex.EncodeToString(h.Sum(nil))
}
