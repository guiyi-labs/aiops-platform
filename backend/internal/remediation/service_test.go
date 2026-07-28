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
	pod        k8sgateway.Pod
	deployment k8sgateway.Deployment
	cronJob    k8sgateway.CronJob
	err        error
	patches    [][]byte
	dryRuns    []bool
	patchKinds []string
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
		} `json:"spec"`
	}
	if json.Unmarshal(patch, &body) == nil && body.Spec.Replicas != nil {
		result.Spec.Replicas = body.Spec.Replicas
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

func contains(value, fragment string) bool {
	for index := 0; index+len(fragment) <= len(value); index++ {
		if value[index:index+len(fragment)] == fragment {
			return true
		}
	}
	return false
}
