package main

import (
	"context"

	"k8s-aiops.local/backend/internal/apiquery"
	"k8s-aiops.local/backend/internal/federation"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

// kubernetesClusterLister adapts the typed kubernetes.Service list methods to
// the federation.ClusterLister interface. It translates a federation.GVR into
// the appropriate typed list call and returns only the Total count (the
// federation resource summary does not need the actual items).
//
// The adapter is intentionally a separate file from main.go so the import
// graph stays clean: the federation package does not depend on the kubernetes
// package, only this adapter does.
type kubernetesClusterLister struct {
	service *k8sgateway.Service
}

// newKubernetesClusterLister returns a federation.ClusterLister backed by the
// kubernetes gateway. The lister uses a limit of 1 because only the Total
// field of the ListResponse is consumed — the resource summary counts
// resources, it does not return them.
func newKubernetesClusterLister(service *k8sgateway.Service) federation.ClusterLister {
	if service == nil {
		return nil
	}
	return &kubernetesClusterLister{service: service}
}

func (a *kubernetesClusterLister) ListResource(ctx context.Context, clusterID int64, gvr federation.GVR, namespace string) (federation.CountResult, error) {
	// limit=1 is sufficient: the kubernetes gateway returns the full Total
	// in the ListResponse envelope, and the resource summary only consumes
	// Total. Page=1, limit=1 keeps the per-cluster call cheap.
	query := apiquery.ListQuery{Page: 1, Limit: 1, SortBy: "name", Ascending: true}
	total, err := a.listTotal(ctx, clusterID, gvr, namespace, query)
	if err != nil {
		return federation.CountResult{Err: err}, err
	}
	return federation.CountResult{Total: total}, nil
}

// listTotal dispatches to the typed kubernetes.Service list method for the
// given GVR. Unknown GVRs return zero with no error; the resource summary
// will simply show a zero count for that row.
func (a *kubernetesClusterLister) listTotal(ctx context.Context, clusterID int64, gvr federation.GVR, namespace string, query apiquery.ListQuery) (int, error) {
	switch {
	case gvr.Group == "" && gvr.Version == "v1" && gvr.Resource == "pods":
		resp, err := a.service.Pods(ctx, clusterID, namespace, query)
		if err != nil {
			return 0, err
		}
		return resp.Total, nil
	case gvr.Group == "" && gvr.Version == "v1" && gvr.Resource == "services":
		resp, err := a.service.Services(ctx, clusterID, namespace, query)
		if err != nil {
			return 0, err
		}
		return resp.Total, nil
	case gvr.Group == "" && gvr.Version == "v1" && gvr.Resource == "nodes":
		resp, err := a.service.Nodes(ctx, clusterID, query)
		if err != nil {
			return 0, err
		}
		return resp.Total, nil
	case gvr.Group == "" && gvr.Version == "v1" && gvr.Resource == "namespaces":
		resp, err := a.service.Namespaces(ctx, clusterID, query)
		if err != nil {
			return 0, err
		}
		return resp.Total, nil
	case gvr.Group == "apps" && gvr.Version == "v1" && gvr.Resource == "deployments":
		resp, err := a.service.Deployments(ctx, clusterID, namespace, query)
		if err != nil {
			return 0, err
		}
		return resp.Total, nil
	case gvr.Group == "apps" && gvr.Version == "v1" && gvr.Resource == "statefulsets":
		resp, err := a.service.StatefulSets(ctx, clusterID, namespace, query)
		if err != nil {
			return 0, err
		}
		return resp.Total, nil
	case gvr.Group == "apps" && gvr.Version == "v1" && gvr.Resource == "daemonsets":
		resp, err := a.service.DaemonSets(ctx, clusterID, namespace, query)
		if err != nil {
			return 0, err
		}
		return resp.Total, nil
	case gvr.Group == "batch" && gvr.Version == "v1" && gvr.Resource == "jobs":
		resp, err := a.service.Jobs(ctx, clusterID, namespace, query)
		if err != nil {
			return 0, err
		}
		return resp.Total, nil
	case gvr.Group == "batch" && gvr.Version == "v1" && gvr.Resource == "cronjobs":
		resp, err := a.service.CronJobs(ctx, clusterID, namespace, query)
		if err != nil {
			return 0, err
		}
		return resp.Total, nil
	}
	return 0, nil
}
