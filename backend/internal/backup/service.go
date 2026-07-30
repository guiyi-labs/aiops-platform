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

	"k8s-aiops.local/backend/internal/cluster"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

var (
	kubernetesNamePattern = regexp.MustCompile(`^[a-z0-9](?:[-a-z0-9.]*[a-z0-9])?$`)
	ttlPattern            = regexp.MustCompile(`^[0-9]+(h|m|s)$`)
)

// KubernetesSource is the subset of kubernetes.Service used by the backup service.
type KubernetesSource interface {
	VeleroCapability(context.Context, int64) (k8sgateway.VeleroCapability, error)
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

	// Preflight 2: storage location must exist.
	locations, err := s.kubernetes.BackupStorageLocations(ctx, clusterID, request.BackupNamespace)
	if err != nil {
		return Plan{}, err
	}
	if !bslExists(locations, request.StorageLocation) {
		return Plan{}, ErrStorageLocationNotFound
	}

	// Preflight 3: backup name must not already exist.
	exists, err := s.kubernetes.VeleroBackupExists(ctx, clusterID, request.BackupNamespace, request.BackupName)
	if err != nil {
		return Plan{}, err
	}
	if exists {
		return Plan{}, ErrBackupNameConflict
	}

	// Preflight 4: server-side dry-run create.
	manifest := buildBackupManifest(request)
	if _, err := s.kubernetes.CreateResource(ctx, clusterID,
		"/apis/velero.io/v1/namespaces/"+url.PathEscape(request.BackupNamespace)+"/backups",
		manifest, true); err != nil {
		return Plan{}, mapCreateError(err)
	}

	id, token, tokenHash, err := newIdentity()
	if err != nil {
		return Plan{}, err
	}
	now := s.now().UTC().Truncate(time.Second)
	plan := Plan{
		ID:                      id,
		ClusterID:               clusterID,
		Status:                  StatusAwaitingConfirmation,
		BackupName:              request.BackupName,
		BackupNamespace:         request.BackupNamespace,
		IncludedNamespaces:      request.IncludedNamespaces,
		StorageLocation:         request.StorageLocation,
		TTL:                     request.TTL,
		IncludeClusterResources: request.IncludeClusterResources,
		SnapshotVolumes:         request.SnapshotVolumes,
		LabelSelector:           LabelSelectorMap(request.LabelSelector),
		VeleroVersion:           cap.Version,
		ConfirmationTokenHash:   tokenHash,
		RequestedByUserID:       &actor.ID,
		RequestedByName:         actor.Name,
		ExpiresAt:               now.Add(s.planTTL),
		ConfirmationToken:       token,
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

	manifest := buildBackupManifest(Request{
		BackupName:              plan.BackupName,
		BackupNamespace:         plan.BackupNamespace,
		IncludedNamespaces:      plan.IncludedNamespaces,
		StorageLocation:         plan.StorageLocation,
		TTL:                     plan.TTL,
		IncludeClusterResources: plan.IncludeClusterResources,
		SnapshotVolumes:         plan.SnapshotVolumes,
		LabelSelector:           plan.LabelSelector,
	})

	_, err = s.kubernetes.CreateResource(ctx, plan.ClusterID,
		"/apis/velero.io/v1/namespaces/"+url.PathEscape(plan.BackupNamespace)+"/backups",
		manifest, false)
	if err != nil {
		failed, saveErr := s.repository.Fail(ctx, plan.ID, idempotencyKey, safeExecutionError(err))
		if saveErr != nil {
			return Plan{}, saveErr
		}
		return failed, fmt.Errorf("%w: %v", ErrExecutionFailed, err)
	}
	return s.repository.Complete(ctx, plan.ID, idempotencyKey, s.now().UTC())
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
	request.BackupName = strings.TrimSpace(request.BackupName)
	request.BackupNamespace = strings.TrimSpace(request.BackupNamespace)
	request.StorageLocation = strings.TrimSpace(request.StorageLocation)
	request.TTL = strings.TrimSpace(request.TTL)

	if !validName(request.BackupName, 253) {
		return ErrInvalidRequest
	}
	if !validName(request.BackupNamespace, 63) {
		return ErrInvalidRequest
	}
	if !validName(request.StorageLocation, 253) {
		return ErrInvalidRequest
	}
	if request.TTL == "" {
		request.TTL = DefaultTTL
	}
	if !ttlPattern.MatchString(request.TTL) {
		return ErrInvalidRequest
	}
	// IncludedNamespaces: 1-10 explicit names, no wildcard.
	if len(request.IncludedNamespaces) == 0 || len(request.IncludedNamespaces) > 10 {
		return ErrInvalidRequest
	}
	for _, ns := range request.IncludedNamespaces {
		if !validName(ns, 63) {
			return ErrInvalidRequest
		}
	}
	// LabelSelector: keys and values must be non-empty, max 10 entries.
	if len(request.LabelSelector) > 10 {
		return ErrInvalidRequest
	}
	for k, v := range request.LabelSelector {
		if k == "" || v == "" || len(k) > 63 || len(v) > 63 {
			return ErrInvalidRequest
		}
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

// buildBackupManifest constructs the Velero Backup CR JSON manifest from the
// fixed-scope request. No hooks, no schedules, no arbitrary fields.
func buildBackupManifest(request Request) []byte {
	spec := map[string]any{
		"storageLocation":         request.StorageLocation,
		"ttl":                     request.TTL,
		"includedNamespaces":      request.IncludedNamespaces,
		"includeClusterResources": request.IncludeClusterResources,
		"snapshotVolumes":         request.SnapshotVolumes,
	}
	if len(request.LabelSelector) > 0 {
		spec["labelSelector"] = map[string]any{
			"matchLabels": request.LabelSelector,
		}
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
