package monitoring

import (
	"context"
	"errors"
	"sort"
	"time"

	"k8s-aiops.local/backend/internal/metricshistory"
	"k8s-aiops.local/backend/internal/workspace"
)

// Bounded fan-out constants (ADR 0065 §2). These mirror the federation
// service's resource-summary bounds so the workspace dashboard cannot fan
// out unboundedly.
const (
	DefaultMaxClusters       = 20
	DefaultMaxConcurrent     = 4
	DefaultPerClusterTimeout = 4 * time.Second
)

var (
	// ErrInvalidTemplate is returned when the requested template name is not
	// in fixedTemplates. The handler maps it to 400 (the caller supplied a
	// bad template name, not a missing resource — this is NOT anti-leakage).
	ErrInvalidTemplate = errors.New("monitoring: invalid dashboard template")
	// ErrInvalidWindow is returned when the from/to window is zero, inverted,
	// or exceeds MaxDashboardWindow.
	ErrInvalidWindow = errors.New("monitoring: invalid dashboard time window")
	// ErrWorkspaceNotFound is returned when the workspace does not exist or
	// the caller lacks workspace_viewer. The handler maps it to 404
	// (anti-leakage — indistinguishable from a missing workspace).
	ErrWorkspaceNotFound = errors.New("monitoring: workspace not found or access denied")
)

// WorkspaceMembershipLister is the minimal interface the monitoring service
// needs from the workspace service. It mirrors the federation service's
// ClusterLister pattern — a narrow interface that is easy to stub in tests.
// The production implementation is *workspace.Service.
type WorkspaceMembershipLister interface {
	ListMemberships(ctx context.Context, actorUserID int64, actorRoles []string, workspaceID int64) ([]workspace.WorkspaceMembership, error)
}

// Config bounds the workspace-dashboard fan-out.
type Config struct {
	MaxClusters       int
	MaxConcurrent     int
	PerClusterTimeout time.Duration
}

// Service provides the monitoring dashboard endpoints. It is a thin
// aggregation layer over metricshistory.Service (M21) and workspace.Service
// (M46) — it does NOT pre-fetch time series; instead it returns the fixed
// template structure and, for the workspace dashboard, the cross-cluster
// (cluster, namespaces) topology that the frontend fans out over (ADR 0065
// §1-§2).
type Service struct {
	config          Config
	metricsHistory  *metricshistory.Service
	workspaceLister WorkspaceMembershipLister
}

// NewService constructs a monitoring Service. metricsHistory may be nil
// (reserved for future pre-fetch; currently unused). workspaceLister may be
// nil — when nil, WorkspaceDashboard returns ErrWorkspaceNotFound so the
// handler can respond 503/404. This mirrors the federation service's
// nil-tolerance pattern.
func NewService(config Config, metricsHistory *metricshistory.Service, workspaceLister WorkspaceMembershipLister) *Service {
	if config.MaxClusters <= 0 {
		config.MaxClusters = DefaultMaxClusters
	}
	if config.MaxConcurrent <= 0 {
		config.MaxConcurrent = DefaultMaxConcurrent
	}
	if config.PerClusterTimeout <= 0 {
		config.PerClusterTimeout = DefaultPerClusterTimeout
	}
	return &Service{config: config, metricsHistory: metricsHistory, workspaceLister: workspaceLister}
}

// ClusterDashboard returns the fixed template for a single cluster. It does
// NOT pre-fetch time series — the frontend uses the returned panel
// descriptors to drive calls to GET /clusters/:cluster_id/metrics/history
// (ADR 0065 §1).
func (s *Service) ClusterDashboard(ctx context.Context, req ClusterDashboardRequest) (ClusterDashboardResponse, error) {
	panels, ok := fixedTemplates[req.Template]
	if !ok {
		return ClusterDashboardResponse{}, ErrInvalidTemplate
	}
	if err := validateWindow(req.From, req.To); err != nil {
		return ClusterDashboardResponse{}, err
	}
	return ClusterDashboardResponse{
		Template:  req.Template,
		ClusterID: req.ClusterID,
		From:      req.From.UTC(),
		To:        req.To.UTC(),
		Panels:    clonePanels(panels),
	}, nil
}

// WorkspaceDashboard returns the fixed template plus the workspace's
// cross-cluster (cluster, namespaces) topology. The topology is fetched via
// workspace.Service.ListMemberships (which enforces workspace_viewer). The
// fan-out is bounded by config.MaxClusters; excess clusters are silently
// dropped (the frontend gets a bounded, stable topology). The backend does
// NOT pre-fetch per-cluster time series — the frontend fans out per-cluster
// /metrics/history calls using the returned topology (ADR 0065 §2).
func (s *Service) WorkspaceDashboard(ctx context.Context, req WorkspaceDashboardRequest) (WorkspaceDashboardResponse, error) {
	if s.workspaceLister == nil {
		// Workspace service disabled — cannot resolve memberships.
		return WorkspaceDashboardResponse{}, ErrWorkspaceNotFound
	}
	panels, ok := fixedTemplates[TemplateWorkspaceOverview]
	if !ok {
		// Should never happen — TemplateWorkspaceOverview is a compile-time constant.
		return WorkspaceDashboardResponse{}, ErrInvalidTemplate
	}
	if err := validateWindow(req.From, req.To); err != nil {
		return WorkspaceDashboardResponse{}, err
	}

	memberships, err := s.workspaceLister.ListMemberships(ctx, req.ActorUserID, req.ActorRoles, req.WorkspaceID)
	if err != nil {
		// workspace.Service.ListMemberships returns workspace.ErrWorkspaceNotFound
		// when the workspace does not exist or the caller lacks viewer. We
		// collapse both into ErrWorkspaceNotFound for anti-leakage (ADR 0065 §4).
		return WorkspaceDashboardResponse{}, ErrWorkspaceNotFound
	}

	// Group memberships by cluster_id → namespaces, bounded by MaxClusters.
	clusterNS := make(map[int64][]string)
	for _, m := range memberships {
		if len(clusterNS) >= s.config.MaxClusters {
			if _, exists := clusterNS[m.ClusterID]; !exists {
				continue // drop new clusters beyond the bound
			}
		}
		clusterNS[m.ClusterID] = append(clusterNS[m.ClusterID], m.Namespace)
	}

	// Sort cluster IDs ascending for stable rendering (mirrors federation).
	clusterIDs := make([]int64, 0, len(clusterNS))
	for id := range clusterNS {
		clusterIDs = append(clusterIDs, id)
	}
	sort.Slice(clusterIDs, func(i, j int) bool { return clusterIDs[i] < clusterIDs[j] })

	entries := make([]WorkspaceClusterEntry, 0, len(clusterIDs))
	for _, id := range clusterIDs {
		namespaces := clusterNS[id]
		sort.Strings(namespaces)
		entries = append(entries, WorkspaceClusterEntry{ClusterID: id, Namespaces: namespaces})
	}

	return WorkspaceDashboardResponse{
		Template:    TemplateWorkspaceOverview,
		WorkspaceID: req.WorkspaceID,
		From:        req.From.UTC(),
		To:          req.To.UTC(),
		Panels:      clonePanels(panels),
		Clusters:    entries,
	}, nil
}

// validateWindow enforces the from/to bounds: both must be non-zero, from
// must be before to, and the window must not exceed MaxDashboardWindow
// (24h, matching metricshistory's MaxQueryWindow — ADR 0065 §2).
func validateWindow(from, to time.Time) error {
	if from.IsZero() || to.IsZero() {
		return ErrInvalidWindow
	}
	if !from.Before(to) {
		return ErrInvalidWindow
	}
	if to.Sub(from) > MaxDashboardWindow {
		return ErrInvalidWindow
	}
	return nil
}

// clonePanels returns a defensive copy so callers cannot mutate fixedTemplates.
func clonePanels(panels []Panel) []Panel {
	out := make([]Panel, len(panels))
	copy(out, panels)
	return out
}
