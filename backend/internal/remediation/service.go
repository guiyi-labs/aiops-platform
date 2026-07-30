package remediation

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"k8s-aiops.local/backend/internal/cluster"
	"k8s-aiops.local/backend/internal/diagnosis"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

var kubernetesNamePattern = regexp.MustCompile(`^[a-z0-9](?:[-a-z0-9.]*[a-z0-9])?$`)

type DiagnosisSource interface {
	Get(context.Context, int64) (diagnosis.Record, error)
}

type KubernetesSource interface {
	Pod(context.Context, int64, string, string) (k8sgateway.Pod, error)
	Deployment(context.Context, int64, string, string) (k8sgateway.Deployment, error)
	PatchDeployment(context.Context, int64, string, string, []byte, bool) (k8sgateway.Deployment, error)
	CronJob(context.Context, int64, string, string) (k8sgateway.CronJob, error)
	PatchCronJob(context.Context, int64, string, string, []byte, bool) (k8sgateway.CronJob, error)
	ReplicaSet(context.Context, int64, string, string) (k8sgateway.ReplicaSet, error)
	ReplicaSetsByOwner(context.Context, int64, string, string) ([]k8sgateway.ReplicaSet, error)
	RolloutHistory(context.Context, int64, string, string) (k8sgateway.RolloutHistory, error)
	RolloutStatus(context.Context, int64, string, string) (k8sgateway.RolloutStatus, error)
}

type OperationRequest struct {
	Action           string
	Namespace        string
	TargetName       string
	DesiredReplicas  *int32
	ContainerName    string
	DesiredImage     string
	RollbackRevision *int32
}

type Service struct {
	diagnoses  DiagnosisSource
	kubernetes KubernetesSource
	repository Repository
	planTTL    time.Duration
	claimTTL   time.Duration
	now        func() time.Time
}

func NewService(diagnoses DiagnosisSource, kubernetes KubernetesSource, repository Repository) *Service {
	return &Service{diagnoses: diagnoses, kubernetes: kubernetes, repository: repository, planTTL: 10 * time.Minute, claimTTL: time.Minute, now: time.Now}
}

func (s *Service) Preview(ctx context.Context, diagnosisID int64, action, targetName string, actor ActorRef) (Plan, error) {
	if action != ActionDeploymentRolloutRestart {
		return Plan{}, ErrUnsupportedAction
	}
	targetName = strings.TrimSpace(targetName)
	if len(targetName) > 253 || !kubernetesNamePattern.MatchString(targetName) {
		return Plan{}, ErrTargetMismatch
	}
	record, err := s.diagnoses.Get(ctx, diagnosisID)
	if err != nil {
		return Plan{}, err
	}
	if record.Status != "confirmed" || record.Resource.Kind != "Pod" || record.Resource.Namespace == "" {
		return Plan{}, ErrDiagnosisNotEligible
	}
	pod, err := s.kubernetes.Pod(ctx, record.ClusterID, record.Resource.Namespace, record.Resource.Name)
	if err != nil {
		return Plan{}, err
	}
	if record.Resource.UID != "" && pod.Metadata.UID != record.Resource.UID {
		return Plan{}, ErrTargetChanged
	}
	deployment, err := s.kubernetes.Deployment(ctx, record.ClusterID, record.Resource.Namespace, targetName)
	if err != nil {
		return Plan{}, err
	}
	if deployment.Metadata.UID == "" || deployment.Metadata.ResourceVersion == "" || !selectorMatches(deployment.Spec.Selector.MatchLabels, pod.Metadata.Labels) {
		return Plan{}, ErrTargetMismatch
	}
	id, token, tokenHash, err := newIdentity()
	if err != nil {
		return Plan{}, err
	}
	now := s.now().UTC().Truncate(time.Second)
	plan := Plan{ID: id, DiagnosisID: &record.ID, ClusterID: record.ClusterID, Action: action, Status: StatusAwaitingConfirmation,
		TargetKind: "Deployment", TargetNamespace: record.Resource.Namespace, TargetName: targetName,
		TargetUID: deployment.Metadata.UID, TargetResourceVersion: deployment.Metadata.ResourceVersion,
		RestartAt: &now, ConfirmationTokenHash: tokenHash, RequestedByUserID: &actor.ID, RequestedByName: actor.Name,
		ExpiresAt: now.Add(s.planTTL)}
	patch, err := patchFor(plan)
	if err != nil {
		return Plan{}, err
	}
	preview, err := s.kubernetes.PatchDeployment(ctx, plan.ClusterID, plan.TargetNamespace, plan.TargetName, patch, true)
	if err != nil {
		return Plan{}, err
	}
	if preview.Metadata.UID != "" && preview.Metadata.UID != plan.TargetUID {
		return Plan{}, ErrTargetChanged
	}
	if err := s.repository.Save(ctx, &plan); err != nil {
		return Plan{}, err
	}
	plan.ConfirmationToken = token
	return plan, nil
}

func (s *Service) PreviewOperation(ctx context.Context, clusterID int64, request OperationRequest, actor ActorRef) (Plan, error) {
	request.Action = strings.TrimSpace(request.Action)
	request.Namespace = strings.TrimSpace(request.Namespace)
	request.TargetName = strings.TrimSpace(request.TargetName)
	request.ContainerName = strings.TrimSpace(request.ContainerName)
	request.DesiredImage = strings.TrimSpace(request.DesiredImage)
	if clusterID < 1 || !validResourceName(request.Namespace, 63) || !validResourceName(request.TargetName, 253) {
		return Plan{}, ErrInvalidOperation
	}

	plan, err := s.newOperationPlan(ctx, clusterID, request, actor)
	if err != nil {
		return Plan{}, err
	}
	patch, err := s.patchForOperation(ctx, plan)
	if err != nil {
		return Plan{}, err
	}
	switch plan.Action {
	case ActionDeploymentScale:
		preview, patchErr := s.kubernetes.PatchDeployment(ctx, clusterID, plan.TargetNamespace, plan.TargetName, patch, true)
		if patchErr != nil {
			return Plan{}, patchErr
		}
		if preview.Metadata.UID != "" && preview.Metadata.UID != plan.TargetUID {
			return Plan{}, ErrTargetChanged
		}
		if preview.Spec.Replicas == nil || plan.DesiredReplicas == nil || *preview.Spec.Replicas != *plan.DesiredReplicas {
			return Plan{}, ErrTargetChanged
		}
	case ActionCronJobSuspend, ActionCronJobResume:
		preview, patchErr := s.kubernetes.PatchCronJob(ctx, clusterID, plan.TargetNamespace, plan.TargetName, patch, true)
		if patchErr != nil {
			return Plan{}, patchErr
		}
		if preview.Metadata.UID != "" && preview.Metadata.UID != plan.TargetUID {
			return Plan{}, ErrTargetChanged
		}
		if preview.Spec.Suspend == nil || plan.DesiredSuspended == nil || *preview.Spec.Suspend != *plan.DesiredSuspended {
			return Plan{}, ErrTargetChanged
		}
	case ActionDeploymentImageUpdate:
		preview, patchErr := s.kubernetes.PatchDeployment(ctx, clusterID, plan.TargetNamespace, plan.TargetName, patch, true)
		if patchErr != nil {
			return Plan{}, patchErr
		}
		if preview.Metadata.UID != "" && preview.Metadata.UID != plan.TargetUID {
			return Plan{}, ErrTargetChanged
		}
		if !previewHasContainerImage(preview, plan.ContainerName, plan.DesiredImage) {
			return Plan{}, ErrTargetChanged
		}
	case ActionDeploymentRollback:
		preview, patchErr := s.kubernetes.PatchDeployment(ctx, clusterID, plan.TargetNamespace, plan.TargetName, patch, true)
		if patchErr != nil {
			return Plan{}, patchErr
		}
		if preview.Metadata.UID != "" && preview.Metadata.UID != plan.TargetUID {
			return Plan{}, ErrTargetChanged
		}
	default:
		return Plan{}, ErrUnsupportedAction
	}
	token := plan.ConfirmationToken
	plan.ConfirmationToken = ""
	if err := s.repository.Save(ctx, &plan); err != nil {
		return Plan{}, err
	}
	plan.ConfirmationToken = token
	return plan, nil
}

func (s *Service) newOperationPlan(ctx context.Context, clusterID int64, request OperationRequest, actor ActorRef) (Plan, error) {
	id, token, tokenHash, err := newIdentity()
	if err != nil {
		return Plan{}, err
	}
	now := s.now().UTC().Truncate(time.Second)
	plan := Plan{ID: id, ClusterID: clusterID, Action: request.Action, Status: StatusAwaitingConfirmation,
		TargetNamespace: request.Namespace, TargetName: request.TargetName, ConfirmationTokenHash: tokenHash,
		RequestedByUserID: &actor.ID, RequestedByName: actor.Name, ExpiresAt: now.Add(s.planTTL), ConfirmationToken: token}

	switch request.Action {
	case ActionDeploymentScale:
		if request.DesiredImage != "" || request.RollbackRevision != nil || request.ContainerName != "" {
			return Plan{}, ErrInvalidOperation
		}
		if request.DesiredReplicas == nil || *request.DesiredReplicas < 0 || *request.DesiredReplicas > 1000 {
			return Plan{}, ErrInvalidOperation
		}
		deployment, err := s.kubernetes.Deployment(ctx, clusterID, request.Namespace, request.TargetName)
		if err != nil {
			return Plan{}, err
		}
		if deployment.Metadata.UID == "" || deployment.Metadata.ResourceVersion == "" {
			return Plan{}, ErrTargetChanged
		}
		before := int32(1)
		if deployment.Spec.Replicas != nil {
			before = *deployment.Spec.Replicas
		}
		if before == *request.DesiredReplicas {
			return Plan{}, ErrOperationNoChange
		}
		plan.TargetKind, plan.TargetUID, plan.TargetResourceVersion = "Deployment", deployment.Metadata.UID, deployment.Metadata.ResourceVersion
		plan.BeforeReplicas, plan.DesiredReplicas = &before, request.DesiredReplicas
	case ActionCronJobSuspend, ActionCronJobResume:
		if request.DesiredReplicas != nil || request.DesiredImage != "" || request.RollbackRevision != nil {
			return Plan{}, ErrInvalidOperation
		}
		cronJob, err := s.kubernetes.CronJob(ctx, clusterID, request.Namespace, request.TargetName)
		if err != nil {
			return Plan{}, err
		}
		if cronJob.Metadata.UID == "" || cronJob.Metadata.ResourceVersion == "" {
			return Plan{}, ErrTargetChanged
		}
		before := cronJob.Spec.Suspend != nil && *cronJob.Spec.Suspend
		desired := request.Action == ActionCronJobSuspend
		if before == desired {
			return Plan{}, ErrOperationNoChange
		}
		plan.TargetKind, plan.TargetUID, plan.TargetResourceVersion = "CronJob", cronJob.Metadata.UID, cronJob.Metadata.ResourceVersion
		plan.BeforeSuspended, plan.DesiredSuspended = &before, &desired
	case ActionDeploymentImageUpdate:
		if request.DesiredReplicas != nil || request.RollbackRevision != nil {
			return Plan{}, ErrInvalidOperation
		}
		if !validResourceName(request.ContainerName, 253) || !validContainerImage(request.DesiredImage) {
			return Plan{}, ErrInvalidOperation
		}
		deployment, err := s.kubernetes.Deployment(ctx, clusterID, request.Namespace, request.TargetName)
		if err != nil {
			return Plan{}, err
		}
		if deployment.Metadata.UID == "" || deployment.Metadata.ResourceVersion == "" {
			return Plan{}, ErrTargetChanged
		}
		beforeImage := ""
		found := false
		for _, container := range deployment.Spec.Template.Spec.Containers {
			if container.Name == request.ContainerName {
				beforeImage, found = container.Image, true
				break
			}
		}
		if !found {
			return Plan{}, ErrInvalidOperation
		}
		if beforeImage == request.DesiredImage {
			return Plan{}, ErrOperationNoChange
		}
		plan.TargetKind, plan.TargetUID, plan.TargetResourceVersion = "Deployment", deployment.Metadata.UID, deployment.Metadata.ResourceVersion
		plan.ContainerName, plan.BeforeImage, plan.DesiredImage = request.ContainerName, beforeImage, request.DesiredImage
	case ActionDeploymentRollback:
		if request.DesiredReplicas != nil || request.DesiredImage != "" || request.ContainerName != "" {
			return Plan{}, ErrInvalidOperation
		}
		if request.RollbackRevision == nil || *request.RollbackRevision < 1 {
			return Plan{}, ErrInvalidOperation
		}
		deployment, err := s.kubernetes.Deployment(ctx, clusterID, request.Namespace, request.TargetName)
		if err != nil {
			return Plan{}, err
		}
		if deployment.Metadata.UID == "" || deployment.Metadata.ResourceVersion == "" {
			return Plan{}, ErrTargetChanged
		}
		history, err := s.kubernetes.RolloutHistory(ctx, clusterID, request.Namespace, request.TargetName)
		if err != nil {
			return Plan{}, err
		}
		var targetRevision *k8sgateway.RolloutRevision
		for index, revision := range history.Revisions {
			if revision.Revision == *request.RollbackRevision {
				targetRevision = &history.Revisions[index]
				break
			}
		}
		if targetRevision == nil {
			return Plan{}, ErrRevisionNotFound
		}
		if targetRevision.Current {
			return Plan{}, ErrOperationNoChange
		}
		plan.TargetKind, plan.TargetUID, plan.TargetResourceVersion = "Deployment", deployment.Metadata.UID, deployment.Metadata.ResourceVersion
		plan.RollbackRevision = request.RollbackRevision
		plan.RollbackReplicaSetName = targetRevision.ReplicaSetName
		plan.RollbackReplicaSetUID, plan.RollbackReplicaSetResourceVersion = targetRevision.UID, targetRevision.ResourceVersion
	default:
		return Plan{}, ErrUnsupportedAction
	}
	return plan, nil
}

func (s *Service) List(ctx context.Context, diagnosisID int64) ([]Plan, error) {
	if _, err := s.diagnoses.Get(ctx, diagnosisID); err != nil {
		return nil, err
	}
	return s.repository.List(ctx, diagnosisID)
}

func (s *Service) ListOperations(ctx context.Context, clusterID int64, namespace, kind, name string) ([]Plan, error) {
	namespace, kind, name = strings.TrimSpace(namespace), strings.TrimSpace(kind), strings.TrimSpace(name)
	if clusterID < 1 || (namespace != "" && !validResourceName(namespace, 63)) ||
		(name != "" && !validResourceName(name, 253)) || (kind != "" && kind != "Deployment" && kind != "CronJob") {
		return nil, ErrInvalidOperation
	}
	return s.repository.ListOperations(ctx, clusterID, namespace, kind, name)
}

func (s *Service) patchForOperation(ctx context.Context, plan Plan) ([]byte, error) {
	if plan.Action == ActionDeploymentRollback {
		return s.buildRollbackPatch(ctx, plan)
	}
	return patchFor(plan)
}

func (s *Service) Execute(ctx context.Context, id, confirmationToken, idempotencyKey string) (Plan, error) {
	id, confirmationToken, idempotencyKey = strings.TrimSpace(id), strings.TrimSpace(confirmationToken), strings.TrimSpace(idempotencyKey)
	if id == "" || confirmationToken == "" {
		return Plan{}, ErrConfirmationInvalid
	}
	if len(idempotencyKey) < 8 || len(idempotencyKey) > 128 {
		return Plan{}, ErrInvalidIdempotency
	}
	tokenHash := sha256.Sum256([]byte(confirmationToken))
	now := s.now().UTC()
	plan, shouldExecute, err := s.repository.Claim(ctx, id, tokenHash[:], idempotencyKey, now, now.Add(-s.claimTTL))
	if err != nil || !shouldExecute {
		return plan, err
	}
	patch, err := s.patchForOperation(ctx, plan)
	if err == nil {
		switch plan.Action {
		case ActionDeploymentRolloutRestart, ActionDeploymentScale, ActionDeploymentImageUpdate, ActionDeploymentRollback:
			_, err = s.kubernetes.PatchDeployment(ctx, plan.ClusterID, plan.TargetNamespace, plan.TargetName, patch, false)
		case ActionCronJobSuspend, ActionCronJobResume:
			_, err = s.kubernetes.PatchCronJob(ctx, plan.ClusterID, plan.TargetNamespace, plan.TargetName, patch, false)
		default:
			err = ErrUnsupportedAction
		}
	}
	if err != nil {
		failed, saveErr := s.repository.Fail(ctx, plan.ID, idempotencyKey, safeExecutionError(err))
		if saveErr != nil {
			return Plan{}, saveErr
		}
		return failed, fmt.Errorf("%w: %v", ErrExecutionFailed, err)
	}
	return s.repository.Complete(ctx, plan.ID, idempotencyKey, s.now().UTC())
}

func (s *Service) RolloutHistory(ctx context.Context, clusterID int64, namespace, name string) (k8sgateway.RolloutHistory, error) {
	if clusterID < 1 || !validResourceName(namespace, 63) || !validResourceName(name, 253) {
		return k8sgateway.RolloutHistory{}, ErrInvalidOperation
	}
	return s.kubernetes.RolloutHistory(ctx, clusterID, namespace, name)
}

func (s *Service) RolloutStatus(ctx context.Context, clusterID int64, namespace, name string) (k8sgateway.RolloutStatus, error) {
	if clusterID < 1 || !validResourceName(namespace, 63) || !validResourceName(name, 253) {
		return k8sgateway.RolloutStatus{}, ErrInvalidOperation
	}
	return s.kubernetes.RolloutStatus(ctx, clusterID, namespace, name)
}

func selectorMatches(selector, labels map[string]string) bool {
	if len(selector) == 0 {
		return false
	}
	for key, value := range selector {
		if labels[key] != value {
			return false
		}
	}
	return true
}

func validResourceName(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && kubernetesNamePattern.MatchString(value)
}

func validContainerImage(image string) bool {
	if image == "" || len(image) > 512 {
		return false
	}
	for _, ch := range image {
		if ch < 0x20 || ch == 0x7f {
			return false
		}
	}
	return true
}

func previewHasContainerImage(deployment k8sgateway.Deployment, containerName, desiredImage string) bool {
	for _, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name == containerName && container.Image == desiredImage {
			return true
		}
	}
	return false
}

func patchFor(plan Plan) ([]byte, error) {
	metadata := map[string]any{"uid": plan.TargetUID, "resourceVersion": plan.TargetResourceVersion}
	switch plan.Action {
	case ActionDeploymentRolloutRestart:
		if plan.RestartAt == nil {
			return nil, ErrInvalidOperation
		}
		return json.Marshal(map[string]any{
			"metadata": metadata,
			"spec": map[string]any{"template": map[string]any{"metadata": map[string]any{"annotations": map[string]string{
				"k8s-aiops.local/remediation-id": plan.ID,
				"k8s-aiops.local/restarted-at":   plan.RestartAt.UTC().Format(time.RFC3339),
			}}}},
		})
	case ActionDeploymentScale:
		if plan.DesiredReplicas == nil || *plan.DesiredReplicas < 0 || *plan.DesiredReplicas > 1000 {
			return nil, ErrInvalidOperation
		}
		return json.Marshal(map[string]any{"metadata": metadata, "spec": map[string]any{"replicas": *plan.DesiredReplicas}})
	case ActionCronJobSuspend, ActionCronJobResume:
		if plan.DesiredSuspended == nil || *plan.DesiredSuspended != (plan.Action == ActionCronJobSuspend) {
			return nil, ErrInvalidOperation
		}
		return json.Marshal(map[string]any{"metadata": metadata, "spec": map[string]any{"suspend": *plan.DesiredSuspended}})
	case ActionDeploymentImageUpdate:
		if plan.ContainerName == "" || plan.DesiredImage == "" {
			return nil, ErrInvalidOperation
		}
		return json.Marshal(map[string]any{
			"metadata": metadata,
			"spec": map[string]any{"template": map[string]any{"spec": map[string]any{
				"containers": []map[string]string{{"name": plan.ContainerName, "image": plan.DesiredImage}},
			}}},
		})
	default:
		return nil, ErrUnsupportedAction
	}
}

func (s *Service) buildRollbackPatch(ctx context.Context, plan Plan) ([]byte, error) {
	if plan.RollbackRevision == nil || plan.RollbackReplicaSetName == "" || plan.RollbackReplicaSetUID == "" {
		return nil, ErrInvalidOperation
	}
	replicaSet, err := s.kubernetes.ReplicaSet(ctx, plan.ClusterID, plan.TargetNamespace, plan.RollbackReplicaSetName)
	if err != nil {
		return nil, err
	}
	if replicaSet.Metadata.UID != plan.RollbackReplicaSetUID {
		return nil, ErrTargetChanged
	}
	if plan.RollbackReplicaSetResourceVersion != "" && replicaSet.Metadata.ResourceVersion != plan.RollbackReplicaSetResourceVersion {
		return nil, ErrTargetChanged
	}
	if len(replicaSet.Spec.Template.Raw) == 0 {
		return nil, ErrInvalidOperation
	}
	var template map[string]any
	if err := json.Unmarshal(replicaSet.Spec.Template.Raw, &template); err != nil {
		return nil, ErrInvalidOperation
	}
	template["$patch"] = "replace"
	metadata, _ := template["metadata"].(map[string]any)
	if metadata == nil {
		metadata = map[string]any{}
	}
	for _, field := range []string{"name", "namespace", "uid", "resourceVersion", "creationTimestamp", "generation", "managedFields", "ownerReferences", "finalizers"} {
		delete(metadata, field)
	}
	labels, _ := metadata["labels"].(map[string]any)
	if labels != nil {
		delete(labels, "pod-template-hash")
		metadata["labels"] = labels
	}
	annotations, _ := metadata["annotations"].(map[string]any)
	if annotations == nil {
		annotations = map[string]any{}
	}
	annotations["k8s-aiops.local/rollback-revision"] = fmt.Sprintf("%d", *plan.RollbackRevision)
	metadata["annotations"] = annotations
	template["metadata"] = metadata
	return json.Marshal(map[string]any{
		"metadata": map[string]any{"uid": plan.TargetUID, "resourceVersion": plan.TargetResourceVersion},
		"spec":     map[string]any{"template": template},
	})
}

func newIdentity() (string, string, []byte, error) {
	idBytes := make([]byte, 16)
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(idBytes); err != nil {
		return "", "", nil, err
	}
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", "", nil, err
	}
	idBytes[6] = (idBytes[6] & 0x0f) | 0x40
	idBytes[8] = (idBytes[8] & 0x3f) | 0x80
	hexID := hex.EncodeToString(idBytes)
	id := hexID[0:8] + "-" + hexID[8:12] + "-" + hexID[12:16] + "-" + hexID[16:20] + "-" + hexID[20:32]
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	hash := sha256.Sum256([]byte(token))
	return id, token, hash[:], nil
}

func safeExecutionError(err error) string {
	var status cluster.APIStatusError
	if errors.As(err, &status) {
		return fmt.Sprintf("Kubernetes API rejected remediation with HTTP %d", status.StatusCode)
	}
	if errors.Is(err, k8sgateway.ErrResourceNotFound) {
		return "Kubernetes remediation target was not found"
	}
	if errors.Is(err, ErrTargetChanged) {
		return "Kubernetes remediation target changed after diagnosis"
	}
	return "Kubernetes remediation request failed"
}
