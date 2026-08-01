package automation

import (
	"context"
	"errors"
	"testing"
	"time"

	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

// --- Fakes ---

// fakeCaseReader is a test double for CaseReader.
type fakeCaseReader struct {
	caseCtx  CaseContext
	caseErr  error
	codes    map[string]bool
	codesErr error
}

func (f *fakeCaseReader) GetCase(context.Context, int64) (CaseContext, error) {
	return f.caseCtx, f.caseErr
}

func (f *fakeCaseReader) EligibleActionCodes(context.Context, int64) (map[string]bool, error) {
	return f.codes, f.codesErr
}

// fakeKubernetesSource is a test double for KubernetesSource.
type fakeKubernetesSource struct {
	deployment   k8sgateway.Deployment
	depErr       error
	cronJob      k8sgateway.CronJob
	cronErr      error
	replicaSet   k8sgateway.ReplicaSet
	rsErr        error
	history      k8sgateway.RolloutHistory
	historyErr   error
	patchDep     k8sgateway.Deployment
	patchDepErr  error
	patchCron    k8sgateway.CronJob
	patchCronErr error
	lastPatch    []byte
	patchCalls   int
}

func (f *fakeKubernetesSource) Deployment(context.Context, int64, string, string) (k8sgateway.Deployment, error) {
	return f.deployment, f.depErr
}
func (f *fakeKubernetesSource) PatchDeployment(_ context.Context, _ int64, _, _ string, patch []byte, _ bool) (k8sgateway.Deployment, error) {
	f.patchCalls++
	f.lastPatch = append([]byte(nil), patch...)
	return f.patchDep, f.patchDepErr
}
func (f *fakeKubernetesSource) CronJob(context.Context, int64, string, string) (k8sgateway.CronJob, error) {
	return f.cronJob, f.cronErr
}
func (f *fakeKubernetesSource) PatchCronJob(_ context.Context, _ int64, _, _ string, patch []byte, _ bool) (k8sgateway.CronJob, error) {
	f.patchCalls++
	f.lastPatch = append([]byte(nil), patch...)
	return f.patchCron, f.patchCronErr
}
func (f *fakeKubernetesSource) ReplicaSet(context.Context, int64, string, string) (k8sgateway.ReplicaSet, error) {
	return f.replicaSet, f.rsErr
}
func (f *fakeKubernetesSource) RolloutHistory(context.Context, int64, string, string) (k8sgateway.RolloutHistory, error) {
	return f.history, f.historyErr
}

var _ KubernetesSource = (*fakeKubernetesSource)(nil)
var _ CaseReader = (*fakeCaseReader)(nil)

// memRepo is an in-memory Repository for tests that need stateful
// GetPlan/SavePlan/ListPlans without a database. It embeds NopRepository
// so every other method is a no-op.
type memRepo struct {
	NopRepository
	plans     map[string]ActionPlan
	listTotal int64 // when 0, ListPlans returns the actual count
}

func newMemRepo() *memRepo {
	return &memRepo{plans: make(map[string]ActionPlan)}
}

func (r *memRepo) SavePlan(_ context.Context, plan *ActionPlan) error {
	r.plans[plan.ID] = *plan
	return nil
}

func (r *memRepo) GetPlan(_ context.Context, id string) (ActionPlan, error) {
	p, ok := r.plans[id]
	if !ok {
		return ActionPlan{}, ErrPlanNotFound
	}
	return p, nil
}

func (r *memRepo) ListPlans(_ context.Context, _ ActionPlanFilter) ([]ActionPlan, int64, error) {
	items := make([]ActionPlan, 0, len(r.plans))
	for _, p := range r.plans {
		items = append(items, p)
	}
	total := r.listTotal
	if total == 0 {
		total = int64(len(items))
	}
	return items, total, nil
}

// listStubRepo is a minimal Repository that returns canned ListPlans
// results. It embeds NopRepository for all other methods.
type listStubRepo struct {
	NopRepository
	items []ActionPlan
	total int64
}

func (s *listStubRepo) ListPlans(context.Context, ActionPlanFilter) ([]ActionPlan, int64, error) {
	return s.items, s.total, nil
}

// --- Helpers ---

// newTestService builds a Service wired with the given fakes and a fixed
// clock. Tests that need specific fakes pass them in; nil reader defaults
// to NopCaseReader.
func newTestService(t *testing.T, repo Repository, reader CaseReader, k8s KubernetesSource) *Service {
	t.Helper()
	if reader == nil {
		reader = NopCaseReader{}
	}
	return NewService(repo, reader, k8s, WithNow(func() time.Time { return fixedTime }))
}

// --- CreatePlan rejection tests ---

func TestCreatePlanRejectsEmptyRunbook(t *testing.T) {
	t.Parallel()
	svc := newTestService(t, NopRepository{}, &fakeCaseReader{}, nil)
	_, err := svc.CreatePlan(context.Background(), CreatePlanInput{
		RunbookID: "",
		Operator:  ActorRef{ID: 1, Name: "alice"},
	})
	if !errors.Is(err, ErrInvalidRunbook) {
		t.Fatalf("err = %v, want ErrInvalidRunbook", err)
	}

	// Whitespace-only is also empty.
	_, err = svc.CreatePlan(context.Background(), CreatePlanInput{
		RunbookID: "   ",
		Operator:  ActorRef{ID: 1, Name: "alice"},
	})
	if !errors.Is(err, ErrInvalidRunbook) {
		t.Fatalf("err for whitespace-only = %v, want ErrInvalidRunbook", err)
	}
}

func TestCreatePlanRejectsUnknownRunbook(t *testing.T) {
	t.Parallel()
	svc := newTestService(t, NopRepository{}, &fakeCaseReader{}, nil)
	_, err := svc.CreatePlan(context.Background(), CreatePlanInput{
		RunbookID: "not_a_real_runbook",
		Operator:  ActorRef{ID: 1, Name: "alice"},
	})
	if !errors.Is(err, ErrRunbookNotInCatalog) {
		t.Fatalf("err = %v, want ErrRunbookNotInCatalog", err)
	}
}

func TestCreatePlanRejectsAdvisoryRunbook(t *testing.T) {
	// The shipped catalog only contains executable runbooks (every entry
	// has a non-empty ActionCode), so ErrAdvisoryRunbookNotExecutable is
	// not reachable via LookupRunbook. First assert the catalog invariant,
	// then exercise the guard by temporarily injecting a hypothetical
	// advisory runbook. This test is intentionally NOT parallel so the
	// map mutation does not race with parallel LookupRunbook reads.
	for id, rb := range catalog {
		if rb.ActionCode == "" {
			t.Fatalf("catalog entry %q has empty ActionCode — advisory runbooks must not be in the executable catalog", id)
		}
	}
	catalog["advisory_hypothetical"] = RunbookDescriptor{RunbookID: "advisory_hypothetical", ActionCode: "", Title: "advisory only"}
	t.Cleanup(func() { delete(catalog, "advisory_hypothetical") })

	svc := newTestService(t, NopRepository{}, &fakeCaseReader{
		caseCtx: CaseContext{CaseID: 1, ClusterID: 7, PrimaryKind: "Deployment", PrimaryNamespace: "default", PrimaryName: "api", PrimaryUID: "uid-1"},
	}, nil)
	_, err := svc.CreatePlan(context.Background(), CreatePlanInput{
		RunbookID: "advisory_hypothetical",
		Operator:  ActorRef{ID: 1, Name: "alice"},
	})
	if !errors.Is(err, ErrAdvisoryRunbookNotExecutable) {
		t.Fatalf("err = %v, want ErrAdvisoryRunbookNotExecutable", err)
	}
}

func TestCreatePlanRejectsIneligibleRunbook(t *testing.T) {
	t.Parallel()
	reader := &fakeCaseReader{
		caseCtx: CaseContext{CaseID: 1, ClusterID: 7, PrimaryKind: "Deployment", PrimaryNamespace: "default", PrimaryName: "api", PrimaryUID: "uid-1"},
		// Only deployment.scale is eligible; the runbook's action
		// (deployment.rollout_restart) is not in the candidate list.
		codes: map[string]bool{"deployment.scale": true},
	}
	svc := newTestService(t, NopRepository{}, reader, nil)
	_, err := svc.CreatePlan(context.Background(), CreatePlanInput{
		RunbookID: "rollout_restart_pods",
		Operator:  ActorRef{ID: 1, Name: "alice"},
	})
	if !errors.Is(err, ErrRunbookNotEligible) {
		t.Fatalf("err = %v, want ErrRunbookNotEligible", err)
	}
}

func TestCreatePlanRejectsWhenCaseNotFound(t *testing.T) {
	t.Parallel()
	reader := &fakeCaseReader{caseErr: ErrCaseNotFound}
	svc := newTestService(t, NopRepository{}, reader, nil)
	_, err := svc.CreatePlan(context.Background(), CreatePlanInput{
		RunbookID: "rollout_restart_pods",
		Operator:  ActorRef{ID: 1, Name: "alice"},
	})
	if !errors.Is(err, ErrCaseNotFound) {
		t.Fatalf("err = %v, want ErrCaseNotFound", err)
	}
}

// --- Approve tests ---

func TestApproveRejectsNonPreviewed(t *testing.T) {
	t.Parallel()
	repo := newMemRepo()
	plan := ActionPlan{
		ID:           "plan-draft",
		Status:       StatusDraft,
		ApprovalType: ApprovalSingle,
	}
	_ = repo.SavePlan(context.Background(), &plan)
	svc := newTestService(t, repo, &fakeCaseReader{}, nil)

	_, err := svc.Approve(context.Background(), "plan-draft", ActorRef{ID: 2, Name: "bob"})
	if !errors.Is(err, ErrNotPreviewed) {
		t.Fatalf("err = %v, want ErrNotPreviewed", err)
	}
}

func TestApproveRejectsSelfApprovalFourEyes(t *testing.T) {
	t.Parallel()
	repo := newMemRepo()
	requester := int64(42)
	plan := ActionPlan{
		ID:                "plan-four-eyes",
		Status:            StatusPreviewed,
		ApprovalType:      ApprovalFourEyes,
		RequestedByUserID: &requester,
		RequestedByName:   "alice",
	}
	_ = repo.SavePlan(context.Background(), &plan)
	svc := newTestService(t, repo, &fakeCaseReader{}, nil)

	// Same approver ID as requester → self-approval forbidden.
	_, err := svc.Approve(context.Background(), "plan-four-eyes", ActorRef{ID: 42, Name: "alice"})
	if !errors.Is(err, ErrSelfApprovalForbidden) {
		t.Fatalf("err = %v, want ErrSelfApprovalForbidden", err)
	}

	// Zero approver ID is also forbidden (no identity).
	_, err = svc.Approve(context.Background(), "plan-four-eyes", ActorRef{ID: 0, Name: ""})
	if !errors.Is(err, ErrSelfApprovalForbidden) {
		t.Fatalf("err for zero approver = %v, want ErrSelfApprovalForbidden", err)
	}
}

func TestApproveAcceptsDifferentApproverFourEyes(t *testing.T) {
	t.Parallel()
	// Approve needs to persist the transition; use an in-memory repo that
	// overrides Approve so the four-eyes happy path can be verified
	// without a database.
	requester := int64(42)
	approvedAt := fixedTime
	plan := ActionPlan{
		ID:                "plan-four-eyes-ok",
		Status:            StatusPreviewed,
		ApprovalType:      ApprovalFourEyes,
		RequestedByUserID: &requester,
		RequestedByName:   "alice",
	}
	mr := &approveMemRepo{
		memRepo:    newMemRepo(),
		approvedAt: approvedAt,
	}
	_ = mr.SavePlan(context.Background(), &plan)
	svc := newTestService(t, mr, &fakeCaseReader{}, nil)

	got, err := svc.Approve(context.Background(), "plan-four-eyes-ok", ActorRef{ID: 99, Name: "bob"})
	if err != nil {
		t.Fatalf("Approve returned error: %v", err)
	}
	if got.Status != StatusApproved {
		t.Errorf("Status = %q, want %q", got.Status, StatusApproved)
	}
	if got.ApproverUserID == nil || *got.ApproverUserID != 99 {
		t.Errorf("ApproverUserID = %v, want 99", got.ApproverUserID)
	}
}

// approveMemRepo embeds memRepo and overrides Approve so the four-eyes
// acceptance test can verify the happy path without a database.
type approveMemRepo struct {
	*memRepo
	approvedAt time.Time
}

func (r *approveMemRepo) Approve(_ context.Context, id string, approver ActorRef, now time.Time) (ActionPlan, error) {
	p, ok := r.plans[id]
	if !ok {
		return ActionPlan{}, ErrPlanNotFound
	}
	p.Status = StatusApproved
	p.ApproverUserID = &approver.ID
	p.ApproverName = approver.Name
	p.ApprovedAt = &now
	p.UpdatedAt = now
	r.plans[id] = p
	return p, nil
}

// --- Execute rejection tests ---

func TestExecuteRejectsEmptyConfirmationToken(t *testing.T) {
	t.Parallel()
	svc := newTestService(t, NopRepository{}, &fakeCaseReader{}, nil)
	_, err := svc.Execute(context.Background(), "plan-1", "", "valid-idempotency-key")
	if !errors.Is(err, ErrConfirmationInvalid) {
		t.Fatalf("err = %v, want ErrConfirmationInvalid", err)
	}
	// Whitespace-only token trims to empty.
	_, err = svc.Execute(context.Background(), "plan-1", "   ", "valid-idempotency-key")
	if !errors.Is(err, ErrConfirmationInvalid) {
		t.Fatalf("err for whitespace token = %v, want ErrConfirmationInvalid", err)
	}
}

func TestExecuteRejectsInvalidIdempotencyKey(t *testing.T) {
	t.Parallel()
	svc := newTestService(t, NopRepository{}, &fakeCaseReader{}, nil)
	cases := []struct {
		name string
		key  string
	}{
		{name: "too_short", key: "short"},
		{name: "empty", key: ""},
		{name: "too_long", key: stringOfLen(129)},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := svc.Execute(context.Background(), "plan-1", "token", tc.key)
			if !errors.Is(err, ErrInvalidIdempotency) {
				t.Fatalf("err for key %q = %v, want ErrInvalidIdempotency", tc.key, err)
			}
		})
	}
	// Boundary: 8 and 128 chars are valid lengths (not rejected here).
	_, err := svc.Execute(context.Background(), "plan-1", "token", stringOfLen(8))
	if errors.Is(err, ErrInvalidIdempotency) {
		t.Fatalf("8-char key should not be rejected as invalid idempotency, got %v", err)
	}
	_, err = svc.Execute(context.Background(), "plan-1", "token", stringOfLen(128))
	if errors.Is(err, ErrInvalidIdempotency) {
		t.Fatalf("128-char key should not be rejected as invalid idempotency, got %v", err)
	}
}

// stringOfLen returns a string of n 'a' characters.
func stringOfLen(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a'
	}
	return string(b)
}

// --- Verify rejection test ---

func TestVerifyRejectsNonVerifiablePlan(t *testing.T) {
	t.Parallel()
	repo := newMemRepo()
	plan := ActionPlan{
		ID:         "plan-draft",
		Status:     StatusDraft,
		ActionCode: "deployment.rollout_restart",
	}
	_ = repo.SavePlan(context.Background(), &plan)
	svc := newTestService(t, repo, &fakeCaseReader{}, nil)

	_, err := svc.Verify(context.Background(), "plan-draft")
	if !errors.Is(err, ErrNotVerifiable) {
		t.Fatalf("err = %v, want ErrNotVerifiable", err)
	}

	// Previewed is also not verifiable.
	previewed := ActionPlan{ID: "plan-previewed", Status: StatusPreviewed, ActionCode: "deployment.rollout_restart"}
	_ = repo.SavePlan(context.Background(), &previewed)
	_, err = svc.Verify(context.Background(), "plan-previewed")
	if !errors.Is(err, ErrNotVerifiable) {
		t.Fatalf("err for previewed = %v, want ErrNotVerifiable", err)
	}
}

// --- Cancel disabled test ---

func TestCancelDisabledService(t *testing.T) {
	t.Parallel()
	// Construct a disabled service directly (enabled=false). The service
	// must have a non-nil repo to reach the enabled check.
	svc := &Service{enabled: false, repo: NopRepository{}, now: func() time.Time { return fixedTime }}
	_, err := svc.Cancel(context.Background(), "plan-1")
	if !errors.Is(err, ErrDisabled) {
		t.Fatalf("err = %v, want ErrDisabled", err)
	}
	// nil repo also disables.
	svc = &Service{enabled: true, repo: nil, now: func() time.Time { return fixedTime }}
	_, err = svc.Cancel(context.Background(), "plan-1")
	if !errors.Is(err, ErrDisabled) {
		t.Fatalf("err for nil repo = %v, want ErrDisabled", err)
	}
}

// --- Pure function tests ---

func TestApprovalTypeFor(t *testing.T) {
	t.Parallel()
	cases := []struct {
		action string
		want   ApprovalType
	}{
		{"deployment.rollback", ApprovalFourEyes},
		{"deployment.image_update", ApprovalFourEyes},
		{"deployment.rollout_restart", ApprovalSingle},
		{"deployment.scale", ApprovalSingle},
		{"cronjob.suspend", ApprovalSingle},
		{"cronjob.resume", ApprovalSingle},
		{"unknown.action", ApprovalSingle},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.action, func(t *testing.T) {
			t.Parallel()
			if got := approvalTypeFor(tc.action); got != tc.want {
				t.Fatalf("approvalTypeFor(%q) = %q, want %q", tc.action, got, tc.want)
			}
		})
	}
}

func TestComputePlanKey(t *testing.T) {
	t.Parallel()
	key1 := computePlanKey(1, "rollout_restart_pods", "uid-1")
	key2 := computePlanKey(1, "rollout_restart_pods", "uid-1")
	if key1 != key2 {
		t.Fatalf("computePlanKey not deterministic: %q != %q", key1, key2)
	}
	if len(key1) != MaxPlanKeyLength {
		t.Fatalf("key length = %d, want %d", len(key1), MaxPlanKeyLength)
	}

	// Different inputs produce different keys.
	if k := computePlanKey(2, "rollout_restart_pods", "uid-1"); k == key1 {
		t.Error("expected different key for different caseID")
	}
	if k := computePlanKey(1, "rollback_last_rollout", "uid-1"); k == key1 {
		t.Error("expected different key for different runbookID")
	}
	if k := computePlanKey(1, "rollout_restart_pods", "uid-2"); k == key1 {
		t.Error("expected different key for different targetUID")
	}
}

// --- ListPlans test ---

func TestListPlansReturnsResponseWithTruncated(t *testing.T) {
	t.Parallel()
	// Two items in the page but total=10 → truncated.
	items := []ActionPlan{
		{ID: "plan-1", Status: StatusVerified, ActionCode: "deployment.rollout_restart", TargetName: "api", TargetNamespace: "default", TargetKind: "Deployment"},
		{ID: "plan-2", Status: StatusVerified, ActionCode: "deployment.scale", TargetName: "worker", TargetNamespace: "default", TargetKind: "Deployment"},
	}
	repo := &listStubRepo{items: items, total: 10}
	svc := newTestService(t, repo, &fakeCaseReader{}, nil)

	resp, err := svc.ListPlans(context.Background(), ActionPlanFilter{Limit: 2})
	if err != nil {
		t.Fatalf("ListPlans returned error: %v", err)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("Items count = %d, want 2", len(resp.Items))
	}
	if resp.Total != 10 {
		t.Errorf("Total = %d, want 10", resp.Total)
	}
	if !resp.Truncated {
		t.Error("Truncated = false, want true (items < total)")
	}
	// Each item must be converted to a response with a Target.
	if resp.Items[0].Target.Name == "" {
		t.Error("expected response item to have a target")
	}
}

func TestListPlansNotTruncatedWhenItemsEqualTotal(t *testing.T) {
	t.Parallel()
	items := []ActionPlan{{ID: "plan-1", Status: StatusVerified}}
	repo := &listStubRepo{items: items, total: 1}
	svc := newTestService(t, repo, &fakeCaseReader{}, nil)

	resp, err := svc.ListPlans(context.Background(), ActionPlanFilter{})
	if err != nil {
		t.Fatalf("ListPlans returned error: %v", err)
	}
	if resp.Truncated {
		t.Error("Truncated = true, want false (items == total)")
	}
}

func TestListPlansNotTruncatedWhenEmpty(t *testing.T) {
	t.Parallel()
	repo := &listStubRepo{items: nil, total: 0}
	svc := newTestService(t, repo, &fakeCaseReader{}, nil)

	resp, err := svc.ListPlans(context.Background(), ActionPlanFilter{})
	if err != nil {
		t.Fatalf("ListPlans returned error: %v", err)
	}
	if resp.Truncated {
		t.Error("Truncated = true, want false (no items)")
	}
}
