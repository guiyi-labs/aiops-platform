package alert

import (
	"context"
	"encoding/json"
	"time"

	"k8s-aiops.local/backend/internal/diagnosis"
	"k8s-aiops.local/backend/internal/metricshistory"
)

type MetricEvaluator interface {
	Evaluate(ctx context.Context, query metricshistory.EvaluationQuery) (metricshistory.EvaluationResponse, error)
}

type DiagnosisRepository interface {
	Save(ctx context.Context, record *diagnosis.Record) error
}

type Service struct {
	repo            Repository
	diagnosisRepo   DiagnosisRepository
	metricEvaluator MetricEvaluator
	now             func() time.Time
	minEvalInterval time.Duration
}

func NewService(repo Repository, diagnosisRepo DiagnosisRepository, metricEvaluator MetricEvaluator, minEvalInterval time.Duration) *Service {
	return &Service{
		repo:            repo,
		diagnosisRepo:   diagnosisRepo,
		metricEvaluator: metricEvaluator,
		now:             time.Now,
		minEvalInterval: minEvalInterval,
	}
}

func (s *Service) CreateRule(ctx context.Context, input CreateRuleInput, actor ActorRef) (Rule, error) {
	if err := ValidateCreate(input); err != nil {
		return Rule{}, err
	}
	now := s.now()
	rule := Rule{
		ClusterID:     input.ClusterID,
		DisplayName:   input.DisplayName,
		ResourceKind:  input.ResourceKind,
		ResourceName:  input.ResourceName,
		MetricName:    input.MetricName,
		Operator:      input.Operator,
		Threshold:     input.Threshold,
		ForSeconds:    input.ForSeconds,
		MinimumPoints: input.MinimumPoints,
		NextDueAt:     now,
		Creator:       actor,
	}
	if err := s.repo.CreateRule(ctx, &rule, s.minEvalInterval); err != nil {
		return Rule{}, err
	}
	return rule, nil
}

func (s *Service) GetRule(ctx context.Context, clusterID, id int64) (Rule, error) {
	rule, err := s.repo.GetRule(ctx, id)
	if err != nil || rule.ClusterID != clusterID {
		if err == nil {
			err = ErrRuleNotFound
		}
		return Rule{}, err
	}
	return rule, nil
}

func (s *Service) ListRules(ctx context.Context, filter RuleListFilter) ([]Rule, error) {
	if filter.Limit <= 0 {
		filter.Limit = 100
	}
	return s.repo.ListRules(ctx, filter)
}

func (s *Service) PatchRule(ctx context.Context, clusterID, id int64, input PatchRuleInput, actor ActorRef) (Rule, error) {
	if err := ValidatePatch(input); err != nil {
		return Rule{}, err
	}
	if _, err := s.GetRule(ctx, clusterID, id); err != nil {
		return Rule{}, err
	}
	return s.repo.PatchRule(ctx, id, input, actor)
}

func (s *Service) DeleteRule(ctx context.Context, clusterID, id int64) error {
	if _, err := s.GetRule(ctx, clusterID, id); err != nil {
		return err
	}
	return s.repo.DeleteRule(ctx, id)
}

func (s *Service) ListInstances(ctx context.Context, filter InstanceListFilter) ([]Instance, error) {
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}
	return s.repo.ListInstances(ctx, filter)
}

func (s *Service) GetInstance(ctx context.Context, clusterID, id int64) (Instance, error) {
	instance, err := s.repo.GetInstance(ctx, id)
	if err != nil {
		return Instance{}, err
	}
	if _, err := s.GetRule(ctx, clusterID, instance.RuleID); err != nil {
		return Instance{}, ErrAlertNotFound
	}
	return instance, nil
}

// EvaluateRule evaluates a single alert rule against the metrics history.
// It implements the state machine from ADR 0043 section 2.
func (s *Service) EvaluateRule(ctx context.Context, rule Rule) error {
	now := s.now()
	evalFrom := now.Add(-evaluationLookback(rule, s.minEvalInterval))
	evalTo := now

	eval, err := s.metricEvaluator.Evaluate(ctx, metricshistory.EvaluationQuery{
		SeriesQuery: metricshistory.SeriesQuery{
			ClusterID:    rule.ClusterID,
			ResourceKind: rule.ResourceKind,
			ResourceName: rule.ResourceName,
			MetricName:   rule.MetricName,
			From:         evalFrom,
			To:           evalTo,
		},
		EvaluationRule: metricshistory.EvaluationRule{
			Operator:      rule.Operator,
			Threshold:     rule.Threshold,
			ForSeconds:    rule.ForSeconds,
			MinimumPoints: rule.MinimumPoints,
		},
	})

	if err != nil {
		nextDue := now.Add(s.minEvalInterval)
		return s.repo.ReleaseClaim(ctx, rule.ID, nextDue, EvalStateError, now, errorCode(err))
	}

	nextDue := now.Add(s.minEvalInterval)

	switch eval.State {
	case metricshistory.EvaluationStateFiring:
		return s.handleFiring(ctx, rule, eval, now, nextDue)
	case metricshistory.EvaluationStateNormal:
		return s.handleNormal(ctx, rule, now, nextDue)
	default:
		// insufficient_data — update health only
		return s.repo.ReleaseClaim(ctx, rule.ID, nextDue, EvalStateInsufficient, now, "")
	}
}

// evaluationLookback keeps scheduler state tied to recent evidence. The
// historical evaluator intentionally reports any sustained window in the
// queried series; querying a fixed six hours would therefore keep a recovered
// alert firing for six hours. Two extra scheduler intervals preserve enough
// boundary slack for collection timestamp jitter and the requested
// duration/minimum-point contract while
// allowing a complete recent normal window to resolve the active instance.
func evaluationLookback(rule Rule, evaluationInterval time.Duration) time.Duration {
	if evaluationInterval <= 0 {
		evaluationInterval = time.Minute
	}
	minimumPoints := rule.MinimumPoints
	if minimumPoints < 2 {
		minimumPoints = 2
	}
	byDuration := time.Duration(rule.ForSeconds) * time.Second
	byPoints := time.Duration(minimumPoints-1) * evaluationInterval
	lookback := byDuration
	if byPoints > lookback {
		lookback = byPoints
	}
	lookback += 2 * evaluationInterval
	if lookback > 24*time.Hour {
		return 24 * time.Hour
	}
	return lookback
}

func (s *Service) handleFiring(ctx context.Context, rule Rule, eval metricshistory.EvaluationResponse, now time.Time, nextDue time.Time) error {
	existing, err := s.repo.GetUnresolvedInstance(ctx, rule.ID)
	if err != nil {
		return err
	}

	anchor, _ := json.Marshal(map[string]any{
		"state":             eval.State,
		"points_evaluated":  eval.PointsEvaluated,
		"breaching_points":  eval.BreachingPoints,
		"observed_span":     eval.ObservedSpanSeconds,
		"sustained_windows": len(eval.SustainedWindows),
	})

	if existing != nil {
		// Repeated firing — touch instance, no new diagnosis
		if err := s.repo.TouchInstance(ctx, rule.ID, now, string(anchor)); err != nil {
			return err
		}
		return s.repo.ReleaseClaim(ctx, rule.ID, nextDue, EvalStateFiring, now, "")
	}

	// First firing — create diagnosis and alert instance
	record, matched := diagnosis.EvaluateSustainedMetricBreach(rule.ClusterID, eval, now)
	if !matched {
		return s.repo.ReleaseClaim(ctx, rule.ID, nextDue, EvalStateFiring, now, "")
	}
	record.SLADueAt = diagnosis.SLADeadline(record.Severity, record.ObservedAt)
	instance := &Instance{
		RuleID:               rule.ID,
		FirstFiredAt:         now,
		LastFiredAt:          now,
		LatestEvidenceAnchor: string(anchor),
	}
	if err := s.repo.CreateFiring(ctx, &record, instance); err != nil {
		return err
	}

	return s.repo.ReleaseClaim(ctx, rule.ID, nextDue, EvalStateFiring, now, "")
}

func (s *Service) handleNormal(ctx context.Context, rule Rule, now time.Time, nextDue time.Time) error {
	existing, err := s.repo.GetUnresolvedInstance(ctx, rule.ID)
	if err != nil {
		return err
	}
	if existing != nil {
		if err := s.repo.ResolveInstance(ctx, rule.ID, now); err != nil {
			return err
		}
	}
	return s.repo.ReleaseClaim(ctx, rule.ID, nextDue, EvalStateNormal, now, "")
}

func errorCode(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if len(msg) > 32 {
		msg = msg[:32]
	}
	return msg
}
