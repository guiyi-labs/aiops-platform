package signal

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"
	"k8s-aiops.local/backend/internal/slo"
)

func burnTransition(mut func(*slo.BurnTransition)) slo.BurnTransition {
	t := slo.BurnTransition{
		SLOID:       41,
		Version:     3,
		ClusterID:   1,
		Service:     slo.ServiceRef{Kind: "Deployment", Namespace: "default", Name: "web", UID: "dep-7"},
		Template:    slo.TemplateRequestSuccessRatio,
		Previous:    slo.StateHealthy,
		Current:     slo.StateBreached,
		Ratio:       0.85,
		BurnRate:    18.0,
		WindowEnd:   time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC),
		EvaluatedAt: time.Date(2026, 8, 12, 9, 0, 5, 0, time.UTC),
		Coverage:    slo.CoverageComplete,
	}
	if mut != nil {
		mut(&t)
	}
	return t
}

type stubSLODefinitionReader struct {
	def  slo.Definition
	err  error
	call int
}

func (s *stubSLODefinitionReader) GetDefinition(_ context.Context, _ int64) (slo.Definition, error) {
	s.call++
	if s.err != nil {
		return slo.Definition{}, s.err
	}
	return s.def, nil
}

func TestSLOBurnSink_NormalizeFastBurn(t *testing.T) {
	reader := &stubSLODefinitionReader{def: slo.Definition{FastBurnRate: 10}}
	sink := NewSLOBurnSignalSink(nil, reader)
	req, ok, err := sink.Normalize(context.Background(), burnTransition(nil))
	if err != nil || !ok {
		t.Fatalf("Normalize err=%v ok=%v", err, ok)
	}
	if req.SignalID != "slo.burn.fast.v1" {
		t.Errorf("signal_id = %s, want slo.burn.fast.v1", req.SignalID)
	}
	if req.Producer != ProducerSLO {
		t.Errorf("producer = %s, want slo", req.Producer)
	}
	if req.State != StateActive {
		t.Errorf("state = %s, want active", req.State)
	}
	if req.Severity != "critical" {
		t.Errorf("severity = %s, want critical", req.Severity)
	}
	if req.Resource.UID != "dep-7" || req.Resource.Kind != "Deployment" {
		t.Errorf("resource = %+v", req.Resource)
	}
	if req.WindowEnd == nil || !req.WindowEnd.Equal(burnTransition(nil).WindowEnd) {
		t.Errorf("window_end mismatch: %v", req.WindowEnd)
	}
	if req.Coverage != CoverageComplete {
		t.Errorf("coverage = %s, want complete", req.Coverage)
	}
	if len(req.Evidence) != 1 || req.Evidence[0].Kind != "slo_burn_window" || req.Evidence[0].ID != 41 {
		t.Errorf("evidence = %+v", req.Evidence)
	}
	if req.Evidence[0].ContentHash == "" {
		t.Error("evidence content hash must be non-empty")
	}
	if reader.call != 1 {
		t.Errorf("reader calls = %d, want 1", reader.call)
	}
}

func TestSLOBurnSink_NormalizeSlowBurn(t *testing.T) {
	sink := NewSLOBurnSignalSink(nil, &stubSLODefinitionReader{def: slo.Definition{FastBurnRate: 14.4}})
	req, ok, err := sink.Normalize(context.Background(), burnTransition(func(t *slo.BurnTransition) { t.BurnRate = 2.5 }))
	if err != nil || !ok {
		t.Fatalf("Normalize err=%v ok=%v", err, ok)
	}
	if req.SignalID != "slo.burn.slow.v1" {
		t.Errorf("signal_id = %s, want slo.burn.slow.v1", req.SignalID)
	}
	if req.Severity != "warning" {
		t.Errorf("severity = %s, want warning", req.Severity)
	}
}

func TestSLOBurnSink_NormalizeFallbackThreshold(t *testing.T) {
	// Reader failure must not block the pipeline; classify against the
	// default fast burn rate.
	sink := NewSLOBurnSignalSink(nil, &stubSLODefinitionReader{err: context.DeadlineExceeded})
	req, ok, err := sink.Normalize(context.Background(), burnTransition(nil))
	if err != nil || !ok {
		t.Fatalf("Normalize err=%v ok=%v", err, ok)
	}
	if req.SignalID != "slo.burn.fast.v1" {
		t.Errorf("signal_id = %s, want slo.burn.fast.v1 (burn rate 18 >= default 14.4)", req.SignalID)
	}
}

func TestSLOBurnSink_NormalizeRecovery(t *testing.T) {
	sink := NewSLOBurnSignalSink(nil, nil)
	req, ok, err := sink.Normalize(context.Background(), burnTransition(func(t *slo.BurnTransition) {
		t.Previous = slo.StateBreached
		t.Current = slo.StateHealthy
		t.Ratio = 0.995
		t.BurnRate = 0.3
	}))
	if err != nil || !ok {
		t.Fatalf("Normalize err=%v ok=%v", err, ok)
	}
	if req.SignalID != "slo.burn.recovery.v1" {
		t.Errorf("signal_id = %s, want slo.burn.recovery.v1", req.SignalID)
	}
	if req.State != StateResolved {
		t.Errorf("state = %s, want resolved", req.State)
	}
	if req.Severity != "info" {
		t.Errorf("severity = %s, want info", req.Severity)
	}
}

func TestSLOBurnSink_NormalizeSkipsSteadyStates(t *testing.T) {
	sink := NewSLOBurnSignalSink(nil, nil)
	steady := []slo.BurnTransition{
		burnTransition(func(t *slo.BurnTransition) { t.Previous = slo.StateHealthy; t.Current = slo.StateHealthy }),
		burnTransition(func(t *slo.BurnTransition) { t.Previous = slo.StateUnavailable; t.Current = slo.StateHealthy }),
		burnTransition(func(t *slo.BurnTransition) { t.Previous = slo.StateHealthy; t.Current = slo.StateUnavailable }),
	}
	for i, tr := range steady {
		if _, ok, err := sink.Normalize(context.Background(), tr); err != nil || ok {
			t.Errorf("case %d: ok=%v err=%v, want skip", i, ok, err)
		}
	}
}

func TestSLOBurnSink_NormalizeCoveragePassthrough(t *testing.T) {
	sink := NewSLOBurnSignalSink(nil, nil)
	req, _, err := sink.Normalize(context.Background(), burnTransition(func(t *slo.BurnTransition) { t.Coverage = slo.CoveragePartial }))
	if err != nil {
		t.Fatalf("Normalize err=%v", err)
	}
	if req.Coverage != CoveragePartial {
		t.Errorf("coverage = %s, want partial", req.Coverage)
	}
}

func TestSLOBurnSink_FingerprintStableAndWindowBound(t *testing.T) {
	sink := NewSLOBurnSignalSink(nil, nil)
	r1, _, _ := sink.Normalize(context.Background(), burnTransition(nil))
	// Re-delivery of the same window must yield the same fingerprint.
	r2, _, _ := sink.Normalize(context.Background(), burnTransition(nil))
	if r1.Fingerprint != r2.Fingerprint || r1.Fingerprint == "" {
		t.Fatalf("fingerprint not stable across redelivery: %q != %q", r1.Fingerprint, r2.Fingerprint)
	}
	// A new window is a new occurrence.
	r3, _, _ := sink.Normalize(context.Background(), burnTransition(func(t *slo.BurnTransition) {
		t.WindowEnd = t.WindowEnd.Add(time.Hour)
	}))
	if r1.Fingerprint == r3.Fingerprint {
		t.Fatal("fingerprint must change when the evaluation window changes")
	}
}

func TestSLOBurnSink_IngestIsIdempotent(t *testing.T) {
	repo := &mockRepo{}
	svc := NewService(ServiceOptions{Repository: repo, Now: fixedNow})
	sink := NewSLOBurnSignalSink(svc, nil)
	tr := burnTransition(nil)
	if err := sink.OnBurnTransition(context.Background(), tr); err != nil {
		t.Fatalf("OnBurnTransition err=%v", err)
	}
	if repo.upsertCount != 1 {
		t.Errorf("upsert count = %d, want 1", repo.upsertCount)
	}
	if err := sink.OnBurnTransition(context.Background(), tr); err != nil {
		t.Fatalf("second OnBurnTransition err=%v", err)
	}
	if repo.upsertCount != 2 {
		t.Errorf("upsert count = %d, want 2 (dedupe is the repository's ON CONFLICT contract)", repo.upsertCount)
	}
}

func TestSLOBurnSink_DisabledWhenServiceNil(t *testing.T) {
	sink := NewSLOBurnSignalSink(nil, nil)
	if err := sink.OnBurnTransition(context.Background(), burnTransition(nil)); err != nil {
		t.Errorf("disabled sink must not error: %v", err)
	}
}

func TestSLOBurnSink_WithLoggerSetsNonNil(t *testing.T) {
	sink := NewSLOBurnSignalSink(nil, nil)
	if returned := sink.WithLogger(zap.NewNop()); returned != sink {
		t.Fatal("WithLogger must return the receiver")
	}
	if sink.logger == nil {
		t.Fatal("WithLogger must set the logger")
	}
	if returned := sink.WithLogger(nil); returned != sink || sink.logger == nil {
		t.Fatal("WithLogger(nil) must keep the existing logger")
	}
}

func TestSLOBurnSink_IngestFailureLoggedNotPropagated(t *testing.T) {
	failing := &failingUpsertRepo{}
	svc := NewService(ServiceOptions{Repository: failing, Now: fixedNow})
	sink := NewSLOBurnSignalSink(svc, nil).WithLogger(zap.NewNop())
	if err := sink.OnBurnTransition(context.Background(), burnTransition(nil)); err != nil {
		t.Fatalf("OnBurnTransition must not propagate an error: %v", err)
	}
}

type failingUpsertRepo struct{ mockRepo }

func (failingUpsertRepo) Upsert(context.Context, *Occurrence) error {
	return errors.New("upsert failed")
}
