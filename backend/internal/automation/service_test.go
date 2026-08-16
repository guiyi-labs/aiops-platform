package automation

import (
	"context"
	"crypto/sha256"
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

// ============================================================================
// M115-1b: automation/service coverage for Preview / GetPlan / materializeParameters
// ============================================================================

func TestGetPlanReturnsPlan(t *testing.T) {
	t.Parallel()
	repo := newMemRepo()
	_ = repo.SavePlan(context.Background(), &ActionPlan{
		ID:     "plan-existing",
		Status: StatusDraft,
	})
	svc := newTestService(t, repo, &fakeCaseReader{}, nil)
	plan, err := svc.GetPlan(context.Background(), "plan-existing")
	if err != nil {
		t.Fatalf("GetPlan err=%v", err)
	}
	if plan.ID != "plan-existing" {
		t.Fatalf("plan.ID=%q, want plan-existing", plan.ID)
	}
}

func TestGetPlanNotFound(t *testing.T) {
	t.Parallel()
	svc := newTestService(t, NopRepository{}, &fakeCaseReader{}, nil)
	_, err := svc.GetPlan(context.Background(), "nope")
	if !errors.Is(err, ErrPlanNotFound) {
		t.Fatalf("GetPlan err=%v, want ErrPlanNotFound", err)
	}
}

func TestPreviewDisabledService(t *testing.T) {
	t.Parallel()
	svc := &Service{enabled: false, repo: NopRepository{}, now: func() time.Time { return fixedTime }}
	_, err := svc.Preview(context.Background(), "x")
	if !errors.Is(err, ErrDisabled) {
		t.Fatalf("err=%v, want ErrDisabled", err)
	}
}

func TestPreviewNotDraftReturnsErrNotDraft(t *testing.T) {
	t.Parallel()
	repo := newMemRepo()
	_ = repo.SavePlan(context.Background(), &ActionPlan{
		ID:     "plan-previewed",
		Status: StatusPreviewed,
	})
	svc := newTestService(t, repo, &fakeCaseReader{}, nil)
	_, err := svc.Preview(context.Background(), "plan-previewed")
	if !errors.Is(err, ErrNotDraft) {
		t.Fatalf("err=%v, want ErrNotDraft", err)
	}
}

func TestPreviewDisabledRepoNil(t *testing.T) {
	t.Parallel()
	svc := &Service{enabled: true, repo: nil, now: func() time.Time { return fixedTime }}
	_, err := svc.Preview(context.Background(), "x")
	if !errors.Is(err, ErrDisabled) {
		t.Fatalf("err=%v, want ErrDisabled", err)
	}
}

func TestCreatePlanWithRolloutRestartMaterializesParams(t *testing.T) {
	t.Parallel()
	repo := newMemRepo()
	reader := &fakeCaseReader{
		caseCtx: CaseContext{
			CaseID:           10,
			ClusterID:        1,
			PrimaryKind:      "Deployment",
			PrimaryNamespace: "default",
			PrimaryName:      "web",
		},
		codes: map[string]bool{"deployment.rollout_restart": true},
	}
	fk8s := &fakeKubernetesSource{
		deployment: k8sgateway.Deployment{
			Metadata: k8sgateway.ObjectMeta{UID: "dep-uid-1", ResourceVersion: "rv-42"},
			Spec: struct {
				Replicas *int32 `json:"replicas,omitempty"`
				Selector struct {
					MatchLabels map[string]string `json:"matchLabels,omitempty"`
				} `json:"selector"`
				Template k8sgateway.WorkloadTemplate `json:"template"`
			}{Replicas: int32Ptr(3)},
		},
	}
	svc := newTestService(t, repo, reader, fk8s)
	plan, err := svc.CreatePlan(context.Background(), CreatePlanInput{
		CaseID:    10,
		RunbookID: "rollout_restart_pods",
		Operator:  ActorRef{ID: 1, Name: "alice"},
	})
	if err != nil {
		t.Fatalf("CreatePlan err=%v", err)
	}
	if plan.TargetUID != "dep-uid-1" || plan.TargetResourceVersion != "rv-42" {
		t.Fatalf("target snapshot = UID=%q RV=%q", plan.TargetUID, plan.TargetResourceVersion)
	}
	if plan.BeforeReplicas == nil || *plan.BeforeReplicas != 3 {
		t.Fatalf("BeforeReplicas = %v, want 3", plan.BeforeReplicas)
	}
	if plan.DesiredReplicas == nil || *plan.DesiredReplicas != 3 {
		t.Fatalf("DesiredReplicas = %v, want 3", plan.DesiredReplicas)
	}
}

func TestCreatePlanWithRollbackMaterializesParams(t *testing.T) {
	t.Parallel()
	repo := newMemRepo()
	reader := &fakeCaseReader{
		caseCtx: CaseContext{
			CaseID:           20,
			ClusterID:        1,
			PrimaryKind:      "Deployment",
			PrimaryNamespace: "prod",
			PrimaryName:      "api",
		},
		codes: map[string]bool{"deployment.rollback": true},
	}
	// History: rev=2 is current, rev=1 is previous (rollback target)
	fk8s := &fakeKubernetesSource{
		deployment: k8sgateway.Deployment{
			Metadata: k8sgateway.ObjectMeta{UID: "dep-uid-2", ResourceVersion: "rv-10"},
		},
		history: k8sgateway.RolloutHistory{
			Revisions: []k8sgateway.RolloutRevision{
				{Revision: 2, Current: true, ReplicaSetName: "api-v2", UID: "rs-2", ResourceVersion: "rs-rv-2"},
				{Revision: 1, Current: false, ReplicaSetName: "api-v1", UID: "rs-1", ResourceVersion: "rs-rv-1"},
			},
		},
	}
	svc := newTestService(t, repo, reader, fk8s)
	plan, err := svc.CreatePlan(context.Background(), CreatePlanInput{
		CaseID:    20,
		RunbookID: "rollback_last_rollout",
		Operator:  ActorRef{ID: 2, Name: "bob"},
	})
	if err != nil {
		t.Fatalf("CreatePlan err=%v", err)
	}
	if plan.RollbackRevision == nil || *plan.RollbackRevision != 1 {
		t.Fatalf("RollbackRevision=%v, want 1", plan.RollbackRevision)
	}
	if plan.RollbackReplicaSetName != "api-v1" {
		t.Fatalf("RollbackReplicaSetName=%q, want api-v1", plan.RollbackReplicaSetName)
	}
	if plan.RollbackReplicaSetUID != "rs-1" {
		t.Fatalf("RollbackReplicaSetUID=%q, want rs-1", plan.RollbackReplicaSetUID)
	}
}

func TestCreatePlanWithRollbackNoRollbackPoint(t *testing.T) {
	t.Parallel()
	repo := newMemRepo()
	reader := &fakeCaseReader{
		caseCtx: CaseContext{
			CaseID:           32,
			ClusterID:        1,
			PrimaryKind:      "Deployment",
			PrimaryNamespace: "default",
			PrimaryName:      "app",
		},
		codes: map[string]bool{"deployment.rollback": true},
	}
	// History with only one revision (current) — no rollback target
	fk8s := &fakeKubernetesSource{
		deployment: k8sgateway.Deployment{
			Metadata: k8sgateway.ObjectMeta{UID: "dep-uid-5"},
		},
		history: k8sgateway.RolloutHistory{
			Revisions: []k8sgateway.RolloutRevision{
				{Revision: 1, Current: true},
			},
		},
	}
	svc := newTestService(t, repo, reader, fk8s)
	_, err := svc.CreatePlan(context.Background(), CreatePlanInput{
		CaseID:    32,
		RunbookID: "rollback_last_rollout",
		Operator:  ActorRef{ID: 5, Name: "eve"},
	})
	if !errors.Is(err, ErrNoRollbackPoint) {
		t.Fatalf("err=%v, want ErrNoRollbackPoint", err)
	}
}

func TestCreatePlanWithRolloutRestartK8sError(t *testing.T) {
	t.Parallel()
	repo := newMemRepo()
	reader := &fakeCaseReader{
		caseCtx: CaseContext{
			CaseID:           33,
			ClusterID:        1,
			PrimaryKind:      "Deployment",
			PrimaryNamespace: "default",
			PrimaryName:      "svc",
		},
		codes: map[string]bool{"deployment.rollout_restart": true},
	}
	fk8s := &fakeKubernetesSource{depErr: errors.New("k8s down")}
	svc := newTestService(t, repo, reader, fk8s)
	_, err := svc.CreatePlan(context.Background(), CreatePlanInput{
		CaseID:    33,
		RunbookID: "rollout_restart_pods",
		Operator:  ActorRef{ID: 6, Name: "frank"},
	})
	if err == nil {
		t.Fatal("expected error for k8s failure")
	}
}

func TestMaterializeParametersDisabled(t *testing.T) {
	t.Parallel()
	svc := &Service{enabled: true, repo: NopRepository{}, k8s: nil, now: func() time.Time { return fixedTime }}
	plan := &ActionPlan{ActionCode: "deployment.rollback", TargetName: "x"}
	err := svc.materializeParameters(context.Background(), plan, nil)
	if !errors.Is(err, ErrDisabled) {
		t.Fatalf("err=%v, want ErrDisabled", err)
	}
}

func TestMaterializeParametersUnsupportedAction(t *testing.T) {
	t.Parallel()
	fk8s := &fakeKubernetesSource{}
	svc := &Service{enabled: true, repo: NopRepository{}, k8s: fk8s, now: func() time.Time { return fixedTime }}
	plan := &ActionPlan{ActionCode: "bogus.action", TargetName: "x"}
	err := svc.materializeParameters(context.Background(), plan, nil)
	if !errors.Is(err, ErrUnsupportedAction) {
		t.Fatalf("err=%v, want ErrUnsupportedAction", err)
	}
}

func TestRefreshSnapshotNilK8s(t *testing.T) {
	t.Parallel()
	svc := &Service{enabled: true, k8s: nil, now: func() time.Time { return fixedTime }}
	plan := &ActionPlan{TargetKind: "Deployment"}
	err := svc.refreshSnapshot(context.Background(), plan)
	if !errors.Is(err, ErrDisabled) {
		t.Fatalf("err=%v, want ErrDisabled", err)
	}
}

func TestRefreshSnapshotUnsupportedKind(t *testing.T) {
	t.Parallel()
	fk8s := &fakeKubernetesSource{}
	svc := &Service{enabled: true, k8s: fk8s, now: func() time.Time { return fixedTime }}
	plan := &ActionPlan{TargetKind: "Service"}
	err := svc.refreshSnapshot(context.Background(), plan)
	if !errors.Is(err, ErrUnsupportedTargetKind) {
		t.Fatalf("err=%v, want ErrUnsupportedTargetKind", err)
	}
}

func TestRefreshSnapshotDeploymentSuccess(t *testing.T) {
	t.Parallel()
	fk8s := &fakeKubernetesSource{
		deployment: k8sgateway.Deployment{
			Metadata: k8sgateway.ObjectMeta{UID: "uid-1", ResourceVersion: "rv-7"},
		},
	}
	svc := &Service{enabled: true, k8s: fk8s, now: func() time.Time { return fixedTime }}
	plan := &ActionPlan{TargetKind: "Deployment", ClusterID: 1, TargetNamespace: "ns", TargetName: "name"}
	err := svc.refreshSnapshot(context.Background(), plan)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if plan.TargetUID != "uid-1" || plan.TargetResourceVersion != "rv-7" {
		t.Fatalf("snapshot = UID=%q RV=%q", plan.TargetUID, plan.TargetResourceVersion)
	}
}

func TestRefreshSnapshotCronJobSuccess(t *testing.T) {
	t.Parallel()
	fk8s := &fakeKubernetesSource{
		cronJob: k8sgateway.CronJob{
			Metadata: k8sgateway.ObjectMeta{UID: "cron-1", ResourceVersion: "cron-rv"},
		},
	}
	svc := &Service{enabled: true, k8s: fk8s, now: func() time.Time { return fixedTime }}
	plan := &ActionPlan{TargetKind: "CronJob", ClusterID: 2, TargetNamespace: "prod", TargetName: "cronjob"}
	err := svc.refreshSnapshot(context.Background(), plan)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if plan.TargetUID != "cron-1" {
		t.Fatalf("TargetUID=%q", plan.TargetUID)
	}
}

func TestPreviewRollbackErrDisabledK8s(t *testing.T) {
	t.Parallel()
	repo := newMemRepo()
	_ = repo.SavePlan(context.Background(), &ActionPlan{
		ID:         "plan-draft",
		Status:     StatusDraft,
		ActionCode: "deployment.rollback",
		TargetKind: "Deployment",
	})
	svc := &Service{enabled: true, repo: repo, k8s: nil, now: func() time.Time { return fixedTime },
		gates: NewGateEvaluator()}
	_, err := svc.Preview(context.Background(), "plan-draft")
	if !errors.Is(err, ErrDisabled) {
		t.Fatalf("err=%v, want ErrDisabled", err)
	}
}

// ============================================================================
// M115-1b: materializeParameters scale/cronjob branches
// ============================================================================

func TestMaterializeParametersScaleBranch(t *testing.T) {
	t.Parallel()
	dep := k8sgateway.Deployment{
		Metadata: k8sgateway.ObjectMeta{UID: "dep-scale", ResourceVersion: "rv-scale"},
	}
	rp := int32(2)
	dep.Spec = struct {
		Replicas *int32 `json:"replicas,omitempty"`
		Selector struct {
			MatchLabels map[string]string `json:"matchLabels,omitempty"`
		} `json:"selector"`
		Template k8sgateway.WorkloadTemplate `json:"template"`
	}{Replicas: &rp}
	fk8s := &fakeKubernetesSource{deployment: dep}
	svc := newTestService(t, NopRepository{}, &fakeCaseReader{}, fk8s)
	plan := &ActionPlan{ActionCode: "deployment.scale", ClusterID: 1, TargetNamespace: "ns", TargetName: "app"}
	desired := int32(4)
	err := svc.materializeParameters(context.Background(), plan, &OperationParameters{DesiredReplicas: &desired})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if plan.BeforeReplicas == nil || *plan.BeforeReplicas != 2 || plan.DesiredReplicas == nil || *plan.DesiredReplicas != 4 {
		t.Fatalf("scale params = before=%v desired=%v", plan.BeforeReplicas, plan.DesiredReplicas)
	}
}

func TestMaterializeParametersScaleNoChange(t *testing.T) {
	t.Parallel()
	rp := int32(3)
	dep := k8sgateway.Deployment{Metadata: k8sgateway.ObjectMeta{UID: "u"}}
	dep.Spec = struct {
		Replicas *int32 `json:"replicas,omitempty"`
		Selector struct {
			MatchLabels map[string]string `json:"matchLabels,omitempty"`
		} `json:"selector"`
		Template k8sgateway.WorkloadTemplate `json:"template"`
	}{Replicas: &rp}
	svc := newTestService(t, NopRepository{}, &fakeCaseReader{}, &fakeKubernetesSource{deployment: dep})
	plan := &ActionPlan{ActionCode: "deployment.scale", ClusterID: 1, TargetNamespace: "ns", TargetName: "app"}
	desired := int32(3)
	err := svc.materializeParameters(context.Background(), plan, &OperationParameters{DesiredReplicas: &desired})
	if !errors.Is(err, ErrOperationNoChange) {
		t.Fatalf("err=%v, want ErrOperationNoChange", err)
	}
}

func TestMaterializeParametersScaleMissingOverride(t *testing.T) {
	t.Parallel()
	svc := newTestService(t, NopRepository{}, &fakeCaseReader{}, &fakeKubernetesSource{})
	plan := &ActionPlan{ActionCode: "deployment.scale", ClusterID: 1, TargetNamespace: "ns", TargetName: "app"}
	err := svc.materializeParameters(context.Background(), plan, nil)
	if !errors.Is(err, ErrInvalidOperation) {
		t.Fatalf("err=%v, want ErrInvalidOperation", err)
	}
}

func TestMaterializeParametersCronJobSuspend(t *testing.T) {
	t.Parallel()
	dep := k8sgateway.CronJob{
		Metadata: k8sgateway.ObjectMeta{UID: "cron-u", ResourceVersion: "cron-rv"},
	}
	s := true
	dep.Spec = struct {
		Schedule                   string `json:"schedule"`
		TimeZone                   string `json:"timeZone,omitempty"`
		ConcurrencyPolicy          string `json:"concurrencyPolicy,omitempty"`
		Suspend                    *bool  `json:"suspend,omitempty"`
		SuccessfulJobsHistoryLimit *int32 `json:"successfulJobsHistoryLimit,omitempty"`
		FailedJobsHistoryLimit     *int32 `json:"failedJobsHistoryLimit,omitempty"`
		JobTemplate                struct {
			Spec k8sgateway.JobSpec `json:"spec"`
		} `json:"jobTemplate"`
	}{Suspend: &s}
	svc := newTestService(t, NopRepository{}, &fakeCaseReader{}, &fakeKubernetesSource{cronJob: dep})
	plan := &ActionPlan{ActionCode: "cronjob.suspend", ClusterID: 1, TargetNamespace: "ns", TargetName: "cronjob"}
	if err := svc.materializeParameters(context.Background(), plan, nil); err == nil {
		t.Fatal("expected ErrOperationNoChange for already-suspended cronjob")
	}
}

// ============================================================================
// M115-1b: Preview happy path (stateful repo transitions draft -> previewed)
// ============================================================================

// previewCapableRepo extends memRepo with MarkPreviewed so Preview can
// transition a draft plan to previewed.
type previewCapableRepo struct {
	*memRepo
	previewed *ActionPlan
}

func (r *previewCapableRepo) MarkPreviewed(_ context.Context, id string, gates []PolicyGate, now time.Time) (ActionPlan, error) {
	plan, ok := r.plans[id]
	if !ok {
		return ActionPlan{}, ErrPlanNotFound
	}
	plan.Status = StatusPreviewed
	plan.PolicyGates = gates
	r.plans[id] = plan
	r.previewed = &plan
	return plan, nil
}

func TestPreviewSuccessTransitionsToPreviewed(t *testing.T) {
	t.Parallel()
	repo := &previewCapableRepo{memRepo: newMemRepo()}
	dep := k8sgateway.Deployment{
		Metadata: k8sgateway.ObjectMeta{UID: "dep-prev", ResourceVersion: "rv-prev"},
	}
	dep.Spec = struct {
		Replicas *int32 `json:"replicas,omitempty"`
		Selector struct {
			MatchLabels map[string]string `json:"matchLabels,omitempty"`
		} `json:"selector"`
		Template k8sgateway.WorkloadTemplate `json:"template"`
	}{}
	// MarkPreviewed must be implemented on memRepo
	_ = repo.SavePlan(context.Background(), &ActionPlan{
		ID:         "plan-preview-target",
		Status:     StatusDraft,
		ActionCode: "deployment.rollout_restart",
		TargetKind: "Deployment",
		ClusterID:  1, TargetNamespace: "ns", TargetName: "app",
	})
	svc := newTestService(t, repo, &fakeCaseReader{}, &fakeKubernetesSource{deployment: dep})
	plan, err := svc.Preview(context.Background(), "plan-preview-target")
	if err != nil {
		t.Fatalf("Preview err=%v", err)
	}
	if plan.Status != StatusPreviewed {
		t.Fatalf("Status=%q, want previewed", plan.Status)
	}
	if repo.previewed == nil || repo.previewed.Status != StatusPreviewed {
		t.Fatal("MarkPreviewed not persisted to previewed status")
	}
}

// ============================================================================
// M115-1b: Execute + Verify happy paths (stateful repo)
// ============================================================================

// execRepo is a stateful in-memory Repository implementing the full
// execute/verify cycle: Claim/Complete/Fail/SaveVerification/
// GetVerificationByPlan/UpdateVerification/MarkVerified.
type execRepo struct {
	*memRepo
	verifications map[int64]ActionVerification
	nextVerID     int64
	verified      map[string]int64 // planID -> verificationID
	claimErr      error
}

func newExecRepo() *execRepo {
	return &execRepo{
		memRepo:       newMemRepo(),
		verifications: make(map[int64]ActionVerification),
		verified:      make(map[string]int64),
		nextVerID:     1,
	}
}

func (r *execRepo) Claim(_ context.Context, id string, tokenHash []byte, idempotencyKey string, now, staleBefore time.Time) (ActionPlan, bool, error) {
	if r.claimErr != nil {
		return ActionPlan{}, false, r.claimErr
	}
	plan, ok := r.plans[id]
	if !ok {
		return ActionPlan{}, false, ErrPlanNotFound
	}
	// Constant-time token comparison.
	if !bytesEqual(plan.ConfirmationTokenHash[:], tokenHash) {
		return plan, false, ErrConfirmationInvalid
	}
	if plan.Status == StatusApproved {
		plan.Status = StatusExecuting
		plan.IdempotencyKey = idempotencyKey
		locked := now
		plan.LockedAt = &locked
		r.plans[id] = plan
		return plan, true, nil
	}
	if plan.Status == StatusSucceeded || plan.Status == StatusFailed {
		if plan.IdempotencyKey != idempotencyKey {
			return plan, false, ErrAlreadyExecuted
		}
		return plan, false, nil // idempotent replay
	}
	return plan, false, ErrAlreadyExecuted
}

func (r *execRepo) Complete(_ context.Context, id, idempotencyKey string, executedAt time.Time) (ActionPlan, error) {
	plan, ok := r.plans[id]
	if !ok || plan.Status != StatusExecuting {
		return ActionPlan{}, ErrPlanNotFound
	}
	plan.Status = StatusSucceeded
	plan.IdempotencyKey = idempotencyKey
	plan.LockedAt = nil
	r.plans[id] = plan
	return plan, nil
}

func (r *execRepo) Fail(_ context.Context, id, idempotencyKey, message string) (ActionPlan, error) {
	plan, ok := r.plans[id]
	if !ok || plan.Status != StatusExecuting {
		return ActionPlan{}, ErrPlanNotFound
	}
	plan.Status = StatusFailed
	plan.IdempotencyKey = idempotencyKey
	plan.LockedAt = nil
	plan.LastError = message
	r.plans[id] = plan
	return plan, nil
}

func (r *execRepo) SaveVerification(_ context.Context, verification *ActionVerification) error {
	verification.ID = r.nextVerID
	r.nextVerID++
	r.verifications[verification.ID] = *verification
	return nil
}

func (r *execRepo) GetVerificationByPlan(_ context.Context, planID string) (ActionVerification, error) {
	for _, v := range r.verifications {
		if v.PlanID == planID {
			return v, nil
		}
	}
	return ActionVerification{}, ErrVerificationNotFound
}

func (r *execRepo) UpdateVerification(_ context.Context, id int64, update VerificationUpdate) (ActionVerification, error) {
	v, ok := r.verifications[id]
	if !ok {
		return ActionVerification{}, ErrVerificationNotFound
	}
	if update.Status != "" {
		v.Status = update.Status
	}
	if update.EvidenceComparison != "" {
		v.EvidenceComparison = update.EvidenceComparison
	}
	if update.PostSnapshot != nil {
		v.PostSnapshot = *update.PostSnapshot
	}
	if update.MissingEvidence != nil {
		v.MissingEvidence = *update.MissingEvidence
	}
	if update.VerifiedAt != nil {
		v.VerifiedAt = update.VerifiedAt
	}
	if update.Reason != "" {
		v.Reason = update.Reason
	}
	if update.RollbackTriggered != nil {
		v.RollbackTriggered = *update.RollbackTriggered
	}
	if update.RollbackPlanID != nil {
		v.RollbackPlanID = update.RollbackPlanID
	}
	r.verifications[id] = v
	return v, nil
}

func (r *execRepo) MarkVerified(_ context.Context, planID string, verificationID int64, now time.Time) (ActionPlan, error) {
	plan, ok := r.plans[planID]
	if !ok {
		return ActionPlan{}, ErrPlanNotFound
	}
	r.verified[planID] = verificationID
	return plan, nil
}

func (r *execRepo) GetVerification(_ context.Context, id int64) (ActionVerification, error) {
	v, ok := r.verifications[id]
	if !ok {
		return ActionVerification{}, ErrVerificationNotFound
	}
	return v, nil
}

func TestExecuteHappyPath(t *testing.T) {
	t.Parallel()
	repo := newExecRepo()
	token := "confirmation-token-123"
	tokenHash := sha256.Sum256([]byte(token))
	dep := k8sgateway.Deployment{
		Metadata: k8sgateway.ObjectMeta{UID: "dep-exec", ResourceVersion: "rv-exec"},
	}
	dep.Spec = struct {
		Replicas *int32 `json:"replicas,omitempty"`
		Selector struct {
			MatchLabels map[string]string `json:"matchLabels,omitempty"`
		} `json:"selector"`
		Template k8sgateway.WorkloadTemplate `json:"template"`
	}{}
	now := fixedTime
	plan := ActionPlan{
		ID:                    "plan-exec",
		Status:                StatusApproved,
		ActionCode:            "deployment.rollout_restart",
		TargetKind:            "Deployment",
		ClusterID:             1,
		TargetNamespace:       "ns",
		TargetName:            "app",
		TargetUID:             "dep-exec",
		TargetResourceVersion: "rv-exec",
		ConfirmationTokenHash: tokenHash[:],
		ExpiresAt:             now.Add(time.Hour),
	}
	_ = repo.SavePlan(context.Background(), &plan)

	provider := &fakeEvidenceProvider{
		pre: validSnapshot(map[string]any{"replicas": int32(3)}, nil),
	}
	svc := NewService(repo, &fakeCaseReader{}, &fakeKubernetesSource{deployment: dep},
		WithNow(func() time.Time { return now }),
		WithEvidenceProvider(provider),
		WithCooldown(120*time.Second))

	completed, err := svc.Execute(context.Background(), "plan-exec", token, "idempotency-key-123456")
	if err != nil {
		t.Fatalf("Execute err=%v", err)
	}
	if completed.Status != StatusSucceeded {
		t.Fatalf("Status=%q, want succeeded", completed.Status)
	}
	// scheduleVerification must have persisted a pending verification.
	verification, err := repo.GetVerificationByPlan(context.Background(), "plan-exec")
	if err != nil {
		t.Fatalf("GetVerificationByPlan err=%v", err)
	}
	if verification.Status != VerificationStatusPending {
		t.Fatalf("verification.Status=%q, want pending", verification.Status)
	}
}

func TestExecutePatchErrorFailsAndSchedulesVerification(t *testing.T) {
	t.Parallel()
	repo := newExecRepo()
	token := "confirmation-token-456"
	tokenHash := sha256.Sum256([]byte(token))
	now := fixedTime
	plan := ActionPlan{
		ID:                    "plan-fail",
		Status:                StatusApproved,
		ActionCode:            "deployment.rollout_restart",
		TargetKind:            "Deployment",
		ClusterID:             1,
		TargetNamespace:       "ns",
		TargetName:            "app",
		TargetUID:             "dep-exec",
		TargetResourceVersion: "rv-exec",
		ConfirmationTokenHash: tokenHash[:],
		ExpiresAt:             now.Add(time.Hour),
	}
	_ = repo.SavePlan(context.Background(), &plan)
	provider := &fakeEvidenceProvider{pre: validSnapshot(map[string]any{}, nil)}
	svc := NewService(repo, &fakeCaseReader{},
		&fakeKubernetesSource{deployment: k8sgateway.Deployment{Metadata: k8sgateway.ObjectMeta{UID: "dep-exec", ResourceVersion: "rv-exec"}}, patchDepErr: errors.New("k8s patch failed")},
		WithNow(func() time.Time { return now }),
		WithEvidenceProvider(provider),
		WithCooldown(120*time.Second))
	failed, err := svc.Execute(context.Background(), "plan-fail", token, "idempotency-key-abcdef")
	if err == nil {
		t.Fatalf("expected Execute error, got plan=%+v", failed)
	}
	if failed.Status != StatusFailed {
		t.Fatalf("Status=%q, want failed", failed.Status)
	}
	// Verification still scheduled for failed executions.
	verification, err := repo.GetVerificationByPlan(context.Background(), "plan-fail")
	if err != nil {
		t.Fatalf("GetVerificationByPlan err=%v", err)
	}
	if verification.Status != VerificationStatusPending {
		t.Fatalf("verification.Status=%q, want pending", verification.Status)
	}
}

func TestExecuteGateRecheckFailure(t *testing.T) {
	t.Parallel()
	repo := newExecRepo()
	token := "confirmation-token-789"
	tokenHash := sha256.Sum256([]byte(token))
	now := fixedTime
	plan := ActionPlan{
		ID:                    "plan-gate",
		Status:                StatusApproved,
		ActionCode:            "deployment.rollout_restart",
		TargetKind:            "Deployment",
		ClusterID:             1,
		TargetNamespace:       "ns",
		TargetName:            "app",
		TargetUID:             "dep-exec",
		TargetResourceVersion: "rv-exec",
		ConfirmationTokenHash: tokenHash[:],
		ExpiresAt:             now.Add(time.Hour),
	}
	_ = repo.SavePlan(context.Background(), &plan)
	// No k8s source: the uid_rv_recheck gate sees CurrentSnapshot empty
	// and should fail closed (nil CurrentSnapshot).
	svc := NewService(repo, &fakeCaseReader{}, nil,
		WithNow(func() time.Time { return now }))
	_, err := svc.Execute(context.Background(), "plan-gate", token, "idempotency-key-ghijkl")
	if err == nil {
		t.Fatal("expected gate recheck error, got nil")
	}
	stored, _ := repo.GetPlan(context.Background(), "plan-gate")
	if stored.Status != StatusFailed {
		t.Fatalf("stored.Status=%q, want failed after gate rejection", stored.Status)
	}
}

func TestExecuteWrongConfirmationToken(t *testing.T) {
	t.Parallel()
	repo := newExecRepo()
	token := "correct-token-111"
	tokenHash := sha256.Sum256([]byte(token))
	now := fixedTime
	plan := ActionPlan{
		ID:                    "plan-token",
		Status:                StatusApproved,
		ActionCode:            "deployment.rollout_restart",
		TargetKind:            "Deployment",
		ClusterID:             1,
		TargetNamespace:       "ns",
		TargetName:            "app",
		TargetUID:             "dep-exec",
		TargetResourceVersion: "rv-exec",
		ConfirmationTokenHash: tokenHash[:],
		ExpiresAt:             now.Add(time.Hour),
	}
	_ = repo.SavePlan(context.Background(), &plan)
	svc := NewService(repo, &fakeCaseReader{}, &fakeKubernetesSource{},
		WithNow(func() time.Time { return now }))
	_, err := svc.Execute(context.Background(), "plan-token", "wrong-token", "idempotency-key-123456")
	if !errors.Is(err, ErrConfirmationInvalid) {
		t.Fatalf("err=%v, want ErrConfirmationInvalid", err)
	}
}

func TestVerifyHappyPathEffective(t *testing.T) {
	t.Parallel()
	repo := newExecRepo()
	now := fixedTime
	plan := ActionPlan{
		ID:                    "plan-verify",
		Status:                StatusSucceeded,
		ActionCode:            "deployment.rollout_restart",
		TargetKind:            "Deployment",
		ClusterID:             1,
		TargetNamespace:       "ns",
		TargetName:            "app",
		TargetUID:             "dep-exec",
		TargetResourceVersion: "rv-exec",
	}
	_ = repo.SavePlan(context.Background(), &plan)
	pre := validSnapshot(map[string]any{"replicas": int32(2), "available_replicas": int32(2)}, sloSnapshot("burning_fast"))
	post := validSnapshot(map[string]any{"replicas": int32(3), "available_replicas": int32(3), "restarted_at": "2026-08-14T10:00:00Z"}, sloSnapshot("healthy"))
	provider := &fakeEvidenceProvider{pre: pre, post: post}
	verifier := NewVerifier(WithVerifierProvider(provider), WithVerifierNow(func() time.Time { return now }), WithVerifierCooldown(120*time.Second))
	verification, _ := verifier.CreateVerification(context.Background(), plan)
	verification.Status = VerificationStatusPending
	_ = repo.SaveVerification(context.Background(), &verification)

	svc := NewService(repo, &fakeCaseReader{}, &fakeKubernetesSource{},
		WithNow(func() time.Time { return now }),
		WithEvidenceProvider(provider),
		WithCooldown(120*time.Second))
	evaluated, err := svc.Verify(context.Background(), "plan-verify")
	if err != nil {
		t.Fatalf("Verify err=%v", err)
	}
	if evaluated.Status != VerificationStatusEffective {
		t.Fatalf("Status=%q, want effective", evaluated.Status)
	}
	if evaluated.VerifiedAt == nil {
		t.Fatal("VerifiedAt is nil, want set")
	}
	if _, ok := repo.verified["plan-verify"]; !ok {
		t.Fatal("MarkVerified not called")
	}
}

func TestScheduleVerificationNilVerifier(t *testing.T) {
	t.Parallel()
	svc := &Service{enabled: true, verifier: nil, now: func() time.Time { return fixedTime }}
	if err := svc.scheduleVerification(context.Background(), ActionPlan{ID: "p"}); err != nil {
		t.Fatalf("scheduleVerification err=%v", err)
	}
}

func TestNopCaseReaderMethods(t *testing.T) {
	reader := NopCaseReader{}
	if _, err := reader.GetCase(context.Background(), 1); err != ErrCaseNotFound {
		t.Fatalf("GetCase err = %v, want ErrCaseNotFound", err)
	}
	codes, err := reader.EligibleActionCodes(context.Background(), 1)
	if err != nil || codes != nil {
		t.Fatalf("EligibleActionCodes = %v, %v", codes, err)
	}
}

func TestCreatePlanRejectsWhenEligibleCodesError(t *testing.T) {
	t.Parallel()
	reader := &fakeCaseReader{
		caseCtx:  CaseContext{CaseID: 1, ClusterID: 7, PrimaryKind: "Deployment", PrimaryNamespace: "default", PrimaryName: "api", PrimaryUID: "uid-1"},
		codesErr: errors.New("codes unavailable"),
	}
	svc := newTestService(t, NopRepository{}, reader, nil)
	_, err := svc.CreatePlan(context.Background(), CreatePlanInput{
		RunbookID: "rollout_restart_pods",
		Operator:  ActorRef{ID: 1, Name: "alice"},
	})
	if err == nil || err.Error() != "codes unavailable" {
		t.Fatalf("err = %v, want codes error", err)
	}
}

func TestCreatePlanDisabledWhenNilRepo(t *testing.T) {
	t.Parallel()
	svc := NewService(nil, nil, nil)
	if _, err := svc.CreatePlan(context.Background(), CreatePlanInput{}); !errors.Is(err, ErrDisabled) {
		t.Fatalf("err = %v, want ErrDisabled", err)
	}
}
