package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s-aiops.local/backend/internal/apiquery"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

// --- stubs ---

type kubernetesStub struct {
	mu sync.Mutex

	node    k8sgateway.Node
	nodeErr error

	pods    []k8sgateway.Pod
	podsErr error

	pdbs    []k8sgateway.PodDisruptionBudget
	pdbsErr error

	patchNodeCalled int
	patchNodeDryRun []bool
	patchNodeBody   [][]byte
	patchNodeResp   k8sgateway.Node
	patchNodeErr    error

	createCalled   int
	createPaths    []string
	createBodies   [][]byte
	createDryRun   []bool
	createResponse []byte
	createErr      error
}

func (s *kubernetesStub) Node(context.Context, int64, string) (k8sgateway.Node, error) {
	return s.node, s.nodeErr
}

func (s *kubernetesStub) Pods(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Pod], error) {
	return apiquery.ListResponse[k8sgateway.Pod]{Items: s.pods}, s.podsErr
}

func (s *kubernetesStub) PodDisruptionBudgets(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.PodDisruptionBudget], error) {
	return apiquery.ListResponse[k8sgateway.PodDisruptionBudget]{Items: s.pdbs}, s.pdbsErr
}

func (s *kubernetesStub) PatchNode(_ context.Context, _ int64, _ string, body []byte, dryRun bool) (k8sgateway.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.patchNodeCalled++
	s.patchNodeDryRun = append(s.patchNodeDryRun, dryRun)
	s.patchNodeBody = append(s.patchNodeBody, append([]byte(nil), body...))
	return s.patchNodeResp, s.patchNodeErr
}

func (s *kubernetesStub) CreateResource(_ context.Context, _ int64, path string, body []byte, dryRun bool) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.createCalled++
	s.createPaths = append(s.createPaths, path)
	s.createBodies = append(s.createBodies, append([]byte(nil), body...))
	s.createDryRun = append(s.createDryRun, dryRun)
	return s.createResponse, s.createErr
}

type repositoryStub struct {
	saved         *Plan
	listPlans     []Plan
	listErr       error
	claimed       Plan
	shouldExecute bool
	claimErr      error
	completed     bool
	completedPlan Plan
	failedMessage string
	failedResult  *ExecutionResultJSON
	failErr       error
}

func (r *repositoryStub) Save(_ context.Context, plan *Plan) error {
	saved := *plan
	r.saved = &saved
	return nil
}

func (r *repositoryStub) List(context.Context, int64) ([]Plan, error) {
	return r.listPlans, r.listErr
}

func (r *repositoryStub) Claim(context.Context, string, []byte, string, time.Time, time.Time) (Plan, bool, error) {
	if r.claimed.PreviewEvidence.NodeResourceVersion == "" {
		r.claimed.PreviewEvidence.NodeResourceVersion = "100"
	}
	return r.claimed, r.shouldExecute, r.claimErr
}

func (r *repositoryStub) Complete(_ context.Context, _, _ string, _ time.Time, plan Plan, result *ExecutionResultJSON) (Plan, error) {
	r.completed = true
	r.completedPlan = plan
	if result != nil {
		plan.ExecutionResult = result
	}
	plan.Status = StatusSucceeded
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

func makeService(kube *kubernetesStub, repo *repositoryStub) *Service {
	svc := NewService(kube, repo)
	svc.now = func() time.Time { return time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC) }
	return svc
}

func workerNode(unschedulable bool) k8sgateway.Node {
	n := k8sgateway.Node{}
	n.Metadata.Name = "worker-1"
	n.Metadata.UID = "node-uid-1"
	n.Metadata.ResourceVersion = "100"
	n.Metadata.Labels = map[string]string{"node-role.kubernetes.io/worker": "true"}
	n.Spec.Unschedulable = unschedulable
	return n
}

func controlPlaneNode() k8sgateway.Node {
	n := workerNode(false)
	n.Metadata.Labels = map[string]string{"node-role.kubernetes.io/control-plane": "true"}
	return n
}

func makePod(name, namespace, nodeName, ownerKind string) k8sgateway.Pod {
	var p k8sgateway.Pod
	p.Metadata.Name = name
	p.Metadata.Namespace = namespace
	p.Metadata.UID = "pod-uid-" + name
	p.Metadata.ResourceVersion = "rv-" + name
	p.Spec.NodeName = nodeName
	if ownerKind != "" {
		controller := true
		p.Metadata.OwnerReferences = []k8sgateway.OwnerReference{
			{Kind: ownerKind, Name: ownerKind + "-" + name, Controller: &controller},
		}
	}
	return p
}

func makePDB(namespace, name string, disruptions int32) k8sgateway.PodDisruptionBudget {
	var pdb k8sgateway.PodDisruptionBudget
	pdb.Metadata.Name = name
	pdb.Metadata.Namespace = namespace
	pdb.Spec.Selector = &metav1.LabelSelector{}
	pdb.Status.DisruptionsAllowed = disruptions
	return pdb
}

// --- Preview tests ---

func TestPreview_InvalidRequest(t *testing.T) {
	cases := []struct {
		name    string
		request Request
		wantErr error
	}{
		{"empty action", Request{NodeName: "worker-1"}, ErrInvalidRequest},
		{"invalid action", Request{Action: "restart", NodeName: "worker-1"}, ErrInvalidRequest},
		{"empty node name", Request{Action: ActionCordon}, ErrInvalidRequest},
		{"too long node name", Request{Action: ActionCordon, NodeName: strings.Repeat("a", 254)}, ErrInvalidRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := makeService(&kubernetesStub{}, &repositoryStub{})
			_, err := svc.Preview(context.Background(), 1, tc.request, ActorRef{ID: 1, Name: "alice"})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("want %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestPreview_InvalidClusterID(t *testing.T) {
	svc := makeService(&kubernetesStub{}, &repositoryStub{})
	_, err := svc.Preview(context.Background(), 0, Request{Action: ActionCordon, NodeName: "worker-1"}, ActorRef{ID: 1, Name: "alice"})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("want ErrInvalidRequest, got %v", err)
	}
}

func TestPreview_NodeLookupError(t *testing.T) {
	kube := &kubernetesStub{nodeErr: errors.New("api unavailable")}
	svc := makeService(kube, &repositoryStub{})
	_, err := svc.Preview(context.Background(), 1, Request{Action: ActionCordon, NodeName: "worker-1"}, ActorRef{ID: 1, Name: "alice"})
	if err == nil {
		t.Fatal("want error from node lookup, got nil")
	}
}

func TestPreview_ControlPlaneRejected(t *testing.T) {
	kube := &kubernetesStub{node: controlPlaneNode()}
	svc := makeService(kube, &repositoryStub{})
	_, err := svc.Preview(context.Background(), 1, Request{Action: ActionCordon, NodeName: "cp-1"}, ActorRef{ID: 1, Name: "alice"})
	if !errors.Is(err, ErrControlPlaneNode) {
		t.Fatalf("want ErrControlPlaneNode, got %v", err)
	}
}

func TestPreview_CordonAlreadyCordoned(t *testing.T) {
	kube := &kubernetesStub{node: workerNode(true)}
	svc := makeService(kube, &repositoryStub{})
	_, err := svc.Preview(context.Background(), 1, Request{Action: ActionCordon, NodeName: "worker-1"}, ActorRef{ID: 1, Name: "alice"})
	if !errors.Is(err, ErrAlreadyCordoned) {
		t.Fatalf("want ErrAlreadyCordoned, got %v", err)
	}
}

func TestPreview_UncordonAlreadyUncordoned(t *testing.T) {
	kube := &kubernetesStub{node: workerNode(false)}
	svc := makeService(kube, &repositoryStub{})
	_, err := svc.Preview(context.Background(), 1, Request{Action: ActionUncordon, NodeName: "worker-1"}, ActorRef{ID: 1, Name: "alice"})
	if !errors.Is(err, ErrAlreadyUncordoned) {
		t.Fatalf("want ErrAlreadyUncordoned, got %v", err)
	}
}

func TestPreview_DrainNotCordoned(t *testing.T) {
	kube := &kubernetesStub{node: workerNode(false)}
	svc := makeService(kube, &repositoryStub{})
	_, err := svc.Preview(context.Background(), 1, Request{Action: ActionDrain, NodeName: "worker-1"}, ActorRef{ID: 1, Name: "alice"})
	if !errors.Is(err, ErrNotCordoned) {
		t.Fatalf("want ErrNotCordoned, got %v", err)
	}
}

func TestPreview_DrainBlockingUnmanagedPod(t *testing.T) {
	kube := &kubernetesStub{
		node: workerNode(true),
		pods: []k8sgateway.Pod{makePod("lonely", "default", "worker-1", "")},
	}
	svc := makeService(kube, &repositoryStub{})
	_, err := svc.Preview(context.Background(), 1, Request{Action: ActionDrain, NodeName: "worker-1"}, ActorRef{ID: 1, Name: "alice"})
	if !errors.Is(err, ErrUnmanagedPod) {
		t.Fatalf("want ErrUnmanagedPod, got %v", err)
	}
}

func TestPreview_DrainBlockingEmptyDirPod(t *testing.T) {
	pod := makePod("ed-pod", "default", "worker-1", "ReplicaSet")
	emptyDir := json.RawMessage(`{}`)
	pod.Spec.Volumes = []k8sgateway.PodVolume{{Name: "cache", EmptyDir: &emptyDir}}
	kube := &kubernetesStub{
		node: workerNode(true),
		pods: []k8sgateway.Pod{pod},
	}
	svc := makeService(kube, &repositoryStub{})
	_, err := svc.Preview(context.Background(), 1, Request{Action: ActionDrain, NodeName: "worker-1"}, ActorRef{ID: 1, Name: "alice"})
	if !errors.Is(err, ErrEmptyDirPod) {
		t.Fatalf("want ErrEmptyDirPod, got %v", err)
	}
}

func TestPreview_DrainBlockingPDBUnavailable(t *testing.T) {
	kube := &kubernetesStub{
		node: workerNode(true),
		pods: []k8sgateway.Pod{makePod("managed", "default", "worker-1", "ReplicaSet")},
		// no PDBs returned
	}
	svc := makeService(kube, &repositoryStub{})
	_, err := svc.Preview(context.Background(), 1, Request{Action: ActionDrain, NodeName: "worker-1"}, ActorRef{ID: 1, Name: "alice"})
	if !errors.Is(err, ErrPDBUnavailable) {
		t.Fatalf("want ErrPDBUnavailable, got %v", err)
	}
}

func TestPreview_DrainPDBLookupError(t *testing.T) {
	kube := &kubernetesStub{
		node:    workerNode(true),
		pods:    []k8sgateway.Pod{makePod("managed", "default", "worker-1", "ReplicaSet")},
		pdbsErr: errors.New("pdb api unavailable"),
	}
	svc := makeService(kube, &repositoryStub{})
	_, err := svc.Preview(context.Background(), 1, Request{Action: ActionDrain, NodeName: "worker-1"}, ActorRef{ID: 1, Name: "alice"})
	if !errors.Is(err, ErrPDBUnavailable) {
		t.Fatalf("want ErrPDBUnavailable, got %v", err)
	}
}

func TestPreview_DrainTooManyEvictable(t *testing.T) {
	var pods []k8sgateway.Pod
	for i := 0; i < maxEvictablePods+1; i++ {
		pods = append(pods, makePod("pod-"+string(rune('a'+i)), "default", "worker-1", "ReplicaSet"))
	}
	kube := &kubernetesStub{
		node: workerNode(true),
		pods: pods,
		pdbs: []k8sgateway.PodDisruptionBudget{makePDB("default", "pdb-default", 1)},
	}
	svc := makeService(kube, &repositoryStub{})
	_, err := svc.Preview(context.Background(), 1, Request{Action: ActionDrain, NodeName: "worker-1"}, ActorRef{ID: 1, Name: "alice"})
	if !errors.Is(err, ErrTooManyPods) {
		t.Fatalf("want ErrTooManyPods, got %v", err)
	}
}

func TestPreview_CordonSuccess(t *testing.T) {
	patched := workerNode(true)
	kube := &kubernetesStub{
		node:          workerNode(false),
		patchNodeResp: patched,
	}
	repo := &repositoryStub{}
	svc := makeService(kube, repo)
	plan, err := svc.Preview(context.Background(), 1, Request{Action: ActionCordon, NodeName: "worker-1"}, ActorRef{ID: 7, Name: "alice"})
	if err != nil {
		t.Fatalf("want nil error, got %v", err)
	}
	if plan.Action != ActionCordon {
		t.Errorf("Action = %q", plan.Action)
	}
	if plan.Status != StatusAwaitingConfirmation {
		t.Errorf("Status = %q", plan.Status)
	}
	if plan.NodeName != "worker-1" {
		t.Errorf("NodeName = %q", plan.NodeName)
	}
	if plan.ConfirmationToken == "" {
		t.Error("confirmation token should be populated for caller")
	}
	if len(kube.patchNodeDryRun) != 1 || !kube.patchNodeDryRun[0] {
		t.Errorf("expected one dry-run patch, got %v", kube.patchNodeDryRun)
	}
	if repo.saved == nil {
		t.Fatal("plan not saved")
	}
	if repo.saved.ConfirmationToken != "" {
		t.Error("persisted plan should not retain confirmation token")
	}
	// token hash should be non-zero
	if len(repo.saved.ConfirmationTokenHash) == 0 {
		t.Error("confirmation token hash should be persisted")
	}
}

func TestPreview_DryRunPatchFailure(t *testing.T) {
	kube := &kubernetesStub{
		node:         workerNode(false),
		patchNodeErr: errors.New("patch forbidden"),
	}
	svc := makeService(kube, &repositoryStub{})
	_, err := svc.Preview(context.Background(), 1, Request{Action: ActionCordon, NodeName: "worker-1"}, ActorRef{ID: 1, Name: "alice"})
	if err == nil {
		t.Fatal("want error from dry-run patch, got nil")
	}
}

func TestPreview_DrainSuccess(t *testing.T) {
	kube := &kubernetesStub{
		node: workerNode(true),
		pods: []k8sgateway.Pod{
			makePod("ds-pod", "kube-system", "worker-1", "DaemonSet"),
			makePod("managed-pod", "default", "worker-1", "ReplicaSet"),
		},
		pdbs: []k8sgateway.PodDisruptionBudget{makePDB("default", "pdb-default", 1)},
	}
	repo := &repositoryStub{}
	svc := makeService(kube, repo)
	plan, err := svc.Preview(context.Background(), 1, Request{Action: ActionDrain, NodeName: "worker-1"}, ActorRef{ID: 7, Name: "alice"})
	if err != nil {
		t.Fatalf("want nil error, got %v", err)
	}
	evidence := PreviewEvidence(plan.PreviewEvidence)
	if evidence.RetainedCount != 1 {
		t.Errorf("RetainedCount = %d", evidence.RetainedCount)
	}
	if evidence.EvictableCount != 1 {
		t.Errorf("EvictableCount = %d", evidence.EvictableCount)
	}
	if evidence.BlockingCount != 0 {
		t.Errorf("BlockingCount = %d", evidence.BlockingCount)
	}
	// drain should not perform dry-run node patch
	if kube.patchNodeCalled != 0 {
		t.Errorf("drain preview should not patch node, got %d calls", kube.patchNodeCalled)
	}
}

// --- Execute tests ---

func TestExecute_InvalidInputs(t *testing.T) {
	cases := []struct {
		name           string
		planID         string
		token          string
		idempotencyKey string
		wantErr        error
	}{
		{"empty plan id", "", "token", "key-12345", ErrConfirmationInvalid},
		{"empty token", "plan-id", "  ", "key-12345", ErrConfirmationInvalid},
		{"short idempotency key", "plan-id", "token", "short", ErrInvalidIdempotency},
		{"long idempotency key", "plan-id", "token", strings.Repeat("k", 129), ErrInvalidIdempotency},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := makeService(&kubernetesStub{}, &repositoryStub{})
			_, err := svc.Execute(context.Background(), tc.planID, tc.token, tc.idempotencyKey)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("want %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestExecute_ClaimError(t *testing.T) {
	repo := &repositoryStub{claimErr: ErrExpired}
	svc := makeService(&kubernetesStub{}, repo)
	_, err := svc.Execute(context.Background(), "plan-id", "token", "key-12345")
	if !errors.Is(err, ErrExpired) {
		t.Fatalf("want ErrExpired, got %v", err)
	}
}

func TestExecute_ClaimNoExecute(t *testing.T) {
	// Claim returns shouldExecute=false with no error (e.g. successful replay)
	repo := &repositoryStub{shouldExecute: false}
	svc := makeService(&kubernetesStub{}, repo)
	_, err := svc.Execute(context.Background(), "plan-id", "token", "key-12345")
	if err != nil {
		t.Fatalf("want nil error on replay, got %v", err)
	}
}

func TestExecute_NodeUIDMismatch(t *testing.T) {
	currentNode := workerNode(true)
	currentNode.Metadata.UID = "different-uid"
	kube := &kubernetesStub{node: currentNode}
	plan := Plan{
		ID:              "plan-1",
		ClusterID:       1,
		Action:          ActionCordon,
		NodeName:        "worker-1",
		PreviewEvidence: PreviewEvidenceJSON(PreviewEvidence{NodeUID: "original-uid"}),
	}
	repo := &repositoryStub{claimed: plan, shouldExecute: true}
	svc := makeService(kube, repo)
	_, err := svc.Execute(context.Background(), "plan-1", "token", "key-12345")
	if !errors.Is(err, ErrStaleTarget) {
		t.Fatalf("want ErrStaleTarget, got %v", err)
	}
	if repo.failedMessage == "" {
		t.Error("Fail should be called on UID mismatch")
	}
}

func TestExecute_NodeLookupError(t *testing.T) {
	kube := &kubernetesStub{nodeErr: errors.New("api down")}
	plan := Plan{
		ID:              "plan-1",
		ClusterID:       1,
		Action:          ActionCordon,
		NodeName:        "worker-1",
		PreviewEvidence: PreviewEvidenceJSON(PreviewEvidence{NodeUID: "node-uid-1"}),
	}
	repo := &repositoryStub{claimed: plan, shouldExecute: true}
	svc := makeService(kube, repo)
	_, err := svc.Execute(context.Background(), "plan-1", "token", "key-12345")
	if !errors.Is(err, ErrExecutionFailed) {
		t.Fatalf("want ErrExecutionFailed, got %v", err)
	}
}

func TestExecute_CordonSuccess(t *testing.T) {
	patched := workerNode(true)
	kube := &kubernetesStub{node: workerNode(false), patchNodeResp: patched}
	plan := Plan{
		ID:              "plan-1",
		ClusterID:       1,
		Action:          ActionCordon,
		NodeName:        "worker-1",
		PreviewEvidence: PreviewEvidenceJSON(PreviewEvidence{NodeUID: "node-uid-1"}),
	}
	repo := &repositoryStub{claimed: plan, shouldExecute: true}
	svc := makeService(kube, repo)
	result, err := svc.Execute(context.Background(), "plan-1", "token", "key-12345")
	if err != nil {
		t.Fatalf("want nil error, got %v", err)
	}
	if !repo.completed {
		t.Error("Complete should be called")
	}
	if len(kube.patchNodeDryRun) != 1 || kube.patchNodeDryRun[0] {
		t.Errorf("expected one non-dry-run patch, got %v", kube.patchNodeDryRun)
	}
	if result.ExecutionResult == nil || !result.ExecutionResult.NodePatched {
		t.Error("execution result should record node_patched=true")
	}
}

func TestExecute_UncordonSuccess(t *testing.T) {
	patched := workerNode(false)
	kube := &kubernetesStub{node: workerNode(true), patchNodeResp: patched}
	plan := Plan{
		ID:              "plan-1",
		ClusterID:       1,
		Action:          ActionUncordon,
		NodeName:        "worker-1",
		PreviewEvidence: PreviewEvidenceJSON(PreviewEvidence{NodeUID: "node-uid-1"}),
	}
	repo := &repositoryStub{claimed: plan, shouldExecute: true}
	svc := makeService(kube, repo)
	result, err := svc.Execute(context.Background(), "plan-1", "token", "key-12345")
	if err != nil {
		t.Fatalf("want nil error, got %v", err)
	}
	if !repo.completed {
		t.Error("Complete should be called")
	}
	if result.ExecutionResult == nil || result.ExecutionResult.UnschedulableNow {
		t.Error("execution result should record unschedulable_now=false")
	}
}

func TestExecute_CordonPatchFailure(t *testing.T) {
	kube := &kubernetesStub{
		node:         workerNode(false),
		patchNodeErr: errors.New("patch forbidden"),
	}
	plan := Plan{
		ID:              "plan-1",
		ClusterID:       1,
		Action:          ActionCordon,
		NodeName:        "worker-1",
		PreviewEvidence: PreviewEvidenceJSON(PreviewEvidence{NodeUID: "node-uid-1"}),
	}
	repo := &repositoryStub{claimed: plan, shouldExecute: true}
	svc := makeService(kube, repo)
	_, err := svc.Execute(context.Background(), "plan-1", "token", "key-12345")
	if !errors.Is(err, ErrExecutionFailed) {
		t.Fatalf("want ErrExecutionFailed, got %v", err)
	}
	if repo.failedMessage == "" {
		t.Error("Fail should be called on patch error")
	}
}

func TestExecute_RejectsChangedNodeResourceVersion(t *testing.T) {
	node := workerNode(false)
	node.Metadata.ResourceVersion = "101"
	kube := &kubernetesStub{node: node}
	plan := Plan{ID: "plan-1", ClusterID: 1, Action: ActionCordon, NodeName: "worker-1", PreviewEvidence: PreviewEvidenceJSON(PreviewEvidence{NodeUID: "node-uid-1", NodeResourceVersion: "100"})}
	repo := &repositoryStub{claimed: plan, shouldExecute: true}
	_, err := makeService(kube, repo).Execute(context.Background(), "plan-1", "token", "key-12345")
	if !errors.Is(err, ErrStaleTarget) || kube.patchNodeCalled != 0 {
		t.Fatalf("changed Node resourceVersion must fail before mutation: err=%v patches=%d", err, kube.patchNodeCalled)
	}
}

func TestExecute_DrainCordonFailureStopsEviction(t *testing.T) {
	pod := makePod("app", "default", "worker-1", "ReplicaSet")
	kube := &kubernetesStub{node: workerNode(false), pods: []k8sgateway.Pod{pod}, pdbs: []k8sgateway.PodDisruptionBudget{makePDB("default", "api", 1)}, patchNodeErr: errors.New("patch denied")}
	svc := makeService(kube, &repositoryStub{})
	evidence, err := svc.collectEvidence(context.Background(), 1, "worker-1", kube.node)
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{ID: "plan-1", ClusterID: 1, Action: ActionDrain, NodeName: "worker-1", PreviewEvidence: PreviewEvidenceJSON(evidence)}
	repo := &repositoryStub{claimed: plan, shouldExecute: true}
	svc.repository = repo
	_, err = svc.Execute(context.Background(), "plan-1", "token", "key-12345")
	if !errors.Is(err, ErrExecutionFailed) || kube.createCalled != 0 {
		t.Fatalf("cordon failure must prevent eviction: err=%v evictions=%d", err, kube.createCalled)
	}
}

func TestExecute_DrainStalePodEvidence(t *testing.T) {
	kube := &kubernetesStub{
		node: workerNode(true),
		pods: []k8sgateway.Pod{
			makePod("extra-pod", "default", "worker-1", "ReplicaSet"),
		},
		pdbs: []k8sgateway.PodDisruptionBudget{makePDB("default", "pdb-default", 1)},
	}
	preview := PreviewEvidence{
		NodeUID:        "node-uid-1",
		Pods:           []PodEvidence{}, // no pods at preview time
		EvictableCount: 0,
	}
	plan := Plan{
		ID:              "plan-1",
		ClusterID:       1,
		Action:          ActionDrain,
		NodeName:        "worker-1",
		PreviewEvidence: PreviewEvidenceJSON(preview),
	}
	repo := &repositoryStub{claimed: plan, shouldExecute: true}
	svc := makeService(kube, repo)
	_, err := svc.Execute(context.Background(), "plan-1", "token", "key-12345")
	if !errors.Is(err, ErrStaleTarget) {
		t.Fatalf("want ErrStaleTarget, got %v", err)
	}
}

func TestExecute_DrainSuccess(t *testing.T) {
	evictablePod := makePod("app-pod", "default", "worker-1", "ReplicaSet")
	kube := &kubernetesStub{
		node: workerNode(true),
		pods: []k8sgateway.Pod{evictablePod},
		pdbs: []k8sgateway.PodDisruptionBudget{makePDB("default", "pdb-default", 1)},
	}
	preview := PreviewEvidence{
		NodeUID: "node-uid-1",
		Pods: []PodEvidence{
			{
				Name:            "app-pod",
				Namespace:       "default",
				UID:             "pod-uid-app-pod",
				ResourceVersion: "rv-app-pod",
				OwnerKind:       "ReplicaSet",
				OwnerName:       "ReplicaSet-app-pod",
				Classification:  PodEvictable,
			},
		},
		EvictableCount: 1,
	}
	plan := Plan{
		ID:              "plan-1",
		ClusterID:       1,
		Action:          ActionDrain,
		NodeName:        "worker-1",
		PreviewEvidence: PreviewEvidenceJSON(preview),
	}
	repo := &repositoryStub{claimed: plan, shouldExecute: true}
	svc := makeService(kube, repo)
	result, err := svc.Execute(context.Background(), "plan-1", "token", "key-12345")
	if err != nil {
		t.Fatalf("want nil error, got %v", err)
	}
	if !repo.completed {
		t.Error("Complete should be called")
	}
	if kube.createCalled != 1 {
		t.Errorf("expected 1 eviction call, got %d", kube.createCalled)
	}
	if !strings.Contains(kube.createPaths[0], "/pods/app-pod/eviction") {
		t.Errorf("eviction path = %q", kube.createPaths[0])
	}
	if result.ExecutionResult == nil {
		t.Fatal("execution result missing")
	}
	if result.ExecutionResult.EvictedCount != 1 {
		t.Errorf("EvictedCount = %d", result.ExecutionResult.EvictedCount)
	}
	if result.ExecutionResult.FailedCount != 0 {
		t.Errorf("FailedCount = %d", result.ExecutionResult.FailedCount)
	}
	if !result.ExecutionResult.UnschedulableNow {
		t.Error("node should remain cordoned after drain")
	}
}

func TestExecute_DrainPartialFailure(t *testing.T) {
	evictablePod := makePod("app-pod", "default", "worker-1", "ReplicaSet")
	kube := &kubernetesStub{
		node:      workerNode(true),
		pods:      []k8sgateway.Pod{evictablePod},
		pdbs:      []k8sgateway.PodDisruptionBudget{makePDB("default", "pdb-default", 1)},
		createErr: errors.New("eviction rejected"),
	}
	preview := PreviewEvidence{
		NodeUID: "node-uid-1",
		Pods: []PodEvidence{
			{
				Name:            "app-pod",
				Namespace:       "default",
				UID:             "pod-uid-app-pod",
				ResourceVersion: "rv-app-pod",
				OwnerKind:       "ReplicaSet",
				OwnerName:       "ReplicaSet-app-pod",
				Classification:  PodEvictable,
			},
		},
		EvictableCount: 1,
	}
	plan := Plan{
		ID:              "plan-1",
		ClusterID:       1,
		Action:          ActionDrain,
		NodeName:        "worker-1",
		PreviewEvidence: PreviewEvidenceJSON(preview),
	}
	repo := &repositoryStub{claimed: plan, shouldExecute: true}
	svc := makeService(kube, repo)
	_, err := svc.Execute(context.Background(), "plan-1", "token", "key-12345")
	if !errors.Is(err, ErrPartialDrain) {
		t.Fatalf("want ErrPartialDrain, got %v", err)
	}
	if repo.failedMessage == "" {
		t.Error("Fail should be called on partial drain")
	}
	if repo.failedResult == nil || !repo.failedResult.Partial {
		t.Error("failed result should be marked partial")
	}
	if repo.failedResult != nil && !repo.failedResult.UnschedulableNow {
		t.Error("node should remain cordoned on partial drain")
	}
}

func TestExecute_DrainCollectEvidenceError(t *testing.T) {
	kube := &kubernetesStub{
		node:    workerNode(true),
		podsErr: errors.New("pods api down"),
	}
	plan := Plan{
		ID:              "plan-1",
		ClusterID:       1,
		Action:          ActionDrain,
		NodeName:        "worker-1",
		PreviewEvidence: PreviewEvidenceJSON(PreviewEvidence{NodeUID: "node-uid-1"}),
	}
	repo := &repositoryStub{claimed: plan, shouldExecute: true}
	svc := makeService(kube, repo)
	_, err := svc.Execute(context.Background(), "plan-1", "token", "key-12345")
	if !errors.Is(err, ErrExecutionFailed) {
		t.Fatalf("want ErrExecutionFailed, got %v", err)
	}
}

func TestExecute_UnknownAction(t *testing.T) {
	kube := &kubernetesStub{node: workerNode(true)}
	plan := Plan{
		ID:              "plan-1",
		ClusterID:       1,
		Action:          "restart",
		NodeName:        "worker-1",
		PreviewEvidence: PreviewEvidenceJSON(PreviewEvidence{NodeUID: "node-uid-1"}),
	}
	repo := &repositoryStub{claimed: plan, shouldExecute: true}
	svc := makeService(kube, repo)
	_, err := svc.Execute(context.Background(), "plan-1", "token", "key-12345")
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("want ErrInvalidRequest, got %v", err)
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

func TestList_DelegatesToRepository(t *testing.T) {
	plans := []Plan{{ID: "p1"}, {ID: "p2"}}
	repo := &repositoryStub{listPlans: plans}
	svc := makeService(&kubernetesStub{}, repo)
	got, err := svc.List(context.Background(), 1)
	if err != nil {
		t.Fatalf("want nil error, got %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len(got) = %d", len(got))
	}
}

// --- helper tests ---

func TestClassifyPod_DaemonSetRetained(t *testing.T) {
	pod := makePod("ds", "kube-system", "worker-1", "DaemonSet")
	pe := classifyPod(pod, nil)
	if pe.Classification != PodRetained {
		t.Errorf("classification = %q, want retained", pe.Classification)
	}
	if pe.OwnerKind != "DaemonSet" {
		t.Errorf("OwnerKind = %q", pe.OwnerKind)
	}
}

func TestClassifyPod_MirrorPodRetained(t *testing.T) {
	pod := makePod("mirror", "default", "worker-1", "")
	pod.Metadata.Annotations = map[string]string{"kubernetes.io/config.mirror": "node-1"}
	pe := classifyPod(pod, nil)
	if pe.Classification != PodRetained {
		t.Errorf("classification = %q, want retained", pe.Classification)
	}
}

func TestClassifyPod_UnmanagedBlocking(t *testing.T) {
	pod := makePod("lonely", "default", "worker-1", "")
	pe := classifyPod(pod, nil)
	if pe.Classification != PodBlocking {
		t.Errorf("classification = %q, want blocking", pe.Classification)
	}
}

func TestClassifyPod_EmptyDirBlocking(t *testing.T) {
	pod := makePod("ed", "default", "worker-1", "ReplicaSet")
	emptyDir := json.RawMessage(`{}`)
	pod.Spec.Volumes = []k8sgateway.PodVolume{{Name: "cache", EmptyDir: &emptyDir}}
	pe := classifyPod(pod, nil)
	if pe.Classification != PodBlocking {
		t.Errorf("classification = %q, want blocking", pe.Classification)
	}
	if !pe.HasEmptyDir {
		t.Error("HasEmptyDir should be true")
	}
}

func TestClassifyPod_ManagedNoPDBBlocking(t *testing.T) {
	pod := makePod("managed", "default", "worker-1", "ReplicaSet")
	pe := classifyPod(pod, map[string]k8sgateway.PodDisruptionBudget{})
	if pe.Classification != PodBlocking {
		t.Errorf("classification = %q, want blocking", pe.Classification)
	}
}

func TestClassifyPod_ManagedWithPDBEvictable(t *testing.T) {
	pod := makePod("managed", "default", "worker-1", "ReplicaSet")
	pdb := makePDB("default", "pdb-default", 1)
	pdbMap := map[string]k8sgateway.PodDisruptionBudget{"default/pdb-default": pdb}
	pe := classifyPod(pod, pdbMap)
	if pe.Classification != PodEvictable {
		t.Errorf("classification = %q, want evictable", pe.Classification)
	}
	if pe.PDBName != "pdb-default" {
		t.Errorf("PDBName = %q", pe.PDBName)
	}
	if pe.PDBDisruptionsOK != 1 {
		t.Errorf("PDBDisruptionsOK = %d", pe.PDBDisruptionsOK)
	}
}

func TestClassifyPod_UnmatchedPDBBlocking(t *testing.T) {
	pod := makePod("managed", "default", "worker-1", "ReplicaSet")
	pod.Metadata.Labels = map[string]string{"app": "api"}
	pdb := makePDB("default", "other", 1)
	pdb.Spec.Selector = &metav1.LabelSelector{MatchLabels: map[string]string{"app": "worker"}}
	pe := classifyPod(pod, map[string]k8sgateway.PodDisruptionBudget{"default/other": pdb})
	if pe.Classification != PodBlocking || pe.PDBName != "" {
		t.Fatalf("unmatched PDB must not protect Pod: %+v", pe)
	}
}

func TestClassifyPod_ZeroDisruptionsBlocking(t *testing.T) {
	pod := makePod("managed", "default", "worker-1", "ReplicaSet")
	pdb := makePDB("default", "blocked", 0)
	pe := classifyPod(pod, map[string]k8sgateway.PodDisruptionBudget{"default/blocked": pdb})
	if pe.Classification != PodBlocking || pe.PDBName != "blocked" {
		t.Fatalf("zero disruptions must block Pod: %+v", pe)
	}
}

func TestEvidenceMatches(t *testing.T) {
	cases := []struct {
		name    string
		preview PreviewEvidence
		current PreviewEvidence
		want    bool
	}{
		{
			name:    "uid mismatch",
			preview: PreviewEvidence{NodeUID: "a"},
			current: PreviewEvidence{NodeUID: "b"},
			want:    false,
		},
		{
			name:    "pod count mismatch",
			preview: PreviewEvidence{NodeUID: "a"},
			current: PreviewEvidence{NodeUID: "a", Pods: []PodEvidence{{Name: "p"}}},
			want:    false,
		},
		{
			name:    "pod missing in current",
			preview: PreviewEvidence{NodeUID: "a", Pods: []PodEvidence{{Name: "p", Namespace: "ns", UID: "u"}}},
			current: PreviewEvidence{NodeUID: "a", Pods: []PodEvidence{}},
			want:    false,
		},
		{
			name:    "pod uid changed",
			preview: PreviewEvidence{NodeUID: "a", Pods: []PodEvidence{{Name: "p", Namespace: "ns", UID: "u1"}}},
			current: PreviewEvidence{NodeUID: "a", Pods: []PodEvidence{{Name: "p", Namespace: "ns", UID: "u2"}}},
			want:    false,
		},
		{
			name:    "match",
			preview: PreviewEvidence{NodeUID: "a", Pods: []PodEvidence{{Name: "p", Namespace: "ns", UID: "u"}}},
			current: PreviewEvidence{NodeUID: "a", Pods: []PodEvidence{{Name: "p", Namespace: "ns", UID: "u"}}},
			want:    true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := evidenceMatches(tc.preview, tc.current)
			if got != tc.want {
				t.Errorf("evidenceMatches = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestClassifyBlockError(t *testing.T) {
	cases := []struct {
		name    string
		pod     PodEvidence
		wantErr error
	}{
		{"unmanaged", PodEvidence{Classification: PodBlocking, OwnerKind: ""}, ErrUnmanagedPod},
		{"emptyDir", PodEvidence{Classification: PodBlocking, OwnerKind: "ReplicaSet", HasEmptyDir: true}, ErrEmptyDirPod},
		{"pdb unavailable", PodEvidence{Classification: PodBlocking, OwnerKind: "ReplicaSet", HasEmptyDir: false}, ErrPDBUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evidence := PreviewEvidence{Pods: []PodEvidence{tc.pod}}
			err := classifyBlockError(evidence)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("want %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestClassifyBlockError_NoBlockingPod(t *testing.T) {
	evidence := PreviewEvidence{Pods: []PodEvidence{{Classification: PodEvictable}}}
	err := classifyBlockError(evidence)
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("want ErrInvalidRequest fallback, got %v", err)
	}
}

func TestBuildNodePatch(t *testing.T) {
	body := buildNodePatch(true)
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	spec, ok := parsed["spec"].(map[string]any)
	if !ok {
		t.Fatal("spec missing")
	}
	if spec["unschedulable"] != true {
		t.Errorf("unschedulable = %v, want true", spec["unschedulable"])
	}
}

func TestBuildEvictionBody(t *testing.T) {
	pod := PodEvidence{Name: "app", Namespace: "ns"}
	body := buildEvictionBody(pod)
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if parsed["apiVersion"] != "policy/v1" {
		t.Errorf("apiVersion = %v", parsed["apiVersion"])
	}
	if parsed["kind"] != "Eviction" {
		t.Errorf("kind = %v", parsed["kind"])
	}
	meta, ok := parsed["metadata"].(map[string]any)
	if !ok {
		t.Fatal("metadata missing")
	}
	if meta["name"] != "app" || meta["namespace"] != "ns" {
		t.Errorf("metadata = %v", meta)
	}
}

func TestNewIdentity(t *testing.T) {
	id, token, hash, err := newIdentity()
	if err != nil {
		t.Fatalf("newIdentity error: %v", err)
	}
	if len(id) != 36 {
		t.Errorf("id length = %d, want 36 (UUID)", len(id))
	}
	if token == "" {
		t.Error("token should not be empty")
	}
	if len(hash) != 32 {
		t.Errorf("hash length = %d, want 32", len(hash))
	}
	// ids should be unique
	id2, _, _, _ := newIdentity()
	if id == id2 {
		t.Error("ids should be unique")
	}
}

func TestPreviewEvidenceJSON_RoundTrip(t *testing.T) {
	original := PreviewEvidenceJSON{
		NodeUID: "uid-1",
		Pods:    []PodEvidence{{Name: "p", Namespace: "ns"}},
	}
	value, err := original.Value()
	if err != nil {
		t.Fatalf("Value error: %v", err)
	}
	var scanned PreviewEvidenceJSON
	if err := scanned.Scan(value); err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if scanned.NodeUID != original.NodeUID {
		t.Errorf("NodeUID = %q", scanned.NodeUID)
	}
	if len(scanned.Pods) != 1 {
		t.Errorf("Pods length = %d", len(scanned.Pods))
	}
}

func TestPreviewEvidenceJSON_ScanNil(t *testing.T) {
	var scanned PreviewEvidenceJSON
	if err := scanned.Scan(nil); err != nil {
		t.Fatalf("Scan(nil) error: %v", err)
	}
}

func TestPreviewEvidenceJSON_ScanInvalid(t *testing.T) {
	var scanned PreviewEvidenceJSON
	if err := scanned.Scan(42); err == nil {
		t.Fatal("Scan should error on invalid type")
	}
}

func TestExecutionResultJSON_RoundTrip(t *testing.T) {
	original := ExecutionResultJSON{
		NodePatched:  true,
		EvictedCount: 3,
		PodOutcomes:  []PodOutcome{{Name: "p", Outcome: "evicted"}},
	}
	value, err := original.Value()
	if err != nil {
		t.Fatalf("Value error: %v", err)
	}
	var scanned ExecutionResultJSON
	if err := scanned.Scan(value); err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if !scanned.NodePatched {
		t.Error("NodePatched lost")
	}
	if scanned.EvictedCount != 3 {
		t.Errorf("EvictedCount = %d", scanned.EvictedCount)
	}
}

func TestExecutionResultJSON_ScanNil(t *testing.T) {
	var scanned ExecutionResultJSON
	if err := scanned.Scan(nil); err != nil {
		t.Fatalf("Scan(nil) error: %v", err)
	}
}

func TestExecutionResultJSON_EmptyPodOutcomesOmitted(t *testing.T) {
	original := ExecutionResultJSON{}
	value, err := original.Value()
	if err != nil {
		t.Fatalf("Value error: %v", err)
	}
	bytes, ok := value.([]byte)
	if !ok {
		t.Fatal("Value should return []byte")
	}
	var parsed map[string]any
	if err := json.Unmarshal(bytes, &parsed); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if _, ok := parsed["pod_outcomes"]; ok {
		t.Error("empty pod_outcomes should be omitted due to omitempty tag")
	}
}

func TestIsControlPlane(t *testing.T) {
	cases := []struct {
		name   string
		labels map[string]string
		want   bool
	}{
		{"empty labels", nil, false},
		{"control-plane label", map[string]string{"node-role.kubernetes.io/control-plane": "true"}, true},
		{"empty control-plane label", map[string]string{"node-role.kubernetes.io/control-plane": ""}, true},
		{"master label", map[string]string{"node-role.kubernetes.io/master": "true"}, true},
		{"empty master label", map[string]string{"node-role.kubernetes.io/master": ""}, true},
		{"worker label", map[string]string{"node-role.kubernetes.io/worker": "true"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node := k8sgateway.Node{}
			node.Metadata.Labels = tc.labels
			if got := isControlPlane(node); got != tc.want {
				t.Errorf("isControlPlane = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEvictablePods(t *testing.T) {
	evidence := PreviewEvidence{
		Pods: []PodEvidence{
			{Classification: PodRetained},
			{Classification: PodEvictable, Name: "e1"},
			{Classification: PodEvictable, Name: "e2"},
			{Classification: PodBlocking},
		},
	}
	got := evidence.EvictablePods()
	if len(got) != 2 {
		t.Errorf("len = %d, want 2", len(got))
	}
}

func TestSafeError(t *testing.T) {
	if msg := safeError(errors.New("generic")); msg == "" {
		t.Error("safeError should return non-empty message")
	}
}

func TestValidateRequest_TrimsWhitespace(t *testing.T) {
	if err := validateRequest(1, Request{Action: "  cordon  ", NodeName: "  worker-1  "}); err != nil {
		t.Fatalf("trimmed valid request should pass, got %v", err)
	}
}
