package topology

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Service orchestrates topology edge collection, persistence and queries. It
// is the single entry point for M40 topology operations: callers never use
// the Collector or Repository directly.
type Service struct {
	collector  *Collector
	repository Repository
	// namespaceLister lists namespace names for cluster-wide collection.
	namespaceLister NamespaceLister
	// closeStaleTimeout bounds how long the service waits when closing stale
	// edges. Zero means no explicit timeout beyond the context.
	closeStaleTimeout time.Duration
}

// NamespaceLister returns the namespace names visible to the caller in a
// cluster. The service uses it only for cluster-wide collection; per-namespace
// collection does not need it.
type NamespaceLister interface {
	VisibleNamespaces(ctx context.Context, clusterID int64) ([]string, error)
}

// CollectionResult reports what a single collection pass persisted and closed.
type CollectionResult struct {
	ClusterID     int64
	Namespace     string
	EdgesSeen     int
	EdgesUpserted int
	EdgesClosed   int
	Partial       bool
	Error         error
}

// ClusterCollectionResult aggregates per-namespace results.
type ClusterCollectionResult struct {
	ClusterID     int64
	Namespaces    int
	TotalSeen     int
	TotalUpserted int
	TotalClosed   int
	Partial       bool
	Errors        []error
}

// NewService creates a Service. repository must be non-nil; collector may be
// nil when collection is disabled (query-only mode).
func NewService(collector *Collector, repository Repository, namespaceLister NamespaceLister) *Service {
	return &Service{
		collector:         collector,
		repository:        repository,
		namespaceLister:   namespaceLister,
		closeStaleTimeout: 30 * time.Second,
	}
}

// CollectNamespace snapshots one namespace, derives edges and persists them.
// Active edges no longer present in the snapshot are closed (valid_to set).
// On partial read failure, the result is marked Partial and the error is
// returned alongside the persisted edges so callers can report completeness.
func (s *Service) CollectNamespace(ctx context.Context, clusterID int64, namespace string, now time.Time) (CollectionResult, error) {
	if s.collector == nil {
		return CollectionResult{}, errors.New("topology collector is disabled")
	}
	if namespace == "" {
		return CollectionResult{}, errors.New("namespace is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	result := CollectionResult{ClusterID: clusterID, Namespace: namespace}

	snap, err := s.collector.Snapshot(ctx, clusterID, namespace)
	if err != nil {
		result.Partial = true
		result.Error = err
		// Continue with partial snapshot so existing edges are closed and
		// observed edges are refreshed.
	}

	derived := s.collector.DeriveEdges(snap, now)
	result.EdgesSeen = len(derived)

	// Persist derived edges (upsert refreshes last_observed_at on conflict).
	for i := range derived {
		if persistErr := s.repository.UpsertEdge(ctx, &derived[i]); persistErr != nil {
			if result.Error == nil {
				result.Error = persistErr
			}
			result.Partial = true
			continue
		}
		result.EdgesUpserted++
	}

	// Close stale edges: list active edges for this namespace and close any
	// not present in the derived set.
	closed, closeErr := s.closeStaleEdges(ctx, clusterID, namespace, derived, now)
	result.EdgesClosed = closed
	if closeErr != nil {
		if result.Error == nil {
			result.Error = closeErr
		}
		result.Partial = true
	}

	return result, result.Error
}

// CollectCluster collects edges for every visible namespace in a cluster.
// Namespace-level errors are aggregated; the result is marked Partial when any
// namespace fails. The caller controls concurrency by invoking this method
// serially per cluster.
func (s *Service) CollectCluster(ctx context.Context, clusterID int64, now time.Time) (ClusterCollectionResult, error) {
	if s.collector == nil {
		return ClusterCollectionResult{}, errors.New("topology collector is disabled")
	}
	if s.namespaceLister == nil {
		return ClusterCollectionResult{}, errors.New("namespace lister is required for cluster-wide collection")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	namespaces, err := s.namespaceLister.VisibleNamespaces(ctx, clusterID)
	if err != nil {
		return ClusterCollectionResult{ClusterID: clusterID, Partial: true, Errors: []error{err}}, err
	}

	clusterResult := ClusterCollectionResult{ClusterID: clusterID, Namespaces: len(namespaces)}
	for _, ns := range namespaces {
		nsResult, _ := s.CollectNamespace(ctx, clusterID, ns, now)
		clusterResult.TotalSeen += nsResult.EdgesSeen
		clusterResult.TotalUpserted += nsResult.EdgesUpserted
		clusterResult.TotalClosed += nsResult.EdgesClosed
		if nsResult.Partial || nsResult.Error != nil {
			clusterResult.Partial = true
			if nsResult.Error != nil {
				clusterResult.Errors = append(clusterResult.Errors, fmt.Errorf("namespace %s: %w", ns, nsResult.Error))
			}
		}
	}
	return clusterResult, nil
}

// GetTopologyGraph returns the current active topology graph for a namespace.
// The graph includes nodes derived from edge endpoints and a completeness
// indicator. An empty result with no error means no edges have been collected
// for this namespace yet.
func (s *Service) GetTopologyGraph(ctx context.Context, clusterID int64, namespace string, limit int) (TopologyGraph, error) {
	filter := EdgeFilter{
		ClusterID: clusterID,
		Namespace: namespace,
		Limit:     limit,
	}
	edges, total, err := s.repository.ListEdges(ctx, filter)
	if err != nil {
		return TopologyGraph{}, err
	}

	graph := TopologyGraph{
		ClusterID:   clusterID,
		Namespace:   namespace,
		Edges:       edges,
		GeneratedAt: time.Now().UTC(),
	}

	// Build nodes from edge endpoints with edge counts.
	nodeMap := make(map[string]*GraphNode)
	for i := range edges {
		addGraphNode(nodeMap, &edges[i].Source)
		addGraphNode(nodeMap, &edges[i].Target)
	}
	graph.Nodes = make([]GraphNode, 0, len(nodeMap))
	for _, node := range nodeMap {
		graph.Nodes = append(graph.Nodes, *node)
	}

	// Completeness: truncated when total exceeds the returned edge count.
	graph.Completeness = GraphCompleteness{State: "complete"}
	if int(total) > len(edges) {
		graph.Completeness.State = "truncated"
		graph.Completeness.Truncated = true
		graph.Completeness.Remaining = int(total) - len(edges)
	}
	if len(edges) == 0 {
		graph.Completeness.State = "partial"
	}

	return graph, nil
}

// GetChangeTimeline returns change events matching the filter.
func (s *Service) GetChangeTimeline(ctx context.Context, filter ChangeTimelineFilter) (ChangeTimelineResponse, error) {
	events, total, err := s.repository.ListChangeEvents(ctx, filter)
	if err != nil {
		return ChangeTimelineResponse{}, err
	}
	resp := ChangeTimelineResponse{Items: events, Total: total}
	if int(total) > len(events) {
		resp.Truncated = true
	}
	return resp, nil
}

// --- internal helpers ---

// closeStaleEdges closes active edges that are no longer present in the
// derived set. It lists active edges for the namespace and compares by edge
// identity (kind, source_uid, target_uid, derivation).
func (s *Service) closeStaleEdges(ctx context.Context, clusterID int64, namespace string, derived []Edge, now time.Time) (int, error) {
	existing, _, err := s.repository.ListEdges(ctx, EdgeFilter{
		ClusterID: clusterID,
		Namespace: namespace,
		Limit:     500,
	})
	if err != nil {
		return 0, err
	}

	derivedSet := make(map[string]bool, len(derived))
	for i := range derived {
		derivedSet[edgeIdentityKey(&derived[i])] = true
	}

	closed := 0
	for i := range existing {
		ex := &existing[i]
		if derivedSet[edgeIdentityKey(ex)] {
			continue
		}
		if closeErr := s.repository.CloseEdge(ctx, clusterID, ex.Kind, ex.Source.UID, ex.Target.UID, ex.Derivation, now); closeErr != nil {
			if errors.Is(closeErr, ErrEdgeNotFound) {
				continue
			}
			return closed, closeErr
		}
		closed++
	}
	return closed, nil
}

// edgeIdentityKey is the stable identity key for an edge, matching the unique
// index on topology_edges.
func edgeIdentityKey(edge *Edge) string {
	return fmt.Sprintf("%s|%s|%s|%s", edge.Kind, edge.Source.UID, edge.Target.UID, edge.Derivation)
}

// addGraphNode adds or updates a node in the node map.
func addGraphNode(nodeMap map[string]*GraphNode, citation *ResourceCitation) {
	key := nodeKey(citation)
	if node, ok := nodeMap[key]; ok {
		node.EdgeCount++
		return
	}
	nodeMap[key] = &GraphNode{
		Resource:  *citation,
		EdgeCount: 1,
	}
}

func nodeKey(citation *ResourceCitation) string {
	return fmt.Sprintf("%s|%s|%s", citation.Kind, citation.Namespace, citation.Name)
}
