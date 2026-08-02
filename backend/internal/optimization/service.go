// Package optimization wires the read-only M61-M63 analyzers
// (FinOps right-sizing, CIS posture, deprecated-API check) behind the HTTP
// API. The analyzers themselves are pure functions over an observation
// bundle; this package carries the shared configuration (e.g. the default
// cost rate), the server-side collector that turns live cluster data into
// those bundles, and lets the http server register the routes.
//
// Per ADR 0004 the analyzers never mutate cluster state: the collector only
// reads and maps, and the caller supplies the bundle (either auto-collected or
// posted in the request body).
package optimization

import (
	"context"

	"k8s-aiops.local/backend/internal/capacity"
	"k8s-aiops.local/backend/internal/cis"
	"k8s-aiops.local/backend/internal/deprecatedapi"
	"k8s-aiops.local/backend/internal/finops"
	"k8s-aiops.local/backend/internal/gitopsdrift"
	"k8s-aiops.local/backend/internal/hpa"
	"k8s-aiops.local/backend/internal/imagepolicy"
	"k8s-aiops.local/backend/internal/netpolicy"
	"k8s-aiops.local/backend/internal/pdb"
	"k8s-aiops.local/backend/internal/policy"
)

// Service carries the shared configuration and the collector for the
// optimization analyzers. It is intentionally thin: the analyzers are
// package-level pure functions and this service exists mainly so the http
// server can register the routes behind a single non-nil Options field,
// matching the per-milestone pattern.
type Service struct {
	defaultCostRate finops.CostRate
	collector       *Collector
}

// NewService constructs the optimization service. rate is the monthly unit
// price used to translate idle resources into dollars when a request does not
// supply one; pass finops.DefaultCostRate() for the illustrative defaults.
// collector may be nil — when nil, the analyze endpoints only accept an
// already-collected observation bundle in the request body (no auto-collect).
func NewService(rate finops.CostRate, collector *Collector) *Service {
	return &Service{defaultCostRate: rate, collector: collector}
}

// HasCollector reports whether auto-collection from a live cluster is wired.
func (s *Service) HasCollector() bool {
	return s != nil && s.collector != nil
}

// DefaultCostRate returns the configured cost rate, falling back to the
// illustrative defaults when the service is nil (e.g. in tests).
func (s *Service) DefaultCostRate() finops.CostRate {
	if s == nil {
		return finops.DefaultCostRate()
	}
	return s.defaultCostRate
}

// CollectCIS auto-collects the CIS observation bundle for a cluster.
func (s *Service) CollectCIS(ctx context.Context, clusterID int64) (cis.Inputs, error) {
	return s.collector.CollectCIS(ctx, clusterID)
}

// CollectFinOps auto-collects the FinOps container-input bundle for a cluster.
func (s *Service) CollectFinOps(ctx context.Context, clusterID int64) ([]finops.ContainerInput, error) {
	return s.collector.CollectFinOps(ctx, clusterID)
}

// CollectDeprecatedAPI auto-collects the deprecated-API object list for a
// cluster.
func (s *Service) CollectDeprecatedAPI(ctx context.Context, clusterID int64) ([]deprecatedapi.ResourceObject, error) {
	return s.collector.CollectDeprecatedAPI(ctx, clusterID)
}

// CollectNetPolicy auto-collects the network posture observation bundle
// (namespaces, pods, services, network policies) for a cluster.
func (s *Service) CollectNetPolicy(ctx context.Context, clusterID int64) (netpolicy.Inputs, error) {
	return s.collector.CollectNetPolicy(ctx, clusterID)
}

// CollectImagePolicy auto-collects the image supply-chain observation bundle
// (every container image referenced by a workload) for a cluster.
func (s *Service) CollectImagePolicy(ctx context.Context, clusterID int64) (imagepolicy.Inputs, error) {
	return s.collector.CollectImagePolicy(ctx, clusterID)
}

// CollectGitOpsDrift auto-collects the GitOps configuration-drift observation
// bundle (last-applied-configuration annotations plus live spec/data, and the
// GitOps-managed namespaces) for a cluster.
func (s *Service) CollectGitOpsDrift(ctx context.Context, clusterID int64) (gitopsdrift.Inputs, error) {
	return s.collector.CollectGitOpsDrift(ctx, clusterID)
}

// CollectCapacity auto-collects the capacity-trend observation bundle (node
// allocatable capacity plus the aggregate node usage time series) for a cluster.
func (s *Service) CollectCapacity(ctx context.Context, clusterID int64) (capacity.Inputs, error) {
	return s.collector.CollectCapacity(ctx, clusterID)
}

// CollectPolicy auto-collects the policy-as-code observation bundle (workload
// controllers with resource/security-context/probe fields plus host access)
// for a cluster.
func (s *Service) CollectPolicy(ctx context.Context, clusterID int64) (policy.Inputs, error) {
	return s.collector.CollectPolicy(ctx, clusterID)
}

// CollectHPA auto-collects the HPA posture observation bundle (scaling
// bounds, current replicas and utilization) for a cluster.
func (s *Service) CollectHPA(ctx context.Context, clusterID int64) (hpa.Inputs, error) {
	return s.collector.CollectHPA(ctx, clusterID)
}

// CollectPDB auto-collects the PDB posture observation bundle (replicable
// workloads plus PodDisruptionBudgets with budget and disruption state) for a
// cluster.
func (s *Service) CollectPDB(ctx context.Context, clusterID int64) (pdb.Inputs, error) {
	return s.collector.CollectPDB(ctx, clusterID)
}
