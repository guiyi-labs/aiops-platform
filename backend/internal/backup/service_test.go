package backup

import (
	"context"
	"errors"
	"testing"
	"time"

	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

type kubernetesStub struct {
	capability  k8sgateway.VeleroCapability
	capErr      error
	locations   []k8sgateway.BackupStorageLocation
	locErr      error
	backupExist bool
	existErr    error
	createErr   error
	dryRuns     []bool
	bodies      [][]byte
}

func (s *kubernetesStub) VeleroCapability(context.Context, int64) (k8sgateway.VeleroCapability, error) {
	return s.capability, s.capErr
}

func (s *kubernetesStub) BackupStorageLocations(context.Context, int64, string) ([]k8sgateway.BackupStorageLocation, error) {
	return s.locations, s.locErr
}

func (s *kubernetesStub) VeleroBackupExists(context.Context, int64, string, string) (bool, error) {
	return s.backupExist, s.existErr
}

func (s *kubernetesStub) CreateResource(_ context.Context, _ int64, _ string, body []byte, dryRun bool) ([]byte, error) {
	s.dryRuns = append(s.dryRuns, dryRun)
	s.bodies = append(s.bodies, append([]byte(nil), body...))
	return []byte("{}"), s.createErr
}

type repositoryStub struct {
	saved         Plan
	claimed       Plan
	shouldExecute bool
	claimErr      error
	completed     bool
	failedMessage string
	plans         []Plan
}

func (s *repositoryStub) Save(_ context.Context, plan *Plan) error    { s.saved = *plan; return nil }
func (s *repositoryStub) List(context.Context, int64) ([]Plan, error) { return s.plans, nil }
func (s *repositoryStub) Claim(context.Context, string, []byte, string, time.Time, time.Time) (Plan, bool, error) {
	return s.claimed, s.shouldExecute, s.claimErr
}
func (s *repositoryStub) Complete(_ context.Context, _, _ string, at time.Time) (Plan, error) {
	s.completed = true
	s.claimed.Status, s.claimed.ExecutedAt = StatusSucceeded, &at
	return s.claimed, nil
}
func (s *repositoryStub) Fail(_ context.Context, _, _, message string) (Plan, error) {
	s.failedMessage = message
	s.claimed.Status, s.claimed.LastError = StatusFailed, message
	return s.claimed, nil
}

func makeService(kube *kubernetesStub, repo *repositoryStub) *Service {
	svc := NewService(kube, repo)
	svc.now = func() time.Time { return time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC) }
	return svc
}

func validRequest() Request {
	return Request{
		BackupName:         "test-backup",
		BackupNamespace:    "velero",
		IncludedNamespaces: []string{"default"},
		StorageLocation:    "default",
		TTL:                "720h",
	}
}

func TestPreviewRejectsInvalidRequest(t *testing.T) {
	kube := &kubernetesStub{capability: k8sgateway.VeleroCapability{Installed: true}}
	repo := &repositoryStub{}
	svc := makeService(kube, repo)

	// Missing backup name
	_, err := svc.Preview(context.Background(), 1, Request{
		BackupName:         "",
		BackupNamespace:    "velero",
		IncludedNamespaces: []string{"default"},
		StorageLocation:    "default",
	}, ActorRef{ID: 1, Name: "admin"})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}

	// Too many namespaces
	many := make([]string, 11)
	for i := range many {
		many[i] = "ns"
	}
	_, err = svc.Preview(context.Background(), 1, Request{
		BackupName:         "ok",
		BackupNamespace:    "velero",
		IncludedNamespaces: many,
		StorageLocation:    "default",
	}, ActorRef{ID: 1, Name: "admin"})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest for too many namespaces, got %v", err)
	}
}

func TestPreviewFailsWhenVeleroNotInstalled(t *testing.T) {
	kube := &kubernetesStub{capability: k8sgateway.VeleroCapability{Installed: false}}
	repo := &repositoryStub{}
	svc := makeService(kube, repo)

	_, err := svc.Preview(context.Background(), 1, validRequest(), ActorRef{ID: 1, Name: "admin"})
	if !errors.Is(err, ErrVeleroNotInstalled) {
		t.Fatalf("expected ErrVeleroNotInstalled, got %v", err)
	}
}

func TestPreviewFailsWhenStorageLocationNotFound(t *testing.T) {
	kube := &kubernetesStub{
		capability: k8sgateway.VeleroCapability{Installed: true},
		locations:  []k8sgateway.BackupStorageLocation{{Name: "other", Namespace: "velero", Phase: "Available"}},
	}
	repo := &repositoryStub{}
	svc := makeService(kube, repo)

	_, err := svc.Preview(context.Background(), 1, validRequest(), ActorRef{ID: 1, Name: "admin"})
	if !errors.Is(err, ErrStorageLocationNotFound) {
		t.Fatalf("expected ErrStorageLocationNotFound, got %v", err)
	}
}

func TestPreviewFailsOnBackupNameConflict(t *testing.T) {
	kube := &kubernetesStub{
		capability:  k8sgateway.VeleroCapability{Installed: true},
		locations:   []k8sgateway.BackupStorageLocation{{Name: "default", Namespace: "velero", Phase: "Available"}},
		backupExist: true,
	}
	repo := &repositoryStub{}
	svc := makeService(kube, repo)

	_, err := svc.Preview(context.Background(), 1, validRequest(), ActorRef{ID: 1, Name: "admin"})
	if !errors.Is(err, ErrBackupNameConflict) {
		t.Fatalf("expected ErrBackupNameConflict, got %v", err)
	}
}

func TestPreviewPerformsDryRunAndStoresHash(t *testing.T) {
	kube := &kubernetesStub{
		capability: k8sgateway.VeleroCapability{Installed: true, Version: "v1"},
		locations:  []k8sgateway.BackupStorageLocation{{Name: "default", Namespace: "velero", Phase: "Available"}},
	}
	repo := &repositoryStub{}
	svc := makeService(kube, repo)

	plan, err := svc.Preview(context.Background(), 1, validRequest(), ActorRef{ID: 1, Name: "admin"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(kube.dryRuns) != 1 || !kube.dryRuns[0] {
		t.Fatal("expected one dry-run create")
	}
	if plan.ConfirmationToken == "" {
		t.Fatal("confirmation token should be returned to caller")
	}
	if len(repo.saved.ConfirmationTokenHash) == 0 {
		t.Fatal("token hash should be persisted")
	}
	if repo.saved.ConfirmationToken != "" {
		t.Fatal("plaintext token must not be persisted")
	}
	if repo.saved.Status != StatusAwaitingConfirmation {
		t.Fatalf("expected status %s, got %s", StatusAwaitingConfirmation, repo.saved.Status)
	}
	if repo.saved.VeleroVersion != "v1" {
		t.Fatalf("expected velero version v1, got %s", repo.saved.VeleroVersion)
	}
}

func TestExecuteCreatesBackupAndCompletes(t *testing.T) {
	kube := &kubernetesStub{
		capability: k8sgateway.VeleroCapability{Installed: true, Version: "v1"},
		locations:  []k8sgateway.BackupStorageLocation{{Name: "default", Namespace: "velero", Phase: "Available"}},
	}
	repo := &repositoryStub{
		shouldExecute: true,
		claimed: Plan{
			ID:              "plan-1",
			ClusterID:       1,
			Status:          StatusExecuting,
			BackupName:      "test-backup",
			BackupNamespace: "velero",
			TTL:             "720h",
		},
	}
	svc := makeService(kube, repo)

	plan, err := svc.Execute(context.Background(), "plan-1", "token", "idem-key-12345678")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(kube.dryRuns) != 1 || kube.dryRuns[0] {
		t.Fatal("execute should not dry-run")
	}
	if !repo.completed {
		t.Fatal("expected repository Complete to be called")
	}
	if plan.Status != StatusSucceeded {
		t.Fatalf("expected status %s, got %s", StatusSucceeded, plan.Status)
	}
}

func TestExecuteFailsOnClaimError(t *testing.T) {
	kube := &kubernetesStub{}
	repo := &repositoryStub{
		claimErr: ErrExpired,
	}
	svc := makeService(kube, repo)

	_, err := svc.Execute(context.Background(), "plan-1", "token", "idem-key-12345678")
	if !errors.Is(err, ErrExpired) {
		t.Fatalf("expected ErrExpired, got %v", err)
	}
	if len(kube.dryRuns) != 0 {
		t.Fatal("should not attempt create on claim error")
	}
}

func TestExecuteFailsAndRecordsError(t *testing.T) {
	kube := &kubernetesStub{
		createErr: errors.New("kubernetes API error"),
	}
	repo := &repositoryStub{
		shouldExecute: true,
		claimed: Plan{
			ID:              "plan-1",
			ClusterID:       1,
			BackupName:      "test-backup",
			BackupNamespace: "velero",
		},
	}
	svc := makeService(kube, repo)

	_, err := svc.Execute(context.Background(), "plan-1", "token", "idem-key-12345678")
	if !errors.Is(err, ErrExecutionFailed) {
		t.Fatalf("expected ErrExecutionFailed, got %v", err)
	}
	if repo.failedMessage == "" {
		t.Fatal("expected error message to be recorded")
	}
}

func TestExecuteRejectsInvalidIdempotencyKey(t *testing.T) {
	kube := &kubernetesStub{}
	repo := &repositoryStub{}
	svc := makeService(kube, repo)

	_, err := svc.Execute(context.Background(), "plan-1", "token", "short")
	if !errors.Is(err, ErrInvalidIdempotency) {
		t.Fatalf("expected ErrInvalidIdempotency, got %v", err)
	}
}

func TestExecuteRejectsEmptyConfirmation(t *testing.T) {
	kube := &kubernetesStub{}
	repo := &repositoryStub{}
	svc := makeService(kube, repo)

	_, err := svc.Execute(context.Background(), "plan-1", "", "idem-key-12345678")
	if !errors.Is(err, ErrConfirmationInvalid) {
		t.Fatalf("expected ErrConfirmationInvalid, got %v", err)
	}
}

func TestValidateRequestAcceptsDefaults(t *testing.T) {
	req := validRequest()
	if err := validateRequest(1, req); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
}

func TestValidateRequestRejectsBadTTL(t *testing.T) {
	req := validRequest()
	req.TTL = "abc"
	if err := validateRequest(1, req); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest for bad TTL, got %v", err)
	}
}

func TestValidateRequestRejectsEmptyNamespaces(t *testing.T) {
	req := validRequest()
	req.IncludedNamespaces = nil
	if err := validateRequest(1, req); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest for empty namespaces, got %v", err)
	}
}
