package federation

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"k8s-aiops.local/backend/internal/cluster"
)

// FederationVersion is the contract version for the federation model. It is
// bumped when the host/member topology or federation_status semantics change
// in a backward-incompatible way.
const FederationVersion = "1.0"

// MaxEventsLimit is the upper bound for ListEvents. The repository enforces
// this as well; the constant is exported so handlers can validate before
// calling the service.
const MaxEventsLimit = 200

// DefaultEventsLimit is the default limit when the caller does not specify one.
const DefaultEventsLimit = 100

// ResourceSummaryTimeout is the per-cluster timeout for the resource summary
// fan-out. It mirrors the fleet service's per-cluster timeout.
const ResourceSummaryTimeout = 4 * time.Second

// ResourceSummaryMaxClusters bounds the resource summary fan-out.
const ResourceSummaryMaxClusters = 20

// ClusterLister is a minimal interface for listing resource counts across
// clusters. The federation service uses it to avoid a hard dependency on the
// kubernetes package's concrete types. Implementations translate the typed
// ListResponse into a CountResult.
type ClusterLister interface {
	// ListResource returns the total count of resources of the given GVR on
	// the given cluster. The namespace argument is empty for cluster-scoped
	// resources. The implementation should use the bounded Kubernetes gateway
	// (ADR 0004) — a limit of 1 is sufficient because only the Total field is
	// consumed.
	ListResource(ctx context.Context, clusterID int64, gvr GVR, namespace string) (CountResult, error)
}

// GVR is the Group/Version/Resource tuple used by the resource summary. It is
// a value type so the federation package does not depend on
// k8s.io/apimachinery.
type GVR struct {
	Group    string
	Version  string
	Resource string
	Kind     string
	// Namespaced is true for namespace-scoped resources. When false, the
	// resource summary fan-out passes an empty namespace to the lister.
	Namespaced bool
}

// CountResult is the per-cluster count returned by ClusterLister.
type CountResult struct {
	Total int
	Err   error
}

// Service is the M48 federation application service. It is the single entry
// point for federation-scoped operations and enforces the invariants
// documented in ADR 0063:
//
//   - At most one host cluster (enforced atomically via the repository).
//   - 404 > 403 anti-leakage: cluster-not-found surfaces as ErrClusterNotFound
//     so the handler can map it to 404.
//   - cluster_role and federation_status are bounded enums.
//   - Federation events are append-only.
//   - The resource summary is a bounded fan-out over the existing kubernetes
//     gateway; missing/unreachable clusters contribute zero counts.
//   - The cross-cluster diagnosis aggregation (P2d) reads the already-centralized
//     diagnosis_records table (platform-side view); no live cluster fan-out.
type Service struct {
	repo          Repository
	lister        ClusterLister
	diagnosisRepo FederationDiagnosisRepository
	now           func() time.Time
}

// NewService constructs a federation Service backed by repo. lister may be
// nil; in that case ResourceSummary returns an empty result. diagnosisRepo may
// be nil; in that case the P2d diagnosis aggregation returns empty results.
func NewService(repo Repository, lister ClusterLister) *Service {
	return &Service{repo: repo, lister: lister, now: time.Now}
}

// WithDiagnosisRepository attaches the cross-cluster diagnosis read surface used
// by the P2d fleet-diagnosis aggregation endpoints. It is a fluent setter so the
// existing wiring stays unchanged when the diagnosis repository is absent.
func (s *Service) WithDiagnosisRepository(repo FederationDiagnosisRepository) *Service {
	s.diagnosisRepo = repo
	return s
}

// RegisterClusterInput is the validated payload for RegisterCluster.
type RegisterClusterInput struct {
	ClusterID int64
	// Role is the desired federation role. Must be host or member. When host,
	// the service enforces the single-host invariant.
	Role string
	// Status is the initial federation_status. Defaults to "registered".
	Status string
}

// RegisterCluster promotes an existing cluster to a federation member or host.
// The cluster must already exist (registration does not provision a new
// cluster — the kubeconfig-direct-connection model is preserved). If the
// cluster is already a federation member or host, ErrClusterAlreadyRegistered
// is returned.
func (s *Service) RegisterCluster(ctx context.Context, in RegisterClusterInput) (cluster.Cluster, error) {
	if in.ClusterID <= 0 {
		return cluster.Cluster{}, errors.New("cluster_id is required")
	}
	if in.Role != cluster.ClusterRoleHost && in.Role != cluster.ClusterRoleMember {
		return cluster.Cluster{}, ErrInvalidClusterRole
	}
	status := in.Status
	if status == "" {
		status = cluster.FederationStatusRegistered
	}
	if !isValidFederationStatus(status) {
		return cluster.Cluster{}, ErrInvalidFederationStatus
	}

	item, err := s.repo.GetCluster(ctx, in.ClusterID)
	if err != nil {
		return cluster.Cluster{}, err
	}
	if item.ClusterRole == cluster.ClusterRoleHost || item.ClusterRole == cluster.ClusterRoleMember {
		return cluster.Cluster{}, ErrClusterAlreadyRegistered
	}

	if in.Role == cluster.ClusterRoleHost {
		hostCount, err := s.repo.CountHost(ctx)
		if err != nil {
			return cluster.Cluster{}, err
		}
		if hostCount > 0 {
			return cluster.Cluster{}, ErrHostAlreadyExists
		}
	}

	now := s.now().UTC()
	if err := s.repo.SetClusterRole(ctx, in.ClusterID, in.Role, now); err != nil {
		return cluster.Cluster{}, err
	}
	if err := s.repo.SetFederationStatus(ctx, in.ClusterID, status, now); err != nil {
		return cluster.Cluster{}, err
	}
	if err := s.repo.TouchHeartbeat(ctx, in.ClusterID, now); err != nil {
		return cluster.Cluster{}, err
	}
	if err := s.repo.AppendEvent(ctx, FederationEvent{
		ClusterID:  in.ClusterID,
		EventType:  EventRegistered,
		Status:     status,
		Message:    fmt.Sprintf("cluster registered as %s", in.Role),
		OccurredAt: now,
	}); err != nil {
		return cluster.Cluster{}, err
	}
	return s.repo.GetCluster(ctx, in.ClusterID)
}

// DeregisterCluster removes a cluster from the federation (sets cluster_role
// back to standalone and federation_status to registered). The cluster row is
// not deleted — this is a soft-delete that preserves the audit trail and the
// stored kubeconfig. The host cluster cannot be deregistered directly; it
// must first be demoted via DemoteHost (defence-in-depth against accidental
// loss of the host).
func (s *Service) DeregisterCluster(ctx context.Context, clusterID int64) (cluster.Cluster, error) {
	if clusterID <= 0 {
		return cluster.Cluster{}, errors.New("cluster_id is required")
	}
	item, err := s.repo.GetCluster(ctx, clusterID)
	if err != nil {
		return cluster.Cluster{}, err
	}
	if item.ClusterRole == cluster.ClusterRoleHost {
		return cluster.Cluster{}, ErrCannotDeregisterHost
	}
	if item.ClusterRole == cluster.ClusterRoleStandalone {
		// Already deregistered; idempotent no-op.
		return item, nil
	}
	now := s.now().UTC()
	if err := s.repo.SetClusterRole(ctx, clusterID, cluster.ClusterRoleStandalone, now); err != nil {
		return cluster.Cluster{}, err
	}
	if err := s.repo.SetFederationStatus(ctx, clusterID, cluster.FederationStatusRegistered, now); err != nil {
		return cluster.Cluster{}, err
	}
	if err := s.repo.AppendEvent(ctx, FederationEvent{
		ClusterID:  clusterID,
		EventType:  EventDeregistered,
		Status:     cluster.FederationStatusRegistered,
		Message:    "cluster deregistered from federation",
		OccurredAt: now,
	}); err != nil {
		return cluster.Cluster{}, err
	}
	return s.repo.GetCluster(ctx, clusterID)
}

// PromoteToHost promotes a cluster to the host role. The service enforces the
// single-host invariant atomically: if another cluster is already the host,
// ErrHostAlreadyExists is returned.
func (s *Service) PromoteToHost(ctx context.Context, clusterID int64) (cluster.Cluster, error) {
	if clusterID <= 0 {
		return cluster.Cluster{}, errors.New("cluster_id is required")
	}
	item, err := s.repo.GetCluster(ctx, clusterID)
	if err != nil {
		return cluster.Cluster{}, err
	}
	if item.ClusterRole == cluster.ClusterRoleHost {
		return item, nil
	}
	hostCount, err := s.repo.CountHost(ctx)
	if err != nil {
		return cluster.Cluster{}, err
	}
	if hostCount > 0 {
		return cluster.Cluster{}, ErrHostAlreadyExists
	}
	now := s.now().UTC()
	if err := s.repo.SetClusterRole(ctx, clusterID, cluster.ClusterRoleHost, now); err != nil {
		return cluster.Cluster{}, err
	}
	if err := s.repo.AppendEvent(ctx, FederationEvent{
		ClusterID:  clusterID,
		EventType:  EventRoleChange,
		Status:     item.FederationStatus,
		Message:    fmt.Sprintf("cluster promoted from %s to host", item.ClusterRole),
		OccurredAt: now,
	}); err != nil {
		return cluster.Cluster{}, err
	}
	return s.repo.GetCluster(ctx, clusterID)
}

// DemoteHost demotes a host cluster to a member (or standalone if targetRole
// is standalone). The cluster must currently be the host.
func (s *Service) DemoteHost(ctx context.Context, clusterID int64, targetRole string) (cluster.Cluster, error) {
	if clusterID <= 0 {
		return cluster.Cluster{}, errors.New("cluster_id is required")
	}
	if targetRole != cluster.ClusterRoleMember && targetRole != cluster.ClusterRoleStandalone {
		return cluster.Cluster{}, ErrInvalidClusterRole
	}
	item, err := s.repo.GetCluster(ctx, clusterID)
	if err != nil {
		return cluster.Cluster{}, err
	}
	if item.ClusterRole != cluster.ClusterRoleHost {
		return cluster.Cluster{}, errors.New("cluster is not the host")
	}
	now := s.now().UTC()
	if err := s.repo.SetClusterRole(ctx, clusterID, targetRole, now); err != nil {
		return cluster.Cluster{}, err
	}
	if err := s.repo.AppendEvent(ctx, FederationEvent{
		ClusterID:  clusterID,
		EventType:  EventRoleChange,
		Status:     item.FederationStatus,
		Message:    fmt.Sprintf("cluster demoted from host to %s", targetRole),
		OccurredAt: now,
	}); err != nil {
		return cluster.Cluster{}, err
	}
	return s.repo.GetCluster(ctx, clusterID)
}

// RecordHeartbeat updates last_heartbeat_at and optionally transitions
// federation_status to "healthy" (when status is non-empty). The heartbeat
// event is appended to the audit trail.
func (s *Service) RecordHeartbeat(ctx context.Context, clusterID int64, status string) (cluster.Cluster, error) {
	if clusterID <= 0 {
		return cluster.Cluster{}, errors.New("cluster_id is required")
	}
	if status != "" && !isValidFederationStatus(status) {
		return cluster.Cluster{}, ErrInvalidFederationStatus
	}
	if _, err := s.repo.GetCluster(ctx, clusterID); err != nil {
		return cluster.Cluster{}, err
	}
	now := s.now().UTC()
	if err := s.repo.TouchHeartbeat(ctx, clusterID, now); err != nil {
		return cluster.Cluster{}, err
	}
	eventStatus := status
	if eventStatus == "" {
		eventStatus = cluster.FederationStatusHealthy
	}
	if status != "" {
		if err := s.repo.SetFederationStatus(ctx, clusterID, status, now); err != nil {
			return cluster.Cluster{}, err
		}
	}
	if err := s.repo.AppendEvent(ctx, FederationEvent{
		ClusterID:  clusterID,
		EventType:  EventHeartbeat,
		Status:     eventStatus,
		Message:    "cluster heartbeat received",
		OccurredAt: now,
	}); err != nil {
		return cluster.Cluster{}, err
	}
	return s.repo.GetCluster(ctx, clusterID)
}

// UpdateFederationStatus transitions a cluster's federation_status. This is
// the operator-facing path for marking a cluster degraded/disconnected
// without re-probing. A status_change event is appended.
func (s *Service) UpdateFederationStatus(ctx context.Context, clusterID int64, status, message string) (cluster.Cluster, error) {
	if clusterID <= 0 {
		return cluster.Cluster{}, errors.New("cluster_id is required")
	}
	if !isValidFederationStatus(status) {
		return cluster.Cluster{}, ErrInvalidFederationStatus
	}
	item, err := s.repo.GetCluster(ctx, clusterID)
	if err != nil {
		return cluster.Cluster{}, err
	}
	if item.FederationStatus == status {
		return item, nil
	}
	now := s.now().UTC()
	if err := s.repo.SetFederationStatus(ctx, clusterID, status, now); err != nil {
		return cluster.Cluster{}, err
	}
	if message == "" {
		message = fmt.Sprintf("federation status changed from %s to %s", item.FederationStatus, status)
	}
	if err := s.repo.AppendEvent(ctx, FederationEvent{
		ClusterID:  clusterID,
		EventType:  EventStatusChange,
		Status:     status,
		Message:    message,
		OccurredAt: now,
	}); err != nil {
		return cluster.Cluster{}, err
	}
	return s.repo.GetCluster(ctx, clusterID)
}

// Overview returns the federation topology (host + members + standalone) and
// aggregated health counts. It is a pure SQL aggregation — no live probing is
// performed. The caller may pass visibleClusterIDs to restrict the result to
// the caller's authorized clusters (nil means all clusters, used by
// SystemAdmin).
func (s *Service) Overview(ctx context.Context, visibleClusterIDs []int64) (Overview, error) {
	items, err := s.repo.ListClusters(ctx)
	if err != nil {
		return Overview{}, err
	}
	visible := make(map[int64]bool, len(visibleClusterIDs))
	for _, id := range visibleClusterIDs {
		visible[id] = true
	}
	overview := Overview{
		Members:     []ClusterSummary{},
		Standalone:  []ClusterSummary{},
		GeneratedAt: s.now().UTC(),
	}
	for _, item := range items {
		if visibleClusterIDs != nil && !visible[item.ID] {
			continue
		}
		summary := toSummary(item)
		overview.TotalClusters++
		switch item.FederationStatus {
		case cluster.FederationStatusHealthy:
			overview.HealthyCount++
		case cluster.FederationStatusDegraded:
			overview.DegradedCount++
		case cluster.FederationStatusDisconnected:
			overview.DisconnectedCount++
		case cluster.FederationStatusRegistered:
			overview.RegisteredCount++
		}
		switch item.ClusterRole {
		case cluster.ClusterRoleHost:
			overview.Host = &summary
		case cluster.ClusterRoleMember:
			overview.Members = append(overview.Members, summary)
		case cluster.ClusterRoleStandalone:
			overview.Standalone = append(overview.Standalone, summary)
		}
	}
	// Stable order: members and standalone by cluster_id ASC.
	sort.Slice(overview.Members, func(i, j int) bool { return overview.Members[i].ClusterID < overview.Members[j].ClusterID })
	sort.Slice(overview.Standalone, func(i, j int) bool { return overview.Standalone[i].ClusterID < overview.Standalone[j].ClusterID })
	return overview, nil
}

// ListEvents returns the most recent federation events across all clusters,
// newest first. limit is bounded by MaxEventsLimit; values <=0 or >Max use
// DefaultEventsLimit.
func (s *Service) ListEvents(ctx context.Context, limit int) ([]FederationEvent, error) {
	if limit <= 0 || limit > MaxEventsLimit {
		limit = DefaultEventsLimit
	}
	return s.repo.ListEvents(ctx, limit)
}

// ListEventsByCluster returns the most recent federation events for a single
// cluster, newest first.
func (s *Service) ListEventsByCluster(ctx context.Context, clusterID int64, limit int) ([]FederationEvent, error) {
	if clusterID <= 0 {
		return nil, errors.New("cluster_id is required")
	}
	if limit <= 0 || limit > MaxEventsLimit {
		limit = DefaultEventsLimit
	}
	return s.repo.ListEventsByCluster(ctx, clusterID, limit)
}

// FixedGVRWhitelist is the operator-curated GVR whitelist used by the resource
// summary. It mirrors the resource families already exposed by the typed list
// methods on kubernetes.Service. M49 will refine this into the full CRD
// browsing surface; M48 deliberately keeps it small and bounded.
var FixedGVRWhitelist = []GVR{
	{Group: "", Version: "v1", Resource: "pods", Kind: "Pod", Namespaced: true},
	{Group: "", Version: "v1", Resource: "services", Kind: "Service", Namespaced: true},
	{Group: "", Version: "v1", Resource: "nodes", Kind: "Node", Namespaced: false},
	{Group: "", Version: "v1", Resource: "namespaces", Kind: "Namespace", Namespaced: false},
	{Group: "apps", Version: "v1", Resource: "deployments", Kind: "Deployment", Namespaced: true},
	{Group: "apps", Version: "v1", Resource: "statefulsets", Kind: "StatefulSet", Namespaced: true},
	{Group: "apps", Version: "v1", Resource: "daemonsets", Kind: "DaemonSet", Namespaced: true},
	{Group: "batch", Version: "v1", Resource: "jobs", Kind: "Job", Namespaced: true},
	{Group: "batch", Version: "v1", Resource: "cronjobs", Kind: "CronJob", Namespaced: true},
}

// ResourceSummary aggregates resource counts across visible clusters for each
// GVR in FixedGVRWhitelist. The fan-out is bounded by ResourceSummaryMaxClusters
// and uses a per-cluster timeout of ResourceSummaryTimeout. Missing/unreachable
// clusters contribute zero counts with an Error code.
func (s *Service) ResourceSummary(ctx context.Context, visibleClusterIDs []int64) (ResourceSummary, error) {
	summary := ResourceSummary{
		Items:       []ResourceSummaryEntry{},
		GeneratedAt: s.now().UTC(),
	}
	if s.lister == nil {
		return summary, nil
	}
	items, err := s.repo.ListClusters(ctx)
	if err != nil {
		return ResourceSummary{}, err
	}
	visible := make(map[int64]bool, len(visibleClusterIDs))
	for _, id := range visibleClusterIDs {
		visible[id] = true
	}
	clusters := make([]cluster.Cluster, 0, len(items))
	for _, item := range items {
		if !item.Enabled {
			continue
		}
		if visibleClusterIDs != nil && !visible[item.ID] {
			continue
		}
		clusters = append(clusters, item)
	}
	if len(clusters) > ResourceSummaryMaxClusters {
		clusters = clusters[:ResourceSummaryMaxClusters]
	}
	sort.Slice(clusters, func(i, j int) bool { return clusters[i].ID < clusters[j].ID })

	for _, gvr := range FixedGVRWhitelist {
		entry := ResourceSummaryEntry{
			Group:     gvr.Group,
			Version:   gvr.Version,
			Resource:  gvr.Resource,
			Kind:      gvr.Kind,
			ByCluster: make([]ClusterCount, 0, len(clusters)),
		}
		results := make([]ClusterCount, len(clusters))
		var wg sync.WaitGroup
		for i, item := range clusters {
			wg.Add(1)
			go func(idx int, c cluster.Cluster) {
				defer wg.Done()
				results[idx] = s.countOne(ctx, c, gvr)
			}(i, item)
		}
		wg.Wait()
		total := 0
		for _, r := range results {
			entry.ByCluster = append(entry.ByCluster, r)
			if r.Error == "" {
				total += r.Count
			}
		}
		entry.TotalCount = total
		summary.Items = append(summary.Items, entry)
	}
	return summary, nil
}

// countOne is the per-cluster count worker. It applies a per-cluster timeout
// and translates errors into stable codes (TIMEOUT / QUERY_FAILED).
func (s *Service) countOne(parent context.Context, c cluster.Cluster, gvr GVR) ClusterCount {
	ctx, cancel := context.WithTimeout(parent, ResourceSummaryTimeout)
	defer cancel()
	result, err := s.lister.ListResource(ctx, c.ID, gvr, "")
	if err != nil {
		code := "QUERY_FAILED"
		if errors.Is(err, context.DeadlineExceeded) {
			code = "TIMEOUT"
		}
		return ClusterCount{ClusterID: c.ID, ClusterName: c.Name, Count: 0, Error: code}
	}
	return ClusterCount{ClusterID: c.ID, ClusterName: c.Name, Count: result.Total}
}

// toSummary maps a cluster.Cluster row to the federation ClusterSummary. It
// is a pure projection — no I/O.
func toSummary(item cluster.Cluster) ClusterSummary {
	return ClusterSummary{
		ClusterID:         item.ID,
		ClusterName:       item.Name,
		APIServer:         item.APIServer,
		Enabled:           item.Enabled,
		Status:            item.Status,
		KubernetesVersion: item.KubernetesVersion,
		LastProbedAt:      item.LastProbedAt,
		ClusterRole:       item.ClusterRole,
		FederationStatus:  item.FederationStatus,
		RegisteredAt:      item.RegisteredAt,
		LastHeartbeatAt:   item.LastHeartbeatAt,
	}
}

func isValidFederationStatus(status string) bool {
	switch status {
	case cluster.FederationStatusRegistered, cluster.FederationStatusHealthy, cluster.FederationStatusDegraded, cluster.FederationStatusDisconnected:
		return true
	}
	return false
}
