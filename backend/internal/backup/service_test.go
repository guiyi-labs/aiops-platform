package backup

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"k8s-aiops.local/backend/internal/apiquery"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

type kubernetesStub struct {
	capability   k8sgateway.VeleroCapability
	capErr       error
	namespace    k8sgateway.Namespace
	namespaceErr error
	locations    []k8sgateway.BackupStorageLocation
	locErr       error
	backupExist  bool
	existErr     error
	createErr    error
	dryRuns      []bool
	bodies       [][]byte
}

func (s *kubernetesStub) VeleroCapability(context.Context, int64) (k8sgateway.VeleroCapability, error) {
	return s.capability, s.capErr
}
func (s *kubernetesStub) Namespaces(context.Context, int64, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Namespace], error) {
	if s.namespaceErr != nil {
		return apiquery.ListResponse[k8sgateway.Namespace]{}, s.namespaceErr
	}
	if s.namespace.Metadata.Name == "" {
		return apiquery.ListResponse[k8sgateway.Namespace]{}, nil
	}
	return apiquery.ListResponse[k8sgateway.Namespace]{Items: []k8sgateway.Namespace{s.namespace}, Total: 1}, nil
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
	if s.createErr != nil {
		return nil, s.createErr
	}
	if dryRun {
		return []byte(`{}`), nil
	}
	return []byte(`{"metadata":{"uid":"backup-uid-1","resourceVersion":"55"}}`), nil
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
func (s *repositoryStub) Complete(_ context.Context, _, _, uid, rv string, at time.Time) (Plan, error) {
	s.completed = true
	s.claimed.Status, s.claimed.ExecutedAt = StatusSucceeded, &at
	s.claimed.BackupUID, s.claimed.BackupResourceVersion = uid, rv
	return s.claimed, nil
}
func (s *repositoryStub) Fail(_ context.Context, _, _, message string) (Plan, error) {
	s.failedMessage = message
	s.claimed.Status, s.claimed.LastError = StatusFailed, message
	return s.claimed, nil
}

func sourceNamespace(rv string) k8sgateway.Namespace {
	return k8sgateway.Namespace{Metadata: k8sgateway.ObjectMeta{Name: "default", UID: "ns-uid-1", ResourceVersion: rv}}
}

func readyKubernetes() *kubernetesStub {
	return &kubernetesStub{
		capability: k8sgateway.VeleroCapability{Installed: true, Version: "v1"},
		namespace:  sourceNamespace("10"),
		locations:  []k8sgateway.BackupStorageLocation{{Name: "default", Namespace: DefaultBackupNamespace, Phase: "Available"}},
	}
}

func makeService(kube *kubernetesStub, repo *repositoryStub) *Service {
	svc := NewService(kube, repo)
	svc.now = func() time.Time { return time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC) }
	return svc
}

func validRequest() Request {
	return Request{SourceNamespace: "default", StorageLocation: "default", TTL: "720h"}
}

func executingPlan() Plan {
	return Plan{ID: "plan-1", ClusterID: 1, Status: StatusExecuting, BackupName: "aiops-default-12345678", BackupNamespace: DefaultBackupNamespace, IncludedNamespaces: []string{"default"}, SourceNamespaceUID: "ns-uid-1", SourceNamespaceResourceVersion: "10", StorageLocation: "default", TTL: "720h"}
}

func TestValidateRequestFixedScope(t *testing.T) {
	for _, ttl := range []string{"24h", "168h", "720h"} {
		req := validRequest()
		req.TTL = ttl
		if err := validateRequest(1, req); err != nil {
			t.Fatalf("TTL %s rejected: %v", ttl, err)
		}
	}
	for _, ttl := range []string{"", "1h", "abc", "721h"} {
		req := validRequest()
		req.TTL = ttl
		if !errors.Is(validateRequest(1, req), ErrInvalidRequest) {
			t.Fatalf("TTL %q must be rejected", ttl)
		}
	}
}

func TestPreviewGeneratesNameAndFixedManifest(t *testing.T) {
	kube, repo := readyKubernetes(), &repositoryStub{}
	plan, err := makeService(kube, repo).Preview(context.Background(), 1, validRequest(), ActorRef{ID: 1, Name: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(plan.BackupName, "aiops-default-") || plan.BackupNamespace != DefaultBackupNamespace {
		t.Fatalf("server-owned identity not generated: %+v", plan)
	}
	if plan.SourceNamespaceUID != "ns-uid-1" || plan.SourceNamespaceResourceVersion != "10" {
		t.Fatalf("source identity not captured: %+v", plan)
	}
	var manifest struct {
		Spec map[string]any `json:"spec"`
	}
	if err := json.Unmarshal(kube.bodies[0], &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Spec["includeClusterResources"] != false || manifest.Spec["snapshotVolumes"] != false {
		t.Fatalf("unsafe Backup scope: %#v", manifest.Spec)
	}
	if _, ok := manifest.Spec["labelSelector"]; ok {
		t.Fatal("fixed manifest must not contain labelSelector")
	}
	if len(kube.dryRuns) != 1 || !kube.dryRuns[0] || repo.saved.ConfirmationToken != "" {
		t.Fatal("preview must be dry-run and persist no plaintext token")
	}
}

func TestPreviewRejectsMissingNamespace(t *testing.T) {
	kube := readyKubernetes()
	kube.namespace = k8sgateway.Namespace{}
	_, err := makeService(kube, &repositoryStub{}).Preview(context.Background(), 1, validRequest(), ActorRef{ID: 1, Name: "admin"})
	if !errors.Is(err, ErrSourceNamespaceNotFound) {
		t.Fatalf("expected missing namespace error, got %v", err)
	}
}

func TestPreviewRejectsUnavailableStorage(t *testing.T) {
	kube := readyKubernetes()
	kube.locations[0].Phase = "Unavailable"
	_, err := makeService(kube, &repositoryStub{}).Preview(context.Background(), 1, validRequest(), ActorRef{ID: 1, Name: "admin"})
	if !errors.Is(err, ErrStorageLocationUnavailable) {
		t.Fatalf("expected unavailable storage error, got %v", err)
	}
}

func TestExecuteRechecksAndStoresBackupIdentity(t *testing.T) {
	kube := readyKubernetes()
	repo := &repositoryStub{claimed: executingPlan(), shouldExecute: true}
	plan, err := makeService(kube, repo).Execute(context.Background(), "plan-1", "token", "idem-key-12345678")
	if err != nil {
		t.Fatal(err)
	}
	if !repo.completed || plan.BackupUID != "backup-uid-1" || plan.BackupResourceVersion != "55" {
		t.Fatalf("created Backup identity not persisted: %+v", plan)
	}
	if len(kube.dryRuns) != 1 || kube.dryRuns[0] {
		t.Fatal("execute must perform exactly one non-dry-run create")
	}
}

func TestExecuteRejectsStaleNamespace(t *testing.T) {
	kube := readyKubernetes()
	kube.namespace = sourceNamespace("11")
	repo := &repositoryStub{claimed: executingPlan(), shouldExecute: true}
	_, err := makeService(kube, repo).Execute(context.Background(), "plan-1", "token", "idem-key-12345678")
	if !errors.Is(err, ErrExecutionFailed) || !errors.Is(err, ErrStaleSourceNamespace) || repo.failedMessage == "" || len(kube.dryRuns) != 0 {
		t.Fatalf("stale namespace must fail closed: err=%v failed=%q", err, repo.failedMessage)
	}
}

func TestExecuteFailsOnCreateError(t *testing.T) {
	kube := readyKubernetes()
	kube.createErr = errors.New("kubernetes API error")
	repo := &repositoryStub{claimed: executingPlan(), shouldExecute: true}
	_, err := makeService(kube, repo).Execute(context.Background(), "plan-1", "token", "idem-key-12345678")
	if !errors.Is(err, ErrExecutionFailed) || repo.failedMessage == "" {
		t.Fatalf("create error not persisted: %v", err)
	}
}

func TestExecuteReturnsClaimErrorWithoutMutation(t *testing.T) {
	kube := readyKubernetes()
	repo := &repositoryStub{claimErr: ErrExpired}
	_, err := makeService(kube, repo).Execute(context.Background(), "plan-1", "token", "idem-key-12345678")
	if !errors.Is(err, ErrExpired) || len(kube.dryRuns) != 0 {
		t.Fatalf("claim error should not mutate: %v", err)
	}
}
