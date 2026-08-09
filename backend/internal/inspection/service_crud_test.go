package inspection

// Covers the service CRUD surface (plans, tasks, results), the GORM array
// column wrappers and table-name conventions. The in-memory repository is
// shared from service_test.go.

import (
	"context"
	"errors"
	"testing"
)

func TestCatalogAndEffectiveRules(t *testing.T) {
	repo := newInMemRepo()
	svc, err := NewService(Config{}, repo, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(svc.Catalog()) == 0 {
		t.Fatal("Catalog should be non-empty")
	}
	// No overrides: effective rules equal catalog.
	eff, err := svc.EffectiveRules(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(eff) != len(svc.Catalog()) {
		t.Errorf("EffectiveRules = %d, want %d", len(eff), len(svc.Catalog()))
	}
	// Disable one rule and re-check.
	if err := repo.UpsertRuleOverride(context.Background(), &RuleOverride{
		ClusterID: 1, RuleCode: "pod_pending", Enabled: false,
	}); err != nil {
		t.Fatal(err)
	}
	eff, err = svc.EffectiveRules(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range eff {
		if d.Code == "pod_pending" {
			t.Error("disabled rule should be filtered out")
		}
	}
}

func TestPlanCRUDLifecycle(t *testing.T) {
	svc, err := NewService(Config{}, newInMemRepo(), nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	created, err := svc.CreatePlan(ctx, &Plan{Name: "nightly", CreatorID: 1, RuleCodes: []string{"pod_pending"}})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	got, err := svc.GetPlan(ctx, created.ID)
	if err != nil || got.Name != "nightly" {
		t.Fatalf("GetPlan = %+v, %v", got, err)
	}
	listed, err := svc.ListPlans(ctx, PlanListFilter{})
	if err != nil || len(listed) != 1 {
		t.Fatalf("ListPlans = %+v, %v", listed, err)
	}
	// PATCH with an invalid rule code fails closed.
	invalid := "not_a_rule"
	if _, err := svc.UpdatePlan(ctx, created.ID, PatchPlanInput{RuleCodes: &[]string{invalid}}); !errors.Is(err, ErrInvalidRuleCode) {
		t.Errorf("UpdatePlan invalid code err = %v, want ErrInvalidRuleCode", err)
	}
	if _, err := svc.UpdatePlan(ctx, created.ID, PatchPlanInput{RuleCodes: &[]string{"node_not_ready"}}); err != nil {
		t.Fatalf("UpdatePlan: %v", err)
	}
	if err := svc.DeletePlan(ctx, created.ID, 1); err != nil {
		t.Fatalf("DeletePlan: %v", err)
	}
	if _, err := svc.GetPlan(ctx, created.ID); !errors.Is(err, ErrPlanNotFound) {
		t.Errorf("GetPlan after delete err = %v, want ErrPlanNotFound", err)
	}
}

func TestCreatePlanValidation(t *testing.T) {
	svc, _ := NewService(Config{}, newInMemRepo(), nil, nil, nil)
	ctx := context.Background()
	if _, err := svc.CreatePlan(ctx, nil); !errors.Is(err, ErrInvalidPlan) {
		t.Errorf("nil plan err = %v", err)
	}
	if _, err := svc.CreatePlan(ctx, &Plan{Name: "", CreatorID: 1}); !errors.Is(err, ErrInvalidPlan) {
		t.Errorf("empty name err = %v", err)
	}
	if _, err := svc.CreatePlan(ctx, &Plan{Name: "x", CreatorID: 0}); !errors.Is(err, ErrInvalidPlan) {
		t.Errorf("zero creator err = %v", err)
	}
	if _, err := svc.CreatePlan(ctx, &Plan{Name: "x", CreatorID: 1, RuleCodes: []string{"nope"}}); !errors.Is(err, ErrInvalidRuleCode) {
		t.Errorf("bad rule code err = %v", err)
	}
	// Enabled plan with a cron spec must produce a NextRunAt (nil from the
	// placeholder cron resolver) without error.
	created, err := svc.CreatePlan(ctx, &Plan{Name: "cron", CreatorID: 1, RuleCodes: []string{"pod_pending"}, Enabled: true, CronSpec: "0 * * * *"})
	if err != nil {
		t.Fatalf("CreatePlan enabled: %v", err)
	}
	if created.Enabled != true {
		t.Errorf("created.Enabled = %v", created.Enabled)
	}
}

func TestTasksAndResultsQueries(t *testing.T) {
	repo := newInMemRepo()
	svc, err := NewService(Config{}, repo, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := svc.GetTask(ctx, 42); !errors.Is(err, ErrTaskNotFound) {
		t.Errorf("GetTask missing err = %v", err)
	}
	if _, err := svc.GetResult(ctx, 42); !errors.Is(err, ErrResultNotFound) {
		t.Errorf("GetResult missing err = %v", err)
	}
	task := Task{
		Status: TaskStatusCompleted, TotalClusters: 1, CompletedClusters: 1, FindingCount: 2,
	}
	if err := svc.repo.CreateTask(ctx, &task); err != nil {
		t.Fatal(err)
	}
	view, err := svc.GetTask(ctx, task.ID)
	if err != nil || view.Status != TaskStatusCompleted {
		t.Fatalf("GetTask = %+v, %v", view, err)
	}
	listed, err := svc.ListTasks(ctx, TaskListFilter{Status: TaskStatusCompleted})
	if err != nil || len(listed.Items) != 1 {
		t.Fatalf("ListTasks = %+v, %v", listed, err)
	}
	// CancelTask is idempotent for unknown ids.
	svc.CancelTask(ctx, task.ID)
	svc.CancelTask(ctx, 999)
	results := []Result{{
		TaskID: task.ID, ClusterID: 1, RuleCode: "r", SignalCode: "s",
		Severity: SeverityWarning, Namespace: "ns", ResourceKind: "Pod",
		ResourceName: "p", ResourceUID: "u", Fingerprint: "fp",
		EvidenceSnapshot: `{"k":"v"}`, ObservedAt: task.CreatedAt,
	}}
	if err := svc.repo.CreateResults(ctx, results); err != nil {
		t.Fatal(err)
	}
	rview, err := svc.GetResult(ctx, results[0].ID)
	if err != nil || rview.Evidence["k"] != "v" {
		t.Fatalf("GetResult = %+v, %v", rview, err)
	}
}

func TestArrayWrappers(t *testing.T) {
	// Int64Array
	var ia Int64Array
	if err := ia.Scan(nil); err != nil || ia != nil {
		t.Errorf("Int64Array.Scan(nil) = %v, %v", ia, err)
	}
	v, err := Int64Array{1, 2}.Value()
	if err != nil || v == nil {
		t.Errorf("Int64Array.Value = %v, %v", v, err)
	}
	var sa StringArray
	if err := sa.Scan(nil); err != nil || sa != nil {
		t.Errorf("StringArray.Scan(nil) = %v, %v", sa, err)
	}
	v2, err := StringArray{"a", "b"}.Value()
	if err != nil || v2 == nil {
		t.Errorf("StringArray.Value = %v, %v", v2, err)
	}
}

func TestTableNames(t *testing.T) {
	if (RuleOverride{}).TableName() != "inspection_rules" {
		t.Error("RuleOverride table name")
	}
	if (Plan{}).TableName() != "inspection_plans" {
		t.Error("Plan table name")
	}
	if (Task{}).TableName() != "inspection_tasks" {
		t.Error("Task table name")
	}
	if (Result{}).TableName() != "inspection_results" {
		t.Error("Result table name")
	}
}
