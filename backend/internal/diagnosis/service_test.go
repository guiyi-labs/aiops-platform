package diagnosis

// Covers the service orchestration layer: each Diagnose* path (source fetch
// -> rule evaluation -> SLA deadline -> repository save), the metric
// evaluator path, error propagation and the workflow repository calls.
// Fixtures mirror the M18 golden fixtures and the unit rule tests.

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
	"k8s-aiops.local/backend/internal/metricshistory"
)

// --- fakes ---

type fakeSource struct {
	pod        k8sgateway.Pod
	events     []k8sgateway.Event
	service    k8sgateway.ServiceResource
	endpoints  k8sgateway.Endpoints
	node       k8sgateway.Node
	deployment k8sgateway.Deployment
	ingress    k8sgateway.Ingress
	pvc        k8sgateway.PersistentVolumeClaim
	hpa        k8sgateway.HorizontalPodAutoscaler
	err        error
}

func (f *fakeSource) Pod(context.Context, int64, string, string) (k8sgateway.Pod, error) {
	return f.pod, f.err
}
func (f *fakeSource) PodEvents(context.Context, int64, string, string) ([]k8sgateway.Event, error) {
	return f.events, f.err
}
func (f *fakeSource) GetService(context.Context, int64, string, string) (k8sgateway.ServiceResource, error) {
	return f.service, f.err
}
func (f *fakeSource) ServiceEndpoints(context.Context, int64, string, string) (k8sgateway.Endpoints, error) {
	return f.endpoints, f.err
}
func (f *fakeSource) Node(context.Context, int64, string) (k8sgateway.Node, error) {
	return f.node, f.err
}
func (f *fakeSource) Deployment(context.Context, int64, string, string) (k8sgateway.Deployment, error) {
	return f.deployment, f.err
}
func (f *fakeSource) Ingress(context.Context, int64, string, string) (k8sgateway.Ingress, error) {
	return f.ingress, f.err
}
func (f *fakeSource) PersistentVolumeClaim(context.Context, int64, string, string) (k8sgateway.PersistentVolumeClaim, error) {
	return f.pvc, f.err
}
func (f *fakeSource) HorizontalPodAutoscaler(context.Context, int64, string, string) (k8sgateway.HorizontalPodAutoscaler, error) {
	return f.hpa, f.err
}
func (f *fakeSource) ResourceEvents(context.Context, int64, string, string) ([]k8sgateway.Event, error) {
	return f.events, f.err
}

type fakeEvaluator struct {
	resp metricshistory.EvaluationResponse
	err  error
}

func (f fakeEvaluator) Evaluate(context.Context, metricshistory.EvaluationQuery) (metricshistory.EvaluationResponse, error) {
	return f.resp, f.err
}

type memRepo struct {
	records map[int64]Record
	seq     int64
}

func newMemRepo() *memRepo { return &memRepo{records: map[int64]Record{}} }

func (m *memRepo) Save(_ context.Context, r *Record) error {
	m.seq++
	r.ID = m.seq
	m.records[r.ID] = *r
	return nil
}

func (m *memRepo) List(_ context.Context, f ListFilter) ([]Record, error) {
	var out []Record
	for _, r := range m.records {
		if f.ClusterID != 0 && r.ClusterID != f.ClusterID {
			continue
		}
		if f.Status != "" && r.Status != f.Status {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

func (m *memRepo) Get(_ context.Context, id int64) (Record, error) {
	r, ok := m.records[id]
	if !ok {
		return Record{}, ErrRecordNotFound
	}
	return r, nil
}

func (m *memRepo) Transition(_ context.Context, id int64, status string, _ ActorRef, _ string) (Record, error) {
	r, ok := m.records[id]
	if !ok {
		return Record{}, ErrRecordNotFound
	}
	if !CanTransition(r.Status, status) {
		return Record{}, ErrInvalidTransition
	}
	r.Status = status
	m.records[id] = r
	return r, nil
}

func (m *memRepo) AddFeedback(_ context.Context, id int64, verdict string, actor ActorRef, comment string) (Record, error) {
	r, ok := m.records[id]
	if !ok {
		return Record{}, ErrRecordNotFound
	}
	r.Feedback = append(r.Feedback, Feedback{Verdict: verdict, Actor: actor, Comment: comment})
	m.records[id] = r
	return r, nil
}

func (m *memRepo) Assign(_ context.Context, id int64, assignee, _ ActorRef, _ string) (Record, error) {
	r, ok := m.records[id]
	if !ok {
		return Record{}, ErrRecordNotFound
	}
	r.Assignee = &assignee
	m.records[id] = r
	return r, nil
}

func (m *memRepo) Summary(_ context.Context) (Summary, error) {
	return Summary{Total: int64(len(m.records))}, nil
}

func (m *memRepo) ListByClusters(_ context.Context, clusters []int64, status, severity string, limit int) ([]FederationDiagnosisRow, error) {
	if len(clusters) == 0 {
		return []FederationDiagnosisRow{}, nil
	}
	visible := make(map[int64]bool, len(clusters))
	for _, id := range clusters {
		visible[id] = true
	}
	out := make([]FederationDiagnosisRow, 0, len(m.records))
	for _, r := range m.records {
		if !visible[r.ClusterID] {
			continue
		}
		if status != "" && r.Status != status {
			continue
		}
		if severity != "" && r.Severity != severity {
			continue
		}
		out = append(out, FederationDiagnosisRow{
			ID: r.ID, ClusterID: r.ClusterID, RuleID: r.RuleID, Severity: r.Severity,
			ResourceKind: r.Resource.Kind, ResourceName: r.Resource.Name, ResourceNamespace: r.Resource.Namespace,
			Status: r.Status, Summary: r.Summary, ObservedAt: r.ObservedAt, ResolvedAt: r.ResolvedAt,
		})
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *memRepo) StatsByClusters(_ context.Context, clusters []int64) (FederationDiagnosisStats, error) {
	stats := FederationDiagnosisStats{ByStatus: map[string]int64{}, BySeverity: map[string]int64{}, ByCluster: []FederationClusterCount{}}
	if len(clusters) == 0 {
		return stats, nil
	}
	visible := make(map[int64]bool, len(clusters))
	for _, id := range clusters {
		visible[id] = true
	}
	clusterTotals := make(map[int64]int64)
	for _, r := range m.records {
		if !visible[r.ClusterID] {
			continue
		}
		stats.Total++
		stats.ByStatus[r.Status]++
		stats.BySeverity[r.Severity]++
		clusterTotals[r.ClusterID]++
	}
	for id, count := range clusterTotals {
		stats.ByCluster = append(stats.ByCluster, FederationClusterCount{ClusterID: id, Count: count})
	}
	sort.Slice(stats.ByCluster, func(i, j int) bool { return stats.ByCluster[i].ClusterID < stats.ByCluster[j].ClusterID })
	return stats, nil
}

func newSvc(t *testing.T) (*Service, *fakeSource, *memRepo) {
	t.Helper()
	src := &fakeSource{}
	repo := newMemRepo()
	return NewService(src, repo), src, repo
}

// --- orchestration tests ---

func TestDiagnosePodOOMKilledFlow(t *testing.T) {
	svc, src, repo := newSvc(t)
	var pod k8sgateway.Pod
	mustDecode(t, `{"metadata":{"name":"memory-api","namespace":"demo","uid":"pod-oom"},"status":{"phase":"Running","containerStatuses":[{"name":"app","restartCount":3,"lastState":{"terminated":{"reason":"OOMKilled","exitCode":137,"finishedAt":"2026-07-17T02:00:00Z"}}}]}}`, &pod)
	src.pod = pod
	src.events = []k8sgateway.Event{{Type: "Warning", Reason: "OOMKilled", Message: "killed", Count: 2}}
	record, err := svc.DiagnosePod(context.Background(), 7, "demo", "memory-api")
	if err != nil {
		t.Fatalf("DiagnosePod: %v", err)
	}
	if record.RuleID != RulePodOOMKilled || record.SLADueAt.IsZero() {
		t.Fatalf("record = %+v", record)
	}
	if _, ok := repo.records[record.ID]; !ok {
		t.Fatal("record was not saved")
	}
}

func TestDiagnosePodNoMatchAndSourceError(t *testing.T) {
	svc, src, _ := newSvc(t)
	var pod k8sgateway.Pod
	mustDecode(t, `{"metadata":{"name":"ok","namespace":"demo","uid":"pod-ok"},"status":{"phase":"Running"}}`, &pod)
	src.pod = pod
	if _, err := svc.DiagnosePod(context.Background(), 7, "demo", "ok"); !errors.Is(err, ErrNoRuleMatch) {
		t.Errorf("healthy pod err = %v, want ErrNoRuleMatch", err)
	}
	src.err = errors.New("gateway down")
	if _, err := svc.DiagnosePod(context.Background(), 7, "demo", "ok"); err == nil || err.Error() != "gateway down" {
		t.Errorf("source error should propagate, got %v", err)
	}
}

func TestDiagnoseServiceNoEndpoints(t *testing.T) {
	svc, src, _ := newSvc(t)
	var service k8sgateway.ServiceResource
	var endpoints k8sgateway.Endpoints
	mustDecode(t, `{"metadata":{"name":"api","namespace":"demo","uid":"service-1"},"spec":{"type":"ClusterIP","selector":{"app":"api"},"ports":[{"port":80,"targetPort":8080,"protocol":"TCP"}]}}`, &service)
	mustDecode(t, `{"metadata":{"name":"api","namespace":"demo"},"subsets":[{"notReadyAddresses":[{"ip":"10.0.0.9"}]}]}`, &endpoints)
	src.service = service
	src.endpoints = endpoints
	record, err := svc.DiagnoseService(context.Background(), 7, "demo", "api")
	if err != nil || record.RuleID != RuleServiceNoEndpoints {
		t.Fatalf("record = %+v, err = %v", record, err)
	}
	// Healthy endpoints must not match.
	var okEndpoints k8sgateway.Endpoints
	mustDecode(t, `{"metadata":{"name":"api","namespace":"demo"},"subsets":[{"addresses":[{"ip":"10.0.0.9"}]}]}`, &okEndpoints)
	src.endpoints = okEndpoints
	if _, err := svc.DiagnoseService(context.Background(), 7, "demo", "api"); !errors.Is(err, ErrNoRuleMatch) {
		t.Errorf("healthy endpoints err = %v, want ErrNoRuleMatch", err)
	}
}

func TestDiagnoseNodeNotReady(t *testing.T) {
	svc, src, _ := newSvc(t)
	var node k8sgateway.Node
	mustDecode(t, `{"metadata":{"name":"worker-1"},"status":{"conditions":[]}}`, &node)
	src.node = node
	record, err := svc.DiagnoseNode(context.Background(), 7, "worker-1")
	if err != nil || record.RuleID != RuleNodeNotReady {
		t.Fatalf("record = %+v, err = %v", record, err)
	}
}

func TestDiagnoseDeploymentUnavailable(t *testing.T) {
	svc, src, _ := newSvc(t)
	var deployment k8sgateway.Deployment
	mustDecode(t, `{"metadata":{"name":"api","namespace":"demo","uid":"deployment-1"},"spec":{"replicas":3},"status":{"replicas":2,"readyReplicas":1,"availableReplicas":1,"updatedReplicas":2,"unavailableReplicas":1}}`, &deployment)
	src.deployment = deployment
	record, err := svc.DiagnoseDeployment(context.Background(), 7, "demo", "api")
	if err != nil || record.RuleID != RuleDeploymentReplicasUnavailable {
		t.Fatalf("record = %+v, err = %v", record, err)
	}
}

func TestDiagnosePVCAndHPAAndIngress(t *testing.T) {
	// PVC pending with warning event.
	svc, src, _ := newSvc(t)
	var pvc k8sgateway.PersistentVolumeClaim
	mustDecode(t, `{"metadata":{"name":"data","namespace":"demo","uid":"pvc-1"},"spec":{"storageClassName":"missing","accessModes":["ReadWriteOnce"],"resources":{"requests":{"storage":"16Mi"}}},"status":{"phase":"Pending"}}`, &pvc)
	src.pvc = pvc
	src.events = []k8sgateway.Event{{Type: "Warning", Reason: "ProvisioningFailed", Message: "storageclass not found", Count: 1}}
	record, err := svc.DiagnosePersistentVolumeClaim(context.Background(), 7, "demo", "data")
	if err != nil || record.RuleID != RulePersistentVolumeClaimPending {
		t.Fatalf("pvc record = %+v, err = %v", record, err)
	}

	// HPA saturated at maximum.
	var hpa k8sgateway.HorizontalPodAutoscaler
	mustDecode(t, `{"metadata":{"name":"api","namespace":"demo","uid":"hpa-1"},"spec":{"scaleTargetRef":{"apiVersion":"apps/v1","kind":"Deployment","name":"api"},"minReplicas":1,"maxReplicas":4},"status":{"currentReplicas":4,"desiredReplicas":4,"conditions":[{"type":"ScalingLimited","status":"True","reason":"TooManyReplicas","message":"desired count is above maximum"}]}}`, &hpa)
	src.hpa = hpa
	record, err = svc.DiagnoseHorizontalPodAutoscaler(context.Background(), 7, "demo", "api")
	if err != nil || record.RuleID != RuleHorizontalPodAutoscalerSaturated {
		t.Fatalf("hpa record = %+v, err = %v", record, err)
	}

	// Ingress backend without ready addresses.
	var ingress k8sgateway.Ingress
	var service k8sgateway.ServiceResource
	var endpoints k8sgateway.Endpoints
	mustDecode(t, `{"metadata":{"name":"broken","namespace":"demo","uid":"ingress-1"},"spec":{"rules":[{"host":"broken.example.test","http":{"paths":[{"path":"/api","pathType":"Prefix","backend":{"service":{"name":"api","port":{"number":80}}}}]}}]}}`, &ingress)
	mustDecode(t, `{"metadata":{"name":"api","namespace":"demo","uid":"service-1"},"spec":{"type":"ClusterIP","selector":{"app":"api"},"ports":[{"port":80,"targetPort":8080}]}}`, &service)
	mustDecode(t, `{"metadata":{"name":"api","namespace":"demo"},"subsets":[{"notReadyAddresses":[{"ip":"10.0.0.8"}]}]}`, &endpoints)
	src.ingress = ingress
	src.service = service
	src.endpoints = endpoints
	record, err = svc.DiagnoseIngress(context.Background(), 7, "demo", "broken")
	if err != nil || record.RuleID != RuleIngressBackendUnavailable {
		t.Fatalf("ingress record = %+v, err = %v", record, err)
	}
}

func TestDiagnoseNodeMetrics(t *testing.T) {
	svc, _, repo := newSvc(t)
	if _, err := svc.DiagnoseNodeMetrics(context.Background(), 7, "worker-a", metricshistory.MetricCPU, metricshistory.EvaluationRule{}); !errors.Is(err, ErrNoRuleMatch) {
		t.Errorf("nil evaluator err = %v, want ErrNoRuleMatch", err)
	}
	start := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	svc.WithMetricEvaluator(fakeEvaluator{resp: metricshistory.EvaluationResponse{
		Series: metricshistory.Series{
			ClusterID: 7, ResourceKind: metricshistory.ResourceNode,
			ResourceName: "worker-a", MetricName: metricshistory.MetricCPU,
			Unit: metricshistory.UnitNanocores,
		},
		From: start, To: start.Add(1 * time.Hour),
		Coverage:  metricshistory.QueryCoverage{Collections: 4, Succeeded: 4, Points: 4},
		State:     metricshistory.EvaluationStateFiring,
		Operator:  metricshistory.OperatorGreaterThanOrEqual,
		Threshold: 50_000_000, ForSeconds: 60, MinimumPoints: 2,
		PointsEvaluated: 4, BreachingPoints: 3,
		SustainedWindows: []metricshistory.SustainedWindow{
			{StartCollectedAt: start, EndCollectedAt: start.Add(2 * time.Minute), BreachingPoints: 3, SpanSeconds: 120},
		},
	}})
	record, err := svc.DiagnoseNodeMetrics(context.Background(), 7, "worker-a", metricshistory.MetricCPU, metricshistory.EvaluationRule{})
	if err != nil || record.RuleID != RuleNodeSustainedMetricBreach {
		t.Fatalf("record = %+v, err = %v", record, err)
	}
	if _, ok := repo.records[record.ID]; !ok {
		t.Fatal("metric breach record not saved")
	}
}

func TestWorkflowRepositoryCalls(t *testing.T) {
	svc, src, _ := newSvc(t)
	var node k8sgateway.Node
	mustDecode(t, `{"metadata":{"name":"worker-1"},"status":{"conditions":[]}}`, &node)
	src.node = node
	record, err := svc.DiagnoseNode(context.Background(), 7, "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	actor := ActorRef{ID: 1, Name: "ops"}

	got, err := svc.Get(ctx, record.ID)
	if err != nil || got.ID != record.ID {
		t.Fatalf("Get = %+v, %v", got, err)
	}
	listed, err := svc.List(ctx, ListFilter{ClusterID: 7})
	if err != nil || len(listed) != 1 {
		t.Fatalf("List = %+v, %v", listed, err)
	}
	trans, err := svc.Transition(ctx, record.ID, "confirmed", actor, "confirmed by ops")
	if err != nil || trans.Status != "confirmed" {
		t.Fatalf("Transition = %+v, %v", trans, err)
	}
	if _, err := svc.Transition(ctx, record.ID, "resolved", actor, ""); err != nil {
		t.Fatalf("Transition confirmed->resolved: %v", err)
	}
	if _, err := svc.AddFeedback(ctx, record.ID, "nope", actor, ""); !errors.Is(err, ErrInvalidFeedback) {
		t.Errorf("invalid verdict err = %v, want ErrInvalidFeedback", err)
	}
	fb, err := svc.AddFeedback(ctx, record.ID, "accurate", actor, "spot on")
	if err != nil || len(fb.Feedback) != 1 {
		t.Fatalf("AddFeedback = %+v, %v", fb, err)
	}
	assigned, err := svc.Assign(ctx, record.ID, ActorRef{ID: 2, Name: "eng"}, actor, "assign")
	if err != nil || assigned.Assignee == nil || assigned.Assignee.ID != 2 {
		t.Fatalf("Assign = %+v, %v", assigned, err)
	}
	sum, err := svc.Summary(ctx)
	if err != nil || sum.Total != 1 {
		t.Fatalf("Summary = %+v, %v", sum, err)
	}
	if _, err := svc.Get(ctx, 9999); !errors.Is(err, ErrRecordNotFound) {
		t.Errorf("Get missing err = %v, want ErrRecordNotFound", err)
	}
}
