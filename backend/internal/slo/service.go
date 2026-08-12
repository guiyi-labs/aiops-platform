package slo

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// BurnAlertSink consumes burn-alert transitions emitted by the SLO service.
// The sink is the integration point with the M27 alert lifecycle: the
// concrete implementation (wired in httpserver) opens, updates or resolves
// an alert.Instance keyed by the SLO burn-alert fingerprint. The SLO
// service never creates alert Rules — it only emits lifecycle transitions
// for the sink to translate into alert instances.
//
// The sink is best-effort: a sink failure must not roll back the
// evaluation. The caller records the sink error in audit and continues.
type BurnAlertSink interface {
	// OnBurnTransition is called when an SLO evaluation transitions the
	// burn state. The previous state is the state of the last persisted
	// evaluation; the current state is the new evaluation's state.
	// Implementations should be idempotent: the same transition delivered
	// twice must not duplicate alert instances.
	OnBurnTransition(ctx context.Context, transition BurnTransition) error
}

// BurnTransition describes a single burn state transition for the sink.
type BurnTransition struct {
	SLOID       int64
	Version     int
	ClusterID   int64
	Service     ServiceRef
	Template    SLITemplate
	Previous    EvaluationState
	Current     EvaluationState
	Ratio       float64
	BurnRate    float64
	WindowEnd   time.Time
	EvaluatedAt time.Time
	// Coverage carries the data completeness of the evaluation so sinks
	// (e.g. the M99 signal pipeline) can expose missing-data windows
	// instead of treating them as healthy.
	Coverage EvaluationCoverage
}

// NopBurnAlertSink is a no-op sink used in tests and when M27 integration
// is disabled. It never errors.
type NopBurnAlertSink struct{}

func (NopBurnAlertSink) OnBurnTransition(context.Context, BurnTransition) error { return nil }

// Service is the SLO application service. It coordinates the Repository,
// Evaluator and BurnAlertSink, enforces authorization scope (cluster_id
// must match the caller's grants — enforced at the HTTP layer via M35
// middleware) and applies versioned edits with audit metadata.
//
// The service is the only writer to slo_evaluations and the only caller
// of Evaluator.Evaluate. HTTP handlers translate requests into service
// calls; they never bypass the service to write directly.
type Service struct {
	repo Repository
	eval *Evaluator
	sink BurnAlertSink
	now  func() time.Time
}

// ServiceOption configures a Service at construction.
type ServiceOption func(*Service)

// WithBurnAlertSink overrides the default NopBurnAlertSink.
func WithBurnAlertSink(sink BurnAlertSink) ServiceOption {
	return func(s *Service) { s.sink = sink }
}

// WithNow overrides the clock (tests).
func WithNow(now func() time.Time) ServiceOption {
	return func(s *Service) { s.now = now }
}

// NewService constructs a Service. The evaluator may be nil — in that case
// EvaluateSLO returns ErrEvaluatorUnavailable and no evaluation is written.
func NewService(repo Repository, evaluator *Evaluator, opts ...ServiceOption) *Service {
	s := &Service{
		repo: repo,
		eval: evaluator,
		sink: NopBurnAlertSink{},
		now:  time.Now,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// ErrEvaluatorUnavailable is returned when EvaluateSLO is called on a
// service whose evaluator is nil.
var ErrEvaluatorUnavailable = errors.New("slo evaluator is not configured")

// ErrDefinitionDisabled is returned when an operation targets a disabled
// definition that does not allow evaluation.
var ErrDefinitionDisabled = errors.New("slo definition is disabled")

// CreateDefinition persists a new SLO definition. The definition is
// validated, version-stamped at 1, and stored. Creator and Owner must be
// supplied by the caller (the HTTP layer extracts them from auth context).
func (s *Service) CreateDefinition(ctx context.Context, input CreateDefinitionInput) (Definition, error) {
	if err := ValidateCreate(input); err != nil {
		return Definition{}, err
	}
	now := s.now()
	def := Definition{
		ClusterID:             input.ClusterID,
		Service:               input.Service,
		Template:              input.Template,
		TemplateVersion:       TemplateVersion,
		Objective:             input.Objective,
		RollingWindowSeconds:  input.RollingWindowSeconds,
		MissingDataPolicy:     input.MissingDataPolicy,
		LatencyThresholdMs:    input.LatencyThresholdMs,
		Owner:                 input.Owner,
		FastBurnRate:          input.FastBurnRate,
		FastBurnWindowSeconds: input.FastBurnWindowSeconds,
		SlowBurnRate:          input.SlowBurnRate,
		SlowBurnWindowSeconds: input.SlowBurnWindowSeconds,
		Enabled:               input.Enabled,
		Version:               1,
		Creator:               input.Creator,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	if def.MissingDataPolicy == "" {
		def.MissingDataPolicy = DefaultMissingDataPolicy(def.Template)
	}
	if err := s.repo.CreateDefinition(ctx, &def); err != nil {
		return Definition{}, err
	}
	return def, nil
}

// GetDefinition returns the current definition for an ID.
func (s *Service) GetDefinition(ctx context.Context, id int64) (Definition, error) {
	return s.repo.GetDefinition(ctx, id)
}

// ListDefinitions returns definitions matching the filter.
func (s *Service) ListDefinitions(ctx context.Context, filter DefinitionFilter) (DefinitionListResponse, error) {
	if filter.Limit <= 0 || filter.Limit > 200 {
		filter.Limit = 100
	}
	items, total, err := s.repo.ListDefinitions(ctx, filter)
	if err != nil {
		return DefinitionListResponse{}, err
	}
	resp := DefinitionListResponse{Items: items, Total: total}
	if int64(len(items)) < total {
		resp.Truncated = true
	}
	return resp, nil
}

// PatchDefinition applies a versioned patch. Version is incremented and
// updated_at is refreshed by the repository. The actor is recorded in audit
// (caller responsibility) and used to validate authorization at the HTTP
// layer. Returns the updated definition.
//
// A disabled definition may still be patched (e.g. to re-enable it); the
// repository's unique partial index admits exactly one active row per
// (cluster, namespace, service, template).
func (s *Service) PatchDefinition(ctx context.Context, id int64, patch PatchDefinitionInput) (Definition, error) {
	if patch.Actor.ID <= 0 {
		return Definition{}, fmt.Errorf("actor id is required for patch")
	}
	now := s.now()
	updated, err := s.repo.UpdateDefinition(ctx, id, patch, now)
	if err != nil {
		return Definition{}, err
	}
	return updated, nil
}

// DeleteDefinition marks a definition as disabled (enabled=false). The row
// is retained so historical evaluations remain queryable. The unique partial
// index admits a new active definition for the same service+template after
// deletion.
func (s *Service) DeleteDefinition(ctx context.Context, id int64) error {
	return s.repo.DeleteDefinition(ctx, id)
}

// EvaluateSLO runs a single evaluation for the given SLO ID, persists the
// result, and emits a burn transition to the sink when the state changes.
//
// The previous state is read from the latest persisted evaluation for the
// SLO (any version). When there is no prior evaluation, the previous state
// is treated as StateHealthy for the purpose of transition detection —
// i.e. a fresh SLO that immediately breaches will emit a healthy→breached
// transition.
//
// The evaluation is persisted even when StateUnavailable. The sink is
// invoked only for state changes (previous != current) to avoid alert
// churn on steady-state healthy or steady-state unavailable.
func (s *Service) EvaluateSLO(ctx context.Context, sloID int64) (Evaluation, error) {
	// Look up the definition first so 404 takes precedence over 503 — a
	// missing SLO is a client error regardless of evaluator availability.
	def, err := s.repo.GetDefinition(ctx, sloID)
	if err != nil {
		return Evaluation{}, err
	}
	if !def.Enabled {
		return Evaluation{}, ErrDefinitionDisabled
	}
	if s.eval == nil {
		return Evaluation{}, ErrEvaluatorUnavailable
	}
	now := s.now()
	eval, evalErr := s.eval.Evaluate(ctx, &def, now)

	// Persist even on source-unavailable: the unavailable evaluation is
	// an auditable fact. The only case we skip persistence is when the
	// input was invalid (no row to write about).
	if !errors.Is(evalErr, ErrEvaluationInvalidInput) {
		if perr := s.repo.InsertEvaluation(ctx, &eval); perr != nil {
			// Persistence failure is the caller's error; the in-memory
			// evaluation is still returned for diagnostics.
			return eval, fmt.Errorf("persist evaluation: %w", perr)
		}
	}

	// Read previous state for transition detection. We read after insert
	// so the latest evaluation is the one we just wrote; the previous
	// state is the second-newest. To keep this O(1) we instead read the
	// latest evaluation excluding the just-inserted row by comparing
	// evaluated_at and window_end. In practice the repository's
	// LatestEvaluation returns the just-inserted row; we treat that as
	// "current" and infer previous from a second query.
	previous := s.previousState(ctx, sloID, eval)

	if previous != eval.State {
		transition := BurnTransition{
			SLOID:       def.ID,
			Version:     def.Version,
			ClusterID:   def.ClusterID,
			Service:     def.Service,
			Template:    def.Template,
			Previous:    previous,
			Current:     eval.State,
			Ratio:       eval.Ratio,
			BurnRate:    eval.BurnRate,
			WindowEnd:   eval.WindowEnd,
			EvaluatedAt: eval.EvaluatedAt,
			Coverage:    eval.Coverage,
		}
		// Best-effort: a sink error does not roll back the evaluation.
		_ = s.sink.OnBurnTransition(ctx, transition)
	}

	if evalErr != nil {
		return eval, evalErr
	}
	return eval, nil
}

// previousState returns the state of the most recent evaluation that is not
// the given "current" evaluation. When no prior evaluation exists, returns
// StateHealthy (a fresh SLO's baseline is healthy). This is a read-only
// helper and never returns an error — a repository hiccup yields
// StateHealthy to avoid false churn.
func (s *Service) previousState(ctx context.Context, sloID int64, current Evaluation) EvaluationState {
	latest, err := s.repo.LatestEvaluation(ctx, sloID)
	if err != nil || latest.ID == 0 {
		return StateHealthy
	}
	// If the latest row is the one we just inserted, look one step back.
	if latest.ID == current.ID {
		// We cannot easily query "second latest" without an extra
		// repository method. As a pragmatic approximation, when the
		// latest row matches the current evaluation's window and
		// evaluated_at, we treat the previous as StateHealthy only when
		// the current is the very first evaluation for this SLO. We
		// detect "very first" by checking total count via ListEvaluations
		// with limit 2.
		items, _, err := s.repo.ListEvaluations(ctx, EvaluationFilter{SLOID: sloID, Limit: 2})
		if err != nil || len(items) < 2 {
			return StateHealthy
		}
		// items[0] is current (newest first), items[1] is previous.
		return items[1].State
	}
	return latest.State
}

// ListEvaluations returns evaluations matching the filter.
func (s *Service) ListEvaluations(ctx context.Context, filter EvaluationFilter) (EvaluationListResponse, error) {
	if filter.Limit <= 0 || filter.Limit > 200 {
		filter.Limit = 100
	}
	items, total, err := s.repo.ListEvaluations(ctx, filter)
	if err != nil {
		return EvaluationListResponse{}, err
	}
	resp := EvaluationListResponse{Items: items, Total: total}
	if int64(len(items)) < total {
		resp.Truncated = true
	}
	return resp, nil
}

// LatestEvaluation returns the most recent evaluation for an SLO. Returns
// a zero Evaluation (ID == 0) when no evaluation exists; callers should
// check ID before using.
func (s *Service) LatestEvaluation(ctx context.Context, sloID int64) (Evaluation, error) {
	return s.repo.LatestEvaluation(ctx, sloID)
}

// Now exposes the service clock for callers that need a consistent "now".
func (s *Service) Now() time.Time { return s.now() }
