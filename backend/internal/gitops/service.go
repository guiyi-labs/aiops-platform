package gitops

import (
	"context"
	"errors"
	"time"

	"k8s-aiops.local/backend/internal/apiquery"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

// Statuses mirrors the ArgoCD sync status enumeration we project. Unknown is
// the default when the CR omits sync status.
const (
	SyncUnknown = ""
	Synced      = "Synced"
	OutOfSync   = "OutOfSync"

	HealthUnknown   = ""
	Healthy         = "Healthy"
	Progressing     = "Progressing"
	Degraded        = "Degraded"
	HealthMissing   = "Missing"
	HealthSuspended = "Suspended"
)

var (
	ErrInvalidRequest    = errors.New("gitops request parameters are invalid")
	ErrGitOpsUnavailable = errors.New("ArgoCD is not installed on the target cluster")
	ErrNotFound          = errors.New("gitops application not found")
)

// KubernetesSource is the subset of kubernetes.Service used by the gitops
// service — typed Application list/get + capability probe.
type KubernetesSource interface {
	GitOpsCapability(context.Context, int64) (k8sgateway.GitOpsCapability, error)
	GitOpsApplications(context.Context, int64, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.GitOpsApplication], error)
	GitOpsApplication(context.Context, int64, string) (k8sgateway.GitOpsApplication, error)
}

// Service wraps the typed kubernetes Application surface so handlers get
// domain-specific sentinel errors and ArgoCD-not-installed is projected
// uniformly (callers don't reason about kubernetes.ErrGitOpsUnavailable
// directly).
type Service struct {
	kubernetes KubernetesSource
	now        func() time.Time
}

func NewService(kubernetes KubernetesSource) *Service {
	return &Service{kubernetes: kubernetes, now: time.Now}
}

// Capability reports whether ArgoCD is installed on the cluster and the
// Application API group is reachable.
func (s *Service) Capability(ctx context.Context, clusterID int64) (k8sgateway.GitOpsCapability, error) {
	if clusterID < 1 {
		return k8sgateway.GitOpsCapability{}, ErrInvalidRequest
	}
	return s.kubernetes.GitOpsCapability(ctx, clusterID)
}

// List returns a page of ArgoCD Application projections for the given
// cluster. When ArgoCD is not installed, List returns an empty list with
// no error (mirrors the M52 empty-list pattern for unavailable addons) so
// the UI renders an empty "ArgoCD not installed" state instead of 503.
func (s *Service) List(ctx context.Context, clusterID int64, query apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.GitOpsApplication], error) {
	if clusterID < 1 {
		return apiquery.ListResponse[k8sgateway.GitOpsApplication]{}, ErrInvalidRequest
	}
	resp, err := s.kubernetes.GitOpsApplications(ctx, clusterID, query)
	if errors.Is(err, k8sgateway.ErrGitOpsUnavailable) {
		return apiquery.ListResponse[k8sgateway.GitOpsApplication]{
			Items: []k8sgateway.GitOpsApplication{},
			Total: 0,
		}, nil
	}
	return resp, err
}

// Get reads a single ArgoCD Application by name. Returns ErrNotFound when
// the named Application is missing, and an empty GitOpsApplication +
// ErrGitOpsUnavailable when ArgoCD is not installed (handlers map to 503
// with a friendly message).
func (s *Service) Get(ctx context.Context, clusterID int64, name string) (k8sgateway.GitOpsApplication, error) {
	if clusterID < 1 {
		return k8sgateway.GitOpsApplication{}, ErrInvalidRequest
	}
	name = trimName(name)
	if name == "" {
		return k8sgateway.GitOpsApplication{}, ErrInvalidRequest
	}
	app, err := s.kubernetes.GitOpsApplication(ctx, clusterID, name)
	if errors.Is(err, k8sgateway.ErrResourceNotFound) {
		return k8sgateway.GitOpsApplication{}, ErrNotFound
	}
	return app, err
}

// IsSynced reports whether the application's sync status is Synced. An
// unknown/empty status is treated as not-synced.
func IsSynced(status string) bool { return status == Synced }

// IsHealthy reports whether the application's health status is Healthy. An
// unknown/empty status is treated as not-healthy.
func IsHealthy(status string) bool { return status == Healthy }

func trimName(name string) string {
	for len(name) > 0 && (name[0] == ' ' || name[0] == '\t' || name[0] == '\n' || name[0] == '\r') {
		name = name[1:]
	}
	for len(name) > 0 && (name[len(name)-1] == ' ' || name[len(name)-1] == '\t' || name[len(name)-1] == '\n' || name[len(name)-1] == '\r') {
		name = name[:len(name)-1]
	}
	return name
}
