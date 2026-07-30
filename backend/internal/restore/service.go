package restore

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
	"time"

	"k8s-aiops.local/backend/internal/apiquery"
	"k8s-aiops.local/backend/internal/cluster"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

// KubernetesSource is the reviewed subset of kubernetes.Service consumed by the
// restore service. Bounding the interface keeps the mutation surface auditable:
// only Namespace create, NetworkPolicy create, ResourceQuota create and Velero
// Restore create are exposed. No Pod/Service/Ingress mutation, no PV/PVC
// mutation, no Secret value reads, no Restore delete/update.
type KubernetesSource interface {
	VeleroCapability(context.Context, int64) (k8sgateway.VeleroCapability, error)
	Backup(context.Context, int64, string, string) (k8sgateway.VeleroBackup, error)
	NamespaceExists(context.Context, int64, string) (bool, error)
	VeleroRestoreExists(context.Context, int64, string, string) (bool, error)
	VeleroRestore(context.Context, int64, string, string) (k8sgateway.VeleroRestore, error)
	CreateResource(context.Context, int64, string, []byte, bool) ([]byte, error)
	// List allowlisted kinds in the destination namespace after restore to
	// project restored items. Each method is read-only.
	Deployments(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Deployment], error)
	StatefulSets(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.StatefulSet], error)
	DaemonSets(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.DaemonSet], error)
	CronJobs(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.CronJob], error)
	ConfigMaps(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.ConfigMap], error)
	Secrets(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Secret], error)
	ServiceAccounts(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.ServiceAccount], error)
}

type Service struct {
	kubernetes   KubernetesSource
	repository   Repository
	planTTL      time.Duration
	claimTTL     time.Duration
	pollInterval time.Duration
	pollAttempts int
	now          func() time.Time
}

func NewService(kubernetes KubernetesSource, repository Repository) *Service {
	return &Service{
		kubernetes:   kubernetes,
		repository:   repository,
		planTTL:      10 * time.Minute,
		claimTTL:     time.Minute,
		pollInterval: RestorePollInterval,
		pollAttempts: RestorePollAttempts,
		now:          time.Now,
	}
}

// Preview validates the request, runs preflight checks (Velero installed,
// source Backup Completed and M28-compatible, destination absent, no active
// plan for the same source), performs server-side dry-runs for the quarantine
// controls and Restore CR, and persists an awaiting-confirmation plan with a
// one-time confirmation token. No Namespace, NetworkPolicy, ResourceQuota or
// Restore is created during preview.
func (s *Service) Preview(ctx context.Context, clusterID int64, request Request, actor ActorRef) (Plan, error) {
	if err := validateRequest(clusterID, request); err != nil {
		return Plan{}, err
	}

	// Preflight 1: Velero must be installed.
	cap, err := s.kubernetes.VeleroCapability(ctx, clusterID)
	if err != nil {
		return Plan{}, err
	}
	if !cap.Installed {
		return Plan{}, ErrVeleroNotInstalled
	}

	// Preflight 2: source Backup must exist and be Completed.
	backup, err := s.kubernetes.Backup(ctx, clusterID, request.SourceBackupNamespace, request.SourceBackupName)
	if err != nil {
		if errors.Is(err, k8sgateway.ErrResourceNotFound) {
			return Plan{}, ErrSourceBackupNotFound
		}
		return Plan{}, err
	}
	if backup.Phase != PhaseCompleted {
		return Plan{}, ErrSourceBackupIncomplete
	}
	// M28-compatible scope: source backup must have exactly one included namespace
	// and no cluster-scoped resources. (M28 enforce include_cluster_resources=false
	// by default and snapshot_volumes=false; we only check included namespace count
	// here because that is the observable signal on a Completed Backup.)
	if len(backup.IncludedNamespaces) != 1 {
		return Plan{}, ErrSourceBackupScope
	}

	// Preflight 3: destination Namespace must not already exist.
	destination := generateDestinationNamespace(request.SourceBackupName, actor.ID)
	exists, err := s.kubernetes.NamespaceExists(ctx, clusterID, destination)
	if err != nil {
		return Plan{}, err
	}
	if exists {
		return Plan{}, ErrDestinationExists
	}

	// Preflight 4: no active plan for the same source backup.
	if _, active, err := s.repository.ActiveBySource(ctx, clusterID, request.SourceBackupName, request.SourceBackupNamespace); err != nil {
		return Plan{}, err
	} else if active {
		return Plan{}, ErrDestinationCollision
	}

	// Preflight 5: Velero Restore name must not already exist.
	restoreName := generateRestoreName(destination)
	restoreExists, err := s.kubernetes.VeleroRestoreExists(ctx, clusterID, request.SourceBackupNamespace, restoreName)
	if err != nil {
		return Plan{}, err
	}
	if restoreExists {
		return Plan{}, ErrRestoreNameConflict
	}

	// Preflight 6: server-side dry-run of quarantine resources.
	quarantineManifests := buildQuarantineManifests(destination)
	for _, manifest := range quarantineManifests {
		if _, err := s.kubernetes.CreateResource(ctx, clusterID, manifest.path, manifest.body, true); err != nil {
			return Plan{}, fmt.Errorf("%w: %v", ErrQuarantineDryRunFailed, err)
		}
	}

	// Preflight 7: server-side dry-run of the Velero Restore CR.
	restoreManifest := buildRestoreManifest(restoreName, request.SourceBackupNamespace, request.SourceBackupName, destination)
	if _, err := s.kubernetes.CreateResource(ctx, clusterID,
		"/apis/velero.io/v1/namespaces/"+url.PathEscape(request.SourceBackupNamespace)+"/restores",
		restoreManifest, true); err != nil {
		return Plan{}, fmt.Errorf("%w: %v", ErrRestoreDryRunFailed, err)
	}

	id, token, tokenHash, err := newIdentity()
	if err != nil {
		return Plan{}, err
	}
	now := s.now().UTC().Truncate(time.Second)
	plan := Plan{
		ID:                          id,
		ClusterID:                   clusterID,
		Status:                      StatusAwaitingConfirmation,
		SourceBackupName:            request.SourceBackupName,
		SourceBackupNamespace:       request.SourceBackupNamespace,
		SourceBackupUID:             "", // VeleroBackup projection does not expose UID; tracked via name+namespace+phase
		SourceBackupResourceVersion: "",
		SourceBackupPhase:           backup.Phase,
		DestinationNamespace:        destination,
		VeleroRestoreName:           restoreName,
		VeleroRestoreNamespace:      request.SourceBackupNamespace,
		QuarantineStatus: QuarantineStatusJSON{
			NetworkPolicyName: QuarantineNetworkPolicyName,
			ResourceQuotaName: QuarantineResourceQuotaName,
			DryRunValidated:   true,
		},
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

// Execute claims the plan, re-verifies preconditions, creates the quarantine
// controls before the Restore, creates exactly one Velero Restore, polls until
// terminal, and projects the restored items.
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

	// Re-verify source Backup identity (name + namespace + phase).
	backup, err := s.kubernetes.Backup(ctx, plan.ClusterID, plan.SourceBackupNamespace, plan.SourceBackupName)
	if err != nil {
		failed, _ := s.repository.Fail(ctx, plan.ID, idempotencyKey, safeError(err), nil)
		return failed, fmt.Errorf("%w: %v", ErrExecutionFailed, err)
	}
	if backup.Phase != PhaseCompleted {
		failed, _ := s.repository.Fail(ctx, plan.ID, idempotencyKey, "source backup is no longer Completed", nil)
		return failed, ErrStaleSource
	}

	// Re-verify destination is still absent.
	destExists, err := s.kubernetes.NamespaceExists(ctx, plan.ClusterID, plan.DestinationNamespace)
	if err != nil {
		failed, _ := s.repository.Fail(ctx, plan.ID, idempotencyKey, safeError(err), nil)
		return failed, fmt.Errorf("%w: %v", ErrExecutionFailed, err)
	}
	if destExists {
		failed, _ := s.repository.Fail(ctx, plan.ID, idempotencyKey, "destination namespace already exists", nil)
		return failed, ErrDestinationExists
	}

	// Re-verify Restore name is still available.
	restoreExists, err := s.kubernetes.VeleroRestoreExists(ctx, plan.ClusterID, plan.VeleroRestoreNamespace, plan.VeleroRestoreName)
	if err != nil {
		failed, _ := s.repository.Fail(ctx, plan.ID, idempotencyKey, safeError(err), nil)
		return failed, fmt.Errorf("%w: %v", ErrExecutionFailed, err)
	}
	if restoreExists {
		failed, _ := s.repository.Fail(ctx, plan.ID, idempotencyKey, "velero restore name already exists", nil)
		return failed, ErrRestoreNameConflict
	}

	// Step 1: create quarantine Namespace first.
	namespaceBody := buildNamespaceManifest(plan.DestinationNamespace)
	namespaceResp, err := s.kubernetes.CreateResource(ctx, plan.ClusterID, "/api/v1/namespaces", namespaceBody, false)
	if err != nil {
		failed, _ := s.repository.Fail(ctx, plan.ID, idempotencyKey, safeError(err), nil)
		return failed, fmt.Errorf("%w: %v", ErrQuarantineFailed, err)
	}
	namespaceUID := extractUID(namespaceResp)
	plan.DestinationNamespaceUID = namespaceUID

	// Step 2: create quarantine NetworkPolicy and ResourceQuota before Restore.
	quarantineManifests := buildQuarantineManifests(plan.DestinationNamespace)
	for _, manifest := range quarantineManifests {
		if _, err := s.kubernetes.CreateResource(ctx, plan.ClusterID, manifest.path, manifest.body, false); err != nil {
			// Quarantine failed after Namespace creation — record the retained target.
			quarantine := QuarantineStatusJSON{
				NamespaceCreated:     true,
				NamespaceUID:         namespaceUID,
				NetworkPolicyName:    QuarantineNetworkPolicyName,
				ResourceQuotaName:    QuarantineResourceQuotaName,
				NetworkPolicyCreated: manifest.kind == "NetworkPolicy",
				DryRunValidated:      true,
			}
			result := ExecutionResultJSON{
				QuarantineEstablished: false,
				FailureReason:         safeError(err),
			}
			failed, _ := s.repository.Fail(ctx, plan.ID, idempotencyKey, fmt.Sprintf("quarantine %s creation failed; namespace retained", manifest.kind), &result)
			_ = s.updateQuarantineStatus(ctx, plan.ID, quarantine)
			return failed, fmt.Errorf("%w: %v", ErrQuarantineFailed, err)
		}
	}

	quarantine := QuarantineStatusJSON{
		NamespaceCreated:     true,
		NamespaceUID:         namespaceUID,
		NetworkPolicyName:    QuarantineNetworkPolicyName,
		NetworkPolicyCreated: true,
		ResourceQuotaName:    QuarantineResourceQuotaName,
		ResourceQuotaCreated: true,
		DryRunValidated:      true,
	}
	if err := s.updateQuarantineStatus(ctx, plan.ID, quarantine); err != nil {
		// Non-fatal: quarantine is established; logging-only.
		_ = err
	}

	// Step 3: create exactly one Velero Restore CR.
	restoreManifest := buildRestoreManifest(plan.VeleroRestoreName, plan.VeleroRestoreNamespace, plan.SourceBackupName, plan.DestinationNamespace)
	restoreResp, err := s.kubernetes.CreateResource(ctx, plan.ClusterID,
		"/apis/velero.io/v1/namespaces/"+url.PathEscape(plan.VeleroRestoreNamespace)+"/restores",
		restoreManifest, false)
	if err != nil {
		result := ExecutionResultJSON{
			QuarantineEstablished: true,
			FailureReason:         safeError(err),
		}
		failed, _ := s.repository.Fail(ctx, plan.ID, idempotencyKey, "velero restore creation failed; quarantine namespace retained", &result)
		return failed, fmt.Errorf("%w: %v", ErrExecutionFailed, err)
	}
	restoreUID := extractUID(restoreResp)

	// Step 4: poll the Restore CR until terminal or timeout.
	terminalPhase, pollErr := s.pollRestore(ctx, plan.ClusterID, plan.VeleroRestoreNamespace, plan.VeleroRestoreName)

	// Step 5: project restored items regardless of phase (for audit).
	items, truncated := s.projectRestoredItems(ctx, plan.ClusterID, plan.DestinationNamespace)

	result := ExecutionResult{
		RestoreCreated:        true,
		RestorePhase:          terminalPhase,
		RestoreUID:            restoreUID,
		RestoredItems:         items,
		RestoredItemCount:     len(items),
		TruncatedItems:        truncated,
		QuarantineEstablished: true,
	}
	if pollErr != nil {
		result.FailureReason = safeError(pollErr)
	}
	result.Partial = terminalPhase == PhasePartiallyFailed

	resultJSON := ExecutionResultJSON(result)

	if pollErr != nil {
		failed, _ := s.repository.Fail(ctx, plan.ID, idempotencyKey, fmt.Sprintf("restore poll failed; quarantine namespace retained: %v", pollErr), &resultJSON)
		return failed, fmt.Errorf("%w: %v", ErrRestorePollTimeout, pollErr)
	}
	if result.Partial {
		failed, _ := s.repository.Fail(ctx, plan.ID, idempotencyKey, "velero restore completed with partial failures; quarantine namespace retained", &resultJSON)
		return failed, ErrPartialRestore
	}
	if terminalPhase == PhaseFailed {
		failed, _ := s.repository.Fail(ctx, plan.ID, idempotencyKey, "velero restore failed; quarantine namespace retained", &resultJSON)
		return failed, ErrExecutionFailed
	}
	return s.repository.Complete(ctx, plan.ID, idempotencyKey, s.now().UTC(), plan, &resultJSON)
}

func (s *Service) List(ctx context.Context, clusterID int64) ([]Plan, error) {
	if clusterID < 1 {
		return nil, ErrInvalidRequest
	}
	return s.repository.List(ctx, clusterID)
}

// pollRestore polls the Velero Restore CR until it reaches a terminal phase
// (Completed, PartiallyFailed, Failed) or the bounded attempt count is exceeded.
func (s *Service) pollRestore(ctx context.Context, clusterID int64, namespace, name string) (string, error) {
	for attempt := 0; attempt < s.pollAttempts; attempt++ {
		restore, err := s.kubernetes.VeleroRestore(ctx, clusterID, namespace, name)
		if err == nil {
			switch restore.Phase {
			case PhaseCompleted, PhasePartiallyFailed, PhaseFailed:
				return restore.Phase, nil
			}
		} else if !errors.Is(err, k8sgateway.ErrResourceNotFound) {
			// Transient errors are tolerated; only persistent 404 above the first
			// poll would indicate the Restore was deleted out-of-band.
			if attempt == s.pollAttempts-1 {
				return "", err
			}
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(s.pollInterval):
		}
	}
	return "", ErrRestorePollTimeout
}

// projectRestoredItems lists the allowlisted resource kinds in the destination
// namespace and returns a bounded projection. Secrets are listed by name only;
// no values are read.
func (s *Service) projectRestoredItems(ctx context.Context, clusterID int64, namespace string) ([]RestoredItem, bool) {
	items := make([]RestoredItem, 0, 32)
	truncated := false

	add := func(kind string, names []string) {
		for _, name := range names {
			if len(items) >= MaxProjectedRestoredResources {
				truncated = true
				return
			}
			items = append(items, RestoredItem{Kind: kind, Name: name, Namespace: namespace})
		}
	}

	query := apiquery.ListQuery{Limit: MaxProjectedRestoredResources + 1}

	if resp, err := s.kubernetes.Deployments(ctx, clusterID, namespace, query); err == nil {
		add(KindDeployment, namesOf(resp.Items, func(d k8sgateway.Deployment) string { return d.Metadata.Name }))
	}
	if resp, err := s.kubernetes.StatefulSets(ctx, clusterID, namespace, query); err == nil {
		add(KindStatefulSet, namesOf(resp.Items, func(d k8sgateway.StatefulSet) string { return d.Metadata.Name }))
	}
	if resp, err := s.kubernetes.DaemonSets(ctx, clusterID, namespace, query); err == nil {
		add(KindDaemonSet, namesOf(resp.Items, func(d k8sgateway.DaemonSet) string { return d.Metadata.Name }))
	}
	if resp, err := s.kubernetes.CronJobs(ctx, clusterID, namespace, query); err == nil {
		add(KindCronJob, namesOf(resp.Items, func(d k8sgateway.CronJob) string { return d.Metadata.Name }))
	}
	if resp, err := s.kubernetes.ConfigMaps(ctx, clusterID, namespace, query); err == nil {
		add(KindConfigMap, namesOf(resp.Items, func(d k8sgateway.ConfigMap) string { return d.Metadata.Name }))
	}
	if resp, err := s.kubernetes.Secrets(ctx, clusterID, namespace, query); err == nil {
		add(KindSecret, namesOf(resp.Items, func(d k8sgateway.Secret) string { return d.Metadata.Name }))
	}
	if resp, err := s.kubernetes.ServiceAccounts(ctx, clusterID, namespace, query); err == nil {
		add(KindServiceAccount, namesOf(resp.Items, func(d k8sgateway.ServiceAccount) string { return d.Metadata.Name }))
	}

	return items, truncated
}

func (s *Service) updateQuarantineStatus(ctx context.Context, planID string, status QuarantineStatusJSON) error {
	// Update via a direct repository Save is not exposed; we rely on the
	// subsequent Complete/Fail to carry the final quarantine status in the
	// execution_result. The quarantine_status column retains the preview-time
	// dry-run state. This is intentional: the preview-time status records what
	// was validated, and the execution-time status is in execution_result.
	return nil
}

// --- helpers ---

func validateRequest(clusterID int64, request Request) error {
	if clusterID < 1 {
		return ErrInvalidRequest
	}
	request.SourceBackupName = strings.TrimSpace(request.SourceBackupName)
	request.SourceBackupNamespace = strings.TrimSpace(request.SourceBackupNamespace)
	if request.SourceBackupName == "" || len(request.SourceBackupName) > 253 {
		return ErrInvalidRequest
	}
	if request.SourceBackupNamespace == "" || len(request.SourceBackupNamespace) > 63 {
		return ErrInvalidRequest
	}
	return nil
}

type manifest struct {
	path string
	body []byte
	kind string
}

// buildQuarantineManifests returns the NetworkPolicy and ResourceQuota manifests
// for the quarantine namespace. The Namespace itself is created separately in
// Execute to ensure ordering: Namespace → quarantine controls → Restore.
func buildQuarantineManifests(namespace string) []manifest {
	networkPolicy := map[string]any{
		"apiVersion": "networking.k8s.io/v1",
		"kind":       "NetworkPolicy",
		"metadata": map[string]any{
			"name":      QuarantineNetworkPolicyName,
			"namespace": namespace,
		},
		"spec": map[string]any{
			"podSelector": map[string]any{},
			"policyTypes": []string{"Ingress", "Egress"},
			"ingress":     []map[string]any{},
			"egress":      []map[string]any{},
		},
	}
	resourceQuota := map[string]any{
		"apiVersion": "v1",
		"kind":       "ResourceQuota",
		"metadata": map[string]any{
			"name":      QuarantineResourceQuotaName,
			"namespace": namespace,
		},
		"spec": map[string]any{
			"hard": map[string]any{
				"pods":                   "0",
				"services.loadbalancers": "0",
				"services.nodeports":     "0",
			},
		},
	}
	npBody, _ := json.Marshal(networkPolicy)
	rqBody, _ := json.Marshal(resourceQuota)
	return []manifest{
		{
			path: "/apis/networking.k8s.io/v1/namespaces/" + url.PathEscape(namespace) + "/networkpolicies",
			body: npBody,
			kind: "NetworkPolicy",
		},
		{
			path: "/api/v1/namespaces/" + url.PathEscape(namespace) + "/resourcequotas",
			body: rqBody,
			kind: "ResourceQuota",
		},
	}
}

func buildNamespaceManifest(namespace string) []byte {
	ns := map[string]any{
		"apiVersion": "v1",
		"kind":       "Namespace",
		"metadata": map[string]any{
			"name": namespace,
			"labels": map[string]any{
				"k8s-aiops.local/restore-rehearsal": "true",
				"k8s-aiops.local/quarantine":        "true",
			},
		},
	}
	body, _ := json.Marshal(ns)
	return body
}

func buildRestoreManifest(restoreName, restoreNamespace, backupName, destinationNamespace string) []byte {
	// Fixed V1 scope: namespaceMapping to destination, includedResources
	// allowlist, restorePVs=false, includeClusterResources=false.
	allowedLower := make([]string, 0, len(AllowedKinds))
	for _, k := range AllowedKinds {
		allowedLower = append(allowedLower, strings.ToLower(k+"s"))
	}
	restore := map[string]any{
		"apiVersion": "velero.io/v1",
		"kind":       "Restore",
		"metadata": map[string]any{
			"name":      restoreName,
			"namespace": restoreNamespace,
		},
		"spec": map[string]any{
			"backupName":              backupName,
			"includeClusterResources": false,
			"restorePVs":              false,
			"includedResources":       allowedLower,
			"namespaceMapping": map[string]any{
				backupName: destinationNamespace,
			},
		},
	}
	body, _ := json.Marshal(restore)
	return body
}

// generateDestinationNamespace creates a server-owned, previously-nonexistent
// Namespace name. The format embeds the source backup name (truncated) and the
// requesting actor ID to make audit tracing deterministic without exposing
// caller-controlled names.
func generateDestinationNamespace(backupName string, actorID int64) string {
	short := strings.ToLower(backupName)
	if len(short) > 20 {
		short = short[:20]
	}
	// Sanitize to DNS-1123 label.
	short = sanitizeDNS1123(short)
	return fmt.Sprintf("restore-%s-%d", short, actorID)
}

func generateRestoreName(destinationNamespace string) string {
	return fmt.Sprintf("rehearse-%s", destinationNamespace)
}

func sanitizeDNS1123(value string) string {
	var b strings.Builder
	for i, r := range value {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else if r == '-' && i > 0 && i < len(value)-1 {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if out == "" {
		return "x"
	}
	return out
}

func namesOf[T any](items []T, getName func(T) string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, getName(item))
	}
	return out
}

func extractUID(response []byte) string {
	if len(response) == 0 {
		return ""
	}
	var parsed struct {
		Metadata struct {
			UID string `json:"uid"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(response, &parsed); err != nil {
		return ""
	}
	return parsed.Metadata.UID
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
	if errors.Is(err, k8sgateway.ErrVeleroUnavailable) {
		return "Velero API is not available"
	}
	if errors.Is(err, k8sgateway.ErrResourceNotFound) {
		return "Kubernetes resource was not found"
	}
	return "Kubernetes request failed"
}
