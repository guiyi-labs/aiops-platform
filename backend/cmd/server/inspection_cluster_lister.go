package main

import (
	"context"

	"k8s-aiops.local/backend/internal/cluster"
	"k8s-aiops.local/backend/internal/inspection"
)

// inspectionClusterLister adapts *cluster.Service to inspection.ClusterLister.
// The interface only exposes reachable cluster IDs + names; the inspection
// service fans out across them for ad-hoc runs (M52 §3).
type inspectionClusterLister struct {
	svc *cluster.Service
}

// List returns all clusters visible to the platform. At M52 we list without
// authorization filtering because the inspection service itself is only
// callable by authenticated users and plan mutations are ops_admin-gated.
func (l inspectionClusterLister) List(ctx context.Context) ([]struct {
	ID   int64
	Name string
}, error) {
	if l.svc == nil {
		return nil, nil
	}
	rows, err := l.svc.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]struct {
		ID   int64
		Name string
	}, 0, len(rows))
	for _, c := range rows {
		if !c.Enabled {
			continue
		}
		out = append(out, struct {
			ID   int64
			Name string
		}{ID: c.ID, Name: c.Name})
	}
	return out, nil
}

var _ inspection.ClusterLister = inspectionClusterLister{}
