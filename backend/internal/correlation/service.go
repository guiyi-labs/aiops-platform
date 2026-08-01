package correlation

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// InputProvider gathers the engine inputs for one correlation pass. The
// service is the only caller; the provider decouples the engine from the
// signal/topology/diagnosis packages. Implementations map their concrete
// rows into the typed correlation inputs.
//
// All methods are bounded by the caller's authorization scope (cluster_id,
// namespace) — M35 enforces scope at the HTTP layer; the provider never
// returns rows outside the caller's grants.
type InputProvider interface {
	// ActiveSignals returns signal occurrences that may trigger or contextualize
	// cases within the bounded lookback window. Only active (non-resolved)
	// occurrences are returned; the engine does not correlate resolved signals.
	ActiveSignals(ctx context.Context, clusterID int64, namespace string, lookback time.Duration) ([]SignalOccurrenceInput, error)
	// RecentChanges returns change events within the lookback window.
	RecentChanges(ctx context.Context, clusterID int64, namespace string, lookback time.Duration) ([]ChangeEventInput, error)
	// TopologyEdges returns active topology edges for the scope.
	TopologyEdges(ctx context.Context, clusterID int64, namespace string) ([]TopologyEdgeInput, error)
	// RecentDiagnoses returns diagnosis records within the lookback window.
	RecentDiagnoses(ctx context.Context, clusterID int64, namespace string, lookback time.Duration) ([]DiagnosisRef, error)
}

// NopInputProvider returns empty inputs. Used when correlation is disabled or
// in tests that construct EngineInputs directly.
type NopInputProvider struct{}

func (NopInputProvider) ActiveSignals(context.Context, int64, string, time.Duration) ([]SignalOccurrenceInput, error) {
	return nil, nil
}
func (NopInputProvider) RecentChanges(context.Context, int64, string, time.Duration) ([]ChangeEventInput, error) {
	return nil, nil
}
func (NopInputProvider) TopologyEdges(context.Context, int64, string) ([]TopologyEdgeInput, error) {
	return nil, nil
}
func (NopInputProvider) RecentDiagnoses(context.Context, int64, string, time.Duration) ([]DiagnosisRef, error) {
	return nil, nil
}

// CorrelateResult reports what one correlation pass produced and persisted.
type CorrelateResult struct {
	ClusterID       int64
	Namespace       string
	InputsGathered  int
	ResultsProduced int
	CasesUpserted   int
	Partial         bool
	Error           error
}

// Service is the M42 correlation application service. It is the only writer
// to correlation_* tables and the only caller of Engine.Correlate. HTTP
// handlers translate requests into service calls; they never bypass the
// service to write directly.
//
// The service is idempotent: running CorrelateNamespace twice with the same
// inputs produces the same persisted rows (deduplicated by case_key and
// unique indexes). The engine is pure; the service gathers inputs, persists
// results and reconciles case status.
type Service struct {
	engine   *Engine
	repo     Repository
	provider InputProvider
	now      func() time.Time
	lookback time.Duration
}

// ServiceOption configures a Service at construction.
type ServiceOption func(*Service)

// WithNow overrides the clock (tests).
func WithNow(now func() time.Time) ServiceOption {
	return func(s *Service) { s.now = now }
}

// WithLookback overrides the default input lookback window.
func WithLookback(d time.Duration) ServiceOption {
	return func(s *Service) { s.lookback = d }
}

// DefaultLookback is the default window for gathering signals, changes and
// diagnoses. Bounded so a correlation pass never reads unbounded history.
const DefaultLookback = 4 * time.Hour

// NewService constructs a Service. repository must be non-nil. The engine
// defaults to NewEngine() when nil; the provider defaults to NopInputProvider
// when nil (query-only mode).
func NewService(repo Repository, engine *Engine, provider InputProvider, opts ...ServiceOption) *Service {
	if engine == nil {
		engine = NewEngine()
	}
	if provider == nil {
		provider = NopInputProvider{}
	}
	s := &Service{
		engine:   engine,
		repo:     repo,
		provider: provider,
		now:      time.Now,
		lookback: DefaultLookback,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// ErrCorrelationDisabled is returned when CorrelateNamespace is called on a
// service whose provider is the NopInputProvider (query-only mode).
var ErrCorrelationDisabled = errors.New("correlation input provider is not configured")

// CorrelateNamespace gathers inputs for one namespace, runs the engine and
// persists the results. The pass is bounded by the lookback window. Partial
// input-gathering failures are recorded but do not block persisting the
// results that were produced.
func (s *Service) CorrelateNamespace(ctx context.Context, clusterID int64, namespace string) (CorrelateResult, error) {
	if s.provider == nil {
		return CorrelateResult{}, ErrCorrelationDisabled
	}
	now := s.now().UTC()
	result := CorrelateResult{ClusterID: clusterID, Namespace: namespace}

	signals, err := s.provider.ActiveSignals(ctx, clusterID, namespace, s.lookback)
	if err != nil {
		result.Partial = true
		result.Error = fmt.Errorf("gather signals: %w", err)
	}
	changes, err := s.provider.RecentChanges(ctx, clusterID, namespace, s.lookback)
	if err != nil {
		result.Partial = true
		if result.Error == nil {
			result.Error = fmt.Errorf("gather changes: %w", err)
		}
	}
	edges, err := s.provider.TopologyEdges(ctx, clusterID, namespace)
	if err != nil {
		result.Partial = true
		if result.Error == nil {
			result.Error = fmt.Errorf("gather edges: %w", err)
		}
	}
	diagnoses, err := s.provider.RecentDiagnoses(ctx, clusterID, namespace, s.lookback)
	if err != nil {
		result.Partial = true
		if result.Error == nil {
			result.Error = fmt.Errorf("gather diagnoses: %w", err)
		}
	}

	inputs := EngineInputs{
		Signals:   signals,
		Changes:   changes,
		Edges:     edges,
		Diagnoses: diagnoses,
		Now:       now,
	}
	result.InputsGathered = len(signals) + len(changes) + len(edges) + len(diagnoses)

	results, err := s.engine.Correlate(ctx, inputs)
	if err != nil {
		if result.Error == nil {
			result.Error = fmt.Errorf("correlate: %w", err)
		}
		return result, err
	}
	result.ResultsProduced = len(results)

	// Persist results idempotently. A persist failure on one result does not
	// abort the others; the error is recorded and the pass is marked partial.
	for i := range results {
		_, pErr := s.repo.UpsertResult(ctx, &results[i])
		if pErr != nil {
			result.Partial = true
			if result.Error == nil {
				result.Error = fmt.Errorf("persist result: %w", pErr)
			}
			continue
		}
		result.CasesUpserted++
	}
	return result, nil
}

// GetCase returns the full case view for one case ID.
func (s *Service) GetCase(ctx context.Context, id int64) (CaseView, error) {
	return s.repo.GetCase(ctx, id)
}

// ListCases returns cases matching the filter.
func (s *Service) ListCases(ctx context.Context, filter CaseFilter) (CaseListResponse, error) {
	items, total, err := s.repo.ListCases(ctx, filter)
	if err != nil {
		return CaseListResponse{}, err
	}
	resp := CaseListResponse{Items: items, Total: total}
	if len(items) > 0 && int64(len(items)) < total {
		resp.Truncated = true
	}
	return resp, nil
}

// ListTimeline returns cases ordered by first_observed_at for the timeline.
func (s *Service) ListTimeline(ctx context.Context, filter CaseFilter) (CaseTimelineResponse, error) {
	items, total, err := s.repo.ListTimeline(ctx, filter)
	if err != nil {
		return CaseTimelineResponse{}, err
	}
	resp := CaseTimelineResponse{Items: items, Total: total}
	if len(items) > 0 && int64(len(items)) < total {
		resp.Truncated = true
	}
	return resp, nil
}

// GetCaseGraph returns the resource links (impact graph) for one case.
func (s *Service) GetCaseGraph(ctx context.Context, caseID int64) ([]ResourceLink, error) {
	// Verify the case exists first so we return ErrCaseNotFound rather than
	// an empty graph for a non-existent case.
	if _, err := s.repo.GetCase(ctx, caseID); err != nil {
		return nil, err
	}
	return s.repo.ListResourceLinks(ctx, caseID)
}

// ListActionCandidates derives the fixed, read-only action candidates for one
// case. The candidates are server-fixed codes from the M19 controlled
// operations catalog; callers cannot inject arbitrary actions. Eligibility is
// a hint — M44 rechecks at preview time.
//
// The mapping is deterministic: a case with a confirmed rollout root cause
// on a Deployment yields deployment.rollback; a pod-failure case on a
// Deployment yields deployment.rollout_restart; a node-failure case yields
// nothing (node maintenance is operator-only, not auto-suggested).
func (s *Service) ListActionCandidates(ctx context.Context, caseID int64) (ActionCandidateListResponse, error) {
	view, err := s.repo.GetCase(ctx, caseID)
	if err != nil {
		return ActionCandidateListResponse{}, err
	}

	var items []ActionCandidate
	primary := view.Case.PrimaryResource

	// deployment.rollback: confirmed rollout/promotion root cause on a
	// Deployment. The target is the Deployment, not the Pod.
	if view.Case.Confidence == ConfidenceConfirmed {
		for _, cc := range view.ChangeCandidates {
			if cc.Confidence != ConfidenceConfirmed {
				continue
			}
			if cc.RuleID == "correlation.rollout_causes_unavailable_deployment.v1" ||
				cc.RuleID == "correlation.rollout_causes_metric_breach.v1" {
				items = appendAction(items, ActionCandidate{
					Code:              "deployment.rollback",
					Target:            primary,
					Eligible:          true,
					SourceCaseID:      caseID,
					SourceCandidateID: &cc.ID,
				})
				break
			}
		}
	}

	// deployment.rollout_restart: pod-failure case whose primary is a Pod.
	// The target is the owning Deployment (upstream resource link); M44
	// rechecks the Deployment UID at preview time.
	if primary.Kind == "Pod" && (view.Case.Confidence == ConfidenceConfirmed || view.Case.Confidence == ConfidenceCandidate) {
		if owner := findUpstreamDeployment(view.ResourceLinks); owner != nil {
			items = appendAction(items, ActionCandidate{
				Code:         "deployment.rollout_restart",
				Target:       *owner,
				Eligible:     true,
				SourceCaseID: caseID,
			})
		}
	}

	// NoEndpoints case on a Service: suggest rollout_restart on the backing
	// Deployment (downstream) when confidence is candidate or better.
	if primary.Kind == "Service" && (view.Case.Confidence == ConfidenceConfirmed || view.Case.Confidence == ConfidenceCandidate) {
		if backing := findDownstreamDeployment(view.ResourceLinks); backing != nil {
			items = appendAction(items, ActionCandidate{
				Code:         "deployment.rollout_restart",
				Target:       *backing,
				Eligible:     true,
				SourceCaseID: caseID,
			})
		}
	}

	return ActionCandidateListResponse{Items: items, Total: int64(len(items))}, nil
}

func appendAction(items []ActionCandidate, a ActionCandidate) []ActionCandidate {
	return append(items, a)
}

// findUpstreamDeployment returns the upstream Deployment resource link, if any.
func findUpstreamDeployment(links []ResourceLink) *ResourceCitation {
	for _, l := range links {
		if l.Relation == ResourceRelationUpstream && l.Resource.Kind == "Deployment" {
			r := l.Resource
			return &r
		}
	}
	return nil
}

// findDownstreamDeployment returns the downstream Deployment resource link, if any.
func findDownstreamDeployment(links []ResourceLink) *ResourceCitation {
	for _, l := range links {
		if l.Relation == ResourceRelationDownstream && l.Resource.Kind == "Deployment" {
			r := l.Resource
			return &r
		}
	}
	return nil
}
