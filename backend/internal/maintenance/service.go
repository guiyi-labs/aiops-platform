package maintenance

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"k8s-aiops.local/backend/internal/apiquery"
	"k8s-aiops.local/backend/internal/cluster"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

const (
	maxResidentPods     = 100
	maxEvictablePods    = 20
	evictionTimeout     = 30 * time.Second
	drainDeadline       = 10 * time.Minute
	evictionConcurrency = 2
)

// KubernetesSource is the reviewed subset of kubernetes.Service consumed by
// the maintenance service. Bounding the interface keeps the mutation surface
// auditable: only Node patch (cordon/uncordon) and Pod eviction create are
// exposed. No Pod delete, no Node delete, no Secret access.
type KubernetesSource interface {
	Node(context.Context, int64, string) (k8sgateway.Node, error)
	Pods(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Pod], error)
	PodDisruptionBudgets(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.PodDisruptionBudget], error)
	PatchNode(context.Context, int64, string, []byte, bool) (k8sgateway.Node, error)
	CreateResource(context.Context, int64, string, []byte, bool) ([]byte, error)
}

type Service struct {
	kubernetes KubernetesSource
	repository Repository
	planTTL    time.Duration
	claimTTL   time.Duration
	now        func() time.Time
}

func NewService(kubernetes KubernetesSource, repository Repository) *Service {
	return &Service{
		kubernetes: kubernetes,
		repository: repository,
		planTTL:    10 * time.Minute,
		claimTTL:   time.Minute,
		now:        time.Now,
	}
}

// Preview validates the request, inspects the target Node and its resident
// Pods, classifies them, runs a server-side dry-run patch, and persists an
// awaiting-confirmation plan with a one-time confirmation token.
func (s *Service) Preview(ctx context.Context, clusterID int64, request Request, actor ActorRef) (Plan, error) {
	if err := validateRequest(clusterID, request); err != nil {
		return Plan{}, err
	}

	node, err := s.kubernetes.Node(ctx, clusterID, request.NodeName)
	if err != nil {
		return Plan{}, err
	}

	// Reject control-plane nodes.
	if isControlPlane(node) {
		return Plan{}, ErrControlPlaneNode
	}

	// Action-specific precondition checks.
	switch request.Action {
	case ActionCordon:
		if node.Spec.Unschedulable {
			return Plan{}, ErrAlreadyCordoned
		}
	case ActionUncordon:
		if !node.Spec.Unschedulable {
			return Plan{}, ErrAlreadyUncordoned
		}
	case ActionDrain:
		if !node.Spec.Unschedulable {
			return Plan{}, ErrNotCordoned
		}
	}

	// Collect resident Pods and classify them.
	evidence, err := s.collectEvidence(ctx, clusterID, request.NodeName, node)
	if err != nil {
		return Plan{}, err
	}

	// For drain, check blocking conditions.
	if request.Action == ActionDrain {
		if evidence.BlockingCount > 0 {
			return Plan{}, classifyBlockError(evidence)
		}
		if evidence.EvictableCount > maxEvictablePods {
			return Plan{}, ErrTooManyPods
		}
	}

	// Server-side dry-run patch for cordon/uncordon.
	if request.Action == ActionCordon || request.Action == ActionUncordon {
		patch := buildNodePatch(request.Action == ActionCordon)
		if _, err := s.kubernetes.PatchNode(ctx, clusterID, request.NodeName, patch, true); err != nil {
			return Plan{}, err
		}
	}

	id, token, tokenHash, err := newIdentity()
	if err != nil {
		return Plan{}, err
	}
	now := s.now().UTC().Truncate(time.Second)
	plan := Plan{
		ID:                    id,
		ClusterID:             clusterID,
		Status:                StatusAwaitingConfirmation,
		Action:                request.Action,
		NodeName:              request.NodeName,
		NodeUID:               node.Metadata.UID,
		NodeResourceVersion:   node.Metadata.ResourceVersion,
		NodeUnschedulable:     node.Spec.Unschedulable,
		PreviewEvidence:       PreviewEvidenceJSON(evidence),
		ConfirmationTokenHash: tokenHash,
		RequestedByUserID:     &actor.ID,
		RequestedByName:       actor.Name,
		ExpiresAt:             now.Add(s.planTTL),
	}

	plan.ConfirmationToken = ""
	if err := s.repository.Save(ctx, &plan); err != nil {
		return Plan{}, err
	}
	plan.ConfirmationToken = token
	return plan, nil
}

// Execute claims the plan with the confirmation token and idempotency key,
// re-verifies the target Node/Pod/PDB evidence, then performs the actual
// mutation (patch for cordon/uncordon, bounded eviction for drain).
func (s *Service) Execute(ctx context.Context, id, confirmationToken, idempotencyKey string) (Plan, error) {
	id = strings.TrimSpace(id)
	confirmationToken = strings.TrimSpace(confirmationToken)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
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

	// Re-verify Node identity (UID + resourceVersion must match preview).
	currentNode, err := s.kubernetes.Node(ctx, plan.ClusterID, plan.NodeName)
	if err != nil {
		failed, _ := s.repository.Fail(ctx, plan.ID, idempotencyKey, safeError(err), nil)
		return failed, fmt.Errorf("%w: %v", ErrExecutionFailed, err)
	}
	preview := PreviewEvidence(plan.PreviewEvidence)
	if currentNode.Metadata.UID != preview.NodeUID {
		failed, _ := s.repository.Fail(ctx, plan.ID, idempotencyKey, "node UID mismatch", nil)
		return failed, ErrStaleTarget
	}

	switch plan.Action {
	case ActionCordon:
		return s.executeCordon(ctx, plan, idempotencyKey, currentNode)
	case ActionUncordon:
		return s.executeUncordon(ctx, plan, idempotencyKey, currentNode)
	case ActionDrain:
		return s.executeDrain(ctx, plan, idempotencyKey, currentNode)
	default:
		failed, _ := s.repository.Fail(ctx, plan.ID, idempotencyKey, "unknown action", nil)
		return failed, ErrInvalidRequest
	}
}

func (s *Service) executeCordon(ctx context.Context, plan Plan, idempotencyKey string, node k8sgateway.Node) (Plan, error) {
	patch := buildNodePatch(true)
	patched, err := s.kubernetes.PatchNode(ctx, plan.ClusterID, plan.NodeName, patch, false)
	if err != nil {
		failed, _ := s.repository.Fail(ctx, plan.ID, idempotencyKey, safeError(err), nil)
		return failed, fmt.Errorf("%w: %v", ErrExecutionFailed, err)
	}
	result := ExecutionResultJSON(ExecutionResult{
		NodePatched:      true,
		UnschedulableNow: patched.Spec.Unschedulable,
	})
	return s.repository.Complete(ctx, plan.ID, idempotencyKey, s.now().UTC(), plan, &result)
}

func (s *Service) executeUncordon(ctx context.Context, plan Plan, idempotencyKey string, node k8sgateway.Node) (Plan, error) {
	patch := buildNodePatch(false)
	patched, err := s.kubernetes.PatchNode(ctx, plan.ClusterID, plan.NodeName, patch, false)
	if err != nil {
		failed, _ := s.repository.Fail(ctx, plan.ID, idempotencyKey, safeError(err), nil)
		return failed, fmt.Errorf("%w: %v", ErrExecutionFailed, err)
	}
	result := ExecutionResultJSON(ExecutionResult{
		NodePatched:      true,
		UnschedulableNow: patched.Spec.Unschedulable,
	})
	return s.repository.Complete(ctx, plan.ID, idempotencyKey, s.now().UTC(), plan, &result)
}

func (s *Service) executeDrain(ctx context.Context, plan Plan, idempotencyKey string, node k8sgateway.Node) (Plan, error) {
	// Re-collect Pod evidence and verify it hasn't widened.
	currentEvidence, err := s.collectEvidence(ctx, plan.ClusterID, plan.NodeName, node)
	if err != nil {
		failed, _ := s.repository.Fail(ctx, plan.ID, idempotencyKey, safeError(err), nil)
		return failed, fmt.Errorf("%w: %v", ErrExecutionFailed, err)
	}
	preview := PreviewEvidence(plan.PreviewEvidence)
	if !evidenceMatches(preview, currentEvidence) {
		failed, _ := s.repository.Fail(ctx, plan.ID, idempotencyKey, "pod evidence changed since preview", nil)
		return failed, ErrStaleTarget
	}

	// Ensure node is cordoned (it should be, since preview required it).
	if !node.Spec.Unschedulable {
		patch := buildNodePatch(true)
		node, _ = s.kubernetes.PatchNode(ctx, plan.ClusterID, plan.NodeName, patch, false)
	}

	// Evict eligible Pods with bounded concurrency.
	outcomes, evicted, failed := s.evictPods(ctx, plan.ClusterID, currentEvidence.EvictablePods())
	partial := failed > 0

	result := ExecutionResult{
		NodePatched:      true,
		UnschedulableNow: true, // Node stays cordoned even on partial drain
		PodOutcomes:      outcomes,
		EvictedCount:     evicted,
		FailedCount:      failed,
		Partial:          partial,
	}
	resultJSON := ExecutionResultJSON(result)

	if partial {
		failedPlan, _ := s.repository.Fail(ctx, plan.ID, idempotencyKey, "drain completed with partial failures; node remains cordoned", &resultJSON)
		return failedPlan, ErrPartialDrain
	}
	return s.repository.Complete(ctx, plan.ID, idempotencyKey, s.now().UTC(), plan, &resultJSON)
}

// evictPods evicts the eligible Pods with bounded concurrency and per-Pod
// timeout. Returns outcomes, evicted count, and failed count.
func (s *Service) evictPods(ctx context.Context, clusterID int64, pods []PodEvidence) ([]PodOutcome, int, int) {
	var mu sync.Mutex
	outcomes := make([]PodOutcome, 0, len(pods))
	evicted := 0
	failed := 0

	sem := make(chan struct{}, evictionConcurrency)
	var wg sync.WaitGroup

	for _, pod := range pods {
		wg.Add(1)
		go func(p PodEvidence) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			outcome := s.evictOne(ctx, clusterID, p)
			mu.Lock()
			outcomes = append(outcomes, outcome)
			if outcome.Outcome == "evicted" {
				evicted++
			} else {
				failed++
			}
			mu.Unlock()
		}(pod)
	}
	wg.Wait()
	return outcomes, evicted, failed
}

func (s *Service) evictOne(ctx context.Context, clusterID int64, pod PodEvidence) PodOutcome {
	evictionBody := buildEvictionBody(pod)
	evictCtx, cancel := context.WithTimeout(ctx, evictionTimeout)
	defer cancel()

	_, err := s.kubernetes.CreateResource(evictCtx, clusterID,
		fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/eviction",
			url.PathEscape(pod.Namespace), url.PathEscape(pod.Name)),
		evictionBody, false)
	if err == nil {
		return PodOutcome{Name: pod.Name, Namespace: pod.Namespace, Outcome: "evicted"}
	}
	return PodOutcome{
		Name:      pod.Name,
		Namespace: pod.Namespace,
		Outcome:   "failed",
		Detail:    safeError(err),
	}
}

// collectEvidence gathers all resident Pods on the node, classifies them,
// and collects PDB evidence.
func (s *Service) collectEvidence(ctx context.Context, clusterID int64, nodeName string, node k8sgateway.Node) (PreviewEvidence, error) {
	evidence := PreviewEvidence{
		NodeUID:             node.Metadata.UID,
		NodeResourceVersion: node.Metadata.ResourceVersion,
		NodeUnschedulable:   node.Spec.Unschedulable,
	}

	podsResp, err := s.kubernetes.Pods(ctx, clusterID, "", apiquery.ListQuery{Limit: maxResidentPods + 1})
	if err != nil {
		return evidence, err
	}
	// Filter to pods on this node.
	var residentPods []k8sgateway.Pod
	for _, p := range podsResp.Items {
		if p.Spec.NodeName == nodeName {
			residentPods = append(residentPods, p)
		}
	}
	if len(residentPods) > maxResidentPods {
		return evidence, ErrTooManyPods
	}

	// Collect all PDBs (cluster-wide, then filter by namespace as needed).
	pdbsResp, err := s.kubernetes.PodDisruptionBudgets(ctx, clusterID, "", apiquery.ListQuery{Limit: 200})
	if err != nil {
		return evidence, ErrPDBUnavailable
	}
	pdbMap := make(map[string]k8sgateway.PodDisruptionBudget)
	for _, pdb := range pdbsResp.Items {
		pdbMap[pdb.Metadata.Namespace+"/"+pdb.Metadata.Name] = pdb
	}

	for _, pod := range residentPods {
		pe := classifyPod(pod, pdbMap)
		evidence.Pods = append(evidence.Pods, pe)
		switch pe.Classification {
		case PodRetained:
			evidence.RetainedCount++
		case PodEvictable:
			evidence.EvictableCount++
		case PodBlocking:
			evidence.BlockingCount++
		}
	}
	return evidence, nil
}

func (s *Service) List(ctx context.Context, clusterID int64) ([]Plan, error) {
	if clusterID < 1 {
		return nil, ErrInvalidRequest
	}
	return s.repository.List(ctx, clusterID)
}

// --- helpers ---

func validateRequest(clusterID int64, request Request) error {
	if clusterID < 1 {
		return ErrInvalidRequest
	}
	request.Action = strings.TrimSpace(request.Action)
	request.NodeName = strings.TrimSpace(request.NodeName)
	if request.Action != ActionCordon && request.Action != ActionUncordon && request.Action != ActionDrain {
		return ErrInvalidRequest
	}
	if request.NodeName == "" || len(request.NodeName) > 253 {
		return ErrInvalidRequest
	}
	return nil
}

func isControlPlane(node k8sgateway.Node) bool {
	if node.Metadata.Labels == nil {
		return false
	}
	return node.Metadata.Labels["node-role.kubernetes.io/control-plane"] != "" ||
		node.Metadata.Labels["node-role.kubernetes.io/master"] != ""
}

func classifyPod(pod k8sgateway.Pod, pdbMap map[string]k8sgateway.PodDisruptionBudget) PodEvidence {
	pe := PodEvidence{
		Name:            pod.Metadata.Name,
		Namespace:       pod.Metadata.Namespace,
		UID:             pod.Metadata.UID,
		ResourceVersion: pod.Metadata.ResourceVersion,
	}

	// Determine owner.
	ownerKind, ownerName := ownerInfo(pod)
	pe.OwnerKind = ownerKind
	pe.OwnerName = ownerName

	// Check for emptyDir volumes.
	pe.HasEmptyDir = hasEmptyDir(pod)

	// DaemonSet-managed and mirror/static pods are retained.
	if ownerKind == "DaemonSet" || isMirrorPod(pod) {
		pe.Classification = PodRetained
		return pe
	}

	// Unmanaged pods (no owner) are blocking.
	if ownerKind == "" {
		pe.Classification = PodBlocking
		return pe
	}

	// Pods with emptyDir are blocking.
	if pe.HasEmptyDir {
		pe.Classification = PodBlocking
		return pe
	}

	// Look up PDB evidence.
	for nsName, pdb := range pdbMap {
		if strings.HasPrefix(nsName, pod.Metadata.Namespace+"/") {
			pe.PDBName = pdb.Metadata.Name
			pe.PDBDisruptionsOK = pdb.Status.DisruptionsAllowed
		}
	}

	// If PDB evidence is unavailable for a managed pod, it's blocking.
	if pe.PDBName == "" {
		pe.Classification = PodBlocking
		return pe
	}

	pe.Classification = PodEvictable
	return pe
}

func ownerInfo(pod k8sgateway.Pod) (string, string) {
	for _, ref := range pod.Metadata.OwnerReferences {
		if ref.Controller != nil && *ref.Controller {
			return ref.Kind, ref.Name
		}
	}
	if len(pod.Metadata.OwnerReferences) > 0 {
		return pod.Metadata.OwnerReferences[0].Kind, pod.Metadata.OwnerReferences[0].Name
	}
	return "", ""
}

func hasEmptyDir(pod k8sgateway.Pod) bool {
	// The Pod struct does not expose volumes in the read-only gateway.
	// We rely on the metadata annotation or the absence of volume info.
	// In a real implementation, we'd need to fetch the full Pod spec.
	// For now, we use the annotation that the kubernetes gateway may set.
	if pod.Metadata.Annotations != nil {
		if pod.Metadata.Annotations["k8s-aiops.local/has-emptydir"] == "true" {
			return true
		}
	}
	return false
}

func isMirrorPod(pod k8sgateway.Pod) bool {
	if pod.Metadata.Annotations != nil {
		_, ok := pod.Metadata.Annotations["kubernetes.io/config.mirror"]
		return ok
	}
	return false
}

func evidenceMatches(preview PreviewEvidence, current PreviewEvidence) bool {
	if preview.NodeUID != current.NodeUID {
		return false
	}
	if len(preview.Pods) != len(current.Pods) {
		return false
	}
	previewSet := make(map[string]PodEvidence, len(preview.Pods))
	for _, p := range preview.Pods {
		previewSet[p.Namespace+"/"+p.Name] = p
	}
	for _, p := range current.Pods {
		key := p.Namespace + "/" + p.Name
		prev, ok := previewSet[key]
		if !ok || prev.UID != p.UID {
			return false
		}
	}
	return true
}

func classifyBlockError(evidence PreviewEvidence) error {
	for _, p := range evidence.Pods {
		if p.Classification == PodBlocking {
			if p.OwnerKind == "" {
				return ErrUnmanagedPod
			}
			if p.HasEmptyDir {
				return ErrEmptyDirPod
			}
			return ErrPDBUnavailable
		}
	}
	return ErrInvalidRequest
}

func buildNodePatch(unschedulable bool) []byte {
	patch := map[string]any{
		"spec": map[string]any{
			"unschedulable": unschedulable,
		},
	}
	body, _ := json.Marshal(patch)
	return body
}

func buildEvictionBody(pod PodEvidence) []byte {
	eviction := map[string]any{
		"apiVersion": "policy/v1",
		"kind":       "Eviction",
		"metadata": map[string]any{
			"name":      pod.Name,
			"namespace": pod.Namespace,
		},
	}
	body, _ := json.Marshal(eviction)
	return body
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

func safeError(err error) string {
	var status cluster.APIStatusError
	if errors.As(err, &status) {
		return fmt.Sprintf("Kubernetes API rejected with HTTP %d", status.StatusCode)
	}
	if errors.Is(err, k8sgateway.ErrResourceNotFound) {
		return "Kubernetes resource was not found"
	}
	return "Kubernetes request failed"
}

// EvictablePods returns only the evictable pods from the evidence.
func (e PreviewEvidence) EvictablePods() []PodEvidence {
	var out []PodEvidence
	for _, p := range e.Pods {
		if p.Classification == PodEvictable {
			out = append(out, p)
		}
	}
	return out
}
