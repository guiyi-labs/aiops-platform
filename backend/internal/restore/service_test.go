package restore

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"k8s-aiops.local/backend/internal/apiquery"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

// --- stubs ---

type kubernetesStub struct {
	mu sync.Mutex

	capInstalled bool
	capErr       error

	backup          k8sgateway.VeleroBackup
	backupErr       error
	backupCalls     int
	backupExists    bool
	backupExistsErr error

	namespaceExists    bool
	namespaceExistsErr error

	restoreExists    bool
	restoreExistsErr error

	restore          k8sgateway.VeleroRestore
	restoreErr       error
	restoreCallCount int

	createCalled  int
	createPaths   []string
	createBodies  [][]byte
	createDryRuns []bool
	createResp    []byte
	createErr     error
	// createErrOnCall, if non-nil, returns the indexed error on the Nth create call (1-based).
	createErrOnCall map[int]error

	deployments     []k8sgateway.Deployment
	statefulSets    []k8sgateway.StatefulSet
	daemonSets      []k8sgateway.DaemonSet
	cronJobs        []k8sgateway.CronJob
	configMaps      []k8sgateway.ConfigMap
	secrets         []k8sgateway.Secret
	serviceAccounts []k8sgateway.ServiceAccount
	listErr         error
}

func (s *kubernetesStub) VeleroCapability(context.Context, int64) (k8sgateway.VeleroCapability, error) {
	return k8sgateway.VeleroCapability{Installed: s.capInstalled}, s.capErr
}

func (s *kubernetesStub) Backup(context.Context, int64, string, string) (k8sgateway.VeleroBackup, error) {
	s.mu.Lock()
	s.backupCalls++
	s.mu.Unlock()
	return s.backup, s.backupErr
}

func (s *kubernetesStub) NamespaceExists(context.Context, int64, string) (bool, error) {
	return s.namespaceExists, s.namespaceExistsErr
}

func (s *kubernetesStub) VeleroRestoreExists(context.Context, int64, string, string) (bool, error) {
	return s.restoreExists, s.restoreExistsErr
}

func (s *kubernetesStub) VeleroRestore(context.Context, int64, string, string) (k8sgateway.VeleroRestore, error) {
	s.mu.Lock()
	s.restoreCallCount++
	s.mu.Unlock()
	return s.restore, s.restoreErr
}

func (s *kubernetesStub) CreateResource(_ context.Context, _ int64, path string, body []byte, dryRun bool) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.createCalled++
	s.createPaths = append(s.createPaths, path)
	s.createBodies = append(s.createBodies, append([]byte(nil), body...))
	s.createDryRuns = append(s.createDryRuns, dryRun)
	if s.createErrOnCall != nil {
		if err, ok := s.createErrOnCall[s.createCalled]; ok {
			return nil, err
		}
	}
	if s.createErr != nil {
		return nil, s.createErr
	}
	return s.createResp, nil
}

func (s *kubernetesStub) Deployments(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Deployment], error) {
	return apiquery.ListResponse[k8sgateway.Deployment]{Items: s.deployments}, s.listErr
}

func (s *kubernetesStub) StatefulSets(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.StatefulSet], error) {
	return apiquery.ListResponse[k8sgateway.StatefulSet]{Items: s.statefulSets}, s.listErr
}

func (s *kubernetesStub) DaemonSets(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.DaemonSet], error) {
	return apiquery.ListResponse[k8sgateway.DaemonSet]{Items: s.daemonSets}, s.listErr
}

func (s *kubernetesStub) CronJobs(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.CronJob], error) {
	return apiquery.ListResponse[k8sgateway.CronJob]{Items: s.cronJobs}, s.listErr
}

func (s *kubernetesStub) ConfigMaps(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.ConfigMap], error) {
	return apiquery.ListResponse[k8sgateway.ConfigMap]{Items: s.configMaps}, s.listErr
}

func (s *kubernetesStub) Secrets(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Secret], error) {
	return apiquery.ListResponse[k8sgateway.Secret]{Items: s.secrets}, s.listErr
}

func (s *kubernetesStub) ServiceAccounts(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.ServiceAccount], error) {
	return apiquery.ListResponse[k8sgateway.ServiceAccount]{Items: s.serviceAccounts}, s.listErr
}

type repositoryStub struct {
	saved         *Plan
	savedErr      error
	listPlans     []Plan
	listErr       error
	claimed       Plan
	shouldExecute bool
	claimErr      error
	completed     bool
	completedPlan Plan
	completeErr   error
	failedMessage string
	failedResult  *ExecutionResultJSON
	failErr       error
	activePlan    Plan
	activeFound   bool
	activeErr     error
}

func (r *repositoryStub) Save(_ context.Context, plan *Plan) error {
	if r.savedErr != nil {
		return r.savedErr
	}
	saved := *plan
	r.saved = &saved
	return nil
}

func (r *repositoryStub) List(context.Context, int64) ([]Plan, error) {
	return r.listPlans, r.listErr
}

func (r *repositoryStub) Claim(context.Context, string, []byte, string, time.Time, time.Time) (Plan, bool, error) {
	return r.claimed, r.shouldExecute, r.claimErr
}

func (r *repositoryStub) Complete(_ context.Context, _, _ string, _ time.Time, plan Plan, result *ExecutionResultJSON) (Plan, error) {
	if r.completeErr != nil {
		return Plan{}, r.completeErr
	}
	r.completed = true
	if result != nil {
		plan.ExecutionResult = result
	}
	plan.Status = StatusSucceeded
	r.completedPlan = plan
	return plan, nil
}

func (r *repositoryStub) Fail(_ context.Context, _, _, message string, result *ExecutionResultJSON) (Plan, error) {
	r.failedMessage = message
	r.failedResult = result
	r.claimed.Status = StatusFailed
	r.claimed.LastError = message
	if result != nil {
		r.claimed.ExecutionResult = result
	}
	return r.claimed, r.failErr
}

func (r *repositoryStub) ActiveBySource(context.Context, int64, string, string) (Plan, bool, error) {
	return r.activePlan, r.activeFound, r.activeErr
}

func makeService(kube *kubernetesStub, repo *repositoryStub) *Service {
	svc := NewService(kube, repo)
	svc.now = func() time.Time { return time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC) }
	svc.pollAttempts = 2
	svc.pollInterval = time.Millisecond
	return svc
}

func completedBackup() k8sgateway.VeleroBackup {
	return k8sgateway.VeleroBackup{
		Name:               "weekly-backup",
		Namespace:          "velero",
		Phase:              PhaseCompleted,
		IncludedNamespaces: []string{"app"},
	}
}

func terminalRestore(phase string) k8sgateway.VeleroRestore {
	return k8sgateway.VeleroRestore{
		Name:      "rehearse-restore-weekly-backup-7",
		Namespace: "velero",
		Phase:     phase,
	}
}

// --- validateRequest tests ---

func TestValidateRequest_InvalidClusterID(t *testing.T) {
	if err := validateRequest(0, Request{SourceBackupName: "b", SourceBackupNamespace: "ns"}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("want ErrInvalidRequest, got %v", err)
	}
}

func TestValidateRequest_EmptyBackupName(t *testing.T) {
	if err := validateRequest(1, Request{SourceBackupName: "", SourceBackupNamespace: "ns"}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("want ErrInvalidRequest, got %v", err)
	}
}

func TestValidateRequest_EmptyBackupNamespace(t *testing.T) {
	if err := validateRequest(1, Request{SourceBackupName: "b", SourceBackupNamespace: ""}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("want ErrInvalidRequest, got %v", err)
	}
}

func TestValidateRequest_TooLongBackupName(t *testing.T) {
	if err := validateRequest(1, Request{SourceBackupName: strings.Repeat("a", 254), SourceBackupNamespace: "ns"}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("want ErrInvalidRequest, got %v", err)
	}
}

func TestValidateRequest_Success(t *testing.T) {
	if err := validateRequest(1, Request{SourceBackupName: "weekly-backup", SourceBackupNamespace: "velero"}); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

// --- Preview tests ---

func TestPreview_InvalidRequest(t *testing.T) {
	svc := makeService(&kubernetesStub{}, &repositoryStub{})
	_, err := svc.Preview(context.Background(), 0, Request{SourceBackupName: "b", SourceBackupNamespace: "ns"}, ActorRef{ID: 1, Name: "alice"})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("want ErrInvalidRequest, got %v", err)
	}
}

func TestPreview_VeleroNotInstalled(t *testing.T) {
	kube := &kubernetesStub{capInstalled: false}
	svc := makeService(kube, &repositoryStub{})
	_, err := svc.Preview(context.Background(), 1, Request{SourceBackupName: "b", SourceBackupNamespace: "ns"}, ActorRef{ID: 1, Name: "alice"})
	if !errors.Is(err, ErrVeleroNotInstalled) {
		t.Fatalf("want ErrVeleroNotInstalled, got %v", err)
	}
}

func TestPreview_VeleroCapabilityError(t *testing.T) {
	kube := &kubernetesStub{capErr: errors.New("api down")}
	svc := makeService(kube, &repositoryStub{})
	_, err := svc.Preview(context.Background(), 1, Request{SourceBackupName: "b", SourceBackupNamespace: "ns"}, ActorRef{ID: 1, Name: "alice"})
	if err == nil {
		t.Fatal("want error from velero capability, got nil")
	}
}

func TestPreview_SourceBackupNotFound(t *testing.T) {
	kube := &kubernetesStub{
		capInstalled: true,
		backupErr:    k8sgateway.ErrResourceNotFound,
	}
	svc := makeService(kube, &repositoryStub{})
	_, err := svc.Preview(context.Background(), 1, Request{SourceBackupName: "b", SourceBackupNamespace: "ns"}, ActorRef{ID: 1, Name: "alice"})
	if !errors.Is(err, ErrSourceBackupNotFound) {
		t.Fatalf("want ErrSourceBackupNotFound, got %v", err)
	}
}

func TestPreview_SourceBackupIncomplete(t *testing.T) {
	kube := &kubernetesStub{
		capInstalled: true,
		backup:       k8sgateway.VeleroBackup{Phase: PhaseInProgress, IncludedNamespaces: []string{"app"}},
	}
	svc := makeService(kube, &repositoryStub{})
	_, err := svc.Preview(context.Background(), 1, Request{SourceBackupName: "b", SourceBackupNamespace: "ns"}, ActorRef{ID: 1, Name: "alice"})
	if !errors.Is(err, ErrSourceBackupIncomplete) {
		t.Fatalf("want ErrSourceBackupIncomplete, got %v", err)
	}
}

func TestPreview_SourceBackupScope(t *testing.T) {
	kube := &kubernetesStub{
		capInstalled: true,
		backup:       k8sgateway.VeleroBackup{Phase: PhaseCompleted, IncludedNamespaces: []string{"app", "other"}},
	}
	svc := makeService(kube, &repositoryStub{})
	_, err := svc.Preview(context.Background(), 1, Request{SourceBackupName: "b", SourceBackupNamespace: "ns"}, ActorRef{ID: 1, Name: "alice"})
	if !errors.Is(err, ErrSourceBackupScope) {
		t.Fatalf("want ErrSourceBackupScope, got %v", err)
	}
}

func TestPreview_DestinationExists(t *testing.T) {
	kube := &kubernetesStub{
		capInstalled:    true,
		backup:          completedBackup(),
		namespaceExists: true,
	}
	svc := makeService(kube, &repositoryStub{})
	_, err := svc.Preview(context.Background(), 1, Request{SourceBackupName: "weekly-backup", SourceBackupNamespace: "velero"}, ActorRef{ID: 1, Name: "alice"})
	if !errors.Is(err, ErrDestinationExists) {
		t.Fatalf("want ErrDestinationExists, got %v", err)
	}
}

func TestPreview_DestinationCollision(t *testing.T) {
	kube := &kubernetesStub{
		capInstalled:    true,
		backup:          completedBackup(),
		namespaceExists: false,
	}
	repo := &repositoryStub{activeFound: true}
	svc := makeService(kube, repo)
	_, err := svc.Preview(context.Background(), 1, Request{SourceBackupName: "weekly-backup", SourceBackupNamespace: "velero"}, ActorRef{ID: 1, Name: "alice"})
	if !errors.Is(err, ErrDestinationCollision) {
		t.Fatalf("want ErrDestinationCollision, got %v", err)
	}
}

func TestPreview_RestoreNameConflict(t *testing.T) {
	kube := &kubernetesStub{
		capInstalled:  true,
		backup:        completedBackup(),
		restoreExists: true,
	}
	svc := makeService(kube, &repositoryStub{})
	_, err := svc.Preview(context.Background(), 1, Request{SourceBackupName: "weekly-backup", SourceBackupNamespace: "velero"}, ActorRef{ID: 1, Name: "alice"})
	if !errors.Is(err, ErrRestoreNameConflict) {
		t.Fatalf("want ErrRestoreNameConflict, got %v", err)
	}
}

func TestPreview_QuarantineDryRunFailed(t *testing.T) {
	kube := &kubernetesStub{
		capInstalled: true,
		backup:       completedBackup(),
		createErr:    errors.New("forbidden"),
	}
	svc := makeService(kube, &repositoryStub{})
	_, err := svc.Preview(context.Background(), 1, Request{SourceBackupName: "weekly-backup", SourceBackupNamespace: "velero"}, ActorRef{ID: 1, Name: "alice"})
	if !errors.Is(err, ErrQuarantineDryRunFailed) {
		t.Fatalf("want ErrQuarantineDryRunFailed, got %v", err)
	}
}

func TestPreview_RestoreDryRunFailed(t *testing.T) {
	kube := &kubernetesStub{
		capInstalled: true,
		backup:       completedBackup(),
		// First two creates (quarantine NP + RQ) succeed; third (restore) fails.
		createErrOnCall: map[int]error{3: errors.New("restore forbidden")},
	}
	svc := makeService(kube, &repositoryStub{})
	_, err := svc.Preview(context.Background(), 1, Request{SourceBackupName: "weekly-backup", SourceBackupNamespace: "velero"}, ActorRef{ID: 1, Name: "alice"})
	if !errors.Is(err, ErrRestoreDryRunFailed) {
		t.Fatalf("want ErrRestoreDryRunFailed, got %v", err)
	}
}

func TestPreview_Success(t *testing.T) {
	kube := &kubernetesStub{
		capInstalled: true,
		backup:       completedBackup(),
		createResp:   []byte(`{"metadata":{"uid":"ns-uid-1"}}`),
	}
	repo := &repositoryStub{}
	svc := makeService(kube, repo)
	plan, err := svc.Preview(context.Background(), 1, Request{SourceBackupName: "weekly-backup", SourceBackupNamespace: "velero"}, ActorRef{ID: 7, Name: "alice"})
	if err != nil {
		t.Fatalf("want nil, got %v", err)
	}
	if plan.Status != StatusAwaitingConfirmation {
		t.Errorf("Status = %q", plan.Status)
	}
	if plan.SourceBackupName != "weekly-backup" {
		t.Errorf("SourceBackupName = %q", plan.SourceBackupName)
	}
	if plan.DestinationNamespace == "" {
		t.Error("DestinationNamespace should be generated")
	}
	if plan.VeleroRestoreName == "" {
		t.Error("VeleroRestoreName should be generated")
	}
	if plan.ConfirmationToken == "" {
		t.Error("confirmation token should be populated for caller")
	}
	if !plan.QuarantineStatus.DryRunValidated {
		t.Error("DryRunValidated should be true")
	}
	// Three dry-runs: NP, RQ, Restore.
	if len(kube.createDryRuns) != 3 {
		t.Fatalf("expected 3 dry-run creates, got %d", len(kube.createDryRuns))
	}
	for i, dry := range kube.createDryRuns {
		if !dry {
			t.Errorf("create %d should be dry-run", i)
		}
	}
	if repo.saved == nil {
		t.Fatal("plan not saved")
	}
	if repo.saved.ConfirmationToken != "" {
		t.Error("persisted plan should not retain confirmation token")
	}
	if len(repo.saved.ConfirmationTokenHash) == 0 {
		t.Error("confirmation token hash should be persisted")
	}
}

func TestPreview_SaveError(t *testing.T) {
	kube := &kubernetesStub{
		capInstalled: true,
		backup:       completedBackup(),
	}
	repo := &repositoryStub{savedErr: errors.New("db down")}
	svc := makeService(kube, repo)
	_, err := svc.Preview(context.Background(), 1, Request{SourceBackupName: "weekly-backup", SourceBackupNamespace: "velero"}, ActorRef{ID: 1, Name: "alice"})
	if err == nil {
		t.Fatal("want error from save, got nil")
	}
}

// --- Execute validation tests ---

func TestExecute_EmptyID(t *testing.T) {
	svc := makeService(&kubernetesStub{}, &repositoryStub{})
	_, err := svc.Execute(context.Background(), "", "token", "idem-key1")
	if !errors.Is(err, ErrConfirmationInvalid) {
		t.Fatalf("want ErrConfirmationInvalid, got %v", err)
	}
}

func TestExecute_EmptyToken(t *testing.T) {
	svc := makeService(&kubernetesStub{}, &repositoryStub{})
	_, err := svc.Execute(context.Background(), "plan-1", "", "idem-key1")
	if !errors.Is(err, ErrConfirmationInvalid) {
		t.Fatalf("want ErrConfirmationInvalid, got %v", err)
	}
}

func TestExecute_ShortIdempotencyKey(t *testing.T) {
	svc := makeService(&kubernetesStub{}, &repositoryStub{})
	_, err := svc.Execute(context.Background(), "plan-1", "token", "short")
	if !errors.Is(err, ErrInvalidIdempotency) {
		t.Fatalf("want ErrInvalidIdempotency, got %v", err)
	}
}

func TestExecute_ClaimNotFound(t *testing.T) {
	repo := &repositoryStub{claimErr: ErrNotFound}
	svc := makeService(&kubernetesStub{}, repo)
	_, err := svc.Execute(context.Background(), "plan-1", "token", "idem-key1")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestExecute_ClaimConfirmationInvalid(t *testing.T) {
	repo := &repositoryStub{claimErr: ErrConfirmationInvalid}
	svc := makeService(&kubernetesStub{}, repo)
	_, err := svc.Execute(context.Background(), "plan-1", "token", "idem-key1")
	if !errors.Is(err, ErrConfirmationInvalid) {
		t.Fatalf("want ErrConfirmationInvalid, got %v", err)
	}
}

func TestExecute_ClaimExpired(t *testing.T) {
	repo := &repositoryStub{claimErr: ErrExpired}
	svc := makeService(&kubernetesStub{}, repo)
	_, err := svc.Execute(context.Background(), "plan-1", "token", "idem-key1")
	if !errors.Is(err, ErrExpired) {
		t.Fatalf("want ErrExpired, got %v", err)
	}
}

func TestExecute_ClaimNoExecute(t *testing.T) {
	repo := &repositoryStub{shouldExecute: false}
	svc := makeService(&kubernetesStub{}, repo)
	plan, err := svc.Execute(context.Background(), "plan-1", "token", "idem-key1")
	if err != nil {
		t.Fatalf("want nil error when claim returns no-execute, got %v", err)
	}
	if plan.ID != "" {
		t.Errorf("expected empty plan when no execute, got %+v", plan)
	}
}

// --- Execute re-verification tests ---

func TestExecute_BackupLookupError(t *testing.T) {
	plan := Plan{
		ID:                     "plan-1",
		ClusterID:              1,
		Status:                 StatusExecuting,
		SourceBackupName:       "weekly-backup",
		SourceBackupNamespace:  "velero",
		DestinationNamespace:   "restore-weekly-backup-7",
		VeleroRestoreName:      "rehearse-restore-weekly-backup-7",
		VeleroRestoreNamespace: "velero",
	}
	kube := &kubernetesStub{
		backupErr: errors.New("api down"),
	}
	repo := &repositoryStub{claimed: plan, shouldExecute: true}
	svc := makeService(kube, repo)
	_, err := svc.Execute(context.Background(), "plan-1", "token", "idem-key1")
	if !errors.Is(err, ErrExecutionFailed) {
		t.Fatalf("want ErrExecutionFailed, got %v", err)
	}
	if repo.failedMessage == "" {
		t.Error("expected Fail to be called with a message")
	}
}

func TestExecute_StaleSource(t *testing.T) {
	plan := Plan{
		ID:                     "plan-1",
		ClusterID:              1,
		Status:                 StatusExecuting,
		SourceBackupName:       "weekly-backup",
		SourceBackupNamespace:  "velero",
		DestinationNamespace:   "restore-weekly-backup-7",
		VeleroRestoreName:      "rehearse-restore-weekly-backup-7",
		VeleroRestoreNamespace: "velero",
	}
	kube := &kubernetesStub{
		backup: k8sgateway.VeleroBackup{Phase: PhaseFailed, IncludedNamespaces: []string{"app"}},
	}
	repo := &repositoryStub{claimed: plan, shouldExecute: true}
	svc := makeService(kube, repo)
	_, err := svc.Execute(context.Background(), "plan-1", "token", "idem-key1")
	if !errors.Is(err, ErrStaleSource) {
		t.Fatalf("want ErrStaleSource, got %v", err)
	}
}

func TestExecute_DestinationExists(t *testing.T) {
	plan := Plan{
		ID:                     "plan-1",
		ClusterID:              1,
		Status:                 StatusExecuting,
		SourceBackupName:       "weekly-backup",
		SourceBackupNamespace:  "velero",
		DestinationNamespace:   "restore-weekly-backup-7",
		VeleroRestoreName:      "rehearse-restore-weekly-backup-7",
		VeleroRestoreNamespace: "velero",
	}
	kube := &kubernetesStub{
		backup:          completedBackup(),
		namespaceExists: true,
	}
	repo := &repositoryStub{claimed: plan, shouldExecute: true}
	svc := makeService(kube, repo)
	_, err := svc.Execute(context.Background(), "plan-1", "token", "idem-key1")
	if !errors.Is(err, ErrDestinationExists) {
		t.Fatalf("want ErrDestinationExists, got %v", err)
	}
}

func TestExecute_RestoreNameConflict(t *testing.T) {
	plan := Plan{
		ID:                     "plan-1",
		ClusterID:              1,
		Status:                 StatusExecuting,
		SourceBackupName:       "weekly-backup",
		SourceBackupNamespace:  "velero",
		DestinationNamespace:   "restore-weekly-backup-7",
		VeleroRestoreName:      "rehearse-restore-weekly-backup-7",
		VeleroRestoreNamespace: "velero",
	}
	kube := &kubernetesStub{
		backup:        completedBackup(),
		restoreExists: true,
	}
	repo := &repositoryStub{claimed: plan, shouldExecute: true}
	svc := makeService(kube, repo)
	_, err := svc.Execute(context.Background(), "plan-1", "token", "idem-key1")
	if !errors.Is(err, ErrRestoreNameConflict) {
		t.Fatalf("want ErrRestoreNameConflict, got %v", err)
	}
}

func TestExecute_NamespaceCreationFails(t *testing.T) {
	plan := Plan{
		ID:                     "plan-1",
		ClusterID:              1,
		Status:                 StatusExecuting,
		SourceBackupName:       "weekly-backup",
		SourceBackupNamespace:  "velero",
		DestinationNamespace:   "restore-weekly-backup-7",
		VeleroRestoreName:      "rehearse-restore-weekly-backup-7",
		VeleroRestoreNamespace: "velero",
	}
	kube := &kubernetesStub{
		backup:    completedBackup(),
		createErr: errors.New("namespace forbidden"),
	}
	repo := &repositoryStub{claimed: plan, shouldExecute: true}
	svc := makeService(kube, repo)
	_, err := svc.Execute(context.Background(), "plan-1", "token", "idem-key1")
	if !errors.Is(err, ErrQuarantineFailed) {
		t.Fatalf("want ErrQuarantineFailed, got %v", err)
	}
}

func TestExecute_QuarantineControlsFail(t *testing.T) {
	plan := Plan{
		ID:                     "plan-1",
		ClusterID:              1,
		Status:                 StatusExecuting,
		SourceBackupName:       "weekly-backup",
		SourceBackupNamespace:  "velero",
		DestinationNamespace:   "restore-weekly-backup-7",
		VeleroRestoreName:      "rehearse-restore-weekly-backup-7",
		VeleroRestoreNamespace: "velero",
	}
	kube := &kubernetesStub{
		backup:          completedBackup(),
		createResp:      []byte(`{"metadata":{"uid":"ns-uid-1"}}`),
		createErrOnCall: map[int]error{2: errors.New("networkpolicy forbidden")},
	}
	repo := &repositoryStub{claimed: plan, shouldExecute: true}
	svc := makeService(kube, repo)
	_, err := svc.Execute(context.Background(), "plan-1", "token", "idem-key1")
	if !errors.Is(err, ErrQuarantineFailed) {
		t.Fatalf("want ErrQuarantineFailed, got %v", err)
	}
}

func TestExecute_RestoreCreationFails(t *testing.T) {
	plan := Plan{
		ID:                     "plan-1",
		ClusterID:              1,
		Status:                 StatusExecuting,
		SourceBackupName:       "weekly-backup",
		SourceBackupNamespace:  "velero",
		DestinationNamespace:   "restore-weekly-backup-7",
		VeleroRestoreName:      "rehearse-restore-weekly-backup-7",
		VeleroRestoreNamespace: "velero",
	}
	kube := &kubernetesStub{
		backup:     completedBackup(),
		createResp: []byte(`{"metadata":{"uid":"ns-uid-1"}}`),
		// call 1: namespace, 2: NP, 3: RQ succeed; call 4: restore fails.
		createErrOnCall: map[int]error{4: errors.New("restore create forbidden")},
	}
	repo := &repositoryStub{claimed: plan, shouldExecute: true}
	svc := makeService(kube, repo)
	_, err := svc.Execute(context.Background(), "plan-1", "token", "idem-key1")
	if !errors.Is(err, ErrExecutionFailed) {
		t.Fatalf("want ErrExecutionFailed, got %v", err)
	}
	if repo.failedResult == nil || !repo.failedResult.QuarantineEstablished {
		t.Error("expected quarantine established in failure result")
	}
}

func TestExecute_PollTimeout(t *testing.T) {
	plan := Plan{
		ID:                     "plan-1",
		ClusterID:              1,
		Status:                 StatusExecuting,
		SourceBackupName:       "weekly-backup",
		SourceBackupNamespace:  "velero",
		DestinationNamespace:   "restore-weekly-backup-7",
		VeleroRestoreName:      "rehearse-restore-weekly-backup-7",
		VeleroRestoreNamespace: "velero",
	}
	kube := &kubernetesStub{
		backup:     completedBackup(),
		createResp: []byte(`{"metadata":{"uid":"ns-uid-1"}}`),
		restore:    k8sgateway.VeleroRestore{Phase: PhaseInProgress},
	}
	repo := &repositoryStub{claimed: plan, shouldExecute: true}
	svc := makeService(kube, repo)
	_, err := svc.Execute(context.Background(), "plan-1", "token", "idem-key1")
	if !errors.Is(err, ErrRestorePollTimeout) {
		t.Fatalf("want ErrRestorePollTimeout, got %v", err)
	}
}

func TestExecute_PartialRestore(t *testing.T) {
	plan := Plan{
		ID:                     "plan-1",
		ClusterID:              1,
		Status:                 StatusExecuting,
		SourceBackupName:       "weekly-backup",
		SourceBackupNamespace:  "velero",
		DestinationNamespace:   "restore-weekly-backup-7",
		VeleroRestoreName:      "rehearse-restore-weekly-backup-7",
		VeleroRestoreNamespace: "velero",
	}
	kube := &kubernetesStub{
		backup:     completedBackup(),
		createResp: []byte(`{"metadata":{"uid":"ns-uid-1"}}`),
		restore:    terminalRestore(PhasePartiallyFailed),
	}
	repo := &repositoryStub{claimed: plan, shouldExecute: true}
	svc := makeService(kube, repo)
	_, err := svc.Execute(context.Background(), "plan-1", "token", "idem-key1")
	if !errors.Is(err, ErrPartialRestore) {
		t.Fatalf("want ErrPartialRestore, got %v", err)
	}
	if repo.failedResult == nil || !repo.failedResult.Partial {
		t.Error("expected partial flag in failure result")
	}
}

func TestExecute_FailedPhase(t *testing.T) {
	plan := Plan{
		ID:                     "plan-1",
		ClusterID:              1,
		Status:                 StatusExecuting,
		SourceBackupName:       "weekly-backup",
		SourceBackupNamespace:  "velero",
		DestinationNamespace:   "restore-weekly-backup-7",
		VeleroRestoreName:      "rehearse-restore-weekly-backup-7",
		VeleroRestoreNamespace: "velero",
	}
	kube := &kubernetesStub{
		backup:     completedBackup(),
		createResp: []byte(`{"metadata":{"uid":"ns-uid-1"}}`),
		restore:    terminalRestore(PhaseFailed),
	}
	repo := &repositoryStub{claimed: plan, shouldExecute: true}
	svc := makeService(kube, repo)
	_, err := svc.Execute(context.Background(), "plan-1", "token", "idem-key1")
	if !errors.Is(err, ErrExecutionFailed) {
		t.Fatalf("want ErrExecutionFailed, got %v", err)
	}
}

func TestExecute_Success(t *testing.T) {
	plan := Plan{
		ID:                     "plan-1",
		ClusterID:              1,
		Status:                 StatusExecuting,
		SourceBackupName:       "weekly-backup",
		SourceBackupNamespace:  "velero",
		DestinationNamespace:   "restore-weekly-backup-7",
		VeleroRestoreName:      "rehearse-restore-weekly-backup-7",
		VeleroRestoreNamespace: "velero",
	}
	kube := &kubernetesStub{
		backup:      completedBackup(),
		createResp:  []byte(`{"metadata":{"uid":"ns-uid-1"}}`),
		restore:     terminalRestore(PhaseCompleted),
		deployments: []k8sgateway.Deployment{{Metadata: k8sgateway.ObjectMeta{Name: "app-deploy"}}},
		secrets:     []k8sgateway.Secret{{Metadata: k8sgateway.ObjectMeta{Name: "app-secret"}}},
	}
	repo := &repositoryStub{claimed: plan, shouldExecute: true}
	svc := makeService(kube, repo)
	result, err := svc.Execute(context.Background(), "plan-1", "token", "idem-key1")
	if err != nil {
		t.Fatalf("want nil, got %v", err)
	}
	if !repo.completed {
		t.Fatal("expected Complete to be called")
	}
	if result.Status != StatusSucceeded {
		t.Errorf("Status = %q", result.Status)
	}
	if repo.completedPlan.ExecutionResult == nil {
		t.Fatal("expected execution result to be recorded")
	}
	if !repo.completedPlan.ExecutionResult.RestoreCreated {
		t.Error("RestoreCreated should be true")
	}
	if !repo.completedPlan.ExecutionResult.QuarantineEstablished {
		t.Error("QuarantineEstablished should be true")
	}
	if repo.completedPlan.ExecutionResult.RestorePhase != PhaseCompleted {
		t.Errorf("RestorePhase = %q", repo.completedPlan.ExecutionResult.RestorePhase)
	}
	if repo.completedPlan.ExecutionResult.RestoredItemCount != 2 {
		t.Errorf("RestoredItemCount = %d", repo.completedPlan.ExecutionResult.RestoredItemCount)
	}
	// Verify create calls: 4 non-dry-run creates (namespace, NP, RQ, restore).
	nonDryRun := 0
	for _, dry := range kube.createDryRuns {
		if !dry {
			nonDryRun++
		}
	}
	if nonDryRun != 4 {
		t.Errorf("expected 4 non-dry-run creates, got %d", nonDryRun)
	}
}

// --- List tests ---

func TestList_InvalidClusterID(t *testing.T) {
	svc := makeService(&kubernetesStub{}, &repositoryStub{})
	_, err := svc.List(context.Background(), 0)
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("want ErrInvalidRequest, got %v", err)
	}
}

func TestList_Success(t *testing.T) {
	plans := []Plan{{ID: "plan-1", Status: StatusSucceeded}}
	repo := &repositoryStub{listPlans: plans}
	svc := makeService(&kubernetesStub{}, repo)
	result, err := svc.List(context.Background(), 1)
	if err != nil {
		t.Fatalf("want nil, got %v", err)
	}
	if len(result) != 1 || result[0].ID != "plan-1" {
		t.Errorf("unexpected result: %+v", result)
	}
}

// --- helper tests ---

func TestGenerateDestinationNamespace(t *testing.T) {
	name := generateDestinationNamespace("weekly-backup", 7)
	if !strings.HasPrefix(name, "restore-weekly-backup-7") {
		t.Errorf("unexpected namespace: %q", name)
	}
}

func TestGenerateDestinationNamespace_TruncatesLongName(t *testing.T) {
	long := strings.Repeat("a", 100)
	name := generateDestinationNamespace(long, 1)
	// Should truncate source to 20 chars then prefix.
	if len(name) > len("restore-")+20+len("-1") {
		t.Errorf("namespace not truncated: %q (len %d)", name, len(name))
	}
}

func TestGenerateRestoreName(t *testing.T) {
	name := generateRestoreName("restore-weekly-backup-7")
	if name != "rehearse-restore-weekly-backup-7" {
		t.Errorf("unexpected restore name: %q", name)
	}
}

func TestSanitizeDNS1123(t *testing.T) {
	// sanitizeDNS1123 only retains lowercase a-z, 0-9, and interior hyphens.
	// Uppercase and other characters are dropped (callers ToLower first).
	cases := []struct {
		input, want string
	}{
		{"weekly-backup", "weekly-backup"},
		{"weekly_backup", "weeklybackup"},
		{"", "x"},
		{"---", "-"},
		{"app123", "app123"},
	}
	for _, tc := range cases {
		if got := sanitizeDNS1123(tc.input); got != tc.want {
			t.Errorf("sanitizeDNS1123(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestExtractUID(t *testing.T) {
	if uid := extractUID([]byte(`{"metadata":{"uid":"abc-123"}}`)); uid != "abc-123" {
		t.Errorf("uid = %q", uid)
	}
	if uid := extractUID(nil); uid != "" {
		t.Errorf("empty response uid = %q", uid)
	}
	if uid := extractUID([]byte(`{bad json}`)); uid != "" {
		t.Errorf("bad json uid = %q", uid)
	}
}

func TestNewIdentity(t *testing.T) {
	id, token, hash, err := newIdentity()
	if err != nil {
		t.Fatalf("newIdentity error: %v", err)
	}
	if id == "" || token == "" || len(hash) == 0 {
		t.Error("identity fields should be non-empty")
	}
	// Verify hash is sha256 of token.
	expected := sha256.Sum256([]byte(token))
	if len(hash) != len(expected[:]) {
		t.Errorf("hash length = %d, want %d", len(hash), len(expected[:]))
	}
}

// --- JSON serialization tests ---

func TestQuarantineStatusJSON_Scan(t *testing.T) {
	var s QuarantineStatusJSON
	if err := s.Scan(nil); err != nil {
		t.Errorf("Scan(nil) error: %v", err)
	}
	if err := s.Scan([]byte(`{"network_policy_name":"np","dry_run_validated":true}`)); err != nil {
		t.Errorf("Scan(bytes) error: %v", err)
	}
	if s.NetworkPolicyName != "np" || !s.DryRunValidated {
		t.Errorf("scanned value = %+v", s)
	}
	if err := s.Scan(123); err == nil {
		t.Error("Scan(int) should error")
	}
}

func TestExecutionResultJSON_Scan(t *testing.T) {
	var r ExecutionResultJSON
	if err := r.Scan(nil); err != nil {
		t.Errorf("Scan(nil) error: %v", err)
	}
	if err := r.Scan([]byte(`{"restore_created":true,"restored_item_count":3}`)); err != nil {
		t.Errorf("Scan(bytes) error: %v", err)
	}
	if !r.RestoreCreated || r.RestoredItemCount != 3 {
		t.Errorf("scanned value = %+v", r)
	}
	if err := r.Scan(123); err == nil {
		t.Error("Scan(int) should error")
	}
}

func TestExecutionResultJSON_EmptyItemsOmittedByOmitempty(t *testing.T) {
	// restored_items has json:",omitempty" so an empty/nil slice is omitted.
	r := ExecutionResultJSON{}
	v, err := r.Value()
	if err != nil {
		t.Fatalf("Value error: %v", err)
	}
	str, ok := v.([]byte)
	if !ok {
		t.Fatalf("Value should return []byte, got %T", v)
	}
	if strings.Contains(string(str), "restored_items") {
		t.Errorf("expected restored_items to be omitted by omitempty, got %s", str)
	}
}

func TestExecutionResultJSON_NonEmptyItemsSerialized(t *testing.T) {
	r := ExecutionResultJSON{
		RestoredItems: []RestoredItem{{Kind: "Deployment", Name: "app", Namespace: "ns"}},
	}
	v, err := r.Value()
	if err != nil {
		t.Fatalf("Value error: %v", err)
	}
	str, ok := v.([]byte)
	if !ok {
		t.Fatalf("Value should return []byte, got %T", v)
	}
	if !strings.Contains(string(str), `"restored_items":[{"kind":"Deployment","name":"app","namespace":"ns"}]`) {
		t.Errorf("expected restored_items serialized, got %s", str)
	}
}

// --- AllowedKinds / ExcludedKinds tests ---

func TestAllowedKinds_ContainsExpectedSet(t *testing.T) {
	want := map[string]bool{
		KindDeployment: true, KindStatefulSet: true, KindDaemonSet: true,
		KindCronJob: true, KindConfigMap: true, KindSecret: true, KindServiceAccount: true,
	}
	if len(AllowedKinds) != len(want) {
		t.Fatalf("AllowedKinds length = %d, want %d", len(AllowedKinds), len(want))
	}
	for _, k := range AllowedKinds {
		if !want[k] {
			t.Errorf("unexpected kind in AllowedKinds: %q", k)
		}
	}
}

func TestExcludedKinds_ContainsPodAndPVC(t *testing.T) {
	found := map[string]bool{}
	for _, k := range ExcludedKinds {
		found[k] = true
	}
	for _, required := range []string{"Pod", "PersistentVolumeClaim", "NetworkPolicy", "Service"} {
		if !found[required] {
			t.Errorf("ExcludedKinds missing required entry: %q", required)
		}
	}
}

// --- Response projection test ---

func TestResponse_Projection(t *testing.T) {
	plan := Plan{
		ID:                    "plan-1",
		ClusterID:             1,
		SourceBackupName:      "weekly-backup",
		SourceBackupNamespace: "velero",
		SourceBackupPhase:     PhaseCompleted,
		DestinationNamespace:  "restore-weekly-backup-7",
		VeleroRestoreName:     "rehearse-restore-weekly-backup-7",
		RequestedByName:       "alice",
	}
	resp := Response(plan)
	if resp.SourceSnapshot.Name != "weekly-backup" {
		t.Errorf("SourceSnapshot.Name = %q", resp.SourceSnapshot.Name)
	}
	if resp.DestinationName != "restore-weekly-backup-7" {
		t.Errorf("DestinationName = %q", resp.DestinationName)
	}
	if len(resp.AllowedKinds) != len(AllowedKinds) {
		t.Errorf("AllowedKinds length = %d", len(resp.AllowedKinds))
	}
	if len(resp.ExcludedKinds) != len(ExcludedKinds) {
		t.Errorf("ExcludedKinds length = %d", len(resp.ExcludedKinds))
	}
	if resp.RequestedBy.Name != "alice" {
		t.Errorf("RequestedBy.Name = %q", resp.RequestedBy.Name)
	}
}
