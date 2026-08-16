package diagnosis

import (
	"context"
	"errors"
	"time"

	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
	"k8s-aiops.local/backend/internal/metricshistory"
)

var ErrNoRuleMatch = errors.New("no diagnosis rule matched the resource")
var ErrRecordNotFound = errors.New("diagnosis record not found")

type Source interface {
	Pod(context.Context, int64, string, string) (k8sgateway.Pod, error)
	PodEvents(context.Context, int64, string, string) ([]k8sgateway.Event, error)
	GetService(context.Context, int64, string, string) (k8sgateway.ServiceResource, error)
	ServiceEndpoints(context.Context, int64, string, string) (k8sgateway.Endpoints, error)
	Node(context.Context, int64, string) (k8sgateway.Node, error)
	Deployment(context.Context, int64, string, string) (k8sgateway.Deployment, error)
	Ingress(context.Context, int64, string, string) (k8sgateway.Ingress, error)
	PersistentVolumeClaim(context.Context, int64, string, string) (k8sgateway.PersistentVolumeClaim, error)
	HorizontalPodAutoscaler(context.Context, int64, string, string) (k8sgateway.HorizontalPodAutoscaler, error)
	ResourceEvents(context.Context, int64, string, string) ([]k8sgateway.Event, error)
}

type MetricEvaluator interface {
	Evaluate(context.Context, metricshistory.EvaluationQuery) (metricshistory.EvaluationResponse, error)
}

type Service struct {
	source          Source
	repository      Repository
	metricEvaluator MetricEvaluator
	ingester        KnowledgeIngester
	now             func() time.Time
}

func NewService(source Source, repository Repository) *Service {
	return &Service{source: source, repository: repository, now: time.Now}
}

// WithKnowledgeIngester makes the service push resolved records into the
// knowledge base.  Must be called before any Transition; nil is harmless and
// keeps the deterministic diagnosis chain unchanged.
func (s *Service) WithKnowledgeIngester(i KnowledgeIngester) *Service {
	s.ingester = i
	return s
}

func (s *Service) WithMetricEvaluator(evaluator MetricEvaluator) *Service {
	s.metricEvaluator = evaluator
	return s
}

func (s *Service) DiagnosePod(ctx context.Context, clusterID int64, namespace, name string) (Record, error) {
	pod, err := s.source.Pod(ctx, clusterID, namespace, name)
	if err != nil {
		return Record{}, err
	}
	events, err := s.source.PodEvents(ctx, clusterID, namespace, pod.Metadata.UID)
	if err != nil {
		return Record{}, err
	}
	observedAt := s.now()
	record, matched := evaluatePod(clusterID, pod, events, observedAt)
	if !matched {
		return Record{}, ErrNoRuleMatch
	}
	return s.save(ctx, record)
}

func evaluatePod(clusterID int64, pod k8sgateway.Pod, events []k8sgateway.Event, observedAt time.Time) (Record, bool) {
	record, matched := EvaluatePodOOMKilled(clusterID, pod, events, observedAt)
	if !matched {
		record, matched = EvaluateImagePullBackOff(clusterID, pod, events, observedAt)
	}
	if !matched {
		record, matched = EvaluateCrashLoopBackOff(clusterID, pod, events, observedAt)
	}
	if !matched {
		record, matched = EvaluatePodPending(clusterID, pod, events, observedAt)
	}
	return record, matched
}

func (s *Service) DiagnoseService(ctx context.Context, clusterID int64, namespace, name string) (Record, error) {
	service, err := s.source.GetService(ctx, clusterID, namespace, name)
	if err != nil {
		return Record{}, err
	}
	endpoints, err := s.source.ServiceEndpoints(ctx, clusterID, namespace, name)
	if err != nil {
		return Record{}, err
	}
	record, matched := EvaluateServiceNoEndpoints(clusterID, service, endpoints, s.now())
	if !matched {
		return Record{}, ErrNoRuleMatch
	}
	return s.save(ctx, record)
}

func (s *Service) DiagnoseNode(ctx context.Context, clusterID int64, name string) (Record, error) {
	node, err := s.source.Node(ctx, clusterID, name)
	if err != nil {
		return Record{}, err
	}
	record, matched := EvaluateNodeNotReady(clusterID, node, s.now())
	if !matched {
		record, matched = EvaluateNodePressure(clusterID, node, s.now())
	}
	if !matched {
		return Record{}, ErrNoRuleMatch
	}
	return s.save(ctx, record)
}

func (s *Service) DiagnoseNodeMetrics(ctx context.Context, clusterID int64, name string, metric string, rule metricshistory.EvaluationRule) (Record, error) {
	if s.metricEvaluator == nil {
		return Record{}, ErrNoRuleMatch
	}
	from := s.now().Add(-6 * time.Hour)
	to := s.now()
	eval, err := s.metricEvaluator.Evaluate(ctx, metricshistory.EvaluationQuery{
		SeriesQuery: metricshistory.SeriesQuery{
			ClusterID: clusterID, ResourceKind: metricshistory.ResourceNode,
			ResourceName: name, MetricName: metric, From: from, To: to,
		},
		EvaluationRule: rule,
	})
	if err != nil {
		return Record{}, err
	}
	record, matched := EvaluateSustainedMetricBreach(clusterID, eval, s.now())
	if !matched {
		return Record{}, ErrNoRuleMatch
	}
	return s.save(ctx, record)
}

func (s *Service) DiagnosePersistentVolumeClaim(ctx context.Context, clusterID int64, namespace, name string) (Record, error) {
	claim, err := s.source.PersistentVolumeClaim(ctx, clusterID, namespace, name)
	if err != nil {
		return Record{}, err
	}
	events, err := s.source.ResourceEvents(ctx, clusterID, namespace, claim.Metadata.UID)
	if err != nil {
		return Record{}, err
	}
	record, matched := EvaluatePersistentVolumeClaimPending(clusterID, claim, events, s.now())
	if !matched {
		return Record{}, ErrNoRuleMatch
	}
	return s.save(ctx, record)
}

func (s *Service) DiagnoseHorizontalPodAutoscaler(ctx context.Context, clusterID int64, namespace, name string) (Record, error) {
	hpa, err := s.source.HorizontalPodAutoscaler(ctx, clusterID, namespace, name)
	if err != nil {
		return Record{}, err
	}
	record, matched := EvaluateHorizontalPodAutoscalerSaturated(clusterID, hpa, s.now())
	if !matched {
		return Record{}, ErrNoRuleMatch
	}
	return s.save(ctx, record)
}

func (s *Service) DiagnoseIngress(ctx context.Context, clusterID int64, namespace, name string) (Record, error) {
	ingress, err := s.source.Ingress(ctx, clusterID, namespace, name)
	if err != nil {
		return Record{}, err
	}
	routes := IngressServiceRoutes(ingress)
	backends := make(map[string]IngressBackendState, len(routes))
	for _, route := range routes {
		if _, exists := backends[route.ServiceName]; exists {
			continue
		}
		service, err := s.source.GetService(ctx, clusterID, namespace, route.ServiceName)
		if err != nil {
			return Record{}, err
		}
		endpoints, err := s.source.ServiceEndpoints(ctx, clusterID, namespace, route.ServiceName)
		if err != nil {
			return Record{}, err
		}
		backends[route.ServiceName] = IngressBackendState{Service: service, Endpoints: endpoints}
	}
	record, matched := EvaluateIngressBackendUnavailable(clusterID, ingress, routes, backends, s.now())
	if !matched {
		return Record{}, ErrNoRuleMatch
	}
	return s.save(ctx, record)
}

func (s *Service) save(ctx context.Context, record Record) (Record, error) {
	record.SLADueAt = SLADeadline(record.Severity, record.ObservedAt)
	if err := s.repository.Save(ctx, &record); err != nil {
		return Record{}, err
	}
	return WithNarrative(record), nil
}

func (s *Service) DiagnoseDeployment(ctx context.Context, clusterID int64, namespace, name string) (Record, error) {
	deployment, err := s.source.Deployment(ctx, clusterID, namespace, name)
	if err != nil {
		return Record{}, err
	}
	record, matched := EvaluateDeploymentReplicasUnavailable(clusterID, deployment, s.now())
	if !matched {
		return Record{}, ErrNoRuleMatch
	}
	return s.save(ctx, record)
}

func (s *Service) List(ctx context.Context, filter ListFilter) ([]Record, error) {
	return s.repository.List(ctx, filter)
}

func (s *Service) Get(ctx context.Context, id int64) (Record, error) {
	record, err := s.repository.Get(ctx, id)
	if err != nil {
		return Record{}, err
	}
	return WithNarrative(record), nil
}

func (s *Service) Transition(ctx context.Context, id int64, status string, actor ActorRef, comment string) (Record, error) {
	record, err := s.repository.Transition(ctx, id, status, actor, comment)
	if err != nil {
		return Record{}, err
	}
	// Distill resolved records into the knowledge base (best-effort, never
	// blocks or fails the transition).
	IngestResolvedIfEligible(ctx, s.ingester, record, s.now)
	return WithNarrative(record), nil
}

func (s *Service) AddFeedback(ctx context.Context, id int64, verdict string, actor ActorRef, comment string) (Record, error) {
	if !ValidFeedbackVerdict(verdict) {
		return Record{}, ErrInvalidFeedback
	}
	record, err := s.repository.AddFeedback(ctx, id, verdict, actor, comment)
	if err != nil {
		return Record{}, err
	}
	return WithNarrative(record), nil
}

func (s *Service) Summary(ctx context.Context) (Summary, error) {
	return s.repository.Summary(ctx)
}

func (s *Service) Assign(ctx context.Context, id int64, assignee, actor ActorRef, comment string) (Record, error) {
	record, err := s.repository.Assign(ctx, id, assignee, actor, comment)
	if err != nil {
		return Record{}, err
	}
	return WithNarrative(record), nil
}
