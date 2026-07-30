package backup

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
	"regexp"
	"strings"
	"time"

	"k8s-aiops.local/backend/internal/apiquery"
	"k8s-aiops.local/backend/internal/cluster"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

var (
	kubernetesNamePattern = regexp.MustCompile(`^[a-z0-9](?:[-a-z0-9.]*[a-z0-9])?$`)
)

// KubernetesSource is the subset of kubernetes.Service used by the backup service.
type KubernetesSource interface {
	VeleroCapability(context.Context, int64) (k8sgateway.VeleroCapability, error)
	Namespaces(context.Context, int64, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Namespace], error)
	BackupStorageLocations(context.Context, int64, string) ([]k8sgateway.BackupStorageLocation, error)
	VeleroBackupExists(context.Context, int64, string, string) (bool, error)
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
	return &Service{kubernetes: kubernetes, repository: repository, planTTL: 10 * time.Minute, claimTTL: time.Minute, now: time.Now}
}

// Preview validates the request, runs preflight checks (Velero installed, BSL
// exists, backup name not taken), performs a server-side dry-run create, and
// persists an awaiting-confirmation plan with a one-time confirmation token.
func (s *Service) Preview(ctx context.Context, clusterID int64, request Request, actor ActorRef) (Plan, error) {
	request = normalizeRequest(request)
	if err := validateRequest(clusterID, request); err != nil {
		return Plan{}, err
	}
	id, token, tokenHash, err := newIdentity()
	if err != nil {
		return Plan{}, err
	}
	backupName := generateBackupName(request.SourceNamespace, id)
	internalRequest := fixedRequest(request, backupName)

	// The source Namespace identity is part of the confirmation contract.
	ns, err := s.sourceNamespace(ctx, clusterID, request.SourceNamespace)
	if err != nil {
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

	// Preflight 2: storage location must exist.
	locations, err := s.kubernetes.BackupStorageLocations(ctx, clusterID, DefaultBackupNamespace)
	if err != nil {
		return Plan{}, err
	}
	if !bslExists(locations, request.StorageLocation) {
		return Plan{}, ErrStorageLocationNotFound
	}
	if !bslAvailable(locations, request.StorageLocation) {
		return Plan{}, ErrStorageLocationUnavailable
	}

	// Preflight 3: backup name must not already exist.
	exists, err := s.kubernetes.VeleroBackupExists(ctx, clusterID, DefaultBackupNamespace, backupName)
	if err != nil {
		return Plan{}, err
	}
	if exists {
		return Plan{}, ErrBackupNameConflict
	}

	// Preflight 4: server-side dry-run create.
	manifest := buildBackupManifest(internalRequest)
	if _, err := s.kubernetes.CreateResource(ctx, clusterID,
		"/apis/velero.io/v1/namespaces/"+url.PathEscape(DefaultBackupNamespace)+"/backups",
		manifest, true); err != nil {
		return Plan{}, mapCreateError(err)
	}

	now := s.now().UTC().Truncate(time.Second)
	plan := Plan{
		ID:                             id,
		ClusterID:                      clusterID,
		Status:                         StatusAwaitingConfirmation,
		BackupName:                     backupName,
		BackupNamespace:                DefaultBackupNamespace,
		IncludedNamespaces:             []string{request.SourceNamespace},
		SourceNamespaceUID:             ns.Metadata.UID,
		SourceNamespaceResourceVersion: ns.Metadata.ResourceVersion,
		StorageLocation:                request.StorageLocation,
		TTL:                            request.TTL,
		IncludeClusterResources:        false,
		SnapshotVolumes:                false,
		LabelSelector:                  LabelSelectorMap{},
		VeleroVersion:                  cap.Version,
		ConfirmationTokenHash:          tokenHash,
		RequestedByUserID:              &actor.ID,
		RequestedByName:                actor.Name,
		ExpiresAt:                      now.Add(s.planTTL),
		ConfirmationToken:              token,
	}

	plan.ConfirmationToken = ""
	if err := s.repository.Save(ctx, &plan); err != nil {
		return Plan{}, err
	}
	plan.ConfirmationToken = token
	return plan, nil
}

// Execute claims the plan with the confirmation token and idempotency key,
// then creates the actual Velero Backup CR on the target cluster.
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

	cap, err := s.kubernetes.VeleroCapability(ctx, plan.ClusterID)
	if err != nil || !cap.Installed {
		return s.failExecution(ctx, plan, idempotencyKey, ErrVeleroNotInstalled)
	}
	locations, err := s.kubernetes.BackupStorageLocations(ctx, plan.ClusterID, plan.BackupNamespace)
	if err != nil {
		return s.failExecution(ctx, plan, idempotencyKey, err)
	}
	if !bslAvailable(locations, plan.StorageLocation) {
		return s.failExecution(ctx, plan, idempotencyKey, ErrStorageLocationUnavailable)
	}
	if len(plan.IncludedNamespaces) != 1 {
		return s.failExecution(ctx, plan, idempotencyKey, ErrStaleSourceNamespace)
	}
	ns, err := s.sourceNamespace(ctx, plan.ClusterID, plan.IncludedNamespaces[0])
	if err != nil || ns.Metadata.UID != plan.SourceNamespaceUID || ns.Metadata.ResourceVersion != plan.SourceNamespaceResourceVersion {
		return s.failExecution(ctx, plan, idempotencyKey, ErrStaleSourceNamespace)
	}
	exists, err := s.kubernetes.VeleroBackupExists(ctx, plan.ClusterID, plan.BackupNamespace, plan.BackupName)
	if err != nil || exists {
		if err == nil {
			err = ErrBackupNameConflict
		}
		return s.failExecution(ctx, plan, idempotencyKey, err)
	}

	manifest := buildBackupManifest(fixedManifestRequest{
		BackupName:         plan.BackupName,
		BackupNamespace:    plan.BackupNamespace,
		IncludedNamespaces: plan.IncludedNamespaces,
		StorageLocation:    plan.StorageLocation,
		TTL:                plan.TTL,
	})

	created, err := s.kubernetes.CreateResource(ctx, plan.ClusterID,
		"/apis/velero.io/v1/namespaces/"+url.PathEscape(plan.BackupNamespace)+"/backups",
		manifest, false)
	if err != nil {
		failed, saveErr := s.repository.Fail(ctx, plan.ID, idempotencyKey, safeExecutionError(err))
		if saveErr != nil {
			return Plan{}, saveErr
		}
		return failed, fmt.Errorf("%w: %v", ErrExecutionFailed, err)
	}
	uid, resourceVersion := extractObjectIdentity(created)
	if uid == "" || resourceVersion == "" {
		return s.failExecution(ctx, plan, idempotencyKey, errors.New("created Backup response omitted identity"))
	}
	return s.repository.Complete(ctx, plan.ID, idempotencyKey, uid, resourceVersion, s.now().UTC())
}

func (s *Service) List(ctx context.Context, clusterID int64) ([]Plan, error) {
	if clusterID < 1 {
		return nil, ErrInvalidRequest
	}
	return s.repository.List(ctx, clusterID)
}

func validateRequest(clusterID int64, request Request) error {
	if clusterID < 1 {
		return ErrInvalidRequest
	}
	request.SourceNamespace = strings.TrimSpace(request.SourceNamespace)
	request.StorageLocation = strings.TrimSpace(request.StorageLocation)
	request.TTL = strings.TrimSpace(request.TTL)

	if !validName(request.SourceNamespace, 63) {
		return ErrInvalidRequest
	}
	if !validName(request.StorageLocation, 253) {
		return ErrInvalidRequest
	}
	if request.TTL != "24h" && request.TTL != "168h" && request.TTL != "720h" {
		return ErrInvalidRequest
	}
	return nil
}

func validName(value string, maximum int) bool {
	value = strings.TrimSpace(value)
	return value != "" && len(value) <= maximum && kubernetesNamePattern.MatchString(value)
}

func bslExists(locations []k8sgateway.BackupStorageLocation, name string) bool {
	for _, loc := range locations {
		if loc.Name == name {
			return true
		}
	}
	return false
}

func bslAvailable(locations []k8sgateway.BackupStorageLocation, name string) bool {
	for _, loc := range locations {
		if loc.Name == name && loc.Phase == "Available" {
			return true
		}
	}
	return false
}

// buildBackupManifest constructs the Velero Backup CR JSON manifest from the
// fixed-scope request. No hooks, no schedules, no arbitrary fields.
type fixedManifestRequest struct {
	BackupName         string
	BackupNamespace    string
	IncludedNamespaces []string
	StorageLocation    string
	TTL                string
}

func buildBackupManifest(request fixedManifestRequest) []byte {
	spec := map[string]any{
		"storageLocation":         request.StorageLocation,
		"ttl":                     request.TTL,
		"includedNamespaces":      request.IncludedNamespaces,
		"includeClusterResources": false,
		"snapshotVolumes":         false,
	}
	manifest := map[string]any{
		"apiVersion": "velero.io/v1",
		"kind":       "Backup",
		"metadata": map[string]any{
			"name":      request.BackupName,
			"namespace": request.BackupNamespace,
		},
		"spec": spec,
	}
	body, _ := json.Marshal(manifest)
	return body
}

func normalizeRequest(request Request) Request {
	request.SourceNamespace = strings.TrimSpace(request.SourceNamespace)
	request.StorageLocation = strings.TrimSpace(request.StorageLocation)
	request.TTL = strings.TrimSpace(request.TTL)
	if request.TTL == "" {
		request.TTL = DefaultTTL
	}
	return request
}

func fixedRequest(request Request, backupName string) fixedManifestRequest {
	return fixedManifestRequest{BackupName: backupName, BackupNamespace: DefaultBackupNamespace, IncludedNamespaces: []string{request.SourceNamespace}, StorageLocation: request.StorageLocation, TTL: request.TTL}
}

func generateBackupName(sourceNamespace, id string) string {
	prefix := strings.Trim(strings.ToLower(sourceNamespace), ".-")
	if len(prefix) > 40 {
		prefix = prefix[:40]
	}
	return "aiops-" + prefix + "-" + id[:8]
}

func (s *Service) sourceNamespace(ctx context.Context, clusterID int64, name string) (k8sgateway.Namespace, error) {
	resp, err := s.kubernetes.Namespaces(ctx, clusterID, apiquery.ListQuery{Name: name, Limit: 1})
	if err != nil {
		return k8sgateway.Namespace{}, err
	}
	if len(resp.Items) != 1 || resp.Items[0].Metadata.Name != name {
		return k8sgateway.Namespace{}, ErrSourceNamespaceNotFound
	}
	if resp.Items[0].Metadata.UID == "" || resp.Items[0].Metadata.ResourceVersion == "" {
		return k8sgateway.Namespace{}, ErrStaleSourceNamespace
	}
	return resp.Items[0], nil
}

func (s *Service) failExecution(ctx context.Context, plan Plan, key string, err error) (Plan, error) {
	failed, saveErr := s.repository.Fail(ctx, plan.ID, key, safeExecutionError(err))
	if saveErr != nil {
		return Plan{}, saveErr
	}
	return failed, errors.Join(ErrExecutionFailed, err)
}

func extractObjectIdentity(body []byte) (string, string) {
	var object struct {
		Metadata struct {
			UID             string `json:"uid"`
			ResourceVersion string `json:"resourceVersion"`
		} `json:"metadata"`
	}
	if json.Unmarshal(body, &object) != nil {
		return "", ""
	}
	return object.Metadata.UID, object.Metadata.ResourceVersion
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
		return fmt.Sprintf("Kubernetes API rejected backup creation with HTTP %d", status.StatusCode)
	}
	if errors.Is(err, k8sgateway.ErrVeleroUnavailable) {
		return "Velero API is not available"
	}
	if errors.Is(err, k8sgateway.ErrResourceNotFound) {
		return "Kubernetes backup target was not found"
	}
	return "Kubernetes backup creation request failed"
}

func mapCreateError(err error) error {
	var status cluster.APIStatusError
	if errors.As(err, &status) {
		switch status.StatusCode {
		case 409:
			return ErrBackupNameConflict
		case 404:
			return ErrVeleroNotInstalled
		}
	}
	return err
}
