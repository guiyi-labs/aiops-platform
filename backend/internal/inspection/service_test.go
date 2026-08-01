package inspection

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
)

// --- fake types for tests ---

type fakeClusterLister struct {
	items []struct {
		ID   int64
		Name string
	}
	err error
}

func (f *fakeClusterLister) List(_ context.Context) ([]struct {
	ID   int64
	Name string
}, error) {
	return f.items, f.err
}

type fakeExecutor struct {
	mu      sync.Mutex
	returns map[string][]Finding
	errors  map[string]error
	calls   [][]interface{}
}

func newFakeExecutor() *fakeExecutor {
	return &fakeExecutor{returns: map[string][]Finding{}, errors: map[string]error{}}
}

func (f *fakeExecutor) Execute(_ context.Context, clusterID int64, rule RuleDescriptor) ([]Finding, error) {
	f.mu.Lock()
	f.calls = append(f.calls, []interface{}{clusterID, rule.Code})
	f.mu.Unlock()
	key := rule.Code
	if err, ok := f.errors[key]; ok {
		return nil, err
	}
	return f.returns[key], nil
}

// --- tests ---

func TestNewServiceAppliesDefaults(t *testing.T) {
	svc, err := NewService(Config{}, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewService error: %v", err)
	}
	if svc.maxConcurrentClusters != DefaultMaxConcurrentClusters {
		t.Errorf("default max concurrent = %d, want %d", svc.maxConcurrentClusters, DefaultMaxConcurrentClusters)
	}
	if svc.perClusterTimeout != DefaultPerClusterTimeout {
		t.Errorf("default per-cluster timeout = %v, want %v", svc.perClusterTimeout, DefaultPerClusterTimeout)
	}
	if len(svc.catalogList) != 8 {
		t.Errorf("catalog has %d rules, want 8", len(svc.catalogList))
	}
}

func TestCatalogContainsAllExpectedRules(t *testing.T) {
	catalog := DefaultCatalog()
	codes := map[string]bool{}
	for _, r := range catalog {
		codes[r.Code] = true
	}
	want := []string{
		"node_not_ready", "node_pressure",
		"pod_restart_loop", "pod_pending", "container_oom_killed",
		"workload_replicas_unavailable",
		"pvc_pending",
		"ingress_backend_unhealthy",
	}
	for _, c := range want {
		if !codes[c] {
			t.Errorf("catalog missing rule %q", c)
		}
	}
	for _, r := range catalog {
		if r.SchemaVersion != "1.0" {
			t.Errorf("rule %q: schema_version = %q, want 1.0", r.Code, r.SchemaVersion)
		}
		if r.SignalCode == "" {
			t.Errorf("rule %q: empty signal_code", r.Code)
		}
		if r.DefaultSeverity != SeverityCritical && r.DefaultSeverity != SeverityWarning {
			t.Errorf("rule %q: bad default severity %q", r.Code, r.DefaultSeverity)
		}
	}
}

func TestIsValidRuleCode(t *testing.T) {
	byCode := CatalogByCode(DefaultCatalog())
	if !IsValidRuleCode(byCode, "node_not_ready") {
		t.Error("node_not_ready should be valid")
	}
	if IsValidRuleCode(byCode, "nope") {
		t.Error("nope should be invalid")
	}
}

func TestCreatePlanValidatesRuleCodes(t *testing.T) {
	repo := newInMemRepo()
	svc, err := NewService(Config{}, repo, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	badPlan := &Plan{Name: "bad", CreatorID: 1, RuleCodes: []string{"nonexistent"}}
	if _, err := svc.CreatePlan(context.Background(), badPlan); !errors.Is(err, ErrInvalidRuleCode) {
		t.Errorf("want ErrInvalidRuleCode, got %v", err)
	}
	goodPlan := &Plan{Name: "good", CreatorID: 1, RuleCodes: []string{"pod_pending"}}
	created, err := svc.CreatePlan(context.Background(), goodPlan)
	if err != nil {
		t.Fatalf("create good plan: %v", err)
	}
	if created.Name != "good" {
		t.Errorf("created.Name = %q, want good", created.Name)
	}
}

func TestRunInspectOnceResolvesClusters(t *testing.T) {
	repo := newInMemRepo()
	cl := &fakeClusterLister{
		items: []struct {
			ID   int64
			Name string
		}{{ID: 7, Name: "c1"}, {ID: 8, Name: "c2"}},
	}
	exec := newFakeExecutor()
	exec.returns["pod_pending"] = []Finding{{
		RuleCode: "pod_pending", SignalCode: "x", Namespace: "ns", ResourceKind: "Pod", ResourceName: "p", ResourceUID: "u",
		ObservedAt: time.Now(), Evidence: map[string]interface{}{"k": "v"},
	}}
	svc, err := NewService(Config{MaxConcurrentClusters: 2, PerClusterTimeout: 200 * time.Millisecond}, repo, exec, cl, nil)
	if err != nil {
		t.Fatal(err)
	}
	task, err := svc.RunInspectOnce(context.Background(), 1, nil, []string{"pod_pending"})
	if err != nil {
		t.Fatalf("RunInspectOnce: %v", err)
	}
	if task.TotalClusters != 2 {
		t.Errorf("task.TotalClusters = %d, want 2", task.TotalClusters)
	}
	// Wait for background run.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		got, _ := svc.repo.GetTask(context.Background(), task.ID)
		if got.Status == TaskStatusCompleted || got.Status == TaskStatusPartial || got.Status == TaskStatusFailed {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	// Assert each cluster was called.
	seen := map[int64]bool{}
	for _, c := range exec.calls {
		id := c[0].(int64)
		if c[1].(string) == "pod_pending" {
			seen[id] = true
		}
	}
	if !seen[7] || !seen[8] {
		t.Errorf("executor calls = %v, want both clusters", exec.calls)
	}
	results, err := svc.repo.ListResults(context.Background(), ListFilter{TaskID: &task.ID, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if results.Total == 0 {
		t.Errorf("expected >=1 results, got %d", results.Total)
	}
}

func TestFingerprintDedupesFindings(t *testing.T) {
	a := Finding{RuleCode: "r", Namespace: "n", ResourceKind: "Pod", ResourceName: "x", ResourceUID: "u1"}
	b := Finding{RuleCode: "r", Namespace: "n", ResourceKind: "Pod", ResourceName: "x", ResourceUID: "u1"}
	c := Finding{RuleCode: "r", Namespace: "n", ResourceKind: "Pod", ResourceName: "x", ResourceUID: "u2"}
	fp1 := fingerprint(1, a)
	fp2 := fingerprint(1, b)
	fp3 := fingerprint(1, c)
	if fp1 != fp2 {
		t.Errorf("identical findings got different fps: %s vs %s", fp1, fp2)
	}
	if fp1 == fp3 {
		t.Error("different UIDs got identical fingerprint")
	}
}

func TestResultEvidenceRoundsTrip(t *testing.T) {
	repo := newInMemRepo()
	err := repo.CreateResults(context.Background(), []Result{{
		TaskID: 1, ClusterID: 1, RuleCode: "r", SignalCode: "s", Severity: SeverityWarning,
		Namespace: "ns", ResourceKind: "Pod", ResourceName: "p", ResourceUID: "u", Fingerprint: "fp",
		EvidenceSnapshot: `{"k":"v","n":42}`,
		ObservedAt:       time.Now(), CreatedAt: time.Now(),
	}})
	if err != nil {
		t.Fatal(err)
	}
	svc, _ := NewService(Config{}, repo, nil, nil, nil)
	views, err := svc.ListResults(context.Background(), ListFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if views.Total != 1 {
		t.Fatalf("total = %d, want 1", views.Total)
	}
	v := views.Items[0]
	if v.Evidence["k"] != "v" {
		t.Errorf("evidence.k = %v, want v", v.Evidence["k"])
	}
}

// --- in-memory repository for isolated service tests ---

type inMemRepo struct {
	rules     map[string]*RuleOverride
	plans     map[int64]*Plan
	tasks     map[int64]*Task
	results   []Result
	planSeq   int64
	taskSeq   int64
	resultSeq int64
}

func newInMemRepo() *inMemRepo {
	return &inMemRepo{
		rules:   map[string]*RuleOverride{},
		plans:   map[int64]*Plan{},
		tasks:   map[int64]*Task{},
		results: []Result{},
	}
}

func (m *inMemRepo) UpsertRuleOverride(_ context.Context, o *RuleOverride) error {
	key := o.RuleCode + ":" + itoa(o.ClusterID)
	m.rules[key] = o
	return nil
}

func (m *inMemRepo) ListRuleOverrides(_ context.Context, clusterID int64) ([]RuleOverride, error) {
	var out []RuleOverride
	for k, v := range m.rules {
		if hasSuffixI64(k, clusterID) {
			out = append(out, *v)
		}
	}
	return out, nil
}

func (m *inMemRepo) GetRuleOverride(_ context.Context, clusterID int64, ruleCode string) (*RuleOverride, error) {
	return m.rules[ruleCode+":"+itoa(clusterID)], nil
}

func (m *inMemRepo) CreatePlan(_ context.Context, plan *Plan) error {
	m.planSeq++
	plan.ID = m.planSeq
	m.plans[plan.ID] = plan
	return nil
}

func (m *inMemRepo) GetPlan(_ context.Context, id int64) (Plan, error) {
	p, ok := m.plans[id]
	if !ok {
		return Plan{}, ErrPlanNotFound
	}
	return *p, nil
}

func (m *inMemRepo) ListPlans(_ context.Context, _ PlanListFilter) ([]Plan, error) {
	out := make([]Plan, 0, len(m.plans))
	for _, p := range m.plans {
		out = append(out, *p)
	}
	return out, nil
}

func (m *inMemRepo) UpdatePlan(_ context.Context, id int64, _ PatchPlanInput) (Plan, error) {
	p, ok := m.plans[id]
	if !ok {
		return Plan{}, ErrPlanNotFound
	}
	return *p, nil
}

func (m *inMemRepo) DeletePlan(_ context.Context, id, _ int64) error {
	if _, ok := m.plans[id]; !ok {
		return ErrPlanNotFound
	}
	delete(m.plans, id)
	return nil
}

func (m *inMemRepo) TouchPlanRun(_ context.Context, _ int64, _, _ *gorm.DeletedAt) error { return nil }

func (m *inMemRepo) CreateTask(_ context.Context, task *Task) error {
	m.taskSeq++
	task.ID = m.taskSeq
	m.tasks[task.ID] = task
	return nil
}

func (m *inMemRepo) GetTask(_ context.Context, id int64) (Task, error) {
	t, ok := m.tasks[id]
	if !ok {
		return Task{}, ErrTaskNotFound
	}
	return *t, nil
}

func (m *inMemRepo) ListTasks(_ context.Context, filter TaskListFilter) (ListResponse[Task], error) {
	out := make([]Task, 0, len(m.tasks))
	for _, t := range m.tasks {
		if filter.Status != "" && t.Status != filter.Status {
			continue
		}
		out = append(out, *t)
	}
	return ListResponse[Task]{Items: out, Total: len(out)}, nil
}

func (m *inMemRepo) UpdateTaskStatus(_ context.Context, id int64, patch PatchTaskInput) error {
	t, ok := m.tasks[id]
	if !ok {
		return ErrTaskNotFound
	}
	if patch.Status != nil {
		t.Status = *patch.Status
	}
	if patch.CompletedClusters != nil {
		t.CompletedClusters = *patch.CompletedClusters
	}
	if patch.FindingCount != nil {
		t.FindingCount = *patch.FindingCount
	}
	if patch.ErrorSummary != nil {
		t.ErrorSummary = *patch.ErrorSummary
	}
	if patch.StartedAt != nil {
		t.StartedAt = &patch.StartedAt.Time
	}
	if patch.FinishedAt != nil {
		t.FinishedAt = &patch.FinishedAt.Time
	}
	return nil
}

func (m *inMemRepo) CreateResults(_ context.Context, results []Result) error {
	for i := range results {
		m.resultSeq++
		results[i].ID = m.resultSeq
		m.results = append(m.results, results[i])
	}
	return nil
}

func (m *inMemRepo) ListResults(_ context.Context, filter ListFilter) (ListResponse[Result], error) {
	out := make([]Result, 0, len(m.results))
	for _, r := range m.results {
		if filter.TaskID != nil && r.TaskID != *filter.TaskID {
			continue
		}
		out = append(out, r)
	}
	limit := filter.Limit
	if limit == 0 || limit > len(out) {
		limit = len(out)
	}
	return ListResponse[Result]{Items: out[:limit], Total: len(out)}, nil
}

func (m *inMemRepo) GetResult(_ context.Context, id int64) (Result, error) {
	for _, r := range m.results {
		if r.ID == id {
			return r, nil
		}
	}
	return Result{}, ErrResultNotFound
}

func itoa(i int64) string { return strconv.FormatInt(i, 10) }

func hasSuffixI64(key string, n int64) bool {
	return strings.HasSuffix(key, ":"+itoa(n))
}
