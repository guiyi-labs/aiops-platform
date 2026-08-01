package gitops_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"k8s-aiops.local/backend/internal/apiquery"
	"k8s-aiops.local/backend/internal/gitops"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

type fakeKubernetes struct {
	capability   func(context.Context, int64) (k8sgateway.GitOpsCapability, error)
	applications func(context.Context, int64, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.GitOpsApplication], error)
	application  func(context.Context, int64, string) (k8sgateway.GitOpsApplication, error)
}

func (f *fakeKubernetes) GitOpsCapability(ctx context.Context, clusterID int64) (k8sgateway.GitOpsCapability, error) {
	return f.capability(ctx, clusterID)
}
func (f *fakeKubernetes) GitOpsApplications(ctx context.Context, clusterID int64, query apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.GitOpsApplication], error) {
	return f.applications(ctx, clusterID, query)
}
func (f *fakeKubernetes) GitOpsApplication(ctx context.Context, clusterID int64, name string) (k8sgateway.GitOpsApplication, error) {
	return f.application(ctx, clusterID, name)
}

func TestService_List_UnavailableReturnsEmptyList(t *testing.T) {
	fake := &fakeKubernetes{
		capability: func(context.Context, int64) (k8sgateway.GitOpsCapability, error) {
			return k8sgateway.GitOpsCapability{Installed: false}, nil
		},
		applications: func(context.Context, int64, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.GitOpsApplication], error) {
			return apiquery.ListResponse[k8sgateway.GitOpsApplication]{}, k8sgateway.ErrGitOpsUnavailable
		},
	}
	svc := gitops.NewService(fake)
	resp, err := svc.List(context.Background(), 1, apiquery.ListQuery{})
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Total)
	assert.Empty(t, resp.Items)
}

func TestService_List_InvalidClusterID(t *testing.T) {
	svc := gitops.NewService(nil)
	_, err := svc.List(context.Background(), 0, apiquery.ListQuery{})
	require.ErrorIs(t, err, gitops.ErrInvalidRequest)
}

func TestService_Get_NotFound(t *testing.T) {
	fake := &fakeKubernetes{
		application: func(context.Context, int64, string) (k8sgateway.GitOpsApplication, error) {
			return k8sgateway.GitOpsApplication{}, k8sgateway.ErrResourceNotFound
		},
	}
	svc := gitops.NewService(fake)
	_, err := svc.Get(context.Background(), 1, "missing-app")
	require.ErrorIs(t, err, gitops.ErrNotFound)
}

func TestService_Get_InvalidName(t *testing.T) {
	svc := gitops.NewService(nil)
	_, err := svc.Get(context.Background(), 1, "   ")
	require.ErrorIs(t, err, gitops.ErrInvalidRequest)
}

func TestService_Capability_Installed(t *testing.T) {
	fake := &fakeKubernetes{
		capability: func(context.Context, int64) (k8sgateway.GitOpsCapability, error) {
			return k8sgateway.GitOpsCapability{Installed: true, Version: "v1alpha1"}, nil
		},
	}
	svc := gitops.NewService(fake)
	cap, err := svc.Capability(context.Background(), 1)
	require.NoError(t, err)
	assert.True(t, cap.Installed)
	assert.Equal(t, "v1alpha1", cap.Version)
}

func TestPredicates(t *testing.T) {
	assert.True(t, gitops.IsSynced(gitops.Synced))
	assert.False(t, gitops.IsSynced(gitops.SyncUnknown))
	assert.False(t, gitops.IsSynced(gitops.OutOfSync))
	assert.True(t, gitops.IsHealthy(gitops.Healthy))
	assert.False(t, gitops.IsHealthy(gitops.Degraded))
	// Now roundtrip against a projected Application returned from List/Get.
	app := k8sgateway.GitOpsApplication{
		Name:         "guestbook",
		SyncStatus:   gitops.Synced,
		HealthStatus: gitops.Healthy,
		CreatedAt:    time.Now().Format(time.RFC3339),
	}
	assert.True(t, gitops.IsSynced(app.SyncStatus))
	assert.True(t, gitops.IsHealthy(app.HealthStatus))
}

func TestService_List_WithData(t *testing.T) {
	items := []k8sgateway.GitOpsApplication{
		{Name: "app-a", Project: "default", SyncStatus: gitops.Synced, HealthStatus: gitops.Healthy},
		{Name: "app-b", Project: "default", SyncStatus: gitops.OutOfSync, HealthStatus: gitops.Degraded},
	}
	fake := &fakeKubernetes{
		applications: func(_ context.Context, _ int64, query apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.GitOpsApplication], error) {
			remaining := 0
			if len(items) > query.Offset+query.Limit && query.Limit > 0 {
				remaining = len(items) - query.Offset - query.Limit
			}
			return apiquery.ListResponse[k8sgateway.GitOpsApplication]{
				Items:     items,
				Total:     len(items),
				Remaining: remaining,
			}, nil
		},
	}
	svc := gitops.NewService(fake)
	resp, err := svc.List(context.Background(), 1, apiquery.ListQuery{Limit: 50})
	require.NoError(t, err)
	assert.Equal(t, 2, resp.Total)
	assert.Equal(t, "app-a", resp.Items[0].Name)
	assert.Equal(t, "app-b", resp.Items[1].Name)
}

func TestService_Get_Unavailable(t *testing.T) {
	fake := &fakeKubernetes{
		application: func(context.Context, int64, string) (k8sgateway.GitOpsApplication, error) {
			return k8sgateway.GitOpsApplication{}, k8sgateway.ErrGitOpsUnavailable
		},
	}
	svc := gitops.NewService(fake)
	app, err := svc.Get(context.Background(), 1, "any")
	require.ErrorIs(t, err, k8sgateway.ErrGitOpsUnavailable)
	assert.Empty(t, app.Name)
	assert.True(t, errors.Is(err, k8sgateway.ErrGitOpsUnavailable))
}
