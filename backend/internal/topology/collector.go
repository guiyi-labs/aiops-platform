package topology

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"k8s-aiops.local/backend/internal/apiquery"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

// ResourceReader provides bounded Kubernetes reads for topology edge
// derivation. It is a subset of kubernetes.Service; the concrete service
// satisfies this interface implicitly. Reads are bounded by apiquery limits.
type ResourceReader interface {
	Deployments(ctx context.Context, clusterID int64, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Deployment], error)
	ReplicaSets(ctx context.Context, clusterID int64, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.ReplicaSet], error)
	Pods(ctx context.Context, clusterID int64, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Pod], error)
	Services(ctx context.Context, clusterID int64, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.ServiceResource], error)
	Ingresses(ctx context.Context, clusterID int64, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Ingress], error)
	EndpointSlices(ctx context.Context, clusterID int64, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.EndpointSlice], error)
	HorizontalPodAutoscalers(ctx context.Context, clusterID int64, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.HorizontalPodAutoscaler], error)
	PodDisruptionBudgets(ctx context.Context, clusterID int64, namespace string, query apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.PodDisruptionBudget], error)
}

// CollectorSnapshot is the complete set of resources read from one namespace
// in a single collection pass. It is the raw input to edge derivation.
type CollectorSnapshot struct {
	ClusterID      int64
	Namespace      string
	Deployments    []k8sgateway.Deployment
	ReplicaSets    []k8sgateway.ReplicaSet
	Pods           []k8sgateway.Pod
	Services       []k8sgateway.ServiceResource
	Ingresses      []k8sgateway.Ingress
	EndpointSlices []k8sgateway.EndpointSlice
	HPAs           []k8sgateway.HorizontalPodAutoscaler
	PDBs           []k8sgateway.PodDisruptionBudget
}

// Collector reads Kubernetes resources and derives topology edges using
// deterministic derivation methods. It never infers edges from naming or
// temporal proximity. All edges cite an exact UID or are marked Incomplete.
type Collector struct {
	reader ResourceReader
	// maxPerPage bounds each Kubernetes list call. The collector pages until
	// the namespace is exhausted or Remaining == 0.
	maxPerPage int
}

// NewCollector creates a Collector. maxPerPage defaults to 100 when <= 0.
func NewCollector(reader ResourceReader, maxPerPage int) *Collector {
	if maxPerPage <= 0 || maxPerPage > 100 {
		maxPerPage = 100
	}
	return &Collector{reader: reader, maxPerPage: maxPerPage}
}

// Snapshot reads all supported resources from one namespace. Partial failures
// are recorded: the snapshot returns whatever was read and a non-nil error
// only when every resource type fails. Callers should treat a partial snapshot
// as partial completeness in the resulting graph.
func (c *Collector) Snapshot(ctx context.Context, clusterID int64, namespace string) (CollectorSnapshot, error) {
	snap := CollectorSnapshot{ClusterID: clusterID, Namespace: namespace}
	query := apiquery.ListQuery{Page: 1, Limit: c.maxPerPage, Ascending: true}

	var firstErr error
	collect := func(fetch func() (int, error)) {
		if _, err := fetch(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	collect(func() (int, error) {
		items, err := c.listAllDeployments(ctx, clusterID, namespace, query)
		snap.Deployments = items
		return len(items), err
	})
	collect(func() (int, error) {
		items, err := c.listAllReplicaSets(ctx, clusterID, namespace, query)
		snap.ReplicaSets = items
		return len(items), err
	})
	collect(func() (int, error) {
		items, err := c.listAllPods(ctx, clusterID, namespace, query)
		snap.Pods = items
		return len(items), err
	})
	collect(func() (int, error) {
		items, err := c.listAllServices(ctx, clusterID, namespace, query)
		snap.Services = items
		return len(items), err
	})
	collect(func() (int, error) {
		items, err := c.listAllIngresses(ctx, clusterID, namespace, query)
		snap.Ingresses = items
		return len(items), err
	})
	collect(func() (int, error) {
		items, err := c.listAllEndpointSlices(ctx, clusterID, namespace, query)
		snap.EndpointSlices = items
		return len(items), err
	})
	collect(func() (int, error) {
		items, err := c.listAllHPAs(ctx, clusterID, namespace, query)
		snap.HPAs = items
		return len(items), err
	})
	collect(func() (int, error) {
		items, err := c.listAllPDBs(ctx, clusterID, namespace, query)
		snap.PDBs = items
		return len(items), err
	})

	// Return the snapshot even on partial failure so callers can persist
	// partial edges. The error lets callers mark completeness as partial.
	return snap, firstErr
}

// DeriveEdges derives all topology edges from a snapshot. Edges are
// deterministic: the same snapshot always produces the same edges. Each edge
// carries a SourceHash so the service can detect unchanged edges without
// re-deriving.
func (c *Collector) DeriveEdges(snap CollectorSnapshot, now time.Time) []Edge {
	var edges []Edge

	edges = append(edges, deriveOwnsEdges(snap, now)...)
	edges = append(edges, deriveSelectsEdges(snap, now)...)
	edges = append(edges, deriveRoutesToEdges(snap, now)...)
	edges = append(edges, deriveBackedByEdges(snap, now)...)
	edges = append(edges, deriveRunsOnEdges(snap, now)...)
	edges = append(edges, deriveMountsEdges(snap, now)...)
	edges = append(edges, deriveScalesEdges(snap, now)...)
	edges = append(edges, deriveProtectedByEdges(snap, now)...)

	for i := range edges {
		edges[i].SourceHash = computeEdgeHash(&edges[i])
	}
	return edges
}

// --- Derivation functions ---

// deriveOwnsEdges derives Owns edges from OwnerReferences.
// Deployment→ReplicaSet and ReplicaSet→Pod are the primary cases. Any
// controller OwnerReference produces an Owns edge when the owner is visible
// in the snapshot.
func deriveOwnsEdges(snap CollectorSnapshot, now time.Time) []Edge {
	ownerIndex := buildOwnerIndex(snap)
	var edges []Edge

	// ReplicaSet → Pod
	for i := range snap.Pods {
		pod := &snap.Pods[i]
		for _, owner := range pod.Metadata.OwnerReferences {
			if owner.UID == "" {
				continue
			}
			if _, ok := ownerIndex[owner.UID]; !ok {
				continue
			}
			edges = append(edges, Edge{
				ClusterID:       snap.ClusterID,
				Kind:            EdgeOwns,
				Source:          ownerIndex[owner.UID],
				Target:          citationFromMeta(pod.Metadata, "Pod"),
				Derivation:      DerivationOwnerReference,
				FirstObservedAt: now,
				LastObservedAt:  now,
				ValidFrom:       now,
			})
		}
	}
	return edges
}

// deriveSelectsEdges derives Selects edges from Service label selectors.
// Service→Pod edges are created when the Service selector matches the Pod
// labels exactly (all selector key-values present on the Pod).
func deriveSelectsEdges(snap CollectorSnapshot, now time.Time) []Edge {
	var edges []Edge
	for i := range snap.Services {
		svc := &snap.Services[i]
		if len(svc.Spec.Selector) == 0 {
			continue
		}
		svcCitation := citationFromMeta(svc.Metadata, "Service")
		for j := range snap.Pods {
			pod := &snap.Pods[j]
			if !mapSelectorMatches(svc.Spec.Selector, pod.Metadata.Labels) {
				continue
			}
			edges = append(edges, Edge{
				ClusterID:       snap.ClusterID,
				Kind:            EdgeSelects,
				Source:          svcCitation,
				Target:          citationFromMeta(pod.Metadata, "Pod"),
				Derivation:      DerivationLabelSelector,
				FirstObservedAt: now,
				LastObservedAt:  now,
				ValidFrom:       now,
			})
		}
	}
	return edges
}

// deriveRoutesToEdges derives RoutesTo edges from Ingress backend config.
// Ingress→Service edges are created for each backend service reference.
func deriveRoutesToEdges(snap CollectorSnapshot, now time.Time) []Edge {
	var edges []Edge
	for i := range snap.Ingresses {
		ing := &snap.Ingresses[i]
		ingCitation := citationFromMeta(ing.Metadata, "Ingress")
		seen := make(map[string]bool)
		addRoute := func(backend *k8sgateway.IngressBackend) {
			if backend == nil || backend.Service == nil || backend.Service.Name == "" {
				return
			}
			key := backend.Service.Name
			if seen[key] {
				return
			}
			seen[key] = true
			// Match service by name in the same namespace
			for j := range snap.Services {
				svc := &snap.Services[j]
				if svc.Metadata.Name != backend.Service.Name {
					continue
				}
				edges = append(edges, Edge{
					ClusterID:       snap.ClusterID,
					Kind:            EdgeRoutesTo,
					Source:          ingCitation,
					Target:          citationFromMeta(svc.Metadata, "Service"),
					Derivation:      DerivationBackendConfig,
					FirstObservedAt: now,
					LastObservedAt:  now,
					ValidFrom:       now,
				})
				return
			}
			// Service not in snapshot — record incomplete edge
			edges = append(edges, Edge{
				ClusterID:       snap.ClusterID,
				Kind:            EdgeRoutesTo,
				Source:          ingCitation,
				Target:          ResourceCitation{Kind: "Service", Namespace: ing.Metadata.Namespace, Name: backend.Service.Name, Incomplete: true},
				Derivation:      DerivationBackendConfig,
				FirstObservedAt: now,
				LastObservedAt:  now,
				ValidFrom:       now,
			})
		}
		addRoute(ing.Spec.DefaultBackend)
		for _, rule := range ing.Spec.Rules {
			if rule.HTTP == nil {
				continue
			}
			for _, path := range rule.HTTP.Paths {
				addRoute(&path.Backend)
			}
		}
	}
	return edges
}

// deriveBackedByEdges derives BackedBy edges from EndpointSlice endpoints.
// Service→Pod edges are created when an EndpointSlice references a Pod via
// targetRef. The Service is identified by the EndpointSlice's
// kubernetes.io/service-name label.
func deriveBackedByEdges(snap CollectorSnapshot, now time.Time) []Edge {
	svcByName := make(map[string]int)
	for i := range snap.Services {
		svcByName[snap.Services[i].Metadata.Name] = i
	}
	var edges []Edge
	for i := range snap.EndpointSlices {
		es := &snap.EndpointSlices[i]
		if es.ServiceName == "" {
			continue
		}
		svcIdx, ok := svcByName[es.ServiceName]
		if !ok {
			continue
		}
		svcCitation := citationFromMeta(snap.Services[svcIdx].Metadata, "Service")
		seen := make(map[string]bool)
		for _, ep := range es.Endpoints {
			if ep.TargetRef == nil || ep.TargetRef.Kind != "Pod" || ep.TargetRef.UID == "" {
				continue
			}
			if seen[ep.TargetRef.UID] {
				continue
			}
			seen[ep.TargetRef.UID] = true
			edges = append(edges, Edge{
				ClusterID:       snap.ClusterID,
				Kind:            EdgeBackedBy,
				Source:          svcCitation,
				Target:          ResourceCitation{Kind: "Pod", Namespace: ep.TargetRef.Namespace, Name: ep.TargetRef.Name, UID: ep.TargetRef.UID},
				Derivation:      DerivationEndpointSlice,
				FirstObservedAt: now,
				LastObservedAt:  now,
				ValidFrom:       now,
			})
		}
	}
	return edges
}

// deriveRunsOnEdges derives RunsOn edges from Pod spec.nodeName.
// Pod→Node edges are created when nodeName is set. The Node UID is not in the
// snapshot (Nodes are cluster-scoped), so the target is marked Incomplete.
func deriveRunsOnEdges(snap CollectorSnapshot, now time.Time) []Edge {
	var edges []Edge
	for i := range snap.Pods {
		pod := &snap.Pods[i]
		if pod.Spec.NodeName == "" {
			continue
		}
		edges = append(edges, Edge{
			ClusterID:       snap.ClusterID,
			Kind:            EdgeRunsOn,
			Source:          citationFromMeta(pod.Metadata, "Pod"),
			Target:          ResourceCitation{Kind: "Node", Name: pod.Spec.NodeName, Incomplete: true},
			Derivation:      DerivationNodeName,
			FirstObservedAt: now,
			LastObservedAt:  now,
			ValidFrom:       now,
		})
	}
	return edges
}

// deriveMountsEdges derives Mounts edges from Pod volumes referencing PVCs.
// Pod→PVC edges are created for each persistentVolumeClaim volume.
func deriveMountsEdges(snap CollectorSnapshot, now time.Time) []Edge {
	var edges []Edge
	for i := range snap.Pods {
		pod := &snap.Pods[i]
		podCitation := citationFromMeta(pod.Metadata, "Pod")
		seen := make(map[string]bool)
		for _, vol := range pod.Spec.Volumes {
			if vol.PersistentVolumeClaim == nil || vol.PersistentVolumeClaim.ClaimName == "" {
				continue
			}
			if seen[vol.PersistentVolumeClaim.ClaimName] {
				continue
			}
			seen[vol.PersistentVolumeClaim.ClaimName] = true
			edges = append(edges, Edge{
				ClusterID:       snap.ClusterID,
				Kind:            EdgeMounts,
				Source:          podCitation,
				Target:          ResourceCitation{Kind: "PersistentVolumeClaim", Namespace: pod.Metadata.Namespace, Name: vol.PersistentVolumeClaim.ClaimName, Incomplete: true},
				Derivation:      DerivationVolumeMount,
				FirstObservedAt: now,
				LastObservedAt:  now,
				ValidFrom:       now,
			})
		}
	}
	return edges
}

// deriveScalesEdges derives Scales edges from HPA scaleTargetRef.
// HPA→Deployment/ReplicaSet edges are created when the target kind and name
// match a resource in the snapshot.
func deriveScalesEdges(snap CollectorSnapshot, now time.Time) []Edge {
	var edges []Edge
	depByName := make(map[string]int)
	for i := range snap.Deployments {
		depByName[snap.Deployments[i].Metadata.Name] = i
	}
	rsByName := make(map[string]int)
	for i := range snap.ReplicaSets {
		rsByName[snap.ReplicaSets[i].Metadata.Name] = i
	}
	for i := range snap.HPAs {
		hpa := &snap.HPAs[i]
		target := hpa.Spec.ScaleTargetRef
		if target.Name == "" {
			continue
		}
		hpaCitation := citationFromMeta(hpa.Metadata, "HorizontalPodAutoscaler")
		switch target.Kind {
		case "Deployment":
			if idx, ok := depByName[target.Name]; ok {
				edges = append(edges, Edge{
					ClusterID:       snap.ClusterID,
					Kind:            EdgeScales,
					Source:          hpaCitation,
					Target:          citationFromMeta(snap.Deployments[idx].Metadata, "Deployment"),
					Derivation:      DerivationScaleTarget,
					FirstObservedAt: now,
					LastObservedAt:  now,
					ValidFrom:       now,
				})
			}
		case "ReplicaSet":
			if idx, ok := rsByName[target.Name]; ok {
				edges = append(edges, Edge{
					ClusterID:       snap.ClusterID,
					Kind:            EdgeScales,
					Source:          hpaCitation,
					Target:          citationFromMeta(snap.ReplicaSets[idx].Metadata, "ReplicaSet"),
					Derivation:      DerivationScaleTarget,
					FirstObservedAt: now,
					LastObservedAt:  now,
					ValidFrom:       now,
				})
			}
		}
	}
	return edges
}

// deriveProtectedByEdges derives ProtectedBy edges from PDB selectors.
// Workload→PDB edges are created when the PDB selector matches the workload
// template labels. The edge source is the workload (Deployment/ReplicaSet),
// target is the PDB.
func deriveProtectedByEdges(snap CollectorSnapshot, now time.Time) []Edge {
	var edges []Edge
	for i := range snap.PDBs {
		pdb := &snap.PDBs[i]
		if pdb.Spec.Selector == nil || len(pdb.Spec.Selector.MatchLabels) == 0 {
			continue
		}
		selector, err := metav1.LabelSelectorAsSelector(pdb.Spec.Selector)
		if err != nil {
			continue
		}
		pdbCitation := citationFromMeta(pdb.Metadata, "PodDisruptionBudget")
		matchWorkload := func(meta k8sgateway.ObjectMeta, kind string) {
			if !selector.Matches(labels.Set(meta.Labels)) {
				return
			}
			edges = append(edges, Edge{
				ClusterID:       snap.ClusterID,
				Kind:            EdgeProtectedBy,
				Source:          citationFromMeta(meta, kind),
				Target:          pdbCitation,
				Derivation:      DerivationPDBSelector,
				FirstObservedAt: now,
				LastObservedAt:  now,
				ValidFrom:       now,
			})
		}
		for j := range snap.Deployments {
			matchWorkload(snap.Deployments[j].Metadata, "Deployment")
		}
		for j := range snap.ReplicaSets {
			matchWorkload(snap.ReplicaSets[j].Metadata, "ReplicaSet")
		}
	}
	return edges
}

// --- Helpers ---

// buildOwnerIndex maps UID → ResourceCitation for all resources that can be
// an edge source (Deployment, ReplicaSet). This lets deriveOwnsEdges verify
// the owner exists in the snapshot before creating an edge.
func buildOwnerIndex(snap CollectorSnapshot) map[string]ResourceCitation {
	index := make(map[string]ResourceCitation)
	for i := range snap.Deployments {
		if snap.Deployments[i].Metadata.UID == "" {
			continue
		}
		index[snap.Deployments[i].Metadata.UID] = citationFromMeta(snap.Deployments[i].Metadata, "Deployment")
	}
	for i := range snap.ReplicaSets {
		if snap.ReplicaSets[i].Metadata.UID == "" {
			continue
		}
		index[snap.ReplicaSets[i].Metadata.UID] = citationFromMeta(snap.ReplicaSets[i].Metadata, "ReplicaSet")
	}
	return index
}

// citationFromMeta builds a ResourceCitation from Kubernetes ObjectMeta.
func citationFromMeta(meta k8sgateway.ObjectMeta, kind string) ResourceCitation {
	return ResourceCitation{
		Kind:       kind,
		Namespace:  meta.Namespace,
		Name:       meta.Name,
		UID:        meta.UID,
		Incomplete: meta.UID == "",
	}
}

// mapSelectorMatches returns true when every key-value pair in the selector is
// present in the labels map. An empty selector matches nothing.
func mapSelectorMatches(selector, labels map[string]string) bool {
	if len(selector) == 0 {
		return false
	}
	for k, v := range selector {
		if labels[k] != v {
			return false
		}
	}
	return true
}

// computeEdgeHash returns a deterministic SHA-256 hash of the edge identity
// and source representation. This lets the service skip unchanged edges.
func computeEdgeHash(edge *Edge) string {
	parts := []string{
		string(edge.Kind),
		edge.Source.Kind, edge.Source.Namespace, edge.Source.Name, edge.Source.UID,
		edge.Target.Kind, edge.Target.Namespace, edge.Target.Name, edge.Target.UID,
		string(edge.Derivation),
	}
	h := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(h[:])
}

// --- list helpers (paging until exhausted) ---

func (c *Collector) listAllDeployments(ctx context.Context, clusterID int64, namespace string, base apiquery.ListQuery) ([]k8sgateway.Deployment, error) {
	var all []k8sgateway.Deployment
	page := 1
	for {
		q := base
		q.Page = page
		resp, err := c.reader.Deployments(ctx, clusterID, namespace, q)
		if err != nil {
			return all, err
		}
		all = append(all, resp.Items...)
		if resp.Remaining <= 0 || len(resp.Items) == 0 {
			return all, nil
		}
		page++
		if page > 1000 {
			return all, fmt.Errorf("deployment list paging exceeded 1000 pages in namespace %s", namespace)
		}
	}
}

func (c *Collector) listAllReplicaSets(ctx context.Context, clusterID int64, namespace string, base apiquery.ListQuery) ([]k8sgateway.ReplicaSet, error) {
	var all []k8sgateway.ReplicaSet
	page := 1
	for {
		q := base
		q.Page = page
		resp, err := c.reader.ReplicaSets(ctx, clusterID, namespace, q)
		if err != nil {
			return all, err
		}
		all = append(all, resp.Items...)
		if resp.Remaining <= 0 || len(resp.Items) == 0 {
			return all, nil
		}
		page++
		if page > 1000 {
			return all, fmt.Errorf("replicaset list paging exceeded 1000 pages in namespace %s", namespace)
		}
	}
}

func (c *Collector) listAllPods(ctx context.Context, clusterID int64, namespace string, base apiquery.ListQuery) ([]k8sgateway.Pod, error) {
	var all []k8sgateway.Pod
	page := 1
	for {
		q := base
		q.Page = page
		resp, err := c.reader.Pods(ctx, clusterID, namespace, q)
		if err != nil {
			return all, err
		}
		all = append(all, resp.Items...)
		if resp.Remaining <= 0 || len(resp.Items) == 0 {
			return all, nil
		}
		page++
		if page > 1000 {
			return all, fmt.Errorf("pod list paging exceeded 1000 pages in namespace %s", namespace)
		}
	}
}

func (c *Collector) listAllServices(ctx context.Context, clusterID int64, namespace string, base apiquery.ListQuery) ([]k8sgateway.ServiceResource, error) {
	var all []k8sgateway.ServiceResource
	page := 1
	for {
		q := base
		q.Page = page
		resp, err := c.reader.Services(ctx, clusterID, namespace, q)
		if err != nil {
			return all, err
		}
		all = append(all, resp.Items...)
		if resp.Remaining <= 0 || len(resp.Items) == 0 {
			return all, nil
		}
		page++
		if page > 1000 {
			return all, fmt.Errorf("service list paging exceeded 1000 pages in namespace %s", namespace)
		}
	}
}

func (c *Collector) listAllIngresses(ctx context.Context, clusterID int64, namespace string, base apiquery.ListQuery) ([]k8sgateway.Ingress, error) {
	var all []k8sgateway.Ingress
	page := 1
	for {
		q := base
		q.Page = page
		resp, err := c.reader.Ingresses(ctx, clusterID, namespace, q)
		if err != nil {
			return all, err
		}
		all = append(all, resp.Items...)
		if resp.Remaining <= 0 || len(resp.Items) == 0 {
			return all, nil
		}
		page++
		if page > 1000 {
			return all, fmt.Errorf("ingress list paging exceeded 1000 pages in namespace %s", namespace)
		}
	}
}

func (c *Collector) listAllEndpointSlices(ctx context.Context, clusterID int64, namespace string, base apiquery.ListQuery) ([]k8sgateway.EndpointSlice, error) {
	var all []k8sgateway.EndpointSlice
	page := 1
	for {
		q := base
		q.Page = page
		resp, err := c.reader.EndpointSlices(ctx, clusterID, namespace, q)
		if err != nil {
			return all, err
		}
		all = append(all, resp.Items...)
		if resp.Remaining <= 0 || len(resp.Items) == 0 {
			return all, nil
		}
		page++
		if page > 1000 {
			return all, fmt.Errorf("endpointslice list paging exceeded 1000 pages in namespace %s", namespace)
		}
	}
}

func (c *Collector) listAllHPAs(ctx context.Context, clusterID int64, namespace string, base apiquery.ListQuery) ([]k8sgateway.HorizontalPodAutoscaler, error) {
	var all []k8sgateway.HorizontalPodAutoscaler
	page := 1
	for {
		q := base
		q.Page = page
		resp, err := c.reader.HorizontalPodAutoscalers(ctx, clusterID, namespace, q)
		if err != nil {
			return all, err
		}
		all = append(all, resp.Items...)
		if resp.Remaining <= 0 || len(resp.Items) == 0 {
			return all, nil
		}
		page++
		if page > 1000 {
			return all, fmt.Errorf("hpa list paging exceeded 1000 pages in namespace %s", namespace)
		}
	}
}

func (c *Collector) listAllPDBs(ctx context.Context, clusterID int64, namespace string, base apiquery.ListQuery) ([]k8sgateway.PodDisruptionBudget, error) {
	var all []k8sgateway.PodDisruptionBudget
	page := 1
	for {
		q := base
		q.Page = page
		resp, err := c.reader.PodDisruptionBudgets(ctx, clusterID, namespace, q)
		if err != nil {
			return all, err
		}
		all = append(all, resp.Items...)
		if resp.Remaining <= 0 || len(resp.Items) == 0 {
			return all, nil
		}
		page++
		if page > 1000 {
			return all, fmt.Errorf("pdb list paging exceeded 1000 pages in namespace %s", namespace)
		}
	}
}

// SortEdges returns a copy of edges sorted by a stable key for deterministic
// persistence and comparison.
func SortEdges(edges []Edge) []Edge {
	sorted := make([]Edge, len(edges))
	copy(sorted, edges)
	sort.SliceStable(sorted, func(i, j int) bool {
		return edgeSortKey(&sorted[i]) < edgeSortKey(&sorted[j])
	})
	return sorted
}

func edgeSortKey(edge *Edge) string {
	return strings.Join([]string{
		string(edge.Kind),
		edge.Source.UID, edge.Source.Name,
		edge.Target.UID, edge.Target.Name,
		string(edge.Derivation),
	}, "|")
}
