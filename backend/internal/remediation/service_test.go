package remediation

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"k8s-aiops.local/backend/internal/cluster"
	"k8s-aiops.local/backend/internal/diagnosis"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

type diagnosisStub struct {
	record diagnosis.Record
	err    error
}

func (s diagnosisStub) Get(context.Context, int64) (diagnosis.Record, error) { return s.record, s.err }

type kubernetesStub struct {
	pod         k8sgateway.Pod
	deployment  k8sgateway.Deployment
	cronJob     k8sgateway.CronJob
	replicaSet  k8sgateway.ReplicaSet
	replicaSets []k8sgateway.ReplicaSet
	rolloutHist k8sgateway.RolloutHistory
	rolloutStat k8sgateway.RolloutStatus
	err         error
	patches     [][]byte
	dryRuns     []bool
	patchKinds  []string
}

func (s *kubernetesStub) Pod(context.Context, int64, string, string) (k8sgateway.Pod, error) {
	return s.pod, s.err
}
func (s *kubernetesStub) Deployment(context.Context, int64, string, string) (k8sgateway.Deployment, error) {
	return s.deployment, s.err
}
func (s *kubernetesStub) PatchDeployment(_ context.Context, _ int64, _, _ string, patch []byte, dryRun bool) (k8sgateway.Deployment, error) {
	s.patches = append(s.patches, append([]byte(nil), patch...))
	s.dryRuns = append(s.dryRuns, dryRun)
	s.patchKinds = append(s.patchKinds, "Deployment")
	result := s.deployment
	var body struct {
		Spec struct {
			Replicas *int32 `json:"replicas"`
			Template struct {
				Spec struct {
					Containers []struct {
						Name  string `json:"name"`
						Image string `json:"image"`
					} `json:"containers"`
				} `json:"spec"`
			} `json:"template"`
		} `json:"spec"`
	}
	if json.Unmarshal(patch, &body) == nil {
		if body.Spec.Replicas != nil {
			result.Spec.Replicas = body.Spec.Replicas
		}
		if len(body.Spec.Template.Spec.Containers) > 0 {
			for i := range result.Spec.Template.Spec.Containers {
				for _, patched := range body.Spec.Template.Spec.Containers {
					if result.Spec.Template.Spec.Containers[i].Name == patched.Name {
						result.Spec.Template.Spec.Containers[i].Image = patched.Image
					}
				}
			}
		}
	}
	return result, s.err
}
func (s *kubernetesStub) CronJob(context.Context, int64, string, string) (k8sgateway.CronJob, error) {
	return s.cronJob, s.err
}
func (s *kubernetesStub) PatchCronJob(_ context.Context, _ int64, _, _ string, patch []byte, dryRun bool) (k8sgateway.CronJob, error) {
	s.patches = append(s.patches, append([]byte(nil), patch...))
	s.dryRuns = append(s.dryRuns, dryRun)
	s.patchKinds = append(s.patchKinds, "CronJob")
	result := s.cronJob
	var body struct {
		Spec struct {
			Suspend *bool `json:"suspend"`
		} `json:"spec"`
	}
	if json.Unmarshal(patch, &body) == nil && body.Spec.Suspend != nil {
		result.Spec.Suspend = body.Spec.Suspend
	}
	return result, s.err
}
func (s *kubernetesStub) ReplicaSet(context.Context, int64, string, string) (k8sgateway.ReplicaSet, error) {
	return s.replicaSet, s.err
}
func (s *kubernetesStub) ReplicaSetsByOwner(context.Context, int64, string, string) ([]k8sgateway.ReplicaSet, error) {
	return s.replicaSets, s.err
}
func (s *kubernetesStub) RolloutHistory(context.Context, int64, string, string) (k8sgateway.RolloutHistory, error) {
	return s.rolloutHist, s.err
}
func (s *kubernetesStub) RolloutStatus(context.Context, int64, string, string) (k8sgateway.RolloutStatus, error) {
	return s.rolloutStat, s.err
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
func (s *repositoryStub) ListOperations(context.Context, int64, string, string, string) ([]Plan, error) {
	return s.plans, nil
}
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

func eligibleFixtures() (diagnosis.Record, *kubernetesStub) {
	record := diagnosis.Record{ID: 9, ClusterID: 7, Status: "confirmed", Resource: diagnosis.ResourceRef{Kind: "Pod", Namespace: "demo", Name: "api-abc", UID: "pod-1"}}
	kube := &kubernetesStub{}
	kube.pod.Metadata = k8sgateway.ObjectMeta{Name: "api-abc", Namespace: "demo", UID: "pod-1", Labels: map[string]string{"app": "api", "track": "stable"}}
	kube.deployment.Metadata = k8sgateway.ObjectMeta{Name: "api", Namespace: "demo", UID: "deployment-1", ResourceVersion: "17"}
	kube.deployment.Spec.Selector.MatchLabels = map[string]string{"app": "api"}
	return record, kube
}

func TestPreviewUsesServerDryRunAndStoresOnlyConfirmationHash(t *testing.T) {
	record, kube := eligibleFixtures()
	repository := &repositoryStub{}
	service := NewService(diagnosisStub{record: record}, kube, repository)
	service.now = func() time.Time { return time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC) }

	plan, err := service.Preview(context.Background(), record.ID, ActionDeploymentRolloutRestart, "api", ActorRef{ID: 3, Name: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	if len(kube.patches) != 1 || !kube.dryRuns[0] {
		t.Fatalf("dry runs = %v patches = %d", kube.dryRuns, len(kube.patches))
	}
	if plan.ConfirmationToken == "" || len(repository.saved.ConfirmationTokenHash) != sha256.Size || repository.saved.ConfirmationToken != "" {
		t.Fatalf("token was not returned once and hashed at rest: plan=%#v saved=%#v", plan, repository.saved)
	}
	if plan.TargetUID != "deployment-1" || plan.TargetResourceVersion != "17" || plan.RestartAt == nil || !plan.ExpiresAt.Equal(plan.RestartAt.Add(10*time.Minute)) {
		t.Fatalf("plan = %#v", plan)
	}
	var patch map[string]any
	if err := json.Unmarshal(kube.patches[0], &patch); err != nil {
		t.Fatal(err)
	}
	encoded := string(kube.patches[0])
	for _, forbidden := range []string{plan.ConfirmationToken, "pod-1", "operator"} {
		if forbidden != "" && contains(encoded, forbidden) {
			t.Fatalf("patch leaked %q: %s", forbidden, encoded)
		}
	}
	if !contains(encoded, `"resourceVersion":"17"`) || !contains(encoded, "k8s-aiops.local/remediation-id") {
		t.Fatalf("patch = %s", encoded)
	}
}

func TestPreviewRejectsIneligibleAndUnrelatedTargets(t *testing.T) {
	record, kube := eligibleFixtures()
	record.Status = "open"
	_, err := NewService(diagnosisStub{record: record}, kube, &repositoryStub{}).Preview(context.Background(), record.ID, ActionDeploymentRolloutRestart, "api", ActorRef{})
	if !errors.Is(err, ErrDiagnosisNotEligible) || len(kube.patches) != 0 {
		t.Fatalf("error=%v patches=%d", err, len(kube.patches))
	}

	record.Status = "confirmed"
	kube.deployment.Spec.Selector.MatchLabels = map[string]string{"app": "other"}
	_, err = NewService(diagnosisStub{record: record}, kube, &repositoryStub{}).Preview(context.Background(), record.ID, ActionDeploymentRolloutRestart, "api", ActorRef{})
	if !errors.Is(err, ErrTargetMismatch) || len(kube.patches) != 0 {
		t.Fatalf("error=%v patches=%d", err, len(kube.patches))
	}
}

func TestExecuteReplaysExactPlanAndCompletes(t *testing.T) {
	_, kube := eligibleFixtures()
	restartAt := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	plan := Plan{ID: "12345678-1234-4234-8234-123456789abc", Action: ActionDeploymentRolloutRestart, ClusterID: 7, TargetNamespace: "demo", TargetName: "api", TargetUID: "deployment-1", TargetResourceVersion: "17", RestartAt: &restartAt, Status: StatusExecuting}
	repository := &repositoryStub{claimed: plan, shouldExecute: true}
	service := NewService(diagnosisStub{}, kube, repository)
	result, err := service.Execute(context.Background(), plan.ID, "confirmation", "request-key-0001")
	if err != nil || result.Status != StatusSucceeded || !repository.completed || len(kube.patches) != 1 || kube.dryRuns[0] {
		t.Fatalf("result=%#v err=%v completed=%v dry=%v", result, err, repository.completed, kube.dryRuns)
	}
	if !contains(string(kube.patches[0]), `"resourceVersion":"17"`) || !contains(string(kube.patches[0]), plan.ID) {
		t.Fatalf("patch = %s", kube.patches[0])
	}
}

func TestExecuteReturnsIdempotentStoredResultWithoutSecondPatch(t *testing.T) {
	_, kube := eligibleFixtures()
	plan := Plan{ID: "12345678-1234-4234-8234-123456789abc", Status: StatusSucceeded}
	repository := &repositoryStub{claimed: plan, shouldExecute: false}
	result, err := NewService(diagnosisStub{}, kube, repository).Execute(context.Background(), plan.ID, "confirmation", "request-key-0001")
	if err != nil || result.Status != StatusSucceeded || len(kube.patches) != 0 {
		t.Fatalf("result=%#v err=%v patches=%d", result, err, len(kube.patches))
	}
}

func TestExecutePersistsSanitizedFailure(t *testing.T) {
	_, kube := eligibleFixtures()
	kube.err = cluster.APIStatusError{StatusCode: 409}
	restartAt := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	plan := Plan{ID: "12345678-1234-4234-8234-123456789abc", Action: ActionDeploymentRolloutRestart, ClusterID: 7, TargetNamespace: "demo", TargetName: "api", TargetResourceVersion: "17", RestartAt: &restartAt, Status: StatusExecuting}
	repository := &repositoryStub{claimed: plan, shouldExecute: true}
	_, err := NewService(diagnosisStub{}, kube, repository).Execute(context.Background(), plan.ID, "confirmation", "request-key-0001")
	if err == nil || repository.failedMessage != "Kubernetes API rejected remediation with HTTP 409" {
		t.Fatalf("err=%v message=%q", err, repository.failedMessage)
	}
}

func TestPreviewDeploymentScaleStoresTypedDiffAndUsesDryRun(t *testing.T) {
	_, kube := eligibleFixtures()
	before, desired := int32(2), int32(4)
	kube.deployment.Spec.Replicas = &before
	repository := &repositoryStub{}
	service := NewService(diagnosisStub{}, kube, repository)
	service.now = func() time.Time { return time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC) }

	plan, err := service.PreviewOperation(context.Background(), 7, OperationRequest{Action: ActionDeploymentScale, Namespace: "demo", TargetName: "api", DesiredReplicas: &desired}, ActorRef{ID: 3, Name: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.DiagnosisID != nil || plan.BeforeReplicas == nil || *plan.BeforeReplicas != 2 || plan.DesiredReplicas == nil || *plan.DesiredReplicas != 4 {
		t.Fatalf("plan = %#v", plan)
	}
	if len(kube.patches) != 1 || !kube.dryRuns[0] || kube.patchKinds[0] != "Deployment" || !contains(string(kube.patches[0]), `"uid":"deployment-1"`) || !contains(string(kube.patches[0]), `"replicas":4`) {
		t.Fatalf("patches=%s dryRuns=%v kinds=%v", kube.patches, kube.dryRuns, kube.patchKinds)
	}
	if plan.ConfirmationToken == "" || repository.saved.ConfirmationToken != "" || len(repository.saved.ConfirmationTokenHash) != sha256.Size {
		t.Fatalf("token handling plan=%#v saved=%#v", plan, repository.saved)
	}
	response := Response(plan)
	if response.Change == nil || response.Change.Field != "spec.replicas" || response.Change.Before != int32(2) || response.Change.After != int32(4) {
		t.Fatalf("response = %#v", response)
	}
}

func TestPreviewCronJobSuspendAndResumeUseFixedBooleanPatch(t *testing.T) {
	for _, tt := range []struct {
		name    string
		action  string
		before  bool
		desired bool
	}{
		{name: "suspend", action: ActionCronJobSuspend, before: false, desired: true},
		{name: "resume", action: ActionCronJobResume, before: true, desired: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			kube := &kubernetesStub{}
			kube.cronJob.Metadata = k8sgateway.ObjectMeta{Name: "cleanup", Namespace: "demo", UID: "cron-1", ResourceVersion: "31"}
			kube.cronJob.Spec.Suspend = &tt.before
			plan, err := NewService(diagnosisStub{}, kube, &repositoryStub{}).PreviewOperation(context.Background(), 7, OperationRequest{Action: tt.action, Namespace: "demo", TargetName: "cleanup"}, ActorRef{})
			if err != nil {
				t.Fatal(err)
			}
			if plan.BeforeSuspended == nil || *plan.BeforeSuspended != tt.before || plan.DesiredSuspended == nil || *plan.DesiredSuspended != tt.desired || kube.patchKinds[0] != "CronJob" || !kube.dryRuns[0] {
				t.Fatalf("plan=%#v kinds=%v dry=%v", plan, kube.patchKinds, kube.dryRuns)
			}
			if !contains(string(kube.patches[0]), fmt.Sprintf(`"suspend":%t`, tt.desired)) || !contains(string(kube.patches[0]), `"resourceVersion":"31"`) {
				t.Fatalf("patch = %s", kube.patches[0])
			}
		})
	}
}

func TestPreviewOperationRejectsInvalidParametersAndNoChanges(t *testing.T) {
	_, kube := eligibleFixtures()
	current, tooMany := int32(2), int32(1001)
	kube.deployment.Spec.Replicas = &current
	service := NewService(diagnosisStub{}, kube, &repositoryStub{})
	for _, request := range []OperationRequest{
		{Action: ActionDeploymentScale, Namespace: "demo", TargetName: "api"},
		{Action: ActionDeploymentScale, Namespace: "demo", TargetName: "api", DesiredReplicas: &tooMany},
		{Action: ActionCronJobSuspend, Namespace: "demo", TargetName: "cleanup", DesiredReplicas: &current},
	} {
		if _, err := service.PreviewOperation(context.Background(), 7, request, ActorRef{}); !errors.Is(err, ErrInvalidOperation) {
			t.Fatalf("request=%#v err=%v", request, err)
		}
	}
	if _, err := service.PreviewOperation(context.Background(), 7, OperationRequest{Action: ActionDeploymentScale, Namespace: "demo", TargetName: "api", DesiredReplicas: &current}, ActorRef{}); !errors.Is(err, ErrOperationNoChange) {
		t.Fatalf("no change err=%v", err)
	}
	if len(kube.patches) != 0 {
		t.Fatalf("unexpected patches = %d", len(kube.patches))
	}
}

func TestExecuteCronJobOperationDispatchesOnce(t *testing.T) {
	desired := true
	plan := Plan{ID: "12345678-1234-4234-8234-123456789abc", Action: ActionCronJobSuspend, ClusterID: 7, TargetNamespace: "demo", TargetName: "cleanup", TargetUID: "cron-1", TargetResourceVersion: "31", DesiredSuspended: &desired, Status: StatusExecuting}
	kube := &kubernetesStub{}
	repository := &repositoryStub{claimed: plan, shouldExecute: true}
	result, err := NewService(diagnosisStub{}, kube, repository).Execute(context.Background(), plan.ID, "confirmation", "request-key-0002")
	if err != nil || result.Status != StatusSucceeded || len(kube.patches) != 1 || kube.patchKinds[0] != "CronJob" || kube.dryRuns[0] {
		t.Fatalf("result=%#v err=%v patches=%d kinds=%v dry=%v", result, err, len(kube.patches), kube.patchKinds, kube.dryRuns)
	}
}

func TestPreviewImageUpdateStoresTypedDiffAndUsesDryRun(t *testing.T) {
	_, kube := eligibleFixtures()
	kube.deployment.Spec.Template.Spec.Containers = []k8sgateway.WorkloadContainer{{Name: "app", Image: "nginx:1.27.0"}}
	repository := &repositoryStub{}
	service := NewService(diagnosisStub{}, kube, repository)
	service.now = func() time.Time { return time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC) }

	plan, err := service.PreviewOperation(context.Background(), 7, OperationRequest{
		Action: ActionDeploymentImageUpdate, Namespace: "demo", TargetName: "api",
		ContainerName: "app", DesiredImage: "nginx:1.27.1",
	}, ActorRef{ID: 3, Name: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.ContainerName != "app" || plan.BeforeImage != "nginx:1.27.0" || plan.DesiredImage != "nginx:1.27.1" {
		t.Fatalf("plan = %#v", plan)
	}
	if len(kube.patches) != 1 || !kube.dryRuns[0] {
		t.Fatalf("dry runs = %v patches = %d", kube.dryRuns, len(kube.patches))
	}
	if !contains(string(kube.patches[0]), `"image":"nginx:1.27.1"`) || !contains(string(kube.patches[0]), `"uid":"deployment-1"`) {
		t.Fatalf("patch = %s", kube.patches[0])
	}
	response := Response(plan)
	if response.Change == nil || response.Change.Before != "nginx:1.27.0" || response.Change.After != "nginx:1.27.1" {
		t.Fatalf("response = %#v", response)
	}
}

func TestPreviewImageUpdateRejectsMissingContainerAndNoChange(t *testing.T) {
	_, kube := eligibleFixtures()
	kube.deployment.Spec.Template.Spec.Containers = []k8sgateway.WorkloadContainer{{Name: "app", Image: "nginx:1.27.0"}}
	service := NewService(diagnosisStub{}, kube, &repositoryStub{})
	if _, err := service.PreviewOperation(context.Background(), 7, OperationRequest{
		Action: ActionDeploymentImageUpdate, Namespace: "demo", TargetName: "api",
		ContainerName: "missing", DesiredImage: "nginx:1.27.1",
	}, ActorRef{}); !errors.Is(err, ErrInvalidOperation) {
		t.Fatalf("missing container err=%v", err)
	}
	if _, err := service.PreviewOperation(context.Background(), 7, OperationRequest{
		Action: ActionDeploymentImageUpdate, Namespace: "demo", TargetName: "api",
		ContainerName: "app", DesiredImage: "nginx:1.27.0",
	}, ActorRef{}); !errors.Is(err, ErrOperationNoChange) {
		t.Fatalf("no change err=%v", err)
	}
	if len(kube.patches) != 0 {
		t.Fatalf("unexpected patches = %d", len(kube.patches))
	}
}

func TestPreviewRollbackDerivesTemplateFromReplicaSetEvidence(t *testing.T) {
	_, kube := eligibleFixtures()
	kube.deployment.Metadata.Annotations = map[string]string{"deployment.kubernetes.io/revision": "2"}
	kube.rolloutHist = k8sgateway.RolloutHistory{CurrentRevision: 2, Revisions: []k8sgateway.RolloutRevision{
		{Revision: 1, ReplicaSetName: "api-<hash1>", UID: "rs-uid-1", ResourceVersion: "10", Images: []string{"nginx:1.27.0"}, Current: false},
		{Revision: 2, ReplicaSetName: "api-<hash2>", UID: "rs-uid-2", ResourceVersion: "20", Images: []string{"nginx:1.27.1"}, Current: true},
	}}
	kube.replicaSet.Metadata = k8sgateway.ObjectMeta{Name: "api-<hash1>", Namespace: "demo", UID: "rs-uid-1", ResourceVersion: "10"}
	kube.replicaSet.Spec.Template.Spec.Containers = []k8sgateway.WorkloadContainer{{Name: "app", Image: "nginx:1.27.0"}}
	kube.replicaSet.Spec.Template.Raw = rollbackTemplateFixture()
	repository := &repositoryStub{}
	service := NewService(diagnosisStub{}, kube, repository)
	revision := int32(1)

	plan, err := service.PreviewOperation(context.Background(), 7, OperationRequest{
		Action: ActionDeploymentRollback, Namespace: "demo", TargetName: "api", RollbackRevision: &revision,
	}, ActorRef{ID: 3, Name: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.RollbackRevision == nil || *plan.RollbackRevision != 1 || plan.RollbackReplicaSetUID != "rs-uid-1" || plan.RollbackReplicaSetName != "api-<hash1>" {
		t.Fatalf("plan = %#v", plan)
	}
	if len(kube.patches) != 1 || !kube.dryRuns[0] {
		t.Fatalf("dry runs = %v patches = %d", kube.dryRuns, len(kube.patches))
	}
	encoded := string(kube.patches[0])
	if !contains(encoded, `"image":"nginx:1.27.0"`) || !contains(encoded, `"$patch":"replace"`) || !contains(encoded, `"uid":"deployment-1"`) ||
		!contains(encoded, `"initContainers"`) || !contains(encoded, `"volumes"`) || !contains(encoded, `"resources"`) || !contains(encoded, `"existing":"kept"`) {
		t.Fatalf("patch = %s", encoded)
	}
	if contains(encoded, "rs-uid-1") || contains(encoded, "pod-template-hash") {
		t.Fatalf("patch leaked ReplicaSet identity fields: %s", encoded)
	}
}

func TestPreviewRollbackRejectsCurrentRevisionAndMissingRevision(t *testing.T) {
	_, kube := eligibleFixtures()
	kube.deployment.Metadata.Annotations = map[string]string{"deployment.kubernetes.io/revision": "2"}
	kube.rolloutHist = k8sgateway.RolloutHistory{CurrentRevision: 2, Revisions: []k8sgateway.RolloutRevision{
		{Revision: 1, ReplicaSetName: "api-<hash1>", UID: "rs-uid-1", ResourceVersion: "10", Current: false},
		{Revision: 2, ReplicaSetName: "api-<hash2>", UID: "rs-uid-2", ResourceVersion: "20", Current: true},
	}}
	service := NewService(diagnosisStub{}, kube, &repositoryStub{})
	current := int32(2)
	if _, err := service.PreviewOperation(context.Background(), 7, OperationRequest{
		Action: ActionDeploymentRollback, Namespace: "demo", TargetName: "api", RollbackRevision: &current,
	}, ActorRef{}); !errors.Is(err, ErrOperationNoChange) {
		t.Fatalf("current revision err=%v", err)
	}
	missing := int32(99)
	if _, err := service.PreviewOperation(context.Background(), 7, OperationRequest{
		Action: ActionDeploymentRollback, Namespace: "demo", TargetName: "api", RollbackRevision: &missing,
	}, ActorRef{}); !errors.Is(err, ErrRevisionNotFound) {
		t.Fatalf("missing revision err=%v", err)
	}
}

func TestExecuteRollbackRevalidatesReplicaSetAndAppliesTemplate(t *testing.T) {
	_, kube := eligibleFixtures()
	kube.replicaSet.Metadata = k8sgateway.ObjectMeta{Name: "api-<hash1>", Namespace: "demo", UID: "rs-uid-1", ResourceVersion: "10"}
	kube.replicaSet.Spec.Template.Spec.Containers = []k8sgateway.WorkloadContainer{{Name: "app", Image: "nginx:1.27.0"}}
	kube.replicaSet.Spec.Template.Raw = rollbackTemplateFixture()
	revision := int32(1)
	plan := Plan{ID: "12345678-1234-4234-8234-123456789abc", Action: ActionDeploymentRollback, ClusterID: 7,
		TargetNamespace: "demo", TargetName: "api", TargetUID: "deployment-1", TargetResourceVersion: "17",
		RollbackRevision: &revision, RollbackReplicaSetName: "api-<hash1>", RollbackReplicaSetUID: "rs-uid-1",
		RollbackReplicaSetResourceVersion: "10", Status: StatusExecuting}
	repository := &repositoryStub{claimed: plan, shouldExecute: true}
	result, err := NewService(diagnosisStub{}, kube, repository).Execute(context.Background(), plan.ID, "confirmation", "rollback-key-0001")
	if err != nil || result.Status != StatusSucceeded || len(kube.patches) != 1 || kube.dryRuns[0] {
		t.Fatalf("result=%#v err=%v patches=%d dry=%v", result, err, len(kube.patches), kube.dryRuns)
	}
	if !contains(string(kube.patches[0]), `"image":"nginx:1.27.0"`) || !contains(string(kube.patches[0]), `"$patch":"replace"`) ||
		!contains(string(kube.patches[0]), `"initContainers"`) || !contains(string(kube.patches[0]), `"volumes"`) {
		t.Fatalf("patch = %s", kube.patches[0])
	}
}

func rollbackTemplateFixture() json.RawMessage {
	return json.RawMessage(`{
		"metadata":{"labels":{"app":"api","pod-template-hash":"source-hash"},"annotations":{"existing":"kept"}},
		"spec":{
			"serviceAccountName":"runtime",
			"containers":[{"name":"app","image":"nginx:1.27.0","resources":{"limits":{"cpu":"250m"}},"readinessProbe":{"httpGet":{"path":"/ready","port":8080}}}],
			"initContainers":[{"name":"migrate","image":"busybox:1.37","command":["sh","-c","true"]}],
			"volumes":[{"name":"config","emptyDir":{}}]
		}
	}`)
}

func TestExecuteRollbackFailsWhenReplicaSetUIDChanged(t *testing.T) {
	_, kube := eligibleFixtures()
	kube.replicaSet.Metadata = k8sgateway.ObjectMeta{Name: "api-<hash1>", Namespace: "demo", UID: "rs-uid-different", ResourceVersion: "10"}
	revision := int32(1)
	plan := Plan{ID: "12345678-1234-4234-8234-123456789abc", Action: ActionDeploymentRollback, ClusterID: 7,
		TargetNamespace: "demo", TargetName: "api", TargetUID: "deployment-1", TargetResourceVersion: "17",
		RollbackRevision: &revision, RollbackReplicaSetName: "api-<hash1>", RollbackReplicaSetUID: "rs-uid-1",
		RollbackReplicaSetResourceVersion: "10", Status: StatusExecuting}
	repository := &repositoryStub{claimed: plan, shouldExecute: true}
	_, err := NewService(diagnosisStub{}, kube, repository).Execute(context.Background(), plan.ID, "confirmation", "rollback-key-0002")
	if err == nil || !contains(repository.failedMessage, "changed") {
		t.Fatalf("err=%v message=%q", err, repository.failedMessage)
	}
}

func TestRolloutHistoryAndStatusDelegateToKubernetes(t *testing.T) {
	kube := &kubernetesStub{}
	kube.rolloutHist = k8sgateway.RolloutHistory{Deployment: "api", Namespace: "demo", CurrentRevision: 2, Revisions: []k8sgateway.RolloutRevision{
		{Revision: 1, ReplicaSetName: "api-old", Images: []string{"nginx:1.27.0"}},
		{Revision: 2, ReplicaSetName: "api-new", Images: []string{"nginx:1.27.1"}, Current: true},
	}}
	kube.rolloutStat = k8sgateway.RolloutStatus{Deployment: "api", Namespace: "demo", Phase: "complete", DesiredReplicas: 3, ReadyReplicas: 3}
	service := NewService(diagnosisStub{}, kube, &repositoryStub{})
	history, err := service.RolloutHistory(context.Background(), 7, "demo", "api")
	if err != nil || len(history.Revisions) != 2 || history.CurrentRevision != 2 {
		t.Fatalf("history=%#v err=%v", history, err)
	}
	status, err := service.RolloutStatus(context.Background(), 7, "demo", "api")
	if err != nil || status.Phase != "complete" || status.DesiredReplicas != 3 {
		t.Fatalf("status=%#v err=%v", status, err)
	}
}

func contains(value, fragment string) bool {
	for index := 0; index+len(fragment) <= len(value); index++ {
		if value[index:index+len(fragment)] == fragment {
			return true
		}
	}
	return false
}
