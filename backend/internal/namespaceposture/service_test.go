package namespaceposture

import (
	"context"
	"errors"
	"testing"
	"time"

	"k8s-aiops.local/backend/internal/apiquery"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

// --- mock KubernetesSource ---

type mockK8sSource struct {
	namespaces           func(context.Context, int64, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Namespace], error)
	nodes                func(context.Context, int64, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Node], error)
	resourceQuotas       func(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.ResourceQuota], error)
	limitRanges          func(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.LimitRange], error)
	podDisruptionBudgets func(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.PodDisruptionBudget], error)
	pods                 func(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Pod], error)
	deployments          func(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Deployment], error)
	statefulSets         func(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.StatefulSet], error)
	daemonSets           func(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.DaemonSet], error)
	jobs                 func(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Job], error)
	cronJobs             func(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.CronJob], error)
}

func (m *mockK8sSource) Namespaces(ctx context.Context, cid int64, q apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Namespace], error) {
	return m.namespaces(ctx, cid, q)
}
func (m *mockK8sSource) Nodes(ctx context.Context, cid int64, q apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Node], error) {
	return m.nodes(ctx, cid, q)
}
func (m *mockK8sSource) ResourceQuotas(ctx context.Context, cid int64, ns string, q apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.ResourceQuota], error) {
	return m.resourceQuotas(ctx, cid, ns, q)
}
func (m *mockK8sSource) LimitRanges(ctx context.Context, cid int64, ns string, q apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.LimitRange], error) {
	return m.limitRanges(ctx, cid, ns, q)
}
func (m *mockK8sSource) PodDisruptionBudgets(ctx context.Context, cid int64, ns string, q apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.PodDisruptionBudget], error) {
	return m.podDisruptionBudgets(ctx, cid, ns, q)
}
func (m *mockK8sSource) Pods(ctx context.Context, cid int64, ns string, q apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Pod], error) {
	return m.pods(ctx, cid, ns, q)
}
func (m *mockK8sSource) Deployments(ctx context.Context, cid int64, ns string, q apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Deployment], error) {
	return m.deployments(ctx, cid, ns, q)
}
func (m *mockK8sSource) StatefulSets(ctx context.Context, cid int64, ns string, q apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.StatefulSet], error) {
	return m.statefulSets(ctx, cid, ns, q)
}
func (m *mockK8sSource) DaemonSets(ctx context.Context, cid int64, ns string, q apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.DaemonSet], error) {
	return m.daemonSets(ctx, cid, ns, q)
}
func (m *mockK8sSource) Jobs(ctx context.Context, cid int64, ns string, q apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Job], error) {
	return m.jobs(ctx, cid, ns, q)
}
func (m *mockK8sSource) CronJobs(ctx context.Context, cid int64, ns string, q apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.CronJob], error) {
	return m.cronJobs(ctx, cid, ns, q)
}

func emptyList[T any]() apiquery.ListResponse[T] {
	return apiquery.ListResponse[T]{Items: []T{}, Total: 0, Remaining: 0}
}

func newFixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }

func ptrInt32(v int32) *int32 { return &v }

// --- test helpers build minimal valid k8s resources ---

func makeNamespace(name, phase string) k8sgateway.Namespace {
	ns := k8sgateway.Namespace{Metadata: k8sgateway.ObjectMeta{Name: name, CreationTimestamp: "2025-01-01T00:00:00Z"}}
	ns.Status.Phase = phase
	return ns
}

func makeDeployment(name string, replicas, ready int32) k8sgateway.Deployment {
	d := k8sgateway.Deployment{Metadata: k8sgateway.ObjectMeta{Name: name}}
	d.Spec.Replicas = ptrInt32(replicas)
	d.Status.ReadyReplicas = ready
	d.Status.AvailableReplicas = ready
	d.Status.UpdatedReplicas = ready
	d.Status.UnavailableReplicas = replicas - ready
	return d
}

func makePod(name, phase, node string) k8sgateway.Pod {
	p := k8sgateway.Pod{Metadata: k8sgateway.ObjectMeta{Name: name}}
	p.Spec.NodeName = node
	p.Status.Phase = phase
	return p
}

func makeNode(name string, schedulable bool, cpu, memory string) k8sgateway.Node {
	n := k8sgateway.Node{Metadata: k8sgateway.ObjectMeta{Name: name}}
	n.Spec.Unschedulable = !schedulable
	n.Status.Capacity = map[string]string{"cpu": cpu, "memory": memory}
	n.Status.Allocatable = map[string]string{"cpu": cpu, "memory": memory}
	return n
}

func makeRQ(name string, hard, used map[string]string) k8sgateway.ResourceQuota {
	rq := k8sgateway.ResourceQuota{Metadata: k8sgateway.ObjectMeta{Name: name}}
	rq.Status.Hard = hard
	rq.Status.Used = used
	return rq
}

// --- tests ---

func TestGet_ReadsNamespaceMetadata(t *testing.T) {
	clock := newFixedClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	mock := &mockK8sSource{
		namespaces: func(_ context.Context, _ int64, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Namespace], error) {
			return apiquery.ListResponse[k8sgateway.Namespace]{Items: []k8sgateway.Namespace{makeNamespace("default", "Active")}, Total: 1}, nil
		},
		resourceQuotas: func(_ context.Context, _ int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.ResourceQuota], error) {
			return emptyList[k8sgateway.ResourceQuota](), nil
		},
		limitRanges: func(_ context.Context, _ int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.LimitRange], error) {
			return emptyList[k8sgateway.LimitRange](), nil
		},
		podDisruptionBudgets: func(_ context.Context, _ int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.PodDisruptionBudget], error) {
			return emptyList[k8sgateway.PodDisruptionBudget](), nil
		},
		pods: func(_ context.Context, _ int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Pod], error) {
			return emptyList[k8sgateway.Pod](), nil
		},
		deployments: func(_ context.Context, _ int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Deployment], error) {
			return emptyList[k8sgateway.Deployment](), nil
		},
		statefulSets: func(_ context.Context, _ int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.StatefulSet], error) {
			return emptyList[k8sgateway.StatefulSet](), nil
		},
		daemonSets: func(_ context.Context, _ int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.DaemonSet], error) {
			return emptyList[k8sgateway.DaemonSet](), nil
		},
		jobs: func(_ context.Context, _ int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Job], error) {
			return emptyList[k8sgateway.Job](), nil
		},
		cronJobs: func(_ context.Context, _ int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.CronJob], error) {
			return emptyList[k8sgateway.CronJob](), nil
		},
		nodes: func(_ context.Context, _ int64, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Node], error) {
			return emptyList[k8sgateway.Node](), nil
		},
	}
	svc := NewService(mock)
	svc.now = clock

	p, err := svc.Get(context.Background(), 1, "default")
	if err != nil {
		t.Fatalf("Get unexpected error: %v", err)
	}
	if p.Name != "default" {
		t.Errorf("name = %q, want default", p.Name)
	}
	if p.Phase != "Active" {
		t.Errorf("phase = %q, want Active", p.Phase)
	}
	if p.ResourceQuotas.Evidence.Status != SourceComplete {
		t.Errorf("rq evidence status = %q, want complete", p.ResourceQuotas.Evidence.Status)
	}
}

func TestGet_NamespaceNotFound(t *testing.T) {
	mock := &mockK8sSource{
		namespaces: func(_ context.Context, _ int64, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Namespace], error) {
			return emptyList[k8sgateway.Namespace](), nil
		},
	}
	svc := NewService(mock)
	_, err := svc.Get(context.Background(), 1, "missing")
	if !errors.Is(err, k8sgateway.ErrResourceNotFound) {
		t.Fatalf("want ErrResourceNotFound, got %v", err)
	}
}

func TestGet_PartialSectionMarked(t *testing.T) {
	clock := newFixedClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	boom := errors.New("rbac denied")
	mock := &mockK8sSource{
		namespaces: func(_ context.Context, _ int64, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Namespace], error) {
			return apiquery.ListResponse[k8sgateway.Namespace]{Items: []k8sgateway.Namespace{makeNamespace("default", "Active")}, Total: 1}, nil
		},
		resourceQuotas: func(_ context.Context, _ int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.ResourceQuota], error) {
			return emptyList[k8sgateway.ResourceQuota](), boom
		},
		limitRanges: func(_ context.Context, _ int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.LimitRange], error) {
			return emptyList[k8sgateway.LimitRange](), nil
		},
		podDisruptionBudgets: func(_ context.Context, _ int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.PodDisruptionBudget], error) {
			return emptyList[k8sgateway.PodDisruptionBudget](), nil
		},
		pods: func(_ context.Context, _ int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Pod], error) {
			return emptyList[k8sgateway.Pod](), nil
		},
		deployments: func(_ context.Context, _ int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Deployment], error) {
			return emptyList[k8sgateway.Deployment](), nil
		},
		statefulSets: func(_ context.Context, _ int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.StatefulSet], error) {
			return emptyList[k8sgateway.StatefulSet](), nil
		},
		daemonSets: func(_ context.Context, _ int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.DaemonSet], error) {
			return emptyList[k8sgateway.DaemonSet](), nil
		},
		jobs: func(_ context.Context, _ int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Job], error) {
			return emptyList[k8sgateway.Job](), nil
		},
		cronJobs: func(_ context.Context, _ int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.CronJob], error) {
			return emptyList[k8sgateway.CronJob](), nil
		},
		nodes: func(_ context.Context, _ int64, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Node], error) {
			return emptyList[k8sgateway.Node](), nil
		},
	}
	svc := NewService(mock)
	svc.now = clock

	p, err := svc.Get(context.Background(), 1, "default")
	if err != nil {
		t.Fatalf("Get unexpected error: %v", err)
	}
	found := false
	for _, s := range p.PartialSections {
		if s == sectionResourceQuotas {
			found = true
		}
	}
	if !found {
		t.Errorf("partial_sections = %v, want resource_quotas listed", p.PartialSections)
	}
	if p.ResourceQuotas.Evidence.Status != SourceUnavailable {
		t.Errorf("rq evidence status = %q, want unavailable", p.ResourceQuotas.Evidence.Status)
	}
}

func TestGet_WorkloadAndPodAggregation(t *testing.T) {
	clock := newFixedClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	mock := &mockK8sSource{
		namespaces: func(_ context.Context, _ int64, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Namespace], error) {
			return apiquery.ListResponse[k8sgateway.Namespace]{Items: []k8sgateway.Namespace{makeNamespace("app", "Active")}, Total: 1}, nil
		},
		resourceQuotas: func(_ context.Context, _ int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.ResourceQuota], error) {
			return emptyList[k8sgateway.ResourceQuota](), nil
		},
		limitRanges: func(_ context.Context, _ int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.LimitRange], error) {
			return emptyList[k8sgateway.LimitRange](), nil
		},
		podDisruptionBudgets: func(_ context.Context, _ int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.PodDisruptionBudget], error) {
			return emptyList[k8sgateway.PodDisruptionBudget](), nil
		},
		pods: func(_ context.Context, _ int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Pod], error) {
			return apiquery.ListResponse[k8sgateway.Pod]{
				Items: []k8sgateway.Pod{
					makePod("p1", "Running", "n1"),
					makePod("p2", "Running", "n1"),
					makePod("p3", "Pending", "n2"),
				},
				Total: 3,
			}, nil
		},
		deployments: func(_ context.Context, _ int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Deployment], error) {
			return apiquery.ListResponse[k8sgateway.Deployment]{
				Items: []k8sgateway.Deployment{makeDeployment("web", 3, 2)},
				Total: 1,
			}, nil
		},
		statefulSets: func(_ context.Context, _ int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.StatefulSet], error) {
			return emptyList[k8sgateway.StatefulSet](), nil
		},
		daemonSets: func(_ context.Context, _ int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.DaemonSet], error) {
			return emptyList[k8sgateway.DaemonSet](), nil
		},
		jobs: func(_ context.Context, _ int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Job], error) {
			return emptyList[k8sgateway.Job](), nil
		},
		cronJobs: func(_ context.Context, _ int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.CronJob], error) {
			return emptyList[k8sgateway.CronJob](), nil
		},
		nodes: func(_ context.Context, _ int64, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Node], error) {
			return apiquery.ListResponse[k8sgateway.Node]{Items: []k8sgateway.Node{makeNode("n1", true, "4", "8Gi")}, Total: 1}, nil
		},
	}
	svc := NewService(mock)
	svc.now = clock

	p, err := svc.Get(context.Background(), 1, "app")
	if err != nil {
		t.Fatalf("Get unexpected error: %v", err)
	}
	if p.Workloads.DesiredTotal != 3 {
		t.Errorf("workloads.desired_total = %d, want 3", p.Workloads.DesiredTotal)
	}
	if p.Workloads.ReadyTotal != 2 {
		t.Errorf("workloads.ready_total = %d, want 2", p.Workloads.ReadyTotal)
	}
	if p.Pods.Total != 3 {
		t.Errorf("pods.total = %d, want 3", p.Pods.Total)
	}
	if p.Pods.UniqueNodeCount != 2 {
		t.Errorf("pods.unique_node_count = %d, want 2", p.Pods.UniqueNodeCount)
	}
	if len(p.PartialSections) != 0 {
		t.Errorf("partial_sections = %v, want empty", p.PartialSections)
	}
}

func TestList_SummarizesCounts(t *testing.T) {
	mock := &mockK8sSource{
		namespaces: func(_ context.Context, _ int64, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Namespace], error) {
			return apiquery.ListResponse[k8sgateway.Namespace]{
				Items: []k8sgateway.Namespace{
					makeNamespace("default", "Active"),
					makeNamespace("kube-system", "Active"),
				},
				Total: 2,
			}, nil
		},
		resourceQuotas: func(_ context.Context, _ int64, ns string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.ResourceQuota], error) {
			if ns == "default" {
				return apiquery.ListResponse[k8sgateway.ResourceQuota]{
					Items: []k8sgateway.ResourceQuota{makeRQ("cpu-mem", map[string]string{"cpu": "10"}, map[string]string{"cpu": "2"})},
					Total: 1,
				}, nil
			}
			return emptyList[k8sgateway.ResourceQuota](), nil
		},
		limitRanges: func(_ context.Context, _ int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.LimitRange], error) {
			return emptyList[k8sgateway.LimitRange](), nil
		},
		podDisruptionBudgets: func(_ context.Context, _ int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.PodDisruptionBudget], error) {
			return emptyList[k8sgateway.PodDisruptionBudget](), nil
		},
		pods: func(_ context.Context, _ int64, ns string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Pod], error) {
			if ns == "kube-system" {
				return apiquery.ListResponse[k8sgateway.Pod]{Items: []k8sgateway.Pod{makePod("coredns", "Running", "n1")}, Total: 1}, nil
			}
			return emptyList[k8sgateway.Pod](), nil
		},
		deployments: func(_ context.Context, _ int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Deployment], error) {
			return emptyList[k8sgateway.Deployment](), nil
		},
		statefulSets: func(_ context.Context, _ int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.StatefulSet], error) {
			return emptyList[k8sgateway.StatefulSet](), nil
		},
		daemonSets: func(_ context.Context, _ int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.DaemonSet], error) {
			return emptyList[k8sgateway.DaemonSet](), nil
		},
		jobs: func(_ context.Context, _ int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Job], error) {
			return emptyList[k8sgateway.Job](), nil
		},
		cronJobs: func(_ context.Context, _ int64, _ string, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.CronJob], error) {
			return emptyList[k8sgateway.CronJob](), nil
		},
		nodes: func(_ context.Context, _ int64, _ apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Node], error) {
			return emptyList[k8sgateway.Node](), nil
		},
	}
	svc := NewService(mock)
	resp, err := svc.List(context.Background(), 1, apiquery.ListQuery{Page: 1, Limit: 100})
	if err != nil {
		t.Fatalf("List unexpected error: %v", err)
	}
	if resp.Total != 2 {
		t.Errorf("total = %d, want 2", resp.Total)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("items len = %d, want 2", len(resp.Items))
	}
	for _, e := range resp.Items {
		switch e.Name {
		case "default":
			if e.QuotaCount != 1 {
				t.Errorf("default quota_count = %d, want 1", e.QuotaCount)
			}
			if e.PodCount != 0 {
				t.Errorf("default pod_count = %d, want 0", e.PodCount)
			}
		case "kube-system":
			if e.PodCount != 1 {
				t.Errorf("kube-system pod_count = %d, want 1", e.PodCount)
			}
			if e.QuotaCount != 0 {
				t.Errorf("kube-system quota_count = %d, want 0", e.QuotaCount)
			}
		}
	}
}

func TestMarkTruncated_SetsTruncatedWhenRemaining(t *testing.T) {
	clock := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	c := newCitation("/x", clock)
	markTruncated(&c, 250, 100, 150)
	if c.Status != SourceTruncated {
		t.Errorf("status = %q, want truncated", c.Status)
	}
	if c.Total != 250 || c.Returned != 100 || c.Remaining != 150 {
		t.Errorf("counts = (%d,%d,%d), want (250,100,150)", c.Total, c.Returned, c.Remaining)
	}
}

func TestSortedPhaseCounts_OrdersAlphabetically(t *testing.T) {
	m := map[string]int32{"Pending": 1, "Running": 5, "Failed": 2}
	out := sortedPhaseCounts(m)
	if len(out) != 3 {
		t.Fatalf("len = %d, want 3", len(out))
	}
	if out[0].Phase != "Failed" || out[1].Phase != "Pending" || out[2].Phase != "Running" {
		t.Errorf("order = [%s,%s,%s], want Failed,Pending,Running", out[0].Phase, out[1].Phase, out[2].Phase)
	}
}

func TestCopyMap_NilOnEmpty(t *testing.T) {
	if got := copyMap(map[string]string{}); got != nil {
		t.Errorf("copyMap(empty) = %v, want nil", got)
	}
	src := map[string]string{"a": "1"}
	dst := copyMap(src)
	dst["a"] = "modified"
	if src["a"] != "1" {
		t.Errorf("copyMap did not copy; src mutated")
	}
}

func TestStringValue_HandlesNumericTypes(t *testing.T) {
	cases := []struct {
		in   interface{}
		want string
	}{
		{nil, ""},
		{"10%", "10%"},
		{int64(3), "3"},
		{int32(5), "5"},
		{int(7), "7"},
		{float64(2), "2"},
	}
	for _, c := range cases {
		got := stringValue(c.in)
		if got != c.want {
			t.Errorf("stringValue(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNamespacedAPIPath_WithAndWithoutNamespace(t *testing.T) {
	if got := namespacedAPIPath("/api/v1", "default", "pods"); got != "/api/v1/namespaces/default/pods" {
		t.Errorf("with ns = %q", got)
	}
	if got := namespacedAPIPath("/api/v1", "", "nodes"); got != "/api/v1/nodes" {
		t.Errorf("without ns = %q", got)
	}
}
