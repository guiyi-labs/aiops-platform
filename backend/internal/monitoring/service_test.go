package monitoring

import (
	"context"
	"errors"
	"testing"
	"time"

	"k8s-aiops.local/backend/internal/workspace"
)

// fakeLister is an in-memory WorkspaceMembershipLister for service tests.
type fakeLister struct {
	memberships []workspace.WorkspaceMembership
	err         error
}

func (f fakeLister) ListMemberships(_ context.Context, _ int64, _ []string, _ int64) ([]workspace.WorkspaceMembership, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.memberships, nil
}

func validWindow() (time.Time, time.Time) {
	from := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	to := from.Add(1 * time.Hour)
	return from, to
}

// ============================================================================
// NewService
// ============================================================================

func TestNewServiceDefaultsConfig(t *testing.T) {
	s := NewService(Config{}, nil, nil)
	if s.config.MaxClusters != DefaultMaxClusters {
		t.Fatalf("MaxClusters = %d, want %d", s.config.MaxClusters, DefaultMaxClusters)
	}
	if s.config.MaxConcurrent != DefaultMaxConcurrent {
		t.Fatalf("MaxConcurrent = %d, want %d", s.config.MaxConcurrent, DefaultMaxConcurrent)
	}
	if s.config.PerClusterTimeout != DefaultPerClusterTimeout {
		t.Fatalf("PerClusterTimeout = %v, want %v", s.config.PerClusterTimeout, DefaultPerClusterTimeout)
	}
}

// ============================================================================
// ClusterDashboard
// ============================================================================

func TestClusterDashboardReturnsFixedPanels(t *testing.T) {
	s := NewService(Config{}, nil, nil)
	from, to := validWindow()
	for _, template := range []string{TemplateNodeOverview, TemplateWorkloadOverview, TemplatePodOverview} {
		resp, err := s.ClusterDashboard(context.Background(), ClusterDashboardRequest{
			ClusterID: 1, Template: template, From: from, To: to,
		})
		if err != nil {
			t.Fatalf("template %q: %v", template, err)
		}
		if resp.Template != template {
			t.Fatalf("template = %q, want %q", resp.Template, template)
		}
		if len(resp.Panels) != 2 {
			t.Fatalf("panels = %d, want 2", len(resp.Panels))
		}
		if resp.ClusterID != 1 {
			t.Fatalf("cluster_id = %d, want 1", resp.ClusterID)
		}
		if !resp.From.Equal(from.UTC()) {
			t.Fatalf("from = %v, want %v", resp.From, from.UTC())
		}
	}
}

func TestClusterDashboardInvalidTemplate(t *testing.T) {
	s := NewService(Config{}, nil, nil)
	from, to := validWindow()
	_, err := s.ClusterDashboard(context.Background(), ClusterDashboardRequest{
		ClusterID: 1, Template: "custom_promql", From: from, To: to,
	})
	if !errors.Is(err, ErrInvalidTemplate) {
		t.Fatalf("error = %v, want ErrInvalidTemplate", err)
	}
}

func TestClusterDashboardInvalidWindow(t *testing.T) {
	s := NewService(Config{}, nil, nil)
	cases := []struct {
		name string
		from time.Time
		to   time.Time
	}{
		{"zero_from", time.Time{}, time.Now()},
		{"zero_to", time.Now(), time.Time{}},
		{"inverted", time.Now().Add(1 * time.Hour), time.Now()},
		{"exceeds_24h", time.Now().Add(-25 * time.Hour), time.Now()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := s.ClusterDashboard(context.Background(), ClusterDashboardRequest{
				ClusterID: 1, Template: TemplateNodeOverview, From: tc.from, To: tc.to,
			})
			if !errors.Is(err, ErrInvalidWindow) {
				t.Fatalf("error = %v, want ErrInvalidWindow", err)
			}
		})
	}
}

func TestClusterDashboardPanelsAreCloned(t *testing.T) {
	s := NewService(Config{}, nil, nil)
	from, to := validWindow()
	resp, err := s.ClusterDashboard(context.Background(), ClusterDashboardRequest{
		ClusterID: 1, Template: TemplateNodeOverview, From: from, To: to,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Panels[0].Title = "MUTATED"
	// Re-fetch — fixedTemplates must not have been mutated.
	resp2, _ := s.ClusterDashboard(context.Background(), ClusterDashboardRequest{
		ClusterID: 1, Template: TemplateNodeOverview, From: from, To: to,
	})
	if resp2.Panels[0].Title == "MUTATED" {
		t.Fatal("fixedTemplates was mutated; clonePanels is not defensive")
	}
}

// ============================================================================
// WorkspaceDashboard
// ============================================================================

func TestWorkspaceDashboardNilListerReturns404(t *testing.T) {
	s := NewService(Config{}, nil, nil)
	from, to := validWindow()
	_, err := s.WorkspaceDashboard(context.Background(), WorkspaceDashboardRequest{
		WorkspaceID: 1, From: from, To: to,
	})
	if !errors.Is(err, ErrWorkspaceNotFound) {
		t.Fatalf("error = %v, want ErrWorkspaceNotFound", err)
	}
}

func TestWorkspaceDashboardListerErrorReturns404(t *testing.T) {
	lister := fakeLister{err: workspace.ErrWorkspaceNotFound}
	s := NewService(Config{}, nil, lister)
	from, to := validWindow()
	_, err := s.WorkspaceDashboard(context.Background(), WorkspaceDashboardRequest{
		WorkspaceID: 1, From: from, To: to,
	})
	if !errors.Is(err, ErrWorkspaceNotFound) {
		t.Fatalf("error = %v, want ErrWorkspaceNotFound (anti-leakage)", err)
	}
}

func TestWorkspaceDashboardReturnsTopology(t *testing.T) {
	lister := fakeLister{memberships: []workspace.WorkspaceMembership{
		{WorkspaceID: 1, ClusterID: 3, Namespace: "ns-b"},
		{WorkspaceID: 1, ClusterID: 1, Namespace: "ns-a"},
		{WorkspaceID: 1, ClusterID: 1, Namespace: "ns-c"},
		{WorkspaceID: 1, ClusterID: 2, Namespace: "ns-d"},
	}}
	s := NewService(Config{}, nil, lister)
	from, to := validWindow()
	resp, err := s.WorkspaceDashboard(context.Background(), WorkspaceDashboardRequest{
		WorkspaceID: 1, From: from, To: to,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Template != TemplateWorkspaceOverview {
		t.Fatalf("template = %q, want %q", resp.Template, TemplateWorkspaceOverview)
	}
	if len(resp.Clusters) != 3 {
		t.Fatalf("clusters = %d, want 3", len(resp.Clusters))
	}
	// Clusters must be sorted ascending.
	if resp.Clusters[0].ClusterID != 1 || resp.Clusters[1].ClusterID != 2 || resp.Clusters[2].ClusterID != 3 {
		t.Fatalf("cluster order = %v %v %v, want 1 2 3", resp.Clusters[0].ClusterID, resp.Clusters[1].ClusterID, resp.Clusters[2].ClusterID)
	}
	// Namespaces must be sorted within each cluster.
	if len(resp.Clusters[0].Namespaces) != 2 {
		t.Fatalf("cluster 1 namespaces = %d, want 2", len(resp.Clusters[0].Namespaces))
	}
	if resp.Clusters[0].Namespaces[0] != "ns-a" || resp.Clusters[0].Namespaces[1] != "ns-c" {
		t.Fatalf("cluster 1 namespaces = %v, want [ns-a ns-c]", resp.Clusters[0].Namespaces)
	}
	if len(resp.Panels) != 2 {
		t.Fatalf("panels = %d, want 2", len(resp.Panels))
	}
}

func TestWorkspaceDashboardBoundedClusters(t *testing.T) {
	// Build memberships spanning 25 clusters (exceeds DefaultMaxClusters=20).
	memberships := make([]workspace.WorkspaceMembership, 0, 25)
	for i := 1; i <= 25; i++ {
		memberships = append(memberships, workspace.WorkspaceMembership{
			WorkspaceID: 1, ClusterID: int64(i), Namespace: "default",
		})
	}
	lister := fakeLister{memberships: memberships}
	s := NewService(Config{MaxClusters: 3}, nil, lister)
	from, to := validWindow()
	resp, err := s.WorkspaceDashboard(context.Background(), WorkspaceDashboardRequest{
		WorkspaceID: 1, From: from, To: to,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only the first 3 clusters (by insertion order, not ID) should appear.
	// The bound drops new clusters beyond MaxClusters; existing clusters keep
	// receiving namespaces. Insertion order is 1,2,3,...,25 so clusters 1-3.
	if len(resp.Clusters) != 3 {
		t.Fatalf("clusters = %d, want 3 (bounded)", len(resp.Clusters))
	}
	for _, entry := range resp.Clusters {
		if entry.ClusterID > 3 {
			t.Fatalf("cluster %d should have been dropped by bound", entry.ClusterID)
		}
	}
}

func TestWorkspaceDashboardInvalidWindow(t *testing.T) {
	lister := fakeLister{}
	s := NewService(Config{}, nil, lister)
	_, err := s.WorkspaceDashboard(context.Background(), WorkspaceDashboardRequest{
		WorkspaceID: 1, From: time.Time{}, To: time.Time{},
	})
	if !errors.Is(err, ErrInvalidWindow) {
		t.Fatalf("error = %v, want ErrInvalidWindow", err)
	}
}

func TestWorkspaceDashboardEmptyMemberships(t *testing.T) {
	lister := fakeLister{memberships: nil}
	s := NewService(Config{}, nil, lister)
	from, to := validWindow()
	resp, err := s.WorkspaceDashboard(context.Background(), WorkspaceDashboardRequest{
		WorkspaceID: 1, From: from, To: to,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Clusters) != 0 {
		t.Fatalf("clusters = %d, want 0", len(resp.Clusters))
	}
	if len(resp.Panels) != 2 {
		t.Fatalf("panels = %d, want 2", len(resp.Panels))
	}
}
